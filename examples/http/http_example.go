package main

import (
	"github.com/AppsFlyer/wrk3"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

var client = &http.Client{
	Timeout: 1 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:    100,
		MaxConnsPerHost: 100,
		IdleConnTimeout: 90 * time.Second,
	},
}

func sendHttpRequest(_ int) error {
	resp, err := client.Get("http://localhost:8080/")
	if resp != nil {
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	return err
}

func main() {
	wrk3.BenchmarkCmd(wrk3.RequestFunc(sendHttpRequest))
}
