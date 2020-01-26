package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
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
	var includeCo bool

	flag.IntVar(&concurrency, "concurrency", 10, "level of benchmark concurrency")
	flag.IntVar(&throughput, "throughput", 10000, "target benchmark throughput")
	flag.DurationVar(&duration, "duration", 20*time.Second, "benchmark time period")
	flag.BoolVar(&includeCo, "co", true, "print coordinated omission latency distribution")
	flag.Parse()

	fmt.Printf("running benchmark for %v...\n", duration)
	result := benchmark(concurrency, throughput, duration, target)
	printBenchResult(throughput, duration, result, includeCo)
}

func printBenchResult(throughput int, duration time.Duration, result BenchResult, includeCoHistogram bool) {
	fmt.Println("benchmark results:")
	fmt.Println("total duration: ", result.totalTime, "(target duration:", duration, ")")
	fmt.Println("total requests: ", result.counter)
	fmt.Println("errors: ", result.errors)
	fmt.Println("omitted requests: ", result.omitted)
	fmt.Println("throughput: ", result.throughput, "(target throughput:", throughput, ")")
	fmt.Println("latency distribution:")
	printHistogram(result.histogram)
	if includeCoHistogram {
		fmt.Println("co(coordinated omission) latency distribution:")
		printHistogram(result.coHistogram)
	}
}

func printHistogram(hist *hdrhistogram.Histogram) {
	brackets := hist.CumulativeDistribution()

	fmt.Println("Quantile    | Count     | Value(usec) ")
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
	errors      int
	counter     int
	histogram   *hdrhistogram.Histogram
	coHistogram *hdrhistogram.Histogram
}

type BenchResult struct {
	throughput  float64
	counter     int
	errors      int
	omitted     int
	histogram   *hdrhistogram.Histogram
	coHistogram *hdrhistogram.Histogram
	totalTime   time.Duration
}

func benchmark(concurrency int, throughput int, duration time.Duration, sendRequest RequestFunc) BenchResult {
	rateLimiter := rate.NewLimiter(rate.Limit(throughput), concurrency)

	var omitted int64 = 0
	eventsBuf := make(chan time.Time, 10000)
	doneCtx, cancel := context.WithTimeout(context.Background(), duration)
	go func() {
		var _omitted int64 = 0
		for err := rateLimiter.Wait(doneCtx); err == nil; err = rateLimiter.Wait(doneCtx) {
			select {
			case eventsBuf <- time.Now():
			default:
				_omitted += 1
			}
		}

		atomic.StoreInt64(&omitted, _omitted)
		close(eventsBuf)
	}()

	results := make(chan localResult, concurrency)
	start := time.Now()
	for i := 0; i < concurrency; i++ {
		go func() {
			res := localResult{
				errors:      0,
				counter:     0,
				histogram:   createHistogram(),
				coHistogram: createHistogram(),
			}

			done := false
			for !done {
				select {
				case <-doneCtx.Done():
					done = true
				case t := <-eventsBuf:
					res.counter += 1
					start := time.Now()
					err := sendRequest()
					if err != nil {
						res.errors += 1
					}

					if err = res.coHistogram.RecordValue(int64(time.Since(t))); err != nil {
						log.Println("error reporting to hdr", err)
					}

					if err = res.histogram.RecordValue(int64(time.Since(start))); err != nil {
						log.Println("error reporting to hdr", err)
					}
				}
			}

			results <- res
		}()
	}

	<-doneCtx.Done()
	cancel()

	counter := 0
	errors := 0
	histogram := createHistogram()
	coHistogram := createHistogram()

	for i := 0; i < concurrency; i++ {
		localRes := <-results
		counter += localRes.counter
		errors += localRes.errors
		histogram.Merge(localRes.histogram)
		coHistogram.Merge(localRes.coHistogram)
	}

	totalTime := time.Since(start)

	return BenchResult{
		throughput:  float64(counter) / totalTime.Seconds(),
		counter:     counter,
		errors:      errors,
		omitted:     int(omitted),
		histogram:   histogram,
		coHistogram: coHistogram,
		totalTime:   totalTime,
	}

}

func createHistogram() *hdrhistogram.Histogram {
	return hdrhistogram.New(0, int64(time.Minute), 3)
}
