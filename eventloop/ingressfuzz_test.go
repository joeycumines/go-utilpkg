package eventloop

import (
	"testing"
)

func FuzzLoopCommandIngressFIFOAndReset(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{255, 254, 253, 252, 251, 250})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		var q loopCommandIngress
		var model []loopCommandKind
		ops := 1 + min(len(data)*4, 4096)

		for range ops {
			switch r.byte() % 6 {
			case 0:
				q.Push(loopCommand{})
			case 1, 2, 3:
				kind := loopCommandKind(1 + r.intn(int(loopCommandShutdown)))
				q.Push(loopCommand{kind: kind, token: r.uint64()})
				model = append(model, kind)
			case 4:
				cmd, ok := q.Pop()
				if len(model) == 0 {
					if ok || cmd.kind != loopCommandNone {
						t.Fatalf("Pop empty = (%+v, %v), want none/false", cmd, ok)
					}
					break
				}
				want := model[0]
				model = model[1:]
				if !ok || cmd.kind != want {
					t.Fatalf("Pop = (%v, %v), want (%v, true)", cmd.kind, ok, want)
				}
			case 5:
				q.Reset()
				model = nil
			}
			if got, want := q.Len(), len(model); got != want {
				t.Fatalf("Len = %d, want %d", got, want)
			}
		}

		for _, want := range model {
			cmd, ok := q.Pop()
			if !ok || cmd.kind != want {
				t.Fatalf("drain Pop = (%v, %v), want (%v, true)", cmd.kind, ok, want)
			}
		}
		cmd, ok := q.Pop()
		if ok || cmd.kind != loopCommandNone || q.Len() != 0 {
			t.Fatalf("queue not empty after drain: cmd=%+v ok=%v len=%d", cmd, ok, q.Len())
		}
	})
}
