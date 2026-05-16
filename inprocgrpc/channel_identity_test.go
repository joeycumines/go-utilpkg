package inprocgrpc_test

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

type channelLoopOwner struct {
	loop *goeventloop.Loop
}

func (o *channelLoopOwner) OwnsLoop(candidate *goeventloop.Loop) bool {
	return o != nil && o.loop != nil && candidate == o.loop
}

type nilUnsafeLoopOwner struct {
	loop *goeventloop.Loop
}

func (o *nilUnsafeLoopOwner) OwnsLoop(candidate *goeventloop.Loop) bool {
	return candidate == o.loop
}

func TestChannelSharesLoop(t *testing.T) {
	loop, err := goeventloop.New()
	foreign, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	channel := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	if !channel.SharesLoop(&channelLoopOwner{loop: loop}) {
		t.Fatal("channel did not match its exact loop owner")
	}
	if channel.SharesLoop(&channelLoopOwner{loop: foreign}) {
		t.Fatal("channel matched a foreign loop owner")
	}
	if channel.SharesLoop((*channelLoopOwner)(nil)) ||
		channel.SharesLoop((*nilUnsafeLoopOwner)(nil)) ||
		(*inprocgrpc.Channel)(nil).SharesLoop(&channelLoopOwner{loop: loop}) {
		t.Fatal("nil channel or owner matched a loop")
	}
}
