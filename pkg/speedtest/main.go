package speedtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	clientTypes "github.com/inonius/v3cli/api/client"
	"github.com/librespeed/speedtest-cli/defs"
	log "github.com/sirupsen/logrus"
)

const (
	// the default ping count for measuring ping and jitter
	pingCount = 10
)

// doSpeedTest is where the actual speed test happens
func doSpeedTest(c clientTypes.Client, ctx *context.Context, logger *slog.Logger, servers []defs.Server, network string, silent bool, noICMP bool) (*clientTypes.SpeedtestResult, error) {
	if serverCount := len(servers); serverCount > 1 {
		logger.Info("Testing agains", "ServerCount", &serverCount)
	}
	var telemetryServer defs.TelemetryServer

	telemetryServer.Level = "full"

	for _, currentServer := range servers {
		// get telemetry level
		currentServer.TLog.SetLevel(telemetryServer.GetLevel())

		u, err := currentServer.GetURL()
		if err != nil {
			logger.Error("Failed to get server URL:", "error", err)
			return nil, err
		}

		logger.Debug("Selected server:", "Server", u.Hostname())

		if currentServer.IsUp() {
			//get ping,jitter value
			logger.Debug("Pinging server... ")
			// skip ICMP if option given
			currentServer.NoICMP = noICMP

			p, jitter, err := currentServer.ICMPPingAndJitter(pingCount, c.Config.Source, network)
			if err != nil {
				logger.Error("Failed to get RTT and jitter:", "error", err)
				return nil, err
			}
			logger.Info(fmt.Sprint("RTT ", RoundTo(p, 3), "ms"))
			logger.Info(fmt.Sprint("Jitter ", RoundTo(jitter, 3), "ms"))

			// get download value
			var downloadValue float64
			var bytesRead uint64
			logger.Info("Download testing.... ")

			download, br, err := currentServer.Download(silent, c.Config.Bytes, c.Config.MebiBytes, c.Config.Concurrent, c.Config.Chunks, time.Duration(c.Config.Duration*time.Second))
			if err != nil {
				logger.Error("Failed to get download speed:", "error", err)
				return nil, err
			}
			downloadValue = download
			bytesRead = uint64(br)
			logger.Info(fmt.Sprint("Download ", RoundTo(downloadValue, 2), "Mbps"))

			// get upload value
			var uploadValue float64
			var bytesWritten uint64
			logger.Info("Upload testing.... ")

			upload, bw, err := currentServer.Upload(c.Config.NoPreAllocate, silent, c.Config.Bytes, c.Config.MebiBytes, c.Config.Concurrent, c.Config.UploadSize, time.Duration(c.Config.Duration*time.Second))
			if err != nil {
				logger.Error("Failed to get upload speed:", "error", err)
				return nil, err
			}
			uploadValue = upload
			bytesWritten = uint64(bw)
			logger.Info(fmt.Sprint("Upload ", RoundTo(uploadValue, 2), "Mbps"))

			var librespeedTestID string
			var extra defs.TelemetryExtra

			extra.ServerName = currentServer.Name

			telemetryServer.Server = currentServer.Server
			telemetryServer.Path = "/results/telemetry.php"

			id, err := sendTelemetry(telemetryServer, downloadValue, uploadValue, p, jitter, currentServer.TLog.String(), extra)
			if err != nil {
				logger.Error("Error when sending telemetry data:", "error", err)
				return nil, err
			} else {
				librespeedTestID = id
				logger.Debug("speedtest telemetry id", "id", id)
			}

			rep := clientTypes.SpeedtestResult{}

			rep.Timestamp = time.Now()
			rep.Server.Name = currentServer.Name
			rep.Server.URL = u.String()
			rep.Ping = math.Round(p*100) / 100
			rep.Jitter = math.Round(jitter*100) / 100
			rep.Download = math.Round(downloadValue*100) / 100
			rep.Upload = math.Round(uploadValue*100) / 100
			rep.BytesReceived = bytesRead
			rep.BytesSent = bytesWritten
			rep.Share = ""
			rep.ID = &librespeedTestID
			return &rep, nil

		} else {
			logger.Error("Selected server %s (%s) is not responding at the moment, try again later", currentServer.Name, u.Hostname())
		}
	}
	logger.Error("Failed to get server")
	return nil, nil
}

// sendTelemetry omit ispInfo from original code
func sendTelemetry(telemetryServer defs.TelemetryServer, download, upload, pingVal, jitter float64, logs string, extra defs.TelemetryExtra) (string, error) {
	var buf bytes.Buffer
	wr := multipart.NewWriter(&buf)

	if fIspInfo, err := wr.CreateFormField("ispinfo"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fIspInfo.Write(nil); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if fDownload, err := wr.CreateFormField("dl"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fDownload.Write([]byte(strconv.FormatFloat(download, 'f', 2, 64))); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if fUpload, err := wr.CreateFormField("ul"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fUpload.Write([]byte(strconv.FormatFloat(upload, 'f', 2, 64))); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if fPing, err := wr.CreateFormField("ping"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fPing.Write([]byte(strconv.FormatFloat(pingVal, 'f', 2, 64))); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if fJitter, err := wr.CreateFormField("jitter"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fJitter.Write([]byte(strconv.FormatFloat(jitter, 'f', 2, 64))); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if fLog, err := wr.CreateFormField("log"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fLog.Write([]byte(logs)); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	b, _ := json.Marshal(extra)
	if fExtra, err := wr.CreateFormField("extra"); err != nil {
		log.Debugf("Error creating form field: %s", err)
		return "", err
	} else if _, err = fExtra.Write(b); err != nil {
		log.Debugf("Error writing form field: %s", err)
		return "", err
	}

	if err := wr.Close(); err != nil {
		log.Debugf("Error flushing form field writer: %s", err)
		return "", err
	}

	telemetryUrl, err := telemetryServer.GetPath()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, telemetryUrl.String(), &buf)
	if err != nil {
		log.Debugf("Error when creating HTTP request: %s", err)
		return "", err
	}
	req.Header.Set("Content-Type", wr.FormDataContentType())
	req.Header.Set("User-Agent", fmt.Sprintf("inonius_v3cli_%s", runtime.GOARCH))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("Error when making HTTP request: %s", err)
		return "", err
	}
	defer resp.Body.Close()

	id, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Error when reading HTTP request: %s", err)
		return "", err
	}
	if str := strings.Split(string(id), " "); len(str) != 2 {
		return "", fmt.Errorf("server returned invalid response: %s", id)
	} else {
		return str[1], nil
	}
}

func RoundTo(num float64, precision int) float64 {
	pow := math.Pow(10, float64(precision))
	return math.Round(num*pow) / pow
}
