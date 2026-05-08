package timerrefclosurecc

import "testing"

func TestOwnerReferenceModel(t *testing.T) {
	runOwnerReferenceModel(t, []byte{
		0, 0, 1, 1,
		2, 2, 3, 3,
		4, 5, 6,
		0, 2, 7,
		1, 3,
	})
}

func TestOwnerReferenceModelExhaustive(t *testing.T) {
	const traceLength = 5
	const traceCount = 1 << (3 * traceLength)
	operations := make([]byte, traceLength)
	for ordinal := range traceCount {
		encoded := ordinal
		for index := range operations {
			operations[index] = byte(encoded & 7)
			encoded >>= 3
		}
		runOwnerReferenceModel(t, operations)
	}
}

func FuzzOwnerReferenceModel(f *testing.F) {
	f.Add([]byte{0, 0, 1, 1, 2, 2, 3, 3, 4, 5, 6, 7})
	f.Add([]byte{0, 2, 0, 2, 6, 1, 3, 7, 4, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 64 {
			operations = operations[:64]
		}
		runOwnerReferenceModel(t, operations)
	})
}

func runOwnerReferenceModel(t *testing.T, operations []byte) {
	t.Helper()
	value := newLoop(true)
	if !value.bindOwner() {
		t.Fatal("owner setup failed")
	}
	first, err := value.scheduleTimer(0, func() {})
	if err != nil || first != 1 {
		t.Fatalf("first ScheduleTimer = (%d, %v)", first, err)
	}
	if err := value.unrefTimer(first); err != nil {
		t.Fatal(err)
	}
	second, err := value.scheduleTimer(0, func() {})
	if err != nil || second != 2 {
		t.Fatalf("second ScheduleTimer = (%d, %v)", second, err)
	}

	refed := [2]bool{false, true}
	refedCount := int64(1)
	epoch := uint64(3)
	wakeAttempts := uint64(1)
	fastToken := true

	assertState := func(step int) {
		t.Helper()
		fastWakePending := 0
		if fastToken {
			fastWakePending = 1
		}
		for index, wantRefed := range refed {
			id := timerID(index + 1)
			want := qualificationSnapshot{
				present: true, refed: wantRefed, refedCount: refedCount,
				submissionEpoch: epoch, fastWakePending: fastWakePending,
				wakeAttempts: wakeAttempts, wakeSuccesses: wakeAttempts, state: stateRunning,
			}
			if got := value.snapshot(id); got != want {
				t.Fatalf("trace %v step %d timer %d snapshot = %+v, want %+v", operations, step, id, got, want)
			}
			timerValue := value.timerMap[id]
			if timerValue == nil || timerValue.id != id || timerValue.heapIndex != index || timerValue.canceled.Load() {
				t.Fatalf("trace %v step %d timer %d node = %+v", operations, step, id, timerValue)
			}
		}
		wantMissing := qualificationSnapshot{
			refedCount: refedCount, submissionEpoch: epoch, fastWakePending: fastWakePending,
			wakeAttempts: wakeAttempts, wakeSuccesses: wakeAttempts, state: stateRunning,
		}
		if got := value.snapshot(3); got != wantMissing {
			t.Fatalf("trace %v step %d missing timer snapshot = %+v, want %+v", operations, step, got, wantMissing)
		}
		if len(value.timerMap) != 2 || len(value.timers) != 2 || value.nextTimerID.Load() != 2 ||
			value.promisifyCount.Load() != 0 || value.userIOFDCount.Load() != 0 || value.quiescing.Load() {
			t.Fatalf("trace %v step %d identity/liveness changed", operations, step)
		}
	}

	assertState(-1)
	for step, operation := range operations {
		operation %= 8
		switch operation {
		case 0, 1, 2, 3:
			index := int(operation / 2)
			id := timerID(index + 1)
			wantRefed := operation%2 == 0
			var err error
			if wantRefed {
				err = value.refTimer(id)
			} else {
				err = value.unrefTimer(id)
			}
			if err != nil {
				t.Fatalf("trace %v step %d timer %d reference change: %v", operations, step, id, err)
			}
			if refed[index] != wantRefed {
				if wantRefed {
					refedCount++
				} else {
					refedCount--
				}
				refed[index] = wantRefed
				epoch++
				wakeAttempts++
				fastToken = true
			}
		case 4:
			if err := value.refTimer(3); err != nil {
				t.Fatalf("trace %v step %d missing Ref: %v", operations, step, err)
			}
		case 5:
			if err := value.unrefTimer(3); err != nil {
				t.Fatalf("trace %v step %d missing Unref: %v", operations, step, err)
			}
		case 6:
			value.consumeFastWake()
			fastToken = false
		case 7:
			if drained := value.drain(); drained != 0 {
				t.Fatalf("trace %v step %d empty drain = %d, want 0", operations, step, drained)
			}
		}
		assertState(step)
	}
}
