package wrk3

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBasicHttpBench(t *testing.T) {
	server := createHTTPServer(":8080")
	expectedThroughput := float64(1000)
	expectedDuration := 10 * time.Second
	benchResult := Benchmark{
		Concurrency: 10,
		Throughput:  expectedThroughput,
		Duration:    expectedDuration,
		SendRequest: createHTTPLoadFunction("http://localhost:8080/", 100*time.Millisecond),
	}.Run()

	assert.Equal(t, expectedThroughput/100, math.Round(benchResult.Throughput/100), "Throughput")
	assert.Equal(t, expectedDuration, benchResult.TotalTime.Truncate(time.Second), "bench time")

	distribution := benchResult.Latency.CumulativeDistribution()
	assert.True(t, time.Duration(distribution[len(distribution)-1].ValueAt) < 100*time.Millisecond, "large percentiles are too large")

	_ = server.Shutdown(context.Background())
}

func TestSlowHttpBench(t *testing.T) {
	server := createSlowHTTPServer(":8081")
	fmt.Println("slow test...")

	expectedDuration := 10 * time.Second
	benchResult := Benchmark{
		Concurrency: 10,
		Throughput:  1000,
		Duration:    expectedDuration,
		SendRequest: createHTTPLoadFunction("http://localhost:8081/", 5*time.Second),
	}.Run()

	assert.True(t, benchResult.Throughput < 500, "throughput too high for slow server. actual value %s", benchResult.Throughput)
	assert.Equal(t, expectedDuration, benchResult.TotalTime.Truncate(time.Second), "bench time")

	distribution := benchResult.Latency.CumulativeDistribution()
	assert.True(t, 5*time.Second < time.Duration(distribution[len(distribution)-1].ValueAt), "missing coordinated omission effect")

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

func createHTTPServer(addr string) *http.Server {
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

func createSlowHTTPServer(addr string) *http.Server {
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

	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	return server
}

func TestEventsGenerator(t *testing.T) {
	throughput := 100000.
	start := time.Now()
	expectedDuration := 100 * time.Millisecond
	generator := newEventsGenerator(expectedDuration, int(throughput*2))
	generator.generateEvents(throughput, 10)
	generator.awaitDone()
	actualDuration := time.Since(start)

	assert.Equal(t, expectedDuration, actualDuration.Truncate(time.Millisecond), "generation duration")
	assert.Equal(t, 0, generator.omittedCount(), "omitted count")
}

func TestSendRequestsWithErrors(t *testing.T) {
	expectedResults := 666
	e := configureExecutioner(expectedResults, func() error { return fmt.Errorf("baaahhh") })
	e.sendRequests()

	result := <-e.results
	assert.Equal(t, expectedResults, result.errors, "errors")
	assert.Equal(t, expectedResults, result.counter, "count")
}

func TestSendRequests(t *testing.T) {
	expectedResults := 666
	e := configureExecutioner(expectedResults, func() error { return nil })
	e.sendRequests()

	result := <-e.results
	assert.Equal(t, 0, result.errors, "errors")
	assert.Equal(t, expectedResults, result.counter, "count")
}

func TestSummarizeResults(t *testing.T) {
	expectedResults := 500
	expectedLatency := time.Millisecond
	e := configureExecutioner(expectedResults, func() error {
		time.Sleep(expectedLatency)
		return nil
	})
	e.sendRequests()

	result := e.summarizeResults()
	assert.Equal(t, 0, result.Errors, "errors")
	assert.Equal(t, expectedResults, result.Counter, "count")
	assert.Equal(t, expectedResults, int(result.Latency.TotalCount()), "histogram counter")
	assert.LessOrEqual(t, expectedLatency, time.Duration(result.Latency.Min()).Truncate(time.Millisecond), "histogram min time")
	assert.GreaterOrEqual(t, expectedLatency*3, time.Duration(result.Latency.Min()), "histogram min time")
}

func configureExecutioner(eventCount int, requestFunc RequestFunc) *executioner {
	events := make(chan time.Time, eventCount)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	e := executioner{
		results: make(chan localResult, 1),
		eventsGenerator: eventsGenerator{
			doneCtx:   ctx,
			cancel:    cancel,
			eventsBuf: events,
		},
		benchmark: Benchmark{
			Concurrency: 1,
			SendRequest: requestFunc,
		},
		startTime: time.Now(),
	}

	for i := 0; i < eventCount; i++ {
		events <- time.Now()
	}
	close(events)
	return &e
}
