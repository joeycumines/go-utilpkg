// Package component defines semantic contracts shared by isolated tournament
// component implementations. Variant-owned entries preserve their native
// layout; these values exist only at adapter boundaries.
package component

import "errors"

var (
	ErrFDRange              = errors.New("tournament component: file descriptor outside implementation range")
	ErrFDDuplicate          = errors.New("tournament component: file descriptor already registered")
	ErrFDMissing            = errors.New("tournament component: file descriptor not registered")
	ErrFDIdentityExhausted  = errors.New("tournament component: file descriptor identity exhausted")
	ErrFDProjectionRequired = errors.New("tournament component: descriptor requires projection-only evaluation")
)

type EventMask uint32

type Callback func(EventMask)

type FDRegistration struct {
	Callback Callback
	Events   EventMask
}

type FDTable interface {
	Register(int, FDRegistration) error
	Lookup(int) (FDRegistration, bool)
	Unregister(int) error
}

type FDTableImplementation interface {
	FDTable
	FDTableDiagnostics
}

// FDTableDiagnostics is deliberately excluded from measured operation
// contracts. Historical implementations did not maintain a common count or
// reset path, so adapters derive these values only during qualification.
type FDTableDiagnostics interface {
	Len() int
	Reset()
	Stats() FDTableStats
}

// FDTableVersion exposes the mutation version carried by implementations that
// invalidate in-flight native poll results after registration changes.
type FDTableVersion interface {
	Version() uint64
}

// FDTableGeneration is an optional capability of implementations that attach
// a generation to descriptor registrations. A plain descriptor table must not
// fabricate this protection.
type FDTableGeneration interface {
	Generation(int) (uint64, bool)
	LookupGeneration(int, uint64) (FDRegistration, bool)
}

// FDTableToken is an optional capability of implementations that publish a
// poll token distinct from the descriptor. Generation-only implementations
// must not be adapted to this interface.
type FDTableToken interface {
	Token(int) (uint64, bool)
	LookupToken(uint64) (int, FDRegistration, bool)
}

type FDProjection struct {
	DenseSlots      int
	AddedDenseSlots int
}

// FDTableProjection predicts storage growth for implementations whose native
// extreme-gap behavior cannot safely be exercised in the shared process.
type FDTableProjection interface {
	Project(int) (FDProjection, error)
}

type FDTableStats struct {
	ActiveCallbacks int
	ActiveEntries   int
	DenseSlots      int
	MapEntries      int
}
