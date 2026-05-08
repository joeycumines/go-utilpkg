package gojagrpc

import (
	"errors"
	"io"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

const workerErrorFallbackMessage = "transport error normalization exited"

var workerErrorFallbackStatus = &statuspb.Status{
	Code:    int32(codes.Internal),
	Message: workerErrorFallbackMessage,
}

var workerEOFFailureStatus = &statuspb.Status{
	Code:    int32(codes.Internal),
	Message: io.EOF.Error(),
}

// workerErrorSnapshot is the only error representation allowed to cross from
// an arbitrary transport implementation into control or owner state. status
// is immutable copied protobuf data; eof is kept separate because io.EOF is a
// successful stream terminal rather than a gRPC failure.
type workerErrorSnapshot struct {
	status *statuspb.Status
	eof    bool
}

func (s workerErrorSnapshot) err() error {
	if s.eof {
		return io.EOF
	}
	return ownerStatusError(s.status)
}

func (s workerErrorSnapshot) result() ownerStatusResult {
	if s.eof {
		return ownerStatusResult{status: workerEOFFailureStatus}
	}
	return ownerStatusResult{status: s.status}
}

func workerErrorFallback() workerErrorSnapshot {
	return workerErrorSnapshot{status: workerErrorFallbackStatus}
}

// snapshotWorkerError contains every user-defined error method behind a child
// return boundary. errors.Is may invoke Unwrap/Is, status conversion may invoke
// GRPCStatus, and canonicalOwnerStatus may invoke Error. A panic or
// runtime.Goexit therefore retires only this child and deterministically
// publishes the precomputed Internal fallback.
func snapshotWorkerError(err error) workerErrorSnapshot {
	if err == nil {
		return workerErrorSnapshot{}
	}
	result := make(chan workerErrorSnapshot, 1)
	go func() {
		snapshot := workerErrorSnapshot{status: workerErrorFallbackStatus}
		defer func() {
			_ = recover()
			result <- snapshot
		}()
		if errors.Is(err, io.EOF) {
			snapshot = workerErrorSnapshot{eof: true}
			return
		}
		snapshot = workerErrorSnapshot{status: canonicalOwnerStatus(err)}
	}()
	return <-result
}

func canonicalWorkerError(err error) error {
	return snapshotWorkerError(err).err()
}

func (d *ownerDispatcher) rejectOwnerPromiseSnapshot(
	id ownerOperationID,
	snapshot workerErrorSnapshot,
) error {
	return d.settleOwnerPromise(id, snapshot.result(), true)
}

// rootWorkerBoundary converts panic and runtime.Goexit in transport-facing
// constructor workers into one static rejection plus complete root retirement.
// transferred is set only after a stream worker owns those obligations.
type rootWorkerBoundary struct {
	root           workerRoot
	promise        ownerOperationID
	transportBound bool
	transferred    bool
}

func (b *rootWorkerBoundary) run(fn func()) {
	returned := false
	defer func() {
		_ = recover()
		if returned || b.transferred {
			return
		}
		fallback := workerErrorFallback()
		_ = b.root.owner.rejectOwnerPromiseSnapshot(b.promise, fallback)
		b.root.control.stop(fallback.err())
		if b.transportBound {
			b.root.finish(fallback.err())
			return
		}
		b.root.failConstruction(fallback.err())
	}()
	fn()
	returned = true
}
