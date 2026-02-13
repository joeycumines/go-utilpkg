package gojaprotobuf

import (
	"google.golang.org/protobuf/reflect/protoregistry"
)

// moduleOptions holds configuration for a [Module] instance.
type moduleOptions struct {
	resolver *protoregistry.Types
	files    *protoregistry.Files
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

// WithResolver configures the [protoregistry.Types] used to resolve
// message and enum types by fully-qualified name. If not set,
// [protoregistry.GlobalTypes] is used by default.
func WithResolver(resolver *protoregistry.Types) Option {
	return &optionFunc{fn: func(opts *moduleOptions) error {
		opts.resolver = resolver
		return nil
	}}
}

// WithFiles configures the [protoregistry.Files] used to resolve
// file descriptors. If not set, [protoregistry.GlobalFiles] is used
// by default.
func WithFiles(files *protoregistry.Files) Option {
	return &optionFunc{fn: func(opts *moduleOptions) error {
		opts.files = files
		return nil
	}}
}

// resolveOptions applies the given options to a default [moduleOptions].
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
	return cfg, nil
}
