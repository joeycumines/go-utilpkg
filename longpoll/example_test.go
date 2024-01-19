package longpoll_test

import (
	"context"
	"fmt"
	"github.com/joeycumines/go-utilpkg/longpoll"
	"strings"
	"time"
)

func ExampleChannel() {
	// we will receive incoming requests on this channel
	ch := make(chan string, 32)

	// in this scenario, we're performing batching of requests, by long-polling from ch
	batch := func() {
		// no mutex is required to write to this - handler blocks longpoll.Channel
		var buffer []string

		// the behavior is configurable, but we'll use the defaults here (see the docs for longpoll.ChannelConfig)
		if err := longpoll.Channel(context.Background(), nil, ch, func(value string) error {
			buffer = append(buffer, value)
			return nil
		}); err != nil {
			panic(err)
		}

		// similarly, we can just read the result of the batch from buffer
		fmt.Printf("Hello to our new friends:\n%s\n", strings.Join(buffer, "\n"))
	}

	// test data
	names := []string{
		"Olivia",
		"Liam",
		"Emma",
		"Noah",
		"Ava",
		"Oliver",
		"Sophia",
		"Elijah",
		"Isabella",
		"William",
		"Mia",
		"James",
		"Charlotte",
		"Benjamin",
		"Amelia",
		"Lucas",
		"Harper",
		"Henry",
		"Evelyn",
		"Alexander",
		"Grace",
		"Jack",
	}

	for i := 0; i < 18; i++ {
		ch <- names[0]
		names = names[1:]
	}

	fmt.Printf("Buffered %d names prior to batch (%d more than the default max size)\n", len(ch), len(ch)-16)

	// the first batch will be immediately filled, up to the max size, which is less than the number of buffered names
	batch()

	// our second batch will immediately fulfil after it reaches the minimum size
	// (waiting until it does, or PartialInterval is reached, which defaults to 50ms, and starts from the first receive)
	const defaultMinSize = 4
	numUntilMin := defaultMinSize - len(ch)
	fmt.Printf("Buffering %d more names to reach the minimum size, while running the batch...\n", numUntilMin)
	done := make(chan struct{})
	go func() {
		defer close(done)
		batch()
	}()
	time.Sleep(time.Millisecond * 5)
	for i := 0; i < numUntilMin; i++ {
		ch <- names[0]
		names = names[1:]
	}
	<-done

	fmt.Printf("We have %d names buffered, and have a min size (%d, the default) - Channel won't start PartialInterval until the first receive\n", len(ch), defaultMinSize)

	done = make(chan struct{})
	go func() {
		defer close(done)
		batch()
	}()

	const defaultPartialInterval = 50 * time.Millisecond
	time.Sleep(defaultPartialInterval * 2)
	select {
	case <-done:
		panic(`expected not done`)
	default:
	}
	fmt.Printf("Slept for double the PartialInterval (%s is the default), and the batch is still blocking, as expected\n", defaultPartialInterval)

	fmt.Println(`Sending two names, 10ms apart, which will cause the batch to complete, after PartialInterval, starting from the first send...`)
	ch <- names[0]
	names = names[1:]
	time.Sleep(time.Millisecond * 10)
	ch <- names[0]
	names = names[1:]

	<-done

	//output:
	//Buffered 18 names prior to batch (2 more than the default max size)
	//Hello to our new friends:
	//Olivia
	//Liam
	//Emma
	//Noah
	//Ava
	//Oliver
	//Sophia
	//Elijah
	//Isabella
	//William
	//Mia
	//James
	//Charlotte
	//Benjamin
	//Amelia
	//Lucas
	//Buffering 2 more names to reach the minimum size, while running the batch...
	//Hello to our new friends:
	//Harper
	//Henry
	//Evelyn
	//Alexander
	//We have 0 names buffered, and have a min size (4, the default) - Channel won't start PartialInterval until the first receive
	//Slept for double the PartialInterval (50ms is the default), and the batch is still blocking, as expected
	//Sending two names, 10ms apart, which will cause the batch to complete, after PartialInterval, starting from the first send...
	//Hello to our new friends:
	//Grace
	//Jack
}
