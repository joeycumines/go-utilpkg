package gojaprotobuf

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/reflect/protoregistry"
)

type moduleConfig struct {
	resolver *protoregistry.Types
	files    *protoregistry.Files
}

// ModuleOption configures a [Module]. [New] panics when an option is nil or
// invalid. Implementations are returned by the With* constructors.
type ModuleOption interface {
	applyModuleOption(*moduleConfig) error
}

// ResolverOption configures the immutable base type-registry snapshot.
type ResolverOption struct {
	resolver *protoregistry.Types
}

// WithResolver configures the [protoregistry.Types] whose current membership
// is snapshotted by the first [New] call for a runtime. Later caller mutations
// are not observed. The caller must not mutate the registry concurrently with
// construction. If omitted, [protoregistry.GlobalTypes] is snapshotted.
func WithResolver(resolver *protoregistry.Types) *ResolverOption {
	return &ResolverOption{resolver: resolver}
}

func (o *ResolverOption) applyModuleOption(cfg *moduleConfig) error {
	if o == nil {
		return errors.New("resolver option is nil")
	}
	if o.resolver == nil {
		return errors.New("resolver must not be nil")
	}
	cfg.resolver = o.resolver
	return nil
}

var _ ModuleOption = (*ResolverOption)(nil)

// FilesOption configures the immutable base file-registry snapshot.
type FilesOption struct {
	files *protoregistry.Files
}

// WithFiles configures the [protoregistry.Files] whose current membership is
// snapshotted by the first [New] call for a runtime. Later caller mutations
// are not observed. The caller must not mutate the registry concurrently with
// construction. If omitted, [protoregistry.GlobalFiles] is snapshotted.
func WithFiles(files *protoregistry.Files) *FilesOption {
	return &FilesOption{files: files}
}

func (o *FilesOption) applyModuleOption(cfg *moduleConfig) error {
	if o == nil {
		return errors.New("files option is nil")
	}
	if o.files == nil {
		return errors.New("files registry must not be nil")
	}
	cfg.files = o.files
	return nil
}

var _ ModuleOption = (*FilesOption)(nil)

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
	if cfg.resolver == nil {
		cfg.resolver = protoregistry.GlobalTypes
	}
	if cfg.files == nil {
		cfg.files = protoregistry.GlobalFiles
	}
	return cfg, nil
}
