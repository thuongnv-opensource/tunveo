package tunnel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/cloudflare/cloudflared/cfapi"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
)

type ApiResponse struct {
	Code    uint32 `json:"Code"`
	Token   string `json:"Token"`
	Message string `json:"Message"`
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
	hostname := generateRandomShortName() + "." + TUNVEO_HOSTNAME

	ingress := []cfapi.IngressConfig{
		{
			Service:  "http://localhost:" + port,
			Hostname: hostname,
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
	fmt.Printf("|         [START TUNNEL HTTP]: http://%s              |\n", hostname)
	fmt.Printf("|         [START TUNNEL HTTPS]: https://%s            |\n", hostname)
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
	error = login(c)
	if error != nil {
		return error
	}
	// start server
	fmt.Println("Server listen on port 3000")

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

			r, err := createTunveoTunnel(c, t, accountToken)

			if err != nil {
				fmt.Println(err)
				json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusBadGateway, Message: "500 error"})
				return
			}
			json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusOK, Token: r.Token})
			return
		}
		json.NewEncoder(w).Encode(&ApiResponse{Code: http.StatusNotFound, Message: "404 not found"})
		return
	}))

	err := http.ListenAndServe(":3000", router)
	if err != nil {
		log.Fatal(err)
	}

	return nil

}

func createTunveoTunnel(c *cli.Context, config cfapi.ConfigurationsTunnelRequest, accountToken string) (*cfapi.TunnelWithToken, error) {
	sc, err := newSubcommandContext(c)
	client, err := sc.client()
	if err != nil {
		return nil, err
	}

	var tunnelSecret []byte

	tunnelSecret, err = generateTunnelSecret()
	if err != nil {
		return nil, err
	}

	tunnelName := generateRandomShortName()

	tunnel, err := client.CreateTunnel(tunnelName, tunnelSecret)

	var hostnames []string
	for _, ingress := range config.Config.Ingress {
		hostnames = append(hostnames, ingress.Hostname)
	}

	err = client.ConfigurationsTunnel(tunnel.ID.String(), config, accountToken)

	fmt.Println(tunnel.ID)

	if err != nil {
		return nil, err
	}

	fmt.Println(tunnel.Token)

	return tunnel, nil
}
