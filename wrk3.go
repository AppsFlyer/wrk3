package wrk3

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/codahale/hdrhistogram"
	"golang.org/x/time/rate"
)

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
	result := RunBenchmark(concurrency, throughput, duration, target)
	PrintBenchResult(throughput, duration, result)
}

func PrintBenchResult(throughput int, duration time.Duration, result BenchResult) {
	fmt.Println("benchmark results:")
	fmt.Println("total duration: ", result.TotalTime, "(target duration:", duration, ")")
	fmt.Println("total requests: ", result.Counter)
	fmt.Println("errors: ", result.Errors)
	fmt.Println("omitted requests: ", result.Omitted)
	fmt.Println("throughput: ", result.Throughput, "(target throughput:", throughput, ")")
	fmt.Println("latency distribution:")
	PrintHistogram(result.Latency)
}

func PrintHistogram(hist *hdrhistogram.Histogram) {
	brackets := hist.CumulativeDistribution()

	fmt.Println("Quantile    | Count     | Value ")
	fmt.Println("------------+-----------+-------------")

	for _, q := range brackets {
		fmt.Printf("%-08.3f    | %-09d | %v\n", q.Quantile, q.Count, time.Duration(q.ValueAt))
	}
}

type localResult struct {
	errors  int
	counter int
	latency *hdrhistogram.Histogram
}

type BenchResult struct {
	Throughput float64
	Counter    int
	Errors     int
	Omitted    int
	Latency    *hdrhistogram.Histogram
	TotalTime  time.Duration
}

func RunBenchmark(concurrency int, throughput int, duration time.Duration, sendRequest RequestFunc) BenchResult {
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
		Throughput: float64(counter) / totalTime.Seconds(),
		Counter:    counter,
		Errors:     errors,
		Omitted:    <-omittedChan,
		Latency:    latency,
		TotalTime:  totalTime,
	}
}

func createHistogram() *hdrhistogram.Histogram {
	return hdrhistogram.New(0, int64(time.Minute), 3)
}
