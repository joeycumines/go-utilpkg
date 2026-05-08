package grpchantest_test

import (
	"testing"

	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	grpchantest "github.com/joeycumines/go-inprocgrpc/internal/grpchantest"
)

func TestRunChannelTestCases(t *testing.T) {
	loop := eventloop.New()
	ctx := t.Context()
	go loop.Run(ctx)

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

	grpchantest.RegisterTestServiceServer(ch, &grpchantest.TestServer{})

	grpchantest.RunChannelTestCases(t, ch, true)
}
