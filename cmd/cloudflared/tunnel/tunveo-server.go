package tunnel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/cloudflare/cloudflared/cfapi"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
)

type ApiResponse struct {
	Code     uint32 `json:"Code"`
	Token    string `json:"Token"`
	HostName string `json:"HostName"`
	Message  string `json:"Message"`
}

type CreateTunnelRequest struct {
	Ports []struct {
		Port   int    `json:"port"`
		Domain string `json:"domain"`
		Type   string `json:"type"`
	} `json:"ports"`
}

func buildTunveoSubcommand(hidden bool) *cli.Command {
	return &cli.Command{
		Name:      "tunveo",
		Action:    cliutil.ConfiguredAction(startTunveoServer),
		Usage:     "start tunveo server",
		ArgsUsage: " ",
		Hidden:    hidden,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "account-token",
				Value: "",
				Usage: "account cloudflare token",
			},
		},
	}
}

func buildTunveoHttpSubcommand(hidden bool) *cli.Command {
	return &cli.Command{
		Name:      "http",
		Action:    cliutil.ConfiguredAction(startTunveoProxyHttpServer),
		Usage:     "proxy http port",
		ArgsUsage: " ",
		Hidden:    hidden,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "port",
				Value: "",
				Usage: "port forward",
			},
		},
	}
}

func generateRandomShortName() string {
	return RandStringBytes(10)
}

const letterBytes = "abcdefghijklmnopqrstuvwxyz"

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func startTunveoProxyHttpServer(c *cli.Context) error {
	sc, err := newSubcommandContext(c)
	if err != nil {
		return err
	}

	port := c.String("port")
	// valid port
	// hostname := generateRandomShortName() + "." + TUNVEO_HOSTNAME

	ingress := []cfapi.IngressConfig{
		{
			Service:  "http://localhost:" + port,
			Hostname: "AUTO",
		},
		{
			Service: "http_status:404",
		},
	}

	requestPayload := cfapi.ConfigurationsTunnelRequest{
		Config: cfapi.ConfigurationsTunnelConfig{
			Ingress: ingress,
		},
	}

	var bodyReader io.Reader
	if bodyBytes, err := json.Marshal(requestPayload); err != nil {
		return errors.Wrap(err, "failed to serialize json body")
	} else {
		bodyReader = bytes.NewBuffer(bodyBytes)
	}

	req, err := http.Post(API_ENDPOINT, "application/json", bodyReader)

	if err != nil {
		return err
	}

	var tokenResult ApiResponse

	json.NewDecoder(req.Body).Decode(&tokenResult)

	colorBlue := "\033[34m"
	colorReset := "\033[0m"

	fmt.Println(string(colorBlue), "")

	fmt.Println("-----------------------------------------------------------------------------")
	fmt.Println("|                                                                           |")
	fmt.Println("|                                                                           |")
	fmt.Printf("|         [START TUNNEL HTTP]: http://%s              |\n", tokenResult.HostName)
	fmt.Printf("|         [START TUNNEL HTTPS]: https://%s            |\n", tokenResult.HostName)
	fmt.Println("|                                                                           |")
	fmt.Println("|                                                                           |")
	fmt.Println("-----------------------------------------------------------------------------")

	fmt.Println(string(colorReset), "")

	// time.Sleep(5 * time.Second)

	if token, err := ParseToken(tokenResult.Token); err == nil {
		// fmt.Println("==============================")
		return sc.runWithCredentials(token.Credentials())
	} else {
		fmt.Println("Parse token error")
	}

	return nil
}

func startTunveoServer(c *cli.Context) error {

	error := login(c)

	sc, err := newSubcommandContext(c)
	client, err := sc.client()
	if err != nil {
		return err
	}

	if error != nil {
		return error
	}
	// start server
	fmt.Println("Server listen on port 9090")

	accountToken := c.String("account-token")

	fmt.Println("accountToken: ", accountToken)

	router := http.NewServeMux()

	router.HandleFunc("/", (func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			decoder := json.NewDecoder(r.Body)
			var t cfapi.ConfigurationsTunnelRequest
			err := decoder.Decode(&t)
			if err != nil {
				panic(err)
			}

			hostname := generateRandomShortName() + "." + TUNVEO_HOSTNAME

			for index, _ := range t.Config.Ingress {
				if t.Config.Ingress[index].Hostname == "AUTO" {
					t.Config.Ingress[index].Hostname = hostname
				}
			}

			r, err := createTunveoTunnel(client, t, accountToken)

			if err != nil {
				fmt.Println(err)
				json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusBadGateway, Message: "500 error"})
				return
			}
			json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusOK, Token: r.Token, HostName: hostname})
			return
		}
		json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusNotFound, Message: "404 not found"})
		return
	}))

	go func() {
		for true {
			dnsRecords, err := client.ListDnsRecord(accountToken)
			if err != nil {
				continue
			}
			for _, dns := range dnsRecords {
				if strings.Index(dns.Content, ".cfargotunnel.com") >= 0 {
					fmt.Println("[CAN DELETE]: ", dns.Name, dns.Content)
					tunnelId := strings.Replace(dns.Content, ".cfargotunnel.com", "", 1)
					tunnelUUid, err := uuid.Parse(tunnelId)
					if err != nil {
						fmt.Println(err)
						continue
					}
					filter := cfapi.NewTunnelFilter()
					filter.NoDeleted()
					filter.ByTunnelID(tunnelUUid)
					tunnels, err := client.ListTunnels(filter)
					if err != nil {
						continue
					}
					if len(tunnels) == 0 {
						fmt.Print("DELETE DNS RECORD:", dns.Name)
						err := client.DeleteDnsRecord(dns.ID, accountToken)
						if err != nil {
							fmt.Println("Delete record error", err)
						}
					}
				}
			}
			time.Sleep(time.Hour * 48)
		}
	}()

	go func() {
		for true {
			filter := cfapi.NewTunnelFilter()
			filter.NoDeleted()
			tunnels, err := client.ListTunnels(filter)
			fmt.Println("tunnels length: ", len(tunnels))
			if err != nil {
				continue
			}
			for _, tunnel := range tunnels {
				fmt.Println("tunnel id: ", tunnel.ID.String())
				activeClient, err := client.ListActiveClients(tunnel.ID)
				if err != nil {
					fmt.Println("GET ACTIVE CLIENT ERROR", err)
				}
				if len(activeClient) == 0 {
					deleteTunnel(tunnel, client)
				}
				time.Sleep(5 * time.Second)
			}
			time.Sleep(time.Second * 10)
		}
	}()

	err = http.ListenAndServe(":9090", router)
	if err != nil {
		log.Fatal(err)
	}

	return nil

}

func deleteTunnel(tunnel *cfapi.Tunnel, client cfapi.Client) {
	fmt.Println("[AWAITING DELETE TUNNEL]", tunnel.ID.String())
	time.Sleep(30 * time.Second)
	activeClient, err := client.ListActiveClients(tunnel.ID)
	if err != nil {
		fmt.Println("[GET ACTIVE CLIENT ERROR]", err)
		return
	}
	if len(activeClient) == 0 {
		fmt.Println("[DELETE TUNNEL]: ", tunnel.ID.String())
		client.DeleteTunnel(tunnel.ID)
	}
}

func createTunveoTunnel(client cfapi.Client, config cfapi.ConfigurationsTunnelRequest, accountToken string) (*cfapi.TunnelWithToken, error) {

	var tunnelSecret []byte
	var err error

	tunnelSecret, err = generateTunnelSecret()
	if err != nil {
		return nil, err
	}

	tunnelName := generateRandomShortName()

	tunnel, err := client.CreateTunnel(tunnelName, tunnelSecret)

	err = client.ConfigurationsTunnel(tunnel.ID.String(), config, accountToken)

	fmt.Println(tunnel.ID)

	if err != nil {
		return nil, err
	}

	fmt.Println(tunnel.Token)

	return tunnel, nil
}
