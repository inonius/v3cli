package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	clientTypes "github.com/inonius/v3cli/api/client"
	"github.com/inonius/v3cli/pkg/speedtest"
	"github.com/librespeed/speedtest-cli/defs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version = "0.0.8"
var configFile string
var ignoreTlsError bool
var logger *slog.Logger

var cmd *cobra.Command = &cobra.Command{
	Use:     "inonius_v3cli",
	RunE:    fn,
	Version: Version,
}

func fn(cmd *cobra.Command, args []string) error {
	logger = slog.Default()

	//Quiet mode
	isQuiet, _ := cmd.PersistentFlags().GetBool("quiet")
	isDebug := viper.GetBool("debug")
	isJson := viper.GetBool("json")

	if isJson {
		isQuiet = true
	}
	if isQuiet {
		slog.SetLogLoggerLevel(slog.LevelError)
	} else if isDebug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	} else {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}

	// config
	configFile, _ = cmd.PersistentFlags().GetString("config")
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}
	if err := viper.ReadInConfig(); err != nil {
		logger.Debug("Config file not found, using default values")
	}

	// ignoreTlsError
	ignoreTlsError = viper.GetBool("ignore-tls-error")

	// orgTag
	var orgTagptr, freeTagptr *string
	orgTag := viper.GetString("orgtag")
	if orgTag == "" {
		orgTagptr = nil
	} else {
		orgTagptr = &orgTag
	}

	// freeTag
	freeTag := viper.GetString("freetag")
	if freeTag == "" {
		freeTagptr = nil
	} else {
		freeTagptr = &freeTag
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	//IPv4,IPv6 force
	forceIPv4 := viper.GetBool("ipv4")
	forceIPv6 := viper.GetBool("ipv6")

	var network string
	switch {
	case forceIPv4:
		network = "ip4"
	case forceIPv6:
		network = "ip6"
	default:
		network = "ip"
	}

	// bind to source IP address if given
	if src := viper.GetString("source"); src != "" {
		var err error
		dialer, err = newDialerAddressBound(src, network)
		if err != nil {
			return err
		}
	}

	// bind to interface if given
	if iface := viper.GetString("interface"); iface != "" {
		var err error
		dialer, err = speedtest.NewDialerInterfaceBound(iface)
		if err != nil {
			return err
		}
	}

	var dialContext func(context.Context, string, string) (net.Conn, error)
	switch {
	case forceIPv4:
		dialContext = func(ctx context.Context, network, address string) (conn net.Conn, err error) {
			return dialer.DialContext(ctx, "tcp4", address)
		}
	case forceIPv6:
		dialContext = func(ctx context.Context, network, address string) (conn net.Conn, err error) {
			return dialer.DialContext(ctx, "tcp6", address)
		}
	default:
		dialContext = dialer.DialContext
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext:           dialContext,
			MaxIdleConnsPerHost:   0,
			TLSHandshakeTimeout:   time.Second,
			ResponseHeaderTimeout: time.Second,
			IdleConnTimeout:       time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: ignoreTlsError,
			},
		},
	}

	//deviceID
	var deviceID string
	if viper.GetString("deviceid") == "" {
		deviceID = generateDeviceID()
		logger.Debug(os.Hostname())
		logger.Debug(deviceID)
	} else {
		deviceID = viper.GetString("deviceid")
	}

	// client初期化
	clientInstance := clientTypes.Client{
		HttpClient: httpClient,
		Config: &clientTypes.Config{
			Endpoint:       viper.GetString("endpoint"),
			IPv4Endpoint:   viper.GetString("ipv4-endpoint"),
			IPv6Endpoint:   viper.GetString("ipv6-endpoint"),
			OrgTag:         orgTagptr,
			FreeTag:        freeTagptr,
			Interface:      viper.GetString("interface"),
			Source:         viper.GetString("source"),
			NoICMP:         !viper.GetBool("icmp"), //WEBと同等にしたくデフォルトtrue
			IPv4:           viper.GetBool("ipv4"),
			IPv6:           viper.GetBool("ipv6"),
			Debug:          isDebug,
			Quiet:          isQuiet,
			IgnoreTLSError: ignoreTlsError,
			DeviceId:       deviceID,
			Concurrent:     3,
			Bytes:          false,
			MebiBytes:      false,
			Distance:       "km",
			Timeout:        2,
			Chunks:         100,
			UploadSize:     1024,
			Duration:       15,
			Secure:         false,
			NoPreAllocate:  false,
		},
		Result: &clientTypes.Result{},
	}

	if isDebug {
		//fmt.Println(clientInstance.Config)
	}

	if clientInstance.Config.Source != "" && clientInstance.Config.Interface != "" {
		return fmt.Errorf("incompatible options '%s' and '%s'", defs.OptionSource, defs.OptionInterface)
	}

	logger.Info("Starting iNonius client")
	speedtestClient := NewSpeedtestClient(&clientInstance)

	// 1. Get client info
	ctx := context.Background()
	clientInstance.Result.ClientInfoPair = clientTypes.ClientInfoPair{}

	v4info, err := speedtestClient.GetClientInfo(ctx, true)
	if err != nil {
		logger.Info("IPv4 connectivity not available")
		logger.Debug("failed to get clientInfo(IPv4)", "error", err.Error())
	} else {
		logger.Info("IPv4 connectivity available")
		clientInstance.Result.IPv4Available = true
		clientInstance.Result.ClientInfoPair.IPv4Info = v4info
	}

	v6info, err := speedtestClient.GetClientInfo(ctx, false)
	if err != nil {
		logger.Info("IPv6 connectivity not available")
		logger.Debug("failed to get clientInfo(IPv6)", "error", err.Error())
	} else {
		logger.Info("IPv6 connectivity available")
		clientInstance.Result.IPv6Available = true
		clientInstance.Result.ClientInfoPair.IPv6Info = v6info
	}

	// abort ipv4 and ipv6 not available
	if !clientInstance.Result.IPv4Available && !clientInstance.Result.IPv6Available {
		err := fmt.Errorf("cannot reach both IPv4 and IPv6 of api endpoint")
		logger.Error("abort", "error", err)
		return nil
	}

	// 2. Register Speedtest Session
	if err := speedtestClient.RegisterSpeedtestSession(ctx); err != nil {
		logger.Error("failed to register session", "error", err)
		return nil
	}

	// 3. Register AccessType Session
	var ipv4Mss int
	var ipv6Mss int
	if clientInstance.Result.IPv4Available {
		ipv4MssResp, err := speedtestClient.GetMSS(ctx, true)
		if err != nil {
			logger.Error("abort", "error", err)
			return err
		}
		ipv4Mss = ipv4MssResp.Mss
	}
	if clientInstance.Result.IPv6Available {
		ipv6MssResp, err := speedtestClient.GetMSS(ctx, false)
		if err != nil {
			logger.Error("abort", "error", err)
			return err
		}
		ipv6Mss = ipv6MssResp.Mss
	}
	if err := speedtestClient.RegisterAccessTypeSession(ctx, &ipv4Mss, &ipv6Mss); err != nil {
		logger.Error("failed to register ats", "error", err)
		return err
	}

	var ipv4server, ipv6server []defs.Server

	server, err := speedtestClient.GetServers(ctx)
	if err != nil {
		logger.Error("abort", "error", err)
		return err
	} else {
		ipv4server, ipv6server = ConvertLibrespeedServersToDefsServers(server.Librespeed)
	}

	// 4. Speedtest
	clientInstance.Result.SpeedtestResultPair = clientTypes.SpeedtestResultPair{}

	// if PreferIPv6 is true, IPv6 first
	if clientInstance.Result.Session.PreferIPv6 {
		//IPv6
		if clientInstance.Result.IPv6Available {

			logger.Info("=====Starting IPv6 Speedtest...=====")
			result, err := speedtest.Speedtest(clientInstance, ctx, logger, ipv6server)
			if err != nil {
				logger.Error("IPv6 Speedtest failed", "error", err)
			} else {
				clientInstance.Result.SpeedtestResultPair.IPv6Result = result
			}
		} else {
			clientInstance.Result.SpeedtestResultPair.IPv6Result = nil
		}

		//IPv4
		if clientInstance.Result.IPv4Available {
			time.Sleep(3 * time.Second)

			logger.Info("=====Starting IPv4 Speedtest...=====")
			result, err := speedtest.Speedtest(clientInstance, ctx, logger, ipv4server)
			if err != nil {
				logger.Error("IPv4 Speedtest failed", "error", err)
			} else {
				clientInstance.Result.SpeedtestResultPair.IPv4Result = result
			}
		} else {
			clientInstance.Result.SpeedtestResultPair.IPv4Result = nil
		}
	} else {
		// if PreferIPv6 is false, IPv4 first
		//IPv4
		if clientInstance.Result.IPv4Available {
			logger.Info("=====Starting IPv4 Speedtest...=====")
			result, err := speedtest.Speedtest(clientInstance, ctx, logger, ipv4server)
			if err != nil {
				logger.Error("IPv4 Speedtest failed", "error", err)
			} else {
				clientInstance.Result.SpeedtestResultPair.IPv4Result = result
			}
		} else {
			clientInstance.Result.SpeedtestResultPair.IPv4Result = nil
		}

		time.Sleep(3 * time.Second)

		//IPv6
		if clientInstance.Result.IPv6Available {

			logger.Info("=====Starting IPv6 Speedtest...=====")
			result, err := speedtest.Speedtest(clientInstance, ctx, logger, ipv6server)
			if err != nil {
				logger.Error("IPv6 Speedtest failed", "error", err)
			} else {
				clientInstance.Result.SpeedtestResultPair.IPv6Result = result
			}
		} else {
			clientInstance.Result.SpeedtestResultPair.IPv6Result = nil
		}
	}
	// 5. Finish
	err = speedtestClient.FinishSpeedtestSession(ctx)
	if err != nil {
		logger.Error("failed to finish session", "error", err)
	}

	logger.Debug("Complete!", "SpeedtestSessionID", clientInstance.Result.Session.UUID)
	if isDebug {
		//fmt.Println(clientInstance.Result)
	}

	if isQuiet {
		if isJson {
			j, _ := json.Marshal(simplifiedResult(*clientInstance.Result))
			fmt.Println(string(j))
		} else {
			if clientInstance.Result.IPv4Available {
				fmt.Println("IPv4Address", clientInstance.Result.ClientInfoPair.IPv4Info.IP.String(), "IPv4mss", *clientInstance.Result.AccessTypeSession.IPv4Mss, "IPv4Upload", clientInstance.Result.SpeedtestResultPair.IPv4Result.Upload, "Mbps", "IPv4Download", clientInstance.Result.SpeedtestResultPair.IPv4Result.Download, "Mbps", "IPv4RTT", fmt.Sprintf("%.2f", clientInstance.Result.SpeedtestResultPair.IPv4Result.Ping), "ms", "IPv4Jitter", clientInstance.Result.SpeedtestResultPair.IPv4Result.Jitter, "ms")
			}
			if clientInstance.Result.IPv6Available {
				fmt.Println("IPv6Address", string(clientInstance.Result.ClientInfoPair.IPv6Info.IP.String()), "IPv6mss", *clientInstance.Result.AccessTypeSession.IPv6Mss, "IPv6Upload", clientInstance.Result.SpeedtestResultPair.IPv6Result.Upload, "Mbps", "IPv6Download", clientInstance.Result.SpeedtestResultPair.IPv6Result.Download, "Mbps", "IPv6RTT", fmt.Sprintf("%.2f", clientInstance.Result.SpeedtestResultPair.IPv6Result.Ping), "ms", "IPv6Jitter", clientInstance.Result.SpeedtestResultPair.IPv6Result.Jitter, "ms")
			}
		}
	}
	logger.Info("Thank you for using inonius_v3cli")
	return nil
}

func NewCommand() *cobra.Command {
	return cmd
}

func init() {
	cmd.SetVersionTemplate(
		"inonius_v3cli: {{.Version}}\n" +
			"https://github.com/inonius/v3cli \n\n" +
			"Licensed under GNU Lesser General Public License v3.0\n" +
			"LibreSpeed  Copyright (C) 2016-2020 Federico Dossena\n" +
			"librespeed-cli  Copyright (C) 2020 Maddie Zhan\n" +
			"librespeed.org  Copyright (C)\n" +
			"Modified by iNonius Project (C) 2026\n")

	cmd.PersistentFlags().BoolP("help", "?", false, "Show help")
	cmd.PersistentFlags().BoolP("debug", "d", false, "Debug mode")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "Quiet mode")
	cmd.PersistentFlags().BoolP("json", "", false, "Json mode")
	cmd.PersistentFlags().BoolP("ignore-tls-error", "k", false, "Ignore tls error")
	cmd.PersistentFlags().StringP("config", "c", "", "--config <CONFIG_PATH> YML, TOML and JSON are available. (default ./config.yml)")
	cmd.PersistentFlags().StringP("orgtag", "O", "", "OrgTag if you have")
	cmd.PersistentFlags().StringP("freetag", "F", "", "FreeTag")
	cmd.PersistentFlags().StringP("interface", "i", "", "Interface Name")
	cmd.PersistentFlags().StringP("source", "s", "", "Source address")
	cmd.PersistentFlags().BoolP("ipv4", "4", false, "Force IPv4")
	cmd.PersistentFlags().BoolP("ipv6", "6", false, "Force IPv6")
	cmd.PersistentFlags().BoolP("icmp", "", false, "Use ICMP ping (default: http ping)")
	cmd.PersistentFlags().StringP("deviceid", "", "", "custom device id (default: hostname based generate)")
	cmd.PersistentFlags().StringP("endpoint", "e", "https://api.inonius.net", "Use: client --endpoint <ENDPOINT>")
	cmd.PersistentFlags().StringP("ipv4-endpoint", "", "https://ipv4-api.inonius.net", "Use: client --ipv4-endpoint <ENDPOINT>")
	cmd.PersistentFlags().StringP("ipv6-endpoint", "", "https://ipv6-api.inonius.net", "Use: client --ipv4-endpoint <ENDPOINT>")

	// Hidden flags
	cmd.PersistentFlags().Lookup("freetag").Hidden = true

	// Bind debug flag to viper
	viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("quiet", cmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("json", cmd.PersistentFlags().Lookup("json"))
	viper.BindPFlag("ignore-tls-error", cmd.PersistentFlags().Lookup("ignore-tls-error"))
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("orgtag", cmd.PersistentFlags().Lookup("orgtag"))
	viper.BindPFlag("freetag", cmd.PersistentFlags().Lookup("freetag"))
	viper.BindPFlag("interface", cmd.PersistentFlags().Lookup("interface"))
	viper.BindPFlag("source", cmd.PersistentFlags().Lookup("source"))
	viper.BindPFlag("ipv4", cmd.PersistentFlags().Lookup("ipv4"))
	viper.BindPFlag("ipv6", cmd.PersistentFlags().Lookup("ipv6"))
	viper.BindPFlag("icmp", cmd.PersistentFlags().Lookup("icmp"))
	viper.BindPFlag("deviceid", cmd.PersistentFlags().Lookup("deviceid"))
	viper.BindPFlag("endpoint", cmd.PersistentFlags().Lookup("endpoint"))
	viper.BindPFlag("ipv4-endpoint", cmd.PersistentFlags().Lookup("ipv4-endpoint"))
	viper.BindPFlag("ipv6-endpoint", cmd.PersistentFlags().Lookup("ipv6-endpoint"))
}

func newDialerAddressBound(src string, network string) (dialer *net.Dialer, err error) {
	// first we parse the IP to see if it's valid
	addr, err := net.ResolveIPAddr(network, src)
	if err != nil {
		if strings.Contains(err.Error(), "no suitable address") {
			if network == "ip6" {
				logger.Error("Address %s is not a valid IPv6 address", "error", src)
			} else {
				logger.Error("Address %s is not a valid IPv4 address", "error", src)
			}
		} else {
			logger.Error("Error", "error parsing source IP:", err)
		}
		return nil, err
	}

	logger.Debug("Error", "Using %s as source IP", src)
	localTCPAddr := &net.TCPAddr{IP: addr.IP}

	defaultDialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	defaultDialer.LocalAddr = localTCPAddr
	return defaultDialer, nil
}
