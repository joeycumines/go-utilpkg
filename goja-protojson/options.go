package gojaprotojson

import (
	"errors"
	"fmt"

	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// ModuleOption configures a [Module]. [New] panics when an option is nil or
// invalid. Implementations are returned by the With* constructors.
type ModuleOption interface {
	applyModuleOption(*moduleConfig) error
}

type moduleConfig struct {
	protobuf *gojaprotobuf.Module
}

func resolveOptions(opts []ModuleOption) (*moduleConfig, error) {
	cfg := &moduleConfig{}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("module option %d is nil", index)
		}
		if err := opt.applyModuleOption(cfg); err != nil {
			return nil, fmt.Errorf("module option %d: %w", index, err)
		}
	}
	if cfg.protobuf == nil {
		return nil, errors.New(
			"protobuf module is required (use WithProtobuf)",
		)
	}
	return cfg, nil
}

// ProtobufOption configures the shared protobuf identity used by a [Module].
type ProtobufOption struct {
	protobuf *gojaprotobuf.Module
}

// WithProtobuf provides the [gojaprotobuf.Module] used for message
// wrapping, unwrapping, and type resolution. This is required.
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
