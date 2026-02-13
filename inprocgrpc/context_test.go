package inprocgrpc

import (
	"context"
	"testing"
	"time"
)

func TestNoValuesContext_Values(t *testing.T) {
	type key string
	parent := context.WithValue(context.Background(), key("k"), "v")
	nv := noValuesContext{parent}

	// Parent value should NOT be visible
	if v := nv.Value(key("k")); v != nil {
		t.Errorf("value leaked through: %v", v)
	}

	// Values set on the noValuesContext ARE visible
	child := context.WithValue(nv, key("k2"), "v2")
	if v := child.Value(key("k2")); v != "v2" {
		t.Errorf("child value not visible: %v", v)
	}

	// Parent value still not visible through child
	if v := child.Value(key("k")); v != nil {
		t.Errorf("parent value leaked through child: %v", v)
	}
}

func TestNoValuesContext_Cancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	nv := noValuesContext{parent}

	select {
	case <-nv.Done():
		t.Error("should not be done yet")
	default:
	}

	cancel()

	select {
	case <-nv.Done():
		// ok
	case <-time.After(time.Second):
		t.Error("cancellation not propagated")
	}

	if err := nv.Err(); err != context.Canceled {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestNoValuesContext_Deadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	nv := noValuesContext{parent}
	dl, ok := nv.Deadline()
	if !ok {
		t.Error("deadline not propagated")
	}
	if !dl.Equal(deadline) {
		t.Errorf("got %v, want %v", dl, deadline)
	}
}

func TestMakeServerContext(t *testing.T) {
	type key string

	parent := context.WithValue(context.Background(), key("client-key"), "client-val")
	svrCtx := makeServerContext(parent)

	// Client values should NOT be visible
	if v := svrCtx.Value(key("client-key")); v != nil {
		t.Errorf("client value leaked: %v", v)
	}

	// Client context should be retrievable
	clientCtx := ClientContext(svrCtx)
	if clientCtx == nil {
		t.Fatal("ClientContext returned nil")
	}
	if v := clientCtx.Value(key("client-key")); v != "client-val" {
		t.Errorf("client context missing value: %v", v)
	}
}

func TestInprocessAddr(t *testing.T) {
	addr := inprocessAddr{}
	if addr.Network() != "inproc" {
		t.Errorf("Network: %q", addr.Network())
	}
	if addr.String() != "0" {
		t.Errorf("String: %q", addr.String())
	}
	if addr.AuthType() != "inproc" {
		t.Errorf("AuthType: %q", addr.AuthType())
	}
}
