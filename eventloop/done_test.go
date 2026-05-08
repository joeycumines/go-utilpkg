package eventloop

import "testing"

func TestDonePublishesTerminalCompletion(t *testing.T) {
	loop := New()
	done := loop.Done()
	if done == nil {
		t.Fatal("Done returned nil")
	}
	if done != loop.Done() {
		t.Fatal("Done signal changed during loop lifetime")
	}
	select {
	case <-done:
		t.Fatal("Done closed before terminal cleanup")
	default:
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Done remained open after Close returned")
	}
}

func TestDoneNilLoopPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil Loop Done did not panic")
		}
	}()
	(*Loop)(nil).Done()
}
