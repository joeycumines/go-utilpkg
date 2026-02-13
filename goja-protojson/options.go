package gojaprotojson

import (
	"errors"

	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// Option configures module behavior. Options are immutable value
// types that validate on construction.
type Option interface {
	apply(*config) error
}

type config struct {
	protobuf *gojaprotobuf.Module
}

func resolveOptions(opts []Option) (*config, error) {
	cfg := &config{}
	for _, opt := range opts {
		if err := opt.apply(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// WithProtobuf provides the [gojaprotobuf.Module] used for message
// wrapping, unwrapping, and type resolution. This is required.
func WithProtobuf(pb *gojaprotobuf.Module) Option {
	return withProtobuf{pb: pb}
}

type withProtobuf struct {
	pb *gojaprotobuf.Module
}

func (o withProtobuf) apply(cfg *config) error {
	if o.pb == nil {
		return errors.New("protobuf module must not be nil")
	}
	cfg.protobuf = o.pb
	return nil
}
