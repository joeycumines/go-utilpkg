package gojagrpc

import (
	"errors"
	"fmt"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

type moduleConfig struct {
	channel  *inprocgrpc.Channel
	protobuf *gojaprotobuf.Module
	adapter  *gojaeventloop.Adapter
}

// ModuleOption configures a [Module].
type ModuleOption interface {
	applyModuleOption(*moduleConfig) error
}

// ChannelOption configures the in-process transport used by a [Module].
type ChannelOption struct {
	channel *inprocgrpc.Channel
}

// WithChannel configures the [inprocgrpc.Channel] used for RPC
// communication. This option is required.
func WithChannel(channel *inprocgrpc.Channel) *ChannelOption {
	return &ChannelOption{channel: channel}
}

func (o *ChannelOption) applyModuleOption(cfg *moduleConfig) error {
	if o == nil {
		return errors.New("channel option is nil")
	}
	if o.channel == nil {
		return errors.New("channel must not be nil")
	}
	cfg.channel = o.channel
	return nil
}

var _ ModuleOption = (*ChannelOption)(nil)

// ProtobufOption configures the shared protobuf identity used by a [Module].
type ProtobufOption struct {
	protobuf *gojaprotobuf.Module
}

// WithProtobuf configures the [gojaprotobuf.Module] used for protobuf
// message encoding and decoding. This option is required.
func WithProtobuf(protobuf *gojaprotobuf.Module) *ProtobufOption {
	return &ProtobufOption{protobuf: protobuf}
}

func (o *ProtobufOption) applyModuleOption(cfg *moduleConfig) error {
	if o == nil {
		return errors.New("protobuf option is nil")
	}
	if o.protobuf == nil {
		return errors.New("protobuf module must not be nil")
	}
	cfg.protobuf = o.protobuf
	return nil
}

var _ ModuleOption = (*ProtobufOption)(nil)

// AdapterOption configures the Goja event-loop owner used by a [Module].
type AdapterOption struct {
	adapter *gojaeventloop.Adapter
}

// WithAdapter configures the [gojaeventloop.Adapter] used for promise
// creation and event-loop integration. This option is required.
func WithAdapter(adapter *gojaeventloop.Adapter) *AdapterOption {
	return &AdapterOption{adapter: adapter}
}

func (o *AdapterOption) applyModuleOption(cfg *moduleConfig) error {
	if o == nil {
		return errors.New("adapter option is nil")
	}
	if o.adapter == nil {
		return errors.New("adapter must not be nil")
	}
	cfg.adapter = o.adapter
	return nil
}

var _ ModuleOption = (*AdapterOption)(nil)

func resolveOptions(opts []ModuleOption) (*moduleConfig, error) {
	cfg := &moduleConfig{}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("module option %d is nil", index)
		}
		if err := opt.applyModuleOption(cfg); err != nil {
			return nil, fmt.Errorf(
				"module option %d: %w",
				index,
				err,
			)
		}
	}
	if cfg.channel == nil {
		return nil, errors.New("channel is required (use WithChannel)")
	}
	if cfg.protobuf == nil {
		return nil, errors.New("protobuf module is required (use WithProtobuf)")
	}
	if cfg.adapter == nil {
		return nil, errors.New("adapter is required (use WithAdapter)")
	}
	return cfg, nil
}
