# 02 — Benchmark Inventory

## 166 Benchmarks Across All Platforms

### Benchmark Categories

| Category | Count | Key Pattern |
|----------|-------|------------|
| Burst Submit | 4 | BenchmarkBurstSubmit/* |
| MicroWakeup Syscall | 11 | BenchmarkMicroWakeupSyscall_* |
| MicroBatchBudget Latency | 24 | BenchmarkMicroBatchBudget_Latency/* |
| MicroBatchBudget Mixed | 10 | BenchmarkMicroBatchBudget_Mixed/* |
| MicroBatchBudget Continuous | 4 | BenchmarkMicroBatchBudget_Continuous/* |
| MicroBatchBudget Throughput | 16 | BenchmarkMicroBatchBudget_Throughput/* |
| MicroCAS Contention | 12 | BenchmarkMicroCASContention/* |
| MultiProducer | 4 | BenchmarkMultiProducer/* |
| MultiProducerContention | 4 | BenchmarkMultiProducerContention/* |
| PingPong | 4 | BenchmarkPingPong/* |
| PingPongLatency | 4 | BenchmarkPingPongLatency/* |
| GCPressure | 4 | BenchmarkGCPressure/* |
| GCPressure_Allocations | 4 | BenchmarkGCPressure_Allocations/* |
| ChainDepth | 10 | BenchmarkChainDepth/* |
| Promises | 17 | BenchmarkPromises/* |
| Tournament | 5 | BenchmarkTournament/* |

## Notable Allocation Consistency

- **Allocs/op match rate**: 150/166 (90.4%)
- **B/op match rate**: 110/166 (66.3%) — lower than expected
- **Zero-allocation benchmarks**: 17

## Key Observation: Mixed Batch Variants

The `BenchmarkMicroBatchBudget_Mixed/AlternateThree` variants show the most extreme cross-platform variance:

| Burst Size | Darwin | Linux | Windows |
|------------|--------|-------|---------|
| 10 | 4,587ns | 35,832ns | 236,164ns |
| 20 | 4,438ns | 30,658ns | 484,421ns |
| 50 | 4,430ns | 29,622ns | 278,520ns |
| 100 | 4,482ns | 30,478ns | 459,524ns |

This pattern — AlternateThree showing extreme Windows slowdown — is consistent across mixed-batch benchmarks.
