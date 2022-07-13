package cfapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var ErrTunnelNameConflict = errors.New("tunnel with name already exists")

type Tunnel struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	CreatedAt   time.Time    `json:"created_at"`
	DeletedAt   time.Time    `json:"deleted_at"`
	Connections []Connection `json:"connections"`
}

type TunnelWithToken struct {
	Tunnel
	Token string `json:"token"`
}

type Connection struct {
	ColoName           string    `json:"colo_name"`
	ID                 uuid.UUID `json:"id"`
	IsPendingReconnect bool      `json:"is_pending_reconnect"`
	OriginIP           net.IP    `json:"origin_ip"`
	OpenedAt           time.Time `json:"opened_at"`
}

type ActiveClient struct {
	ID          uuid.UUID    `json:"id"`
	Features    []string     `json:"features"`
	Version     string       `json:"version"`
	Arch        string       `json:"arch"`
	RunAt       time.Time    `json:"run_at"`
	Connections []Connection `json:"conns"`
}

type newTunnel struct {
	Name         string `json:"name"`
	TunnelSecret []byte `json:"tunnel_secret"`
	ConfigSrc    string `json:"config_src"`
}

type CleanupParams struct {
	queryParams url.Values
}

type ConfigurationsTunnelRequest struct {
	Config ConfigurationsTunnelConfig `json:"config"`
}

type ConfigurationsTunnelConfig struct {
	Ingress []IngressConfig `json:"ingress"`
}

type IngressConfig struct {
	Hostname      string `json:"hostname,omitempty"`
	Service       string `json:"service"`
	OriginRequest struct {
	} `json:"originRequest,omitempty"`
}

type CreateDnsRecordRequest struct {
	Type    string `json:"type"`
	Proxied bool   `json:"proxied"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func NewCleanupParams() *CleanupParams {
	return &CleanupParams{
		queryParams: url.Values{},
	}
}

func (cp *CleanupParams) ForClient(clientID uuid.UUID) {
	cp.queryParams.Set("client_id", clientID.String())
}

func (cp CleanupParams) encode() string {
	return cp.queryParams.Encode()
}

func (r *RESTClient) ConfigurationsTunnel(tunnelId string, data ConfigurationsTunnelRequest, accountToken string) error {
	if tunnelId == "" {
		return errors.New("tunnel id required")
	}

	fmt.Println(r.baseEndpoints.zoneLevel.String())
	//+"/cfd_tunnel/"+tunnelId+"/configurations"
	u := r.baseEndpoints.accountLevel
	u.Path = u.Path + "/" + tunnelId + "/configurations"
	fmt.Println(u.String())

	resp, err := r.sendRequest("PUT", u, data)

	if err != nil {
		return errors.Wrap(err, "REST request failed")
	}

	fmt.Printf("%+v\n", data)

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Println("CREATE TUNNEL DONE")
		fmt.Println("CONFIG INGRESS")

		for _, ingress := range data.Config.Ingress {

			if ingress.Service == "http_status:404" {
				continue
			}

			p := CreateDnsRecordRequest{
				Type:    "CNAME",
				Proxied: true,
				Name:    ingress.Hostname,
				Content: tunnelId + ".cfargotunnel.com",
			}

			fmt.Println(p)

			u = r.baseEndpoints.zoneLevel
			u.Path = strings.Replace(u.Path, "tunnels", "dns_records", 1)
			// u.Path = strings.Replace(u.Path, "v4", "api/v4", 1)
			fmt.Println("Dns config endpoint: ", u.String())

			var bodyReader io.Reader
			if bodyBytes, err := json.Marshal(p); err != nil {
				return errors.Wrap(err, "failed to serialize json body")
			} else {
				bodyReader = bytes.NewBuffer(bodyBytes)
			}

			req, err := http.NewRequest("POST", u.String(), bodyReader)
			if err != nil {
				return errors.Wrapf(err, "can't create %s request")
			}
			req.Header.Set("User-Agent", r.userAgent)
			if bodyReader != nil {
				req.Header.Set("Content-Type", jsonContentType)
			}
			req.Header.Add("Authorization", "Bearer "+accountToken)

			resp, err := r.client.Do(req)

			body, _ := ioutil.ReadAll(resp.Body)
			fmt.Println("body create ingress", string(body))

			if err != nil {
				return errors.Wrap(err, "REST request failed")
			}

			if resp.StatusCode != http.StatusOK {
				return errors.Wrap(err, "REST create dns return != 200")
			}
		}
		return nil
	default:
		body, _ := ioutil.ReadAll(resp.Body)

		// fmt.Println("data", data)
		fmt.Println("config tunnel", string(body))
		return errors.Wrap(err, "REST request failed")
	}
}

type DnsRecord struct {
	ID        string `json:"id"`
	ZoneID    string `json:"zone_id"`
	ZoneName  string `json:"zone_name"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	Proxiable bool   `json:"proxiable"`
	Proxied   bool   `json:"proxied"`
	TTL       int    `json:"ttl"`
	Locked    bool   `json:"locked"`
	Meta      struct {
		AutoAdded           bool   `json:"auto_added"`
		ManagedByApps       bool   `json:"managed_by_apps"`
		ManagedByArgoTunnel bool   `json:"managed_by_argo_tunnel"`
		Source              string `json:"source"`
	} `json:"meta"`
	CreatedOn  time.Time `json:"created_on"`
	ModifiedOn time.Time `json:"modified_on"`
}

func (r *RESTClient) ListDnsRecord(accountToken string) ([]*DnsRecord, error) {
	u := r.baseEndpoints.zoneLevel
	u.Path = strings.Replace(u.Path, "tunnels", "dns_records", 1)
	// u.Path = strings.Replace(u.Path, "v4", "api/v4", 1)
	fmt.Println("Dns config endpoint: ", u.String())

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "can't create %s request")
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Add("Authorization", "Bearer "+accountToken)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	// rr, _ := ioutil.ReadAll(resp.Body)
	// fmt.Println("r", string(rr))

	if resp.StatusCode == http.StatusOK {
		return parseListTunnelDnsRespRecords(resp.Body)
	}

	return nil, r.statusCodeToError("list tunnels dns", resp)
}

func (r *RESTClient) DeleteDnsRecord(dnsID string, accountToken string) error {
	u := r.baseEndpoints.zoneLevel
	u.Path = strings.Replace(u.Path, "tunnels", "dns_records", 1) + "/" + dnsID

	req, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		return errors.Wrapf(err, "can't create %s request")
	}
	req.Header.Set("User-Agent", r.userAgent)
	req.Header.Add("Authorization", "Bearer "+accountToken)

	resp, err := r.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	// rr, _ := ioutil.ReadAll(resp.Body)
	// fmt.Println("r", string(rr))

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return r.statusCodeToError("delete dns record", resp)
}

func parseListTunnelDnsRespRecords(body io.ReadCloser) ([]*DnsRecord, error) {
	var dns []*DnsRecord
	err := parseResponse(body, &dns)
	return dns, err
}

func (r *RESTClient) CreateTunnel(name string, tunnelSecret []byte) (*TunnelWithToken, error) {
	if name == "" {
		return nil, errors.New("tunnel name required")
	}
	if _, err := uuid.Parse(name); err == nil {
		return nil, errors.New("you cannot use UUIDs as tunnel names")
	}
	body := &newTunnel{
		Name:         name,
		TunnelSecret: tunnelSecret,
		ConfigSrc:    "cloudflare",
	}

	resp, err := r.sendRequest("POST", r.baseEndpoints.accountLevel, body)
	if err != nil {
		return nil, errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var tunnel TunnelWithToken
		if serdeErr := parseResponse(resp.Body, &tunnel); err != nil {
			return nil, serdeErr
		}
		return &tunnel, nil
	case http.StatusConflict:
		return nil, ErrTunnelNameConflict
	}

	return nil, r.statusCodeToError("create tunnel", resp)
}

func (r *RESTClient) GetTunnel(tunnelID uuid.UUID) (*Tunnel, error) {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.Path = path.Join(endpoint.Path, fmt.Sprintf("%v", tunnelID))
	resp, err := r.sendRequest("GET", endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return unmarshalTunnel(resp.Body)
	}

	return nil, r.statusCodeToError("get tunnel", resp)
}

func (r *RESTClient) GetTunnelToken(tunnelID uuid.UUID) (token string, err error) {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.Path = path.Join(endpoint.Path, fmt.Sprintf("%v/token", tunnelID))
	resp, err := r.sendRequest("GET", endpoint, nil)
	if err != nil {
		return "", errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		err = parseResponse(resp.Body, &token)
		return token, err
	}

	return "", r.statusCodeToError("get tunnel token", resp)
}

func (r *RESTClient) DeleteTunnel(tunnelID uuid.UUID) error {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.Path = path.Join(endpoint.Path, fmt.Sprintf("%v", tunnelID))
	resp, err := r.sendRequest("DELETE", endpoint, nil)
	if err != nil {
		return errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	return r.statusCodeToError("delete tunnel", resp)
}

func (r *RESTClient) ListTunnels(filter *TunnelFilter) ([]*Tunnel, error) {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.RawQuery = filter.encode()
	resp, err := r.sendRequest("GET", endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return parseListTunnels(resp.Body)
	}

	return nil, r.statusCodeToError("list tunnels", resp)
}

func parseListTunnels(body io.ReadCloser) ([]*Tunnel, error) {
	var tunnels []*Tunnel
	err := parseResponse(body, &tunnels)
	return tunnels, err
}

func (r *RESTClient) ListActiveClients(tunnelID uuid.UUID) ([]*ActiveClient, error) {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.Path = path.Join(endpoint.Path, fmt.Sprintf("%v/connections", tunnelID))
	resp, err := r.sendRequest("GET", endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return parseConnectionsDetails(resp.Body)
	}

	return nil, r.statusCodeToError("list connection details", resp)
}

func parseConnectionsDetails(reader io.Reader) ([]*ActiveClient, error) {
	var clients []*ActiveClient
	err := parseResponse(reader, &clients)
	return clients, err
}

func (r *RESTClient) CleanupConnections(tunnelID uuid.UUID, params *CleanupParams) error {
	endpoint := r.baseEndpoints.accountLevel
	endpoint.RawQuery = params.encode()
	endpoint.Path = path.Join(endpoint.Path, fmt.Sprintf("%v/connections", tunnelID))
	resp, err := r.sendRequest("DELETE", endpoint, nil)
	if err != nil {
		return errors.Wrap(err, "REST request failed")
	}
	defer resp.Body.Close()

	return r.statusCodeToError("cleanup connections", resp)
}

func unmarshalTunnel(reader io.Reader) (*Tunnel, error) {
	var tunnel Tunnel
	err := parseResponse(reader, &tunnel)
	return &tunnel, err
}
