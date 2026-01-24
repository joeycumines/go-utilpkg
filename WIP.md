# WIP - Performance Optimization: sync.Map & setImmediate

## Current Goal
Investigate and fix sync.Map misuse and setImmediate suboptimality per user allegations.

## Status
- [x] Investigate allegation 1: sync.Map misuse - **CONFIRMED**
- [x] Investigate allegation 2: setImmediate suboptimality - **CONFIRMED**
- [x] Create implementation plan
- [x] Update blueprint.json with CHUNK_4
- [ ] **BLOCKED: Awaiting user approval of implementation plan**
- [ ] Create BEFORE benchmarks
- [ ] Implement fixes
- [ ] Create AFTER benchmarks
- [ ] Run comparison

## Investigation Summary

### Allegation 1: sync.Map Misuse ✅ CONFIRMED

Per `go doc sync.Map`, optimal for: (1) write-once/read-many, (2) disjoint key access.

Actual usage in `js.go`:
- `intervals`: Store→Load→Delete per API call — **full CRUD**
- `unhandledRejections`: Store→Range→Delete — **full lifecycle**  
- `promiseHandlers`: Store→Load→Delete — **full lifecycle**

**Neither pattern matches.** Solution: Replace with `map[uint64]*XXX + sync.RWMutex`.

### Allegation 2: setImmediate ✅ CONFIRMED

Current: `setImmediate` wraps `setTimeout(fn, 0)` → goes through timer heap.

Proposed: `Loop.Submit` + internal `map[uint64]*setImmediateState` + `atomic.Bool` CAS.

## Reference
- Implementation plan: See `implementation_plan.md`
- Blueprint: See `blueprint.json` CHUNK_4
