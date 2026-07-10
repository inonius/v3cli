package speedtest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"os"
	"sync"

	clientTypes "github.com/inonius/v3cli/api/client"
	"github.com/librespeed/speedtest-cli/defs"
	log "github.com/sirupsen/logrus"
)

type PingJob struct {
	Index  int
	Server defs.Server
}

type PingResult struct {
	Index int
	Ping  float64
}

// SpeedTest is the actual main function that handles the speed test(s)
func Speedtest(c clientTypes.Client, ctx context.Context, logger *slog.Logger, servers []defs.Server) (*clientTypes.SpeedtestResult, error) {
	// check for suppressed output flags
	var silent bool = true
	/*
		if c.Bool(defs.OptionVersion) {
			log.SetOutput(os.Stdout)
			log.Warnf("%s %s (built on %s)", defs.ProgName, defs.ProgVersion, defs.BuildDate)
			log.Warn("https://github.com/librespeed/speedtest-cli")
			log.Warn("Licensed under GNU Lesser General Public License v3.0")
			log.Warn("LibreSpeed\tCopyright (C) 2016-2020 Federico Dossena")
			log.Warn("librespeed-cli\tCopyright (C) 2020 Maddie Zhan")
			log.Warn("librespeed.org\tCopyright (C)")
			return nil
		}
	*/
	noICMP := c.Config.NoICMP

	// HTTP requests timeout
	//c.HttpClient.Timeout = time.Duration(c.Config.Timeout) * time.Second
	var network string

	transport := c.HttpClient.Transport.(*http.Transport).Clone()
	//transport := http.DefaultTransport.(*http.Transport).Clone()

	if caCertFileName := c.Config.CACert; caCertFileName != "" {
		caCert, err := os.ReadFile(caCertFileName)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: c.Config.IgnoreTLSError,
			RootCAs:            caCertPool,
		}
	} else {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: c.Config.IgnoreTLSError,
		}
	}

	http.DefaultClient.Transport = transport

	/*
		// if --server is given, do speed tests with all of them
		if len(c.Config.Server) > 0 {
			_, err := doSpeedTest(c, &ctx, logger, servers, telemetryServer, network, silent, noICMP)
			return nil, err
		} else {
	*/
	// else select the fastest server from the list
	logger.Debug("Selecting the fastest server based on ping")

	var wg sync.WaitGroup
	jobs := make(chan PingJob, len(servers))
	results := make(chan PingResult, len(servers))
	done := make(chan struct{})

	pingList := make(map[int]float64)

	// spawn 10 concurrent pingers
	for i := 0; i < 10; i++ {
		go pingWorker(jobs, results, &wg, c.Config.Source, network, noICMP)
	}

	// send ping jobs to workers
	for idx, server := range servers {
		wg.Add(1)
		jobs <- PingJob{Index: idx, Server: server}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(done)
	}()

Loop:
	for {
		select {
		case result := <-results:
			pingList[result.Index] = result.Ping
		case <-done:
			break Loop
		}
	}

	if len(pingList) == 0 {
		log.Fatal("No server is currently available, please try again later.")
	}

	// get the fastest server's index in the `servers` array
	var serverIdx int
	for idx, newPing := range pingList {
		oldPing, ok := pingList[serverIdx]

		if ok {
			if newPing > 0 && newPing <= oldPing {
				serverIdx = idx
			}
		} else {
			serverIdx = idx
		}
	}

	// do speed test on the server
	response, err := doSpeedTest(c, &ctx, logger, []defs.Server{servers[serverIdx]}, network, silent, noICMP)
	return response, err
	//}
}

func pingWorker(jobs <-chan PingJob, results chan<- PingResult, wg *sync.WaitGroup, srcIp, network string, noICMP bool) {
	for job := range jobs {
		server := job.Server
		// get the URL of the speed test server from the JSON
		u, err := server.GetURL()
		if err != nil {
			log.Debugf("Server URL is invalid for %s (%s), skipping", server.Name, server.Server)
			wg.Done()
			continue
		}

		// check the server is up by accessing the ping URL and checking its returned value == empty and status code == 200
		if server.IsUp() {
			// skip ICMP if option given
			server.NoICMP = noICMP

			// if server is up, get ping
			ping, _, err := server.ICMPPingAndJitter(1, srcIp, network)
			if err != nil {
				log.Debugf("Can't ping server %s (%s), skipping", server.Name, u.Hostname())
				wg.Done()
				continue
			}
			// return result
			results <- PingResult{Index: job.Index, Ping: ping}
			wg.Done()
		} else {
			log.Debugf("Server %s (%s) doesn't seem to be up, skipping", server.Name, u.Hostname())
			wg.Done()
		}
	}
}
