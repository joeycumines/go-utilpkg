package inprocgrpc

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestClientStreamRecvLoopDeathWithoutClaim covers the loop-death race in
// which the caller context and the loop share one cancellation: the RPC's
// watch goroutine can win its initial select on the context branch (never
// reaching the loop.Done branch), and the terminal gets selected only by the
// rpcControlSchedulerDone reducer through a claim probe. That path used to
// skip releaseAfterScheduler entirely, so installSchedulerRecovery never ran:
// materialReady/resultReady/recoveryReady stayed open, an in-flight RecvMsg
// hung forever, and the RPC never released (Done stayed open, hanging any
// Close that joined the root control). Regression: with no admitted terminal
// claim, a loop death must still publish scheduler recovery so the in-flight
// recv returns and the RPC releases.
func TestClientStreamRecvLoopDeathWithoutClaim(t *testing.T) {
	const iterations = 100
	recvErrs := make([]error, iterations)
	releases := make([]bool, iterations)
	for iteration := range iterations {
		func() {
			loop, err := goeventloop.New()
			if err != nil {
				t.Fatal(err)
			}
			// One context drives both the loop and the RPC, so context
			// cancellation and loop death become ready at the same moment.
			ctx, cancel := context.WithCancel(context.Background())
			loopDone := make(chan struct{})
			go func() {
				defer close(loopDone)
				_ = loop.Run(ctx)
			}()

			channel := NewChannel(WithLoop(loop))
			channel.RegisterStreamHandler("/test.Service/Bidi", func(
				_ context.Context,
				stream *RPCStream,
			) {
				var recvNext func()
				recvNext = func() {
					stream.Recv().Recv(func(msg any, recvErr error) {
						if recvErr != nil {
							stream.Finish(nil)
							return
						}
						// Echo forever; the client sends exactly one message
						// and then abandons the stream in-flight.
						if sendErr := stream.Send().Send(msg); sendErr != nil {
							stream.Finish(sendErr)
							return
						}
						recvNext()
					})
				}
				recvNext()
			})

			cs, err := channel.NewStream(ctx, &grpc.StreamDesc{
				ClientStreams: true,
				ServerStreams: true,
			}, "/test.Service/Bidi")
			if err != nil {
				t.Fatal(err)
			}
			adapter := cs.(*clientStreamAdapter)

			if err := cs.SendMsg(&wrapperspb.StringValue{Value: "one"}); err != nil {
				t.Fatal(err)
			}
			echo := new(wrapperspb.StringValue)
			if err := cs.RecvMsg(echo); err != nil {
				t.Fatal(err)
			}

			recvDone := make(chan error, 1)
			recvStarted := make(chan struct{})
			go func() {
				close(recvStarted)
				recvDone <- cs.RecvMsg(new(wrapperspb.StringValue))
			}()
			<-recvStarted
			cancel()

			select {
			case err := <-recvDone:
				recvErrs[iteration] = err
			case <-time.After(10 * time.Second):
				t.Fatalf(
					"iteration %d: RecvMsg did not complete after loop death "+
						"(scheduler recovery was never published)",
					iteration,
				)
			}

			select {
			case <-adapter.Done():
				releases[iteration] = true
			case <-time.After(10 * time.Second):
				t.Fatalf(
					"iteration %d: RPC never released (Done open) after loop death",
					iteration,
				)
			}

			select {
			case <-loopDone:
			case <-time.After(10 * time.Second):
				t.Fatalf("iteration %d: loop did not stop", iteration)
			}
		}()
	}

	// Every iteration must return an error (never a message: the server only
	// echoes the single request and the client abandons the stream), and the
	// error must be the scheduler loss or the caller cancellation that the
	// terminal selection raced for.
	for iteration, err := range recvErrs {
		switch status.Code(err) {
		case codes.Canceled, codes.Unavailable:
		default:
			t.Fatalf(
				"iteration %d: RecvMsg = %v, want Canceled or Unavailable",
				iteration,
				err,
			)
		}
		if !releases[iteration] {
			t.Fatalf("iteration %d: RPC did not release", iteration)
		}
	}
}
