package gojagrpc

import (
	"errors"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// moduleOptions holds configuration for a [Module] instance.
type moduleOptions struct {
	channel  *inprocgrpc.Channel
	protobuf *gojaprotobuf.Module
	adapter  *gojaeventloop.Adapter
}

// Option configures a [Module] instance. Options are applied during
// module construction.
type Option interface {
	applyOption(*moduleOptions) error
}

// optionFunc implements [Option] via a closure.
type optionFunc struct {
	fn func(*moduleOptions) error
}

func (o *optionFunc) applyOption(opts *moduleOptions) error {
	return o.fn(opts)
}

// WithChannel configures the [inprocgrpc.Channel] used for RPC
// communication. This option is required; passing nil returns an
// error during module construction.
func WithChannel(ch *inprocgrpc.Channel) Option {
	return &optionFunc{fn: func(opts *moduleOptions) error {
		if ch == nil {
			return errors.New("gojagrpc: channel must not be nil")
		}
		opts.channel = ch
		return nil
	}}
}

// WithProtobuf configures the [gojaprotobuf.Module] used for protobuf
// message encoding and decoding. This option is required; passing nil
// returns an error during module construction.
func WithProtobuf(pb *gojaprotobuf.Module) Option {
	return &optionFunc{fn: func(opts *moduleOptions) error {
		if pb == nil {
			return errors.New("gojagrpc: protobuf module must not be nil")
		}
		opts.protobuf = pb
		return nil
	}}
}

// WithAdapter configures the [gojaeventloop.Adapter] used for
// promise creation and event loop integration. This option is
// required; passing nil returns an error during module construction.
func WithAdapter(a *gojaeventloop.Adapter) Option {
	return &optionFunc{fn: func(opts *moduleOptions) error {
		if a == nil {
			return errors.New("gojagrpc: adapter must not be nil")
		}
		opts.adapter = a
		return nil
	}}
}

// resolveOptions applies the given options to a default [moduleOptions]
// and validates that all required fields are set.
func resolveOptions(opts []Option) (*moduleOptions, error) {
	cfg := &moduleOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.applyOption(cfg); err != nil {
			return nil, err
		}
	}
	if cfg.channel == nil {
		return nil, errors.New("gojagrpc: channel is required (use WithChannel)")
	}
	if cfg.protobuf == nil {
		return nil, errors.New("gojagrpc: protobuf module is required (use WithProtobuf)")
	}
	if cfg.adapter == nil {
		return nil, errors.New("gojagrpc: adapter is required (use WithAdapter)")
	}
	return cfg, nil
}
