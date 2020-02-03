package wrk3_test

import (
	"context"
	"fmt"
	"github.com/AppsFlyer/wrk3"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"testing"
	"time"
)

func TestBasicHttpBench(t *testing.T) {
	fmt.Println("regular test...")

	server := createHttpServer(":8080")
	benchResult := wrk3.RunBenchmark(10, 1000,
		10*time.Second, createHTTPLoadFunction("http://localhost:8080/", 100*time.Millisecond))

	fmt.Println(benchResult)

	if benchResult.Throughput < 900 || benchResult.Throughput > 1100 {
		t.Fatal("actual throughput is not 1000. actual value:", benchResult.Throughput)
	}

	if benchResult.TotalTime < 9*time.Second || benchResult.TotalTime > 11*time.Second {
		t.Fatal("actual bench time is not 10sec. actual value:", benchResult.TotalTime)
	}

	distribution := benchResult.Latency.CumulativeDistribution()
	if time.Duration(distribution[len(distribution)-1].ValueAt) > 100*time.Millisecond {
		t.Fatal("large percentiles are too large.")
	}

	_ = server.Shutdown(context.Background())
}

func TestSlowHttpBench(t *testing.T) {
	server := createSlowHttpServer(":8081")
	fmt.Println("slow test...")

	benchResult := wrk3.RunBenchmark(10, 1000,
		10*time.Second, createHTTPLoadFunction("http://localhost:8081/", 5*time.Second))

	if benchResult.Throughput > 500 {
		t.Fatal("throughput too high for slow server. actual value:", benchResult.Throughput)
	}

	if benchResult.TotalTime < 9*time.Second || benchResult.TotalTime > 11*time.Second {
		t.Fatal("actual bench time is not 10sec. actual value:", benchResult.TotalTime)
	}

	distribution := benchResult.Latency.CumulativeDistribution()
	if time.Duration(distribution[len(distribution)-1].ValueAt) < 5*time.Second {
		t.Fatal("missing coordinated omission effect")
	}

	_ = server.Close()
}

func createHTTPLoadFunction(url string, timeout time.Duration) func() error {
	client := &http.Client{
		Transport: &http.Transport{
			MaxConnsPerHost: 200,
		},
		Timeout: timeout,
	}

	return func() error {
		resp, err := client.Get(url)
		if resp != nil {
			_, _ = io.Copy(ioutil.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		if err != nil {
			fmt.Println("error sending get request:", err)
		}
		return err
	}
}

func createHttpServer(addr string) *http.Server {
	server := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
		w.WriteHeader(http.StatusOK)
	})}

	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	return server
}

func createSlowHttpServer(addr string) *http.Server {
	workerCh := make(chan chan bool)
	ticker := time.NewTicker(5 * time.Millisecond)
	go func() {
		for range ticker.C {
			respCh := <-workerCh
			respCh <- true
		}
	}()

	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			resp := make(chan bool)
			workerCh <- resp
			<-resp
			w.WriteHeader(http.StatusOK)
		}),
	}

	//server.RegisterOnShutdown(func() {
	//	ticker.Stop()
	//})

	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	return server
}
