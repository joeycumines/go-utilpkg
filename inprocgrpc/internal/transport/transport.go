// Package transport provides gRPC server transport stream implementations.
package transport

import (
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ErrHeadersAlreadySent is returned when trying to set headers after they've been sent.
var ErrHeadersAlreadySent = errHeadersAlreadySent("headers already sent")

// ErrTrailersAlreadySent is returned when trying to set trailers after they've been sent.
var ErrTrailersAlreadySent = errTrailersAlreadySent("trailers already sent")

type errHeadersAlreadySent string

func (e errHeadersAlreadySent) Error() string { return string(e) }

type errTrailersAlreadySent string

func (e errTrailersAlreadySent) Error() string { return string(e) }

// UnaryServerTransportStream is a grpc.ServerTransportStream for unary RPCs.
// It accumulates headers and trailers until they are consumed.
type UnaryServerTransportStream struct {
	hdrs metadata.MD
	tlrs metadata.MD
	Name string // "/service/method"

	mu       sync.Mutex
	hdrsSent bool
	tlrsSent bool
}

// Method returns the full method name.
func (sts *UnaryServerTransportStream) Method() string { return sts.Name }

// SetHeader accumulates headers. Returns an error if headers were already sent.
func (sts *UnaryServerTransportStream) SetHeader(md metadata.MD) error {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	if sts.hdrsSent {
		return ErrHeadersAlreadySent
	}
	sts.hdrs = metadata.Join(sts.hdrs, md)
	return nil
}

// SendHeader accumulates headers and marks them as sent.
func (sts *UnaryServerTransportStream) SendHeader(md metadata.MD) error {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	if sts.hdrsSent {
		return ErrHeadersAlreadySent
	}
	sts.hdrs = metadata.Join(sts.hdrs, md)
	sts.hdrsSent = true
	return nil
}

// SetTrailer accumulates trailers.
func (sts *UnaryServerTransportStream) SetTrailer(md metadata.MD) error {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	if sts.tlrsSent {
		return ErrTrailersAlreadySent
	}
	sts.tlrs = metadata.Join(sts.tlrs, md)
	return nil
}

// Finish marks both headers and trailers as sent.
func (sts *UnaryServerTransportStream) Finish() {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	sts.hdrsSent = true
	sts.tlrsSent = true
}

// GetHeaders returns accumulated headers.
func (sts *UnaryServerTransportStream) GetHeaders() metadata.MD {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	return sts.hdrs
}

// GetTrailers returns accumulated trailers.
func (sts *UnaryServerTransportStream) GetTrailers() metadata.MD {
	sts.mu.Lock()
	defer sts.mu.Unlock()
	return sts.tlrs
}

// ServerTransportStream is a grpc.ServerTransportStream for streaming RPCs.
// It delegates to the underlying grpc.ServerStream.
type ServerTransportStream struct {
	Stream grpc.ServerStream
	Name   string
}

// Method returns the full method name.
func (sts *ServerTransportStream) Method() string { return sts.Name }

// SetHeader delegates to the underlying stream.
func (sts *ServerTransportStream) SetHeader(md metadata.MD) error {
	return sts.Stream.SetHeader(md)
}

// SendHeader delegates to the underlying stream.
func (sts *ServerTransportStream) SendHeader(md metadata.MD) error {
	return sts.Stream.SendHeader(md)
}

// SetTrailer delegates to the underlying stream. If the stream implements
// TrySetTrailer, it uses that; otherwise it calls SetTrailer on the stream
// which has no return value.
func (sts *ServerTransportStream) SetTrailer(md metadata.MD) error {
	type tryTrailer interface {
		TrySetTrailer(metadata.MD) error
	}
	if tt, ok := sts.Stream.(tryTrailer); ok {
		return tt.TrySetTrailer(md)
	}
	sts.Stream.SetTrailer(md)
	return nil
}
