// tournament_test.go benchmarks our inprocgrpc.Channel against
// fullstorydev/grpchan/inprocgrpc.Channel and a real gRPC server+client
// over loopback, using identical workloads for fair comparison.
//
// All three competitors use the same TestService proto definition
// (in this package) and the same TestServer implementation.
//
// Run with:
//
//	go test -bench=BenchmarkTournament -benchmem -count=1 ./...
package grpchantest_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"testing"
	"time"

	fsinprocgrpc "github.com/fullstorydev/grpchan/inprocgrpc"
	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	grpchantest "github.com/joeycumines/go-inprocgrpc/internal/grpchantest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------- Competitor Setup ----------

// setupOurChannel creates our event-loop-based inprocgrpc channel.
func setupOurChannel(b *testing.B) grpc.ClientConnInterface {
	b.Helper()
	loop, err := eventloop.New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)
	b.Cleanup(cancel)

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
	grpchantest.RegisterTestServiceServer(ch, &grpchantest.TestServer{})
	return ch
}

// setupGrpchanChannel creates a fullstorydev/grpchan inprocgrpc channel.
func setupGrpchanChannel(b *testing.B) grpc.ClientConnInterface {
	b.Helper()
	ch := &fsinprocgrpc.Channel{}
	grpchantest.RegisterTestServiceServer(ch, &grpchantest.TestServer{})
	return ch
}

// setupRealGRPC creates a real gRPC server over loopback TCP.
func setupRealGRPC(b *testing.B) grpc.ClientConnInterface {
	b.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	srv := grpc.NewServer()
	grpchantest.RegisterTestServiceServer(srv, &grpchantest.TestServer{})
	go func() { _ = srv.Serve(lis) }()
	b.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { conn.Close() })
	return conn
}

type competitor struct {
	name  string
	setup func(b *testing.B) grpc.ClientConnInterface
}

var competitors = []competitor{
	{"OurChannel", setupOurChannel},
	{"GrpchanChannel", setupGrpchanChannel},
	{"RealGRPC", setupRealGRPC},
}

// ---------- Unary Throughput ----------

func BenchmarkTournament_Unary(b *testing.B) {
	for _, c := range competitors {
		b.Run(c.name, func(b *testing.B) {
			ch := c.setup(b)
			cli := grpchantest.NewTestServiceClient(ch)

			req := &grpchantest.Message{Payload: make([]byte, 32)}
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := cli.Unary(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------- Unary Large Message ----------

func BenchmarkTournament_UnaryLargeMessage(b *testing.B) {
	sizes := []int{64, 1024, 10240, 102400, 1048576}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			for _, c := range competitors {
				b.Run(c.name, func(b *testing.B) {
					ch := c.setup(b)
					cli := grpchantest.NewTestServiceClient(ch)

					req := &grpchantest.Message{Payload: make([]byte, size)}
					b.ResetTimer()
					b.ReportAllocs()

					for i := 0; i < b.N; i++ {
						_, err := cli.Unary(context.Background(), req)
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// ---------- Server-Streaming Throughput ----------

func BenchmarkTournament_ServerStream(b *testing.B) {
	for _, c := range competitors {
		b.Run(c.name, func(b *testing.B) {
			ch := c.setup(b)
			cli := grpchantest.NewTestServiceClient(ch)

			// Server sends 100 messages per stream.
			req := &grpchantest.Message{
				Count:   100,
				Payload: make([]byte, 32),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				stream, err := cli.ServerStream(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
				for {
					_, recvErr := stream.Recv()
					if recvErr == io.EOF {
						break
					}
					if recvErr != nil {
						b.Fatal(recvErr)
					}
				}
			}
		})
	}
}

// ---------- Bidi-Streaming Echo Throughput ----------

func BenchmarkTournament_BidiEcho(b *testing.B) {
	for _, c := range competitors {
		b.Run(c.name, func(b *testing.B) {
			ch := c.setup(b)
			cli := grpchantest.NewTestServiceClient(ch)

			stream, err := cli.BidiStream(context.Background())
			if err != nil {
				b.Fatal(err)
			}
			msg := &grpchantest.Message{Payload: make([]byte, 32)}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if err := stream.Send(msg); err != nil {
					b.Fatal(err)
				}
				if _, err := stream.Recv(); err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			_ = stream.CloseSend()
		})
	}
}

// ---------- Concurrent Unary Load ----------

func BenchmarkTournament_ConcurrentUnary(b *testing.B) {
	concurrencies := []int{10, 50, 100}
	for _, conc := range concurrencies {
		b.Run(fmt.Sprintf("goroutines_%d", conc), func(b *testing.B) {
			for _, c := range competitors {
				b.Run(c.name, func(b *testing.B) {
					ch := c.setup(b)
					cli := grpchantest.NewTestServiceClient(ch)

					b.SetParallelism(conc)
					b.ResetTimer()
					b.ReportAllocs()

					b.RunParallel(func(pb *testing.PB) {
						req := &grpchantest.Message{Payload: make([]byte, 32)}
						for pb.Next() {
							_, err := cli.Unary(context.Background(), req)
							if err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			}
		})
	}
}

// ---------- Latency Percentiles ----------

// BenchmarkTournament_LatencyPercentiles measures p50/p90/p95/p99
// latency for unary RPCs under moderate load (10 concurrent goroutines).
// It uses a fixed iteration count and custom timing rather than testing.B
// iteration scaling, reporting percentiles via b.ReportMetric.
func BenchmarkTournament_LatencyPercentiles(b *testing.B) {
	const (
		warmup     = 100
		iterations = 5000
		workers    = 10
	)
	for _, c := range competitors {
		b.Run(c.name, func(b *testing.B) {
			ch := c.setup(b)
			cli := grpchantest.NewTestServiceClient(ch)
			req := &grpchantest.Message{Payload: make([]byte, 32)}

			// Warm-up phase to establish connections / prime caches.
			for i := 0; i < warmup; i++ {
				if _, err := cli.Unary(context.Background(), req); err != nil {
					b.Fatal(err)
				}
			}

			// Collect latency samples across workers.
			perWorker := iterations / workers
			latencies := make([]time.Duration, 0, iterations)
			ch2 := make(chan []time.Duration, workers)

			for w := 0; w < workers; w++ {
				go func() {
					local := make([]time.Duration, 0, perWorker)
					for i := 0; i < perWorker; i++ {
						start := time.Now()
						if _, err := cli.Unary(context.Background(), req); err != nil {
							// Can't call b.Fatal from non-test goroutine;
							// channel will be short, causing timeout below.
							return
						}
						local = append(local, time.Since(start))
					}
					ch2 <- local
				}()
			}
			for w := 0; w < workers; w++ {
				latencies = append(latencies, <-ch2...)
			}

			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			p := func(pct float64) time.Duration {
				idx := int(float64(len(latencies)) * pct)
				if idx >= len(latencies) {
					idx = len(latencies) - 1
				}
				return latencies[idx]
			}
			b.ReportMetric(float64(p(0.50).Microseconds()), "p50-µs")
			b.ReportMetric(float64(p(0.90).Microseconds()), "p90-µs")
			b.ReportMetric(float64(p(0.95).Microseconds()), "p95-µs")
			b.ReportMetric(float64(p(0.99).Microseconds()), "p99-µs")
		})
	}
}
