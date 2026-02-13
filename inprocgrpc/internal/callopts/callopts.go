// Package callopts provides gRPC call option extraction and processing.
package callopts

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// CallOptions holds the processed gRPC call options.
type CallOptions struct {
	Creds    credentials.PerRPCCredentials
	Headers  []*metadata.MD
	Trailers []*metadata.MD
	Peer     []*peer.Peer
	MaxRecv  int
	MaxSend  int
}

// GetCallOptions extracts structured call options from the given gRPC options.
func GetCallOptions(opts []grpc.CallOption) *CallOptions {
	var co CallOptions
	for _, o := range opts {
		switch v := o.(type) {
		case grpc.HeaderCallOption:
			co.Headers = append(co.Headers, v.HeaderAddr)
		case grpc.TrailerCallOption:
			co.Trailers = append(co.Trailers, v.TrailerAddr)
		case grpc.PeerCallOption:
			co.Peer = append(co.Peer, v.PeerAddr)
		case grpc.PerRPCCredsCallOption:
			co.Creds = v.Creds
		case grpc.MaxRecvMsgSizeCallOption:
			co.MaxRecv = v.MaxRecvMsgSize
		case grpc.MaxSendMsgSizeCallOption:
			co.MaxSend = v.MaxSendMsgSize
		}
	}
	return &co
}

// SetHeaders sets the response headers on all registered header addresses.
func (co *CallOptions) SetHeaders(md metadata.MD) {
	for _, h := range co.Headers {
		*h = md
	}
}

// SetTrailers sets the response trailers on all registered trailer addresses.
func (co *CallOptions) SetTrailers(md metadata.MD) {
	for _, t := range co.Trailers {
		*t = md
	}
}

// SetPeer sets the peer info on all registered peer addresses.
func (co *CallOptions) SetPeer(p *peer.Peer) {
	for _, pp := range co.Peer {
		*pp = *p
	}
}

// ApplyPerRPCCreds applies per-RPC credentials to the context, merging any
// credential-provided metadata into the outgoing context metadata.
func ApplyPerRPCCreds(ctx context.Context, copts *CallOptions, uri string, isChannelSecure bool) (context.Context, error) {
	if copts.Creds == nil {
		return ctx, nil
	}
	if copts.Creds.RequireTransportSecurity() && !isChannelSecure {
		return ctx, status.Errorf(codes.Unauthenticated, "transport security is required")
	}
	md, err := copts.Creds.GetRequestMetadata(ctx, uri)
	if err != nil {
		return ctx, status.Errorf(codes.Unauthenticated, "getting request metadata: %v", err)
	}
	if len(md) > 0 {
		pairs := make([]string, 0, len(md)*2)
		for k, v := range md {
			pairs = append(pairs, k, v)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}
	return ctx, nil
}
