# wrk3
[![Build Status](https://travis-ci.org/AppsFlyer/wrk3.svg?branch=master)](https://travis-ci.org/AppsFlyer/wrk3)
[![Coverage Status](https://coveralls.io/repos/github/AppsFlyer/wrk3/badge.svg?branch=master)](https://coveralls.io/github/AppsFlyer/wrk3?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/AppsFlyer/wrk3)](https://goreportcard.com/report/github.com/AppsFlyer/wrk3)
[![Godocs](https://img.shields.io/badge/golang-documentation-blue.svg)](https://godoc.org/github.com/AppsFlyer/wrk3)

A golang generic benchmarking tool based mostly on Gil Tene's wrk2, only rewritten in go, and extended to allow running arbitrary protocols.

## Usage
using the tool is done as follows:
1. create a go project
2. import the wrk3 lib
3. implement a function that creates the actual load e.g. call an http endpoint
4. create a main function that calls the wrk3.BenchmarkCmd() with the function mentioned above.
5. compile and run the benchmark 

let's look at an example of calling an http endpoint:
```go
package main

import (
	"github.com/AppsFlyer/wrk3"
	"io"
	"io/ioutil"
	"net/http"
)

func sendHttpRequest(_ int) error {
	resp, err := http.Get("http://localhost:8080/")
	if resp != nil {
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	return err
}


func main() {
	wrk3.BenchmarkCmd(wrk3.RequestFunc(sendHttpRequest))
}
```

the function that should be provided receives a local iteration index and should return an error if the call failed.\
the errors are collected and included in the final report.\
user supplied functions can also have state that can be used to create the request, to keep state an interface implementation can be supplied.\
the interface looks as follows:
```go
type RequestHandler interface {
	ExecuteRequest(localIndex int) error
}
```
for more examples please refer to the examples folder in the codebase.

## Running a benchmark
to run a benchmark simply compile the code and run it from the command line. you can pass it the following  arguments:
1. concurrency - sets the level of concurrent requests generated by the tool
2. throughput - sets the target throughput the tool will try to reach. setting this target helps prevent coordinated omission which is the biggest advantage of wrk2
3. duration - sets the amount of time the load test will run. any valid go duration units can be used.

an example could be:\
`./bench --concurrency=20 --throughput=10000 --duration=20s`\
 other flags could be added by using the default flag library and calling `wrk3.DefineBenchmarkFlags()` before calling `flag.Parse()`

## Final report
at the end of the benchmark a report will be produced to the console, displaying the actual throughput as well as latency distribution.\
an example report of the http benchmark could look as follows:
```text
running benchmark for 20s...
benchmark results:
total duration:  20.014761791s (target duration: 20s )
total requests:  196089
errors:  0
omitted requests:  0
throughput:  9797.218775202959 (target throughput: 10000 )
latency distribution:
Quantile    | Count     | Value 
------------+-----------+-------------
0.000       | 1         | 52.159µs
50.000      | 98271     | 78.911µs
75.000      | 147068    | 434.431µs
87.500      | 171578    | 14.565375ms
93.750      | 183849    | 39.124991ms
96.875      | 189964    | 77.594623ms
98.438      | 193026    | 123.600895ms
99.219      | 194559    | 157.941759ms
99.609      | 195326    | 192.020479ms
99.805      | 195708    | 226.099199ms
99.902      | 195898    | 238.551039ms
99.951      | 195996    | 244.449279ms
99.976      | 196043    | 246.022143ms
99.988      | 196067    | 246.939647ms
99.994      | 196079    | 247.726079ms
99.997      | 196085    | 247.988223ms
99.998      | 196089    | 248.119295ms
100.000     | 196089    | 248.119295ms
```
