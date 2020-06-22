package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/AppsFlyer/wrk3"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/benchmark"
	testpb "google.golang.org/grpc/benchmark/grpc_testing"
)

var (
	host = flag.String("host", "localhost", "host address/name to connect to.")
	port = flag.Int("port", 50051, "host port to connect to.")

	numConn = flag.Int("c", 1, "The number of parallel connections.")
	rqSize  = flag.Int("req", 1, "Request message size in bytes.")
	rspSize = flag.Int("resp", 1, "Response message size in bytes.")
)

type bench struct {
	clients []testpb.BenchmarkServiceClient
	req     *testpb.SimpleRequest
}

func (b *bench) ExecuteRequest(localIndex int) error {
	client := b.clients[localIndex%len(b.clients)]
	_, err := client.UnaryCall(context.Background(), b.req)
	return err
}

func main() {
	wrk3.DefineBenchmarkFlags()
	flag.Parse()

	request := &testpb.SimpleRequest{
		ResponseType: testpb.PayloadType_COMPRESSABLE,
		ResponseSize: int32(*rspSize),
		Payload: &testpb.Payload{
			Type: testpb.PayloadType_COMPRESSABLE,
			Body: make([]byte, *rqSize),
		},
	}

	b := &bench{
		clients: buildClients(*numConn, *host, *port),
		req:     request,
	}

	wrk3.BenchmarkCmd(b)
}

func buildClients(numConn int, host string, port int) []testpb.BenchmarkServiceClient {
	ctx, connectCancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer connectCancel()

	clients := make([]testpb.BenchmarkServiceClient, numConn)
	addr := fmt.Sprintf("%v:%d", host, port)
	for i := range clients {
		conn := benchmark.NewClientConnWithContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock())
		clients[i] = testpb.NewBenchmarkServiceClient(conn)
	}
	return clients
}
