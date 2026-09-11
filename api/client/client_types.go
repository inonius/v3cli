package client

import (
	"net"
	"net/http"
	"time"

	v3 "github.com/inonius/v3cli/api/v3"
	"github.com/librespeed/speedtest-cli/report"
)

type SimplifiedClientInfo struct {
	IP     net.IP `json:"ip"`
	Port   int    `json:"port"`
	IsIPv4 bool   `json:"is_ipv4"`
	Org    string `json:"org"`
	Mss    *int   `json:"mss"`
}

type SimplifiedSpeedtestResult struct {
	SpeedtestType string  `json:"speedtest_type"`
	UnixTime      int64   `json:"timestamp"`
	Server        string  `json:"server"`
	Upload        float64 `json:"upload"`
	Download      float64 `json:"download"`
	Ping          float64 `json:"ping"`
	Jitter        float64 `json:"jitter"`
}

type SimplifiedResult struct {
	Timestamp       int64                       `json:"timestamp"`
	IPv4Available   bool                        `json:"ipv4_available"`
	IPv6Available   bool                        `json:"ipv6_available"`
	IPv4Info        *SimplifiedClientInfo       `json:"ipv4_info,omitempty"`
	IPv6Info        *SimplifiedClientInfo       `json:"ipv6_info,omitempty"`
	SpeedtestResult []SimplifiedSpeedtestResult `json:"result"` //測定先が増えた際に連携先が壊れないように
}

type ClientInfoPair struct {
	IPv4Info v3.ClientInfo
	IPv6Info v3.ClientInfo
}

type SpeedtestResult struct {
	report.JSONReport
	ID *string
}

type SpeedtestResultPair struct {
	IPv4Result *SpeedtestResult
	IPv6Result *SpeedtestResult
}

type Result struct {
	IPv4Available       bool
	IPv6Available       bool
	ClientInfoPair      ClientInfoPair
	AccessTypeSession   v3.AccessTypeSession
	SpeedtestResultPair SpeedtestResultPair
	Session             v3.SpeedtestSession
}

type Config struct {
	Endpoint       string
	IPv4Endpoint   string
	IPv6Endpoint   string
	DeviceId       string
	Help           bool          `json:"help,omitempty"`
	Debug          bool          `json:"debug,omitempty"`
	Quiet          bool          `json:"quiet,omitempty"`
	IgnoreTLSError bool          `json:"ignore-tls-error,omitempty"`
	OrgTag         *string       `json:"org,omitempty"`
	FreeTag        *string       `json:"freetag,omitempty"`
	ConfigPath     string        `json:"config,omitempty"`
	IPv4           bool          `json:"ipv4,omitempty"`
	IPv6           bool          `json:"ipv6,omitempty"`
	NoICMP         bool          `json:"no-icmp,omitempty"`
	Concurrent     int           `json:"concurrent,omitempty"`
	Bytes          bool          `json:"bytes,omitempty"`
	MebiBytes      bool          `json:"mebibytes,omitempty"`
	Distance       string        `json:"distance,omitempty"`
	List           bool          `json:"list,omitempty"`
	Server         []int         `json:"server,omitempty"`
	Source         string        `json:"source,omitempty"`
	Interface      string        `json:"interface,omitempty"`
	Timeout        int           `json:"timeout,omitempty"`
	Chunks         int           `json:"chunks,omitempty"`
	UploadSize     int           `json:"upload-size,omitempty"`
	Duration       time.Duration `json:"duration,omitempty"`
	Secure         bool          `json:"secure,omitempty"`
	CACert         string        `json:"ca-cert,omitempty"`
	NoPreAllocate  bool          `json:"no-pre-allocate,omitempty"`
}

type Client struct {
	HttpClient *http.Client
	Config     *Config
	Result     *Result
	UserAgent  string
}
