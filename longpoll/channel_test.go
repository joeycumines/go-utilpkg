package longpoll

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"
)

// simple input/output test cases
func TestChannel(t *testing.T) {
	tests := []struct {
		name           string
		ctx            context.Context
		cfg            *ChannelConfig
		channel        func() <-chan int
		handler        func(value int) error
		expectedResult error
		expectedPanic  string
	}{
		{
			name: "NilContext",
			ctx:  nil,
			channel: func() <-chan int {
				ch := make(chan int)
				close(ch)
				return ch
			},
			handler:       func(value int) error { return nil },
			expectedPanic: "longpoll: nil context",
		},
		{
			name: "NilChannel",
			ctx:  context.Background(),
			channel: func() <-chan int {
				return nil
			},
			handler:       func(value int) error { return nil },
			expectedPanic: "longpoll: nil channel",
		},
		{
			name: "NilHandler",
			ctx:  context.Background(),
			channel: func() <-chan int {
				ch := make(chan int)
				close(ch)
				return ch
			},
			handler:       nil,
			expectedPanic: "longpoll: nil handler",
		},
		{
			name: "ContextCanceled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			channel: func() <-chan int {
				ch := make(chan int)
				close(ch)
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: context.Canceled,
		},
		{
			name: "ChannelClosed",
			ctx:  context.Background(),
			channel: func() <-chan int {
				ch := make(chan int)
				close(ch)
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: io.EOF,
		},
		{
			name: "HandlerError",
			ctx:  context.Background(),
			channel: func() <-chan int {
				ch := make(chan int, 1)
				ch <- 1
				close(ch)
				return ch
			},
			handler:        func(value int) error { return errors.New("handler error") },
			expectedResult: errors.New("handler error"),
		},
		{
			name: "MaxSizeExceededClosed",
			ctx:  context.Background(),
			cfg: &ChannelConfig{
				MaxSize: 3,
			},
			channel: func() <-chan int {
				ch := make(chan int, 4)
				ch <- 1
				ch <- 2
				ch <- 3
				ch <- 4
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: nil,
		},
		{
			name: "MaxSizeExceededClosed",
			ctx:  context.Background(),
			cfg: &ChannelConfig{
				MaxSize: 3,
			},
			channel: func() <-chan int {
				ch := make(chan int, 3)
				ch <- 1
				ch <- 2
				ch <- 3
				close(ch)
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: nil,
		},
		{
			name: "EOF",
			ctx:  context.Background(),
			cfg: &ChannelConfig{
				MaxSize: 3,
			},
			channel: func() <-chan int {
				ch := make(chan int, 3)
				ch <- 1
				ch <- 2
				close(ch)
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: io.EOF,
		},
		{
			name: "MinSizeNotReached",
			ctx:  context.Background(),
			cfg: &ChannelConfig{
				MinSize: 2,
			},
			channel: func() <-chan int {
				ch := make(chan int, 1)
				ch <- 1
				close(ch)
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: io.EOF,
		},
		{
			name: "PartialTimeoutReached",
			ctx:  context.Background(),
			cfg: &ChannelConfig{
				MinSize:        -1,
				PartialTimeout: 50 * time.Millisecond,
			},
			channel: func() <-chan int {
				ch := make(chan int)
				go func() {
					time.Sleep(100 * time.Millisecond)
					ch <- 1
					close(ch)
				}()
				return ch
			},
			handler:        func(value int) error { return nil },
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := tt.channel()

			var success bool
			defer func() {
				if tt.expectedPanic != `` {
					if success {
						t.Errorf("Expected panic %q, got none", tt.expectedPanic)
					} else if v := fmt.Sprint(recover()); v != tt.expectedPanic {
						t.Errorf("Expected panic %q, got %q", tt.expectedPanic, v)
					}
				} else if !success {
					t.Errorf("Expected success, got panic %q", tt.expectedPanic)
				}
			}()

			result := Channel(tt.ctx, tt.cfg, ch, tt.handler)

			success = true

			if (result == nil && tt.expectedResult != nil) || (result != nil && tt.expectedResult == nil) {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			} else if result != nil && tt.expectedResult != nil && result.Error() != tt.expectedResult.Error() {
				t.Errorf("Expected result %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

// test that it drains as much as possible prior to exit
func TestChannel_maxSizeLoopNoMoreAvailable(t *testing.T) {
	ch := make(chan int, 4)
	ch <- 1
	ch <- 2
	ch <- 3

	var values []int
	err := Channel(context.Background(), &ChannelConfig{
		MaxSize:        10,
		MinSize:        -1, // disable, trigger second loop immediately
		PartialTimeout: -1, // disable, trigger second loop immediately
	}, ch, func(value int) error {
		values = append(values, value)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	} else if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	} else if values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Errorf("Expected values [1, 2, 3], got %v", values)
	}
}

// similar to the above, but the terminal case is the channel is closed
func TestChannel_maxSizeLoopClosed(t *testing.T) {
	ch := make(chan int, 4)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)

	var values []int
	err := Channel(context.Background(), &ChannelConfig{
		MaxSize:        10,
		MinSize:        -1, // disable, trigger second loop immediately
		PartialTimeout: -1, // disable, trigger second loop immediately
	}, ch, func(value int) error {
		values = append(values, value)
		return nil
	})

	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	} else if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	} else if values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Errorf("Expected values [1, 2, 3], got %v", values)
	}
}

// similar to the above again, but the terminal case is we hit the max size
func TestChannel_maxSizeLoopHitMax(t *testing.T) {
	ch := make(chan int, 4)
	ch <- 1
	ch <- 2
	ch <- 3
	ch <- 4

	var values []int
	err := Channel(context.Background(), &ChannelConfig{
		MaxSize:        3,
		MinSize:        -1, // disable, trigger second loop immediately
		PartialTimeout: -1, // disable, trigger second loop immediately
	}, ch, func(value int) error {
		values = append(values, value)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	} else if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	} else if values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Errorf("Expected values [1, 2, 3], got %v", values)
	}

	if v := <-ch; v != 4 {
		t.Errorf("Expected value 4, got %d", v)
	}
}

// testing that disabling min size when using partial timeout causes it to wait
// for the first value, at least (or timeout/cancel)
func TestChannel_waitForSingleValuePartialTimeoutNoMinSize(t *testing.T) {
	ch := make(chan int, 1)
	in := make(chan int)
	out := make(chan error)

	startTime := time.Now()

	go func() {
		out <- Channel(context.Background(), &ChannelConfig{
			MaxSize:        10,
			MinSize:        -1,
			PartialTimeout: time.Second * 3,
		}, ch, func(value int) error {
			in <- value
			return nil
		})
	}()

	time.Sleep(time.Millisecond * 30)
	select {
	case <-in:
		t.Fatal()
	case <-out:
		t.Fatal()
	default:
	}

	ch <- 1

	if v := <-in; v != 1 {
		t.Fatal(v)
	}

	if err := <-out; err != nil {
		t.Fatal(err)
	}

	if d := time.Since(startTime); d > time.Second*2 {
		t.Fatal(d)
	}
}

// tests an immediate exit case where the configuration specifies no waiting,
// and there are no values available
func TestChannel_noMinSizeOrPartialTimeoutNoValues(t *testing.T) {
	ch := make(chan bool)
	if err := Channel(context.Background(), &ChannelConfig{
		MaxSize:        -1,
		MinSize:        -1,
		PartialTimeout: -1,
	}, ch, func(value bool) error {
		t.Fatal(value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChannel_minSizeNoPartialTimeout(t *testing.T) {
	ch := make(chan float64)

	out := make(chan error)
	var values []float64
	go func() {
		out <- Channel(context.Background(), &ChannelConfig{
			MaxSize:        -1,
			MinSize:        5,
			PartialTimeout: -1,
		}, ch, func(value float64) error {
			values = append(values, value)
			return nil
		})
	}()

	time.Sleep(time.Millisecond * 30)

	ch <- 1
	ch <- 2
	ch <- 3

	time.Sleep(time.Millisecond * 30)

	ch <- 4

	time.Sleep(time.Millisecond * 30)

	select {
	case <-out:
		t.Fatal()
	default:
	}

	ch <- 5

	if err := <-out; err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(values, []float64{1, 2, 3, 4, 5}) {
		t.Fatal(values)
	}
}
