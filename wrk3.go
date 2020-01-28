package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/codahale/hdrhistogram"
	"golang.org/x/time/rate"
)

func main() {
	Benchmark(sendHttpRequest)
}

type RequestFunc func() error

func Benchmark(target RequestFunc) {
	var concurrency int
	var throughput int
	var duration time.Duration

	flag.IntVar(&concurrency, "concurrency", 10, "level of benchmark concurrency")
	flag.IntVar(&throughput, "throughput", 10000, "target benchmark throughput")
	flag.DurationVar(&duration, "duration", 20*time.Second, "benchmark time period")
	flag.Parse()

	fmt.Printf("running benchmark for %v...\n", duration)
	result := benchmark(concurrency, throughput, duration, target)
	printBenchResult(throughput, duration, result)
}

func printBenchResult(throughput int, duration time.Duration, result BenchResult) {
	fmt.Println("benchmark results:")
	fmt.Println("total duration: ", result.totalTime, "(target duration:", duration, ")")
	fmt.Println("total requests: ", result.counter)
	fmt.Println("errors: ", result.errors)
	fmt.Println("omitted requests: ", result.omitted)
	fmt.Println("throughput: ", result.throughput, "(target throughput:", throughput, ")")
	fmt.Println("latency distribution:")
	printHistogram(result.latency)
}

func printHistogram(hist *hdrhistogram.Histogram) {
	brackets := hist.CumulativeDistribution()

	fmt.Println("Quantile    | Count     | Value ")
	fmt.Println("------------+-----------+-------------")

	for _, q := range brackets {
		fmt.Printf("%-08.3f    | %-09d | %v\n", q.Quantile, q.Count, time.Duration(q.ValueAt))
	}
}

func sendHttpRequest() error {
	resp, err := http.Get("http://localhost:8080/")
	if resp != nil {
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	if err != nil {
		fmt.Println("error sending get request:", err)
	}
	return err
}

type localResult struct {
	errors  int
	counter int
	latency *hdrhistogram.Histogram
}

type BenchResult struct {
	throughput float64
	counter    int
	errors     int
	omitted    int
	latency    *hdrhistogram.Histogram
	totalTime  time.Duration
}

func benchmark(concurrency int, throughput int, duration time.Duration, sendRequest RequestFunc) BenchResult {
	eventsBuf := make(chan time.Time, 10000)
	omittedChan := make(chan int, 1)
	doneCtx, cancel := context.WithTimeout(context.Background(), duration)

	go generateEvents(throughput, concurrency, doneCtx, eventsBuf, omittedChan)

	results := make(chan localResult, concurrency)
	start := time.Now()
	for i := 0; i < concurrency; i++ {
		go sendRequests(doneCtx, sendRequest, eventsBuf, results)
	}

	<-doneCtx.Done()
	cancel()

	return summarizeResults(concurrency, results, start, omittedChan)
}

func generateEvents(throughput int, concurrency int, doneCtx context.Context, eventsBuf chan time.Time, omittedChan chan int) {
	omitted := 0
	rateLimiter := rate.NewLimiter(rate.Limit(throughput), 2*concurrency)
	for err := rateLimiter.Wait(doneCtx); err == nil; err = rateLimiter.Wait(doneCtx) {
		select {
		case eventsBuf <- time.Now():
		default:
			omitted++
		}
	}

	close(eventsBuf)
	omittedChan <- omitted
}

func sendRequests(doneCtx context.Context, sendRequest RequestFunc, eventsBuf <-chan time.Time, results chan localResult) {
	res := localResult{
		errors:  0,
		counter: 0,
		latency: createHistogram(),
	}

	done := false
	for !done {
		select {
		case <-doneCtx.Done():
			done = true
		case t, ok := <-eventsBuf:
			if ok {
				res.counter++
				err := sendRequest()
				if err != nil {
					res.errors++
				}

				if err = res.latency.RecordValue(int64(time.Since(t))); err != nil {
					log.Println("failed to record latency", err)
				}
			} else {
				done = true
			}
		}
	}

	results <- res
}

func summarizeResults(concurrency int, results <-chan localResult, start time.Time, omittedChan chan int) BenchResult {
	counter := 0
	errors := 0
	latency := createHistogram()

	for i := 0; i < concurrency; i++ {
		localRes := <-results
		counter += localRes.counter
		errors += localRes.errors
		latency.Merge(localRes.latency)
	}

	totalTime := time.Since(start)

	return BenchResult{
		throughput: float64(counter) / totalTime.Seconds(),
		counter:    counter,
		errors:     errors,
		omitted:    <-omittedChan,
		latency:    latency,
		totalTime:  totalTime,
	}
}

func createHistogram() *hdrhistogram.Histogram {
	return hdrhistogram.New(0, int64(time.Minute), 3)
}
