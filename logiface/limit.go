package logiface

import (
	runtimeutil "github.com/joeycumines/logiface/internal/runtime"
	"path/filepath"
	"runtime"
	"time"
)

type (
	LimitOption func(c *limitConfig)

	limitConfig struct {
	}

	// WARNING: Omits PC because it may differ for the same code location in
	// cases where inlining occurs, which is not desirable for rate limiting.
	callerForRateLimiting struct {
		Function string
		File     string
		Entry    uintptr
		Line     int
	}
)

// used to automatically skip this package when determining the caller
var pkgPath = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}()

// used for testing
var (
	runtimeutilCallerSkipPackage = runtimeutil.CallerSkipPackage
)

// Limit configures limiting behavior for this log message.
//
// Only a single "mode" is currently supported, which is category-based rate
// limiting, where the category is determined by the caller.
// Thus, this method currently won't anything if the logger was not configured
// [WithCategoryRateLimits].
//
// TODO: Support for other modes + allow configuration of "skip" for caller.
//
// This method is not implemented by [Context].
func (x *Builder[E]) Limit(...LimitOption) *Builder[E] {
	if x.Enabled() {
		// default / no option behavior
		switch {
		case x.shared.catrate != nil:
			x.mode |= builderModeCallerCategoryRateLimit
		}
	}
	return x
}

// CallerCategoryRateLimitModifier returns a modifier that will perform
// category-based rate limiting, using the caller as the category. If the
// receiver is nil, or otherwise not configured for category-based rate
// limiting, this method returns nil.
//
// See also [ErrLimited], and [Builder.Limit].
func (x *Logger[E]) CallerCategoryRateLimitModifier() Modifier[E] {
	if x != nil && x.shared != nil && x.shared.catrate != nil {
		return ModifierFunc[E](x.shared.callerCategoryRateLimitModifier)
	}
	return nil
}

// WARNING: Behavior should be kept in sync with [Builder.Limit]->[Builder.log].
func (x *loggerShared[E]) callerCategoryRateLimitModifier(event E) error {
	// skip 2, this method + [ModifierFunc.Modify]
	caller, next, ok := x.catrateAllowCaller(2)
	if !ok {
		return ErrLimited
	}
	if next != (time.Time{}) {
		b := x.newBuilder(event)
		defer b.release(false)
		b.attachCallerRateLimitWarning(caller, next)
	}
	return nil
}

func (x *loggerShared[E]) catrateAllowCaller(skip int) (caller runtimeutil.Caller, next time.Time, ok bool) {
	caller = runtimeutilCallerSkipPackage(pkgPath, skip+1)
	if caller == (runtimeutil.Caller{}) {
		ok = true
	} else {
		next, ok = x.catrate.Allow(callerForRateLimiting(caller))
	}
	return
}

func (x *Builder[E]) attachCallerRateLimitWarning(caller runtimeutil.Caller, next time.Time) {
	x.ObjectFunc(`_limited`, func(b *ObjectBuilder[E, *Chain[E, *Builder[E]]]) {
		b.
			ObjectFunc(`category`, func(b *ObjectBuilder[E, *Chain[E, *Builder[E]]]) {
				// TODO should have a nicer formatter for uintptr
				b.Str(`function`, caller.Function).
					Uint64(`entry`, uint64(caller.Entry)).
					Str(`file`, caller.File).
					Int(`line`, caller.Line)
			}).
			Time(`next`, next).
			Dur(`until`, time.Until(next))
	})
}
