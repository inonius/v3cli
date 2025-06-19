package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"

	"github.com/google/uuid"
	clientTypes "github.com/inonius/v3cli/api/client"
	v3 "github.com/inonius/v3cli/api/v3"
	"github.com/librespeed/speedtest-cli/defs"
)

type SpeedtestClient struct {
	v3Client *clientTypes.Client
	logger   *slog.Logger
}

func NewSpeedtestClient(clientInstance *clientTypes.Client) *SpeedtestClient {
	return &SpeedtestClient{
		v3Client: clientInstance,
		logger:   logger,
	}
}

func (c *SpeedtestClient) GetClientInfo(ctx context.Context, isIPv4 bool) (v3.ClientInfo, error) {
	ci := v3.ClientInfo{}
	var endpoint string
	if c.v3Client.Config == nil {
		return v3.ClientInfo{}, fmt.Errorf("config is not initialized")
	}
	// switch ipv4 or ipv6
	if isIPv4 {
		endpoint = c.v3Client.Config.IPv4Endpoint
	} else {
		endpoint = c.v3Client.Config.IPv6Endpoint
	}

	err := c.call(ctx, "GET", endpoint, "/clientinfo", nil, &ci)
	return ci, err
}

func (c *SpeedtestClient) GetMSS(ctx context.Context, isIPv4 bool) (v3.MSSResponse, error) {
	mr := v3.MSSResponse{}

	var endpoint string
	// switch ipv4 or ipv6
	if isIPv4 {
		endpoint = c.v3Client.Config.IPv4Endpoint
	} else {
		endpoint = c.v3Client.Config.IPv6Endpoint
	}
	err := c.call(ctx, "GET", endpoint, "/mss", nil, &mr)
	return mr, err
}

func ConvertLibrespeedToServer(lts v3.LibrespeedTestServer, id int) defs.Server {
	return defs.Server{
		ID:          id,
		Name:        lts.Name,
		Server:      lts.Server,
		DownloadURL: lts.DlUrl,
		UploadURL:   lts.UlUrl,
		PingURL:     lts.PingUrl,
		GetIPURL:    lts.GetIpUrl,
		SponsorName: "",
		SponsorURL:  "",
		NoICMP:      false,
		TLog:        defs.TelemetryLog{},
	}
}

func (c *SpeedtestClient) GetServers(ctx context.Context) (v3.TestServersList, error) {
	tsl := v3.TestServersList{}
	var endpoint = c.v3Client.Config.Endpoint

	err := c.call(ctx, "GET", endpoint, "/servers", nil, &tsl)
	return tsl, err
}

func ConvertLibrespeedServersToDefsServers(libreServers []v3.LibrespeedTestServer) (ipv4 []defs.Server, ipv6 []defs.Server) {
	var ipv4defsServers []defs.Server
	var ipv6defsServers []defs.Server

	for i, ls := range libreServers {
		server := ConvertLibrespeedToServer(ls, i+1)
		switch ls.TypeName {
		case "ipv4":
			ipv4defsServers = append(ipv4defsServers, server)
		case "ipv6":
			ipv6defsServers = append(ipv6defsServers, server)
		}
	}
	return ipv4defsServers, ipv6defsServers
}

func (c *SpeedtestClient) RegisterAccessTypeSession(ctx context.Context, IPv4Mss *int, IPv6Mss *int) error {
	ats := v3.AccessTypeSession{
		SpeedtestSessionUUID: c.v3Client.Result.Session.UUID,
		IPv4Mss:              IPv4Mss,
		IPv6Mss:              IPv6Mss,
	}
	resp := v3.AccessTypeSession{}
	if err := c.call(ctx, "POST", c.v3Client.Config.Endpoint, "/accesstype/new", ats, &resp); err != nil {
		return err
	}
	c.v3Client.Result.AccessTypeSession = resp

	return nil
}

// HostnameBaseでUUIDしてみる
func generateDeviceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "111903fc-5e21-3052-83e4-1615a4760d0a" //something.went.wrong
	}
	return uuid.NewMD5(uuid.NameSpaceDNS, []byte(hostname)).String()
}

func (c *SpeedtestClient) RegisterSpeedtestSession(ctx context.Context) error {
	sts := v3.SpeedtestSession{
		DeviceId: c.v3Client.Config.DeviceId,
		OrgId:    c.v3Client.Config.OrgTag,
		FreeTag:  c.v3Client.Config.FreeTag,
	}
	if c.v3Client.Result.IPv4Available {
		IPv4Addr := c.v3Client.Result.ClientInfoPair.IPv4Info.IP.String()
		sts.IPv4Addr = IPv4Addr
		IPv4Org := c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.Org
		sts.InfoIPv4Org = &IPv4Org
		IPv4Country := c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.Country
		sts.InfoIPv4Country = &IPv4Country
		IPv4City := c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.City
		sts.InfoIPv4City = &IPv4City
		if c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.ASN != nil {
			IPv4AS := c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.ASN.ASN
			sts.InfoIPv4AS = &IPv4AS
			IPv4route := c.v3Client.Result.ClientInfoPair.IPv4Info.IPInfo.ASN.Route
			sts.InfoIPv4Route = &IPv4route
		}
	} else {
		sts.IPv4Addr = "None"
	}
	if c.v3Client.Result.IPv6Available {
		IPv6Addr := c.v3Client.Result.ClientInfoPair.IPv6Info.IP.String()
		sts.IPv6Addr = IPv6Addr
		IPv6Org := c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.Org
		sts.InfoIPv6Org = &IPv6Org
		IPv6Country := c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.Country
		sts.InfoIPv6Country = &IPv6Country
		IPv6City := c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.City
		sts.InfoIPv6City = &IPv6City
		if c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.ASN != nil {
			IPv6AS := c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.ASN.ASN
			sts.InfoIPv6AS = &IPv6AS
			IPv6route := c.v3Client.Result.ClientInfoPair.IPv6Info.IPInfo.ASN.Route
			sts.InfoIPv6Route = &IPv6route
		}
	} else {
		sts.IPv6Addr = "None"
	}

	resp := v3.SpeedtestSession{}

	err := c.call(ctx, "POST", c.v3Client.Config.Endpoint, "/session/new", sts, &resp)
	if err != nil {
		return err
	}
	c.v3Client.Result.Session = resp
	return nil
}

func (c *SpeedtestClient) FinishSpeedtestSession(ctx context.Context) error {
	var speedIPv4Id, speedIPv6Id *string

	if c.v3Client.Result == nil {
		return fmt.Errorf("v3Client.Result is nil")
	}

	if c.v3Client.Result.SpeedtestResultPair.IPv4Result != nil {
		speedIPv4Id = c.v3Client.Result.SpeedtestResultPair.IPv4Result.ID
	}

	if c.v3Client.Result.SpeedtestResultPair.IPv6Result != nil {
		speedIPv6Id = c.v3Client.Result.SpeedtestResultPair.IPv6Result.ID
	}

	fr := v3.FinishSessionRequest{
		UUID:        c.v3Client.Result.Session.UUID,
		DeviceId:    c.v3Client.Result.Session.DeviceId,
		SpeedIPv4Id: speedIPv4Id,
		SpeedIPv6Id: speedIPv6Id,
	}

	resp := v3.SpeedtestSession{}

	err := c.call(ctx, "POST", c.v3Client.Config.Endpoint, "/session/finish", fr, &resp)
	if err != nil {
		return err
	}
	//fmt.Println(resp)

	c.v3Client.Result.Session = resp
	if !resp.Finished {
		return fmt.Errorf("session is not finished")
	}
	return nil
}

func (c *SpeedtestClient) call(ctx context.Context, method string, endpoint string, apiEndpoint string, params interface{}, res interface{}) error {
	// thx: https://qiita.com/yyoshiki41/items/a0354d9ad70c1b8225b6
	if (endpoint) == "" {
		return fmt.Errorf("endpoint is not set")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, apiEndpoint)

	jsonParams, err := json.Marshal(params)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(jsonParams))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("inonius_v3cli_%s_%s", Version, runtime.GOARCH))
	req = req.WithContext(ctx)

	response, err := c.v3Client.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if res == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(&res)
}

func simplifiedResult(result clientTypes.Result) clientTypes.SimplifiedResult {
	simplifiedResult := clientTypes.SimplifiedResult{
		Timestamp:     result.Session.CreatedAt.Unix(),
		IPv4Available: result.IPv4Available,
		IPv6Available: result.IPv6Available,
	}

	if result.IPv4Available {
		simplifiedResult.IPv4Info = &clientTypes.SimplifiedClientInfo{
			IP:     result.ClientInfoPair.IPv4Info.IP,
			Port:   result.ClientInfoPair.IPv4Info.Port,
			IsIPv4: result.ClientInfoPair.IPv4Info.IsIPv4,
			Org:    result.ClientInfoPair.IPv4Info.IPInfo.Org,
			Mss:    result.AccessTypeSession.IPv4Mss,
		}

		if ipv4Result := result.SpeedtestResultPair.IPv4Result; ipv4Result != nil {
			simplifiedResult.SpeedtestResult = append(simplifiedResult.SpeedtestResult, clientTypes.SimplifiedSpeedtestResult{
				SpeedtestType: "IPv4",
				UnixTime:      ipv4Result.Timestamp.Unix(),
				Server:        ipv4Result.Server.Name,
				Upload:        ipv4Result.Upload,
				Download:      ipv4Result.Download,
				Ping:          ipv4Result.Ping,
				Jitter:        ipv4Result.Jitter,
			})
		}
	}

	if result.IPv6Available {
		simplifiedResult.IPv6Info = &clientTypes.SimplifiedClientInfo{
			IP:     result.ClientInfoPair.IPv6Info.IP,
			Port:   result.ClientInfoPair.IPv6Info.Port,
			IsIPv4: result.ClientInfoPair.IPv6Info.IsIPv4,
			Org:    result.ClientInfoPair.IPv6Info.IPInfo.Org,
			Mss:    result.AccessTypeSession.IPv6Mss,
		}

		if ipv6Result := result.SpeedtestResultPair.IPv6Result; ipv6Result != nil {
			simplifiedResult.SpeedtestResult = append(simplifiedResult.SpeedtestResult, clientTypes.SimplifiedSpeedtestResult{
				SpeedtestType: "IPv6",
				UnixTime:      ipv6Result.Timestamp.Unix(),
				Server:        ipv6Result.Server.Name,
				Upload:        ipv6Result.Upload,
				Download:      ipv6Result.Download,
				Ping:          ipv6Result.Ping,
				Jitter:        ipv6Result.Jitter,
			})
		}
	}
	return simplifiedResult
}
