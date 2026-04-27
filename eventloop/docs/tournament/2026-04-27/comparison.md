comparison.md written to /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-04-27/comparison.md
   Common benchmarks: 166
   Darwin wins: 133, Linux wins: 33
   Significant differences: 142
 -count=5 -run=^$ -benchtime=1s -timeout=10m`
**Benchmarks Compared:** 166 common benchmarks

## Executive Summary

This report compares eventloop benchmark performance between **Darwin (macOS)** and **Linux**,
both running on **ARM64** architecture. Since the architecture is identical, performance
differences reflect OS-level differences: kernel scheduling, memory management, syscall
overhead, and Go runtime behavior on each OS.

### Key Metrics

| Metric | Value |
|--------|-------|
| Common benchmarks | 166 |
| Darwin-only benchmarks | 0 |
| Linux-only benchmarks | 0 |
| Darwin wins (faster) | **133** (80.1%) |
| Linux wins (faster) | **33** (19.9%) |
| Ties | 0 |
| Statistically significant differences | 142 |
| Darwin mean (common benchmarks) | 2,167.40 ns/op |
| Linux mean (common benchmarks) | 6,863.83 ns/op |
| Mean ratio (Darwin/Linux) | 0.640x |
| Median ratio (Darwin/Linux) | 0.607x |
| Allocation match rate | 150/166 (90.4%) |
| Zero-allocation benchmarks (both) | 17 |

## Full Statistical Comparison Table

| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |
|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|
| 1 | BenchmarkBurstSubmit/AlternateThree |         105.48 |       1.4% |        418.58 |      5.9% | Darwin |  0.25x | yes |
| 2 | BenchmarkBurstSubmit/AlternateTwo |         153.60 |       2.4% |        279.96 |     11.8% | Darwin |  0.55x | yes |
| 3 | BenchmarkBurstSubmit/Baseline |         105.89 |       7.2% |        191.90 |      4.6% | Darwin |  0.55x | yes |
| 4 | BenchmarkBurstSubmit/Main |          54.89 |       1.3% |         73.14 |      3.6% | Darwin |  0.75x | yes |
| 5 | BenchmarkChainDepth/ChainedPromise/Depth=10 |       2,641.60 |      13.8% |      8,875.80 |      2.6% | Darwin |  0.30x | yes |
| 6 | BenchmarkChainDepth/ChainedPromise/Depth=100 |      10,787.40 |       9.0% |     42,142.60 |      2.0% | Darwin |  0.26x | yes |
| 7 | BenchmarkChainDepth/PromiseAltFive/Depth=10 |       2,740.00 |       6.1% |      9,831.80 |      4.2% | Darwin |  0.28x | yes |
| 8 | BenchmarkChainDepth/PromiseAltFive/Depth=100 |      10,516.00 |       1.8% |     42,403.40 |      1.5% | Darwin |  0.25x | yes |
| 9 | BenchmarkChainDepth/PromiseAltFour/Depth=10 |       4,610.40 |       5.9% |     15,663.40 |      4.9% | Darwin |  0.29x | yes |
| 10 | BenchmarkChainDepth/PromiseAltFour/Depth=100 |      27,562.20 |       5.7% |     83,769.80 |      1.0% | Darwin |  0.33x | yes |
| 11 | BenchmarkChainDepth/PromiseAltOne/Depth=10 |       2,504.00 |      14.8% |      9,164.40 |      3.8% | Darwin |  0.27x | yes |
| 12 | BenchmarkChainDepth/PromiseAltOne/Depth=100 |      10,268.20 |       2.4% |     41,109.20 |      1.5% | Darwin |  0.25x | yes |
| 13 | BenchmarkChainDepth/PromiseAltTwo/Depth=10 |       3,284.60 |      12.6% |     10,525.40 |      5.7% | Darwin |  0.31x | yes |
| 14 | BenchmarkChainDepth/PromiseAltTwo/Depth=100 |      14,605.60 |       4.7% |     54,261.40 |      2.5% | Darwin |  0.27x | yes |
| 15 | BenchmarkGCPressure/AlternateThree |       2,403.36 |     184.6% |        889.00 |      6.1% | Linux |  2.70x |  |
| 16 | BenchmarkGCPressure/AlternateTwo |         435.10 |       1.3% |        832.10 |      8.8% | Darwin |  0.52x | yes |
| 17 | BenchmarkGCPressure/Baseline |         322.80 |       1.6% |      1,035.60 |      1.0% | Darwin |  0.31x | yes |
| 18 | BenchmarkGCPressure/Main |         441.30 |       0.4% |      1,085.80 |      4.6% | Darwin |  0.41x | yes |
| 19 | BenchmarkGCPressure_Allocations/AlternateThree |          99.44 |       1.2% |        341.02 |      4.8% | Darwin |  0.29x | yes |
| 20 | BenchmarkGCPressure_Allocations/AlternateTwo |         175.64 |       7.7% |        204.42 |      9.1% | Darwin |  0.86x | yes |
| 21 | BenchmarkGCPressure_Allocations/Baseline |          95.42 |       4.6% |        135.58 |      5.6% | Darwin |  0.70x | yes |
| 22 | BenchmarkGCPressure_Allocations/Main |         100.98 |       3.6% |        112.66 |      5.1% | Darwin |  0.90x | yes |
| 23 | BenchmarkMicroBatchBudget_Continuous/AlternateThree |         192.86 |       1.5% |      1,488.00 |      4.4% | Darwin |  0.13x | yes |
| 24 | BenchmarkMicroBatchBudget_Continuous/AlternateTwo |         224.88 |       4.4% |        239.02 |      4.8% | Darwin |  0.94x |  |
| 25 | BenchmarkMicroBatchBudget_Continuous/Baseline |         239.14 |       0.8% |        206.26 |      2.6% | Linux |  1.16x | yes |
| 26 | BenchmarkMicroBatchBudget_Continuous/Main |         191.26 |       2.4% |        126.80 |      3.5% | Linux |  1.51x | yes |
| 27 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         104.02 |       1.7% |        265.04 |      7.4% | Darwin |  0.39x | yes |
| 28 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         139.44 |       1.2% |        497.72 |      3.9% | Darwin |  0.28x | yes |
| 29 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |          98.05 |       1.5% |        323.86 |      2.1% | Darwin |  0.30x | yes |
| 30 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         127.84 |       4.8% |        293.42 |      2.8% | Darwin |  0.44x | yes |
| 31 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |          93.23 |       1.3% |        323.06 |      1.0% | Darwin |  0.29x | yes |
| 32 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         110.46 |       0.2% |        279.38 |      2.3% | Darwin |  0.40x | yes |
| 33 | BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         221.64 |       3.1% |        922.94 |      0.9% | Darwin |  0.24x | yes |
| 34 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=10 |         160.36 |       1.7% |        288.38 |      1.2% | Darwin |  0.56x | yes |
| 35 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=12 |         186.34 |       2.0% |        563.82 |      2.1% | Darwin |  0.33x | yes |
| 36 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=20 |         153.88 |       7.3% |        242.40 |      2.7% | Darwin |  0.63x | yes |
| 37 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=25 |         207.66 |      10.0% |        389.74 |      1.8% | Darwin |  0.53x | yes |
| 38 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=40 |         136.76 |       1.4% |        232.44 |      3.4% | Darwin |  0.59x | yes |
| 39 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=51 |         177.30 |       2.5% |        322.84 |      4.2% | Darwin |  0.55x | yes |
| 40 | BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=64 |         261.96 |       2.0% |        996.36 |      1.4% | Darwin |  0.26x | yes |
| 41 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=1024 |         110.32 |       3.0% |        143.16 |      0.5% | Darwin |  0.77x | yes |
| 42 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=128 |         107.70 |       2.9% |        145.10 |      1.6% | Darwin |  0.74x | yes |
| 43 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=2048 |         101.05 |       2.2% |        140.48 |      1.7% | Darwin |  0.72x | yes |
| 44 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=256 |         106.10 |       3.0% |        144.22 |      1.8% | Darwin |  0.74x | yes |
| 45 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=4096 |         106.16 |       7.6% |        138.50 |      0.8% | Darwin |  0.77x | yes |
| 46 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=512 |         102.28 |       2.6% |        144.18 |      0.4% | Darwin |  0.71x | yes |
| 47 | BenchmarkMicroBatchBudget_Latency/Baseline/Burst=64 |         109.30 |       3.9% |        147.28 |      1.7% | Darwin |  0.74x | yes |
| 48 | BenchmarkMicroBatchBudget_Latency/Main/Burst=1024 |          75.05 |       1.4% |         60.41 |      2.0% | Linux |  1.24x | yes |
| 49 | BenchmarkMicroBatchBudget_Latency/Main/Burst=128 |          50.76 |       1.3% |         67.65 |      4.1% | Darwin |  0.75x | yes |
| 50 | BenchmarkMicroBatchBudget_Latency/Main/Burst=2048 |          86.65 |       2.3% |         61.82 |      7.4% | Linux |  1.40x | yes |
| 51 | BenchmarkMicroBatchBudget_Latency/Main/Burst=256 |          56.27 |       0.4% |         62.10 |      1.6% | Darwin |  0.91x | yes |
| 52 | BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 |          89.34 |       1.4% |         61.53 |      6.8% | Linux |  1.45x | yes |
| 53 | BenchmarkMicroBatchBudget_Latency/Main/Burst=512 |          52.82 |       0.9% |         61.28 |      2.7% | Darwin |  0.86x | yes |
| 54 | BenchmarkMicroBatchBudget_Latency/Main/Burst=64 |          49.89 |       0.6% |         65.37 |      1.1% | Darwin |  0.76x | yes |
| 55 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=10 |       4,587.60 |       1.3% |     35,832.80 |      3.0% | Darwin |  0.13x | yes |
| 56 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=10 |       4,436.20 |       1.2% |     30,573.60 |      4.6% | Darwin |  0.15x | yes |
| 57 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=20 |       4,438.60 |       1.2% |     30,658.00 |      4.6% | Darwin |  0.14x | yes |
| 58 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=50 |       4,430.80 |       1.2% |     29,622.40 |      2.2% | Darwin |  0.15x | yes |
| 59 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=50 |       4,482.60 |       1.1% |     30,478.40 |      4.3% | Darwin |  0.15x | yes |
| 60 | BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=100 |       2,848.40 |       2.2% |     17,126.60 |      4.0% | Darwin |  0.17x | yes |
| 61 | BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=1000 |       3,095.00 |       7.8% |      8,452.60 |      2.6% | Darwin |  0.37x | yes |
| 62 | BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=2000 |       3,240.20 |       4.4% |      8,890.00 |      4.3% | Darwin |  0.36x | yes |
| 63 | BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=500 |       2,775.80 |       2.3% |     10,053.00 |      7.7% | Darwin |  0.28x | yes |
| 64 | BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=5000 |       3,133.00 |       1.8% |      8,875.60 |      3.5% | Darwin |  0.35x | yes |
| 65 | BenchmarkMicroBatchBudget_Mixed/Main/Burst=100 |       2,858.60 |       2.7% |     14,281.60 |      5.8% | Darwin |  0.20x | yes |
| 66 | BenchmarkMicroBatchBudget_Mixed/Main/Burst=1000 |       2,819.00 |       3.7% |      9,070.40 |      4.6% | Darwin |  0.31x | yes |
| 67 | BenchmarkMicroBatchBudget_Mixed/Main/Burst=2000 |       2,761.20 |       1.4% |      9,030.60 |      5.9% | Darwin |  0.31x | yes |
| 68 | BenchmarkMicroBatchBudget_Mixed/Main/Burst=500 |       2,825.00 |       1.1% |     10,408.00 |      4.8% | Darwin |  0.27x | yes |
| 69 | BenchmarkMicroBatchBudget_Mixed/Main/Burst=5000 |       2,832.00 |       1.0% |      8,986.40 |      2.8% | Darwin |  0.32x | yes |
| 70 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         116.36 |       1.5% |        984.20 |      9.9% | Darwin |  0.12x | yes |
| 71 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         194.00 |       0.6% |      8,701.60 |      8.9% | Darwin |  0.02x | yes |
| 72 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         111.02 |       0.6% |        639.26 |      1.9% | Darwin |  0.17x | yes |
| 73 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         147.30 |       2.1% |      3,576.00 |      4.8% | Darwin |  0.04x | yes |
| 74 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         105.74 |       1.3% |        488.32 |      3.2% | Darwin |  0.22x | yes |
| 75 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         130.30 |       0.9% |      1,633.80 |     15.3% | Darwin |  0.08x | yes |
| 76 | BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         312.80 |       1.1% |     14,807.80 |      3.3% | Darwin |  0.02x | yes |
| 77 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         213.12 |       3.0% |        577.84 |      6.6% | Darwin |  0.37x | yes |
| 78 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         251.72 |       0.7% |      8,029.40 |      5.4% | Darwin |  0.03x | yes |
| 79 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         201.48 |       6.9% |        339.74 |      5.4% | Darwin |  0.59x | yes |
| 80 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         241.08 |       2.4% |      2,917.80 |      4.5% | Darwin |  0.08x | yes |
| 81 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         185.70 |       9.9% |        266.78 |      2.1% | Darwin |  0.70x | yes |
| 82 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         223.10 |       3.5% |      1,249.40 |      7.0% | Darwin |  0.18x | yes |
| 83 | BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         375.98 |       1.7% |     18,791.20 |      6.9% | Darwin |  0.02x | yes |
| 84 | BenchmarkMicroBatchBudget_Throughput/Baseline/Burst=102 |         111.72 |       2.8% |        196.56 |      3.0% | Darwin |  0.57x | yes |
| 85 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=1024 |          99.92 |       3.8% |         97.58 |      7.7% | Linux |  1.02x |  |
| 86 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 |         157.32 |       0.3% |      7,442.20 |     11.0% | Darwin |  0.02x | yes |
| 87 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=2048 |         102.87 |       3.8% |         92.17 |      9.3% | Linux |  1.12x |  |
| 88 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 |         102.32 |       1.8% |      4,916.00 |      7.9% | Darwin |  0.02x | yes |
| 89 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=4096 |          99.19 |       1.6% |         86.71 |      5.6% | Linux |  1.14x | yes |
| 90 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=512 |          86.23 |       8.1% |        305.86 |      3.9% | Darwin |  0.28x | yes |
| 91 | BenchmarkMicroBatchBudget_Throughput/Main/Burst=64 |         273.32 |       0.5% |      7,328.40 |     17.2% | Darwin |  0.04x | yes |
| 92 | BenchmarkMicroCASContention/AlternateThree/N=01 |          86.73 |       3.8% |        248.84 |      1.0% | Darwin |  0.35x | yes |
| 93 | BenchmarkMicroCASContention/AlternateThree/N=02 |         125.84 |       4.6% |      1,810.20 |     14.4% | Darwin |  0.07x | yes |
| 94 | BenchmarkMicroCASContention/AlternateThree/N=04 |         143.72 |       0.9% |      2,081.20 |      7.4% | Darwin |  0.07x | yes |
| 95 | BenchmarkMicroCASContention/AlternateThree/N=08 |         139.94 |       3.2% |      1,743.80 |     14.9% | Darwin |  0.08x | yes |
| 96 | BenchmarkMicroCASContention/AlternateThree/N=16 |         137.84 |       1.4% |      1,586.20 |      8.5% | Darwin |  0.09x | yes |
| 97 | BenchmarkMicroCASContention/AlternateThree/N=32 |         126.96 |       0.6% |        970.36 |      8.1% | Darwin |  0.13x | yes |
| 98 | BenchmarkMicroCASContention/Baseline/N=01 |         138.10 |       4.8% |        113.10 |      2.0% | Linux |  1.22x | yes |
| 99 | BenchmarkMicroCASContention/Baseline/N=02 |         156.72 |       4.3% |        138.24 |      0.7% | Linux |  1.13x | yes |
| 100 | BenchmarkMicroCASContention/Baseline/N=04 |         202.00 |       1.0% |        159.86 |      2.0% | Linux |  1.26x | yes |
| 101 | BenchmarkMicroCASContention/Baseline/N=08 |         220.06 |       2.6% |        185.06 |      1.4% | Linux |  1.19x | yes |
| 102 | BenchmarkMicroCASContention/Baseline/N=16 |         198.18 |       0.5% |        203.46 |      0.8% | Darwin |  0.97x | yes |
| 103 | BenchmarkMicroCASContention/Baseline/N=32 |         184.34 |       2.0% |        210.06 |      0.9% | Darwin |  0.88x | yes |
| 104 | BenchmarkMicroCASContention/Main/N=01 |          67.99 |       0.8% |         64.59 |      1.7% | Linux |  1.05x | yes |
| 105 | BenchmarkMicroCASContention/Main/N=02 |         114.20 |       1.3% |         72.93 |      1.8% | Linux |  1.57x | yes |
| 106 | BenchmarkMicroCASContention/Main/N=04 |         124.16 |       1.6% |         78.06 |      1.4% | Linux |  1.59x | yes |
| 107 | BenchmarkMicroCASContention/Main/N=08 |         132.34 |       4.0% |         83.32 |      0.6% | Linux |  1.59x | yes |
| 108 | BenchmarkMicroCASContention/Main/N=16 |         124.32 |       1.5% |         98.96 |      1.9% | Linux |  1.26x | yes |
| 109 | BenchmarkMicroCASContention/Main/N=32 |         119.02 |       2.5% |        120.14 |      2.0% | Darwin |  0.99x |  |
| 110 | BenchmarkMicroWakeupSyscall_Burst/AlternateThree |       1,281.20 |       0.6% |     19,956.40 |      0.1% | Darwin |  0.06x | yes |
| 111 | BenchmarkMicroWakeupSyscall_Burst/AlternateTwo |       1,298.40 |       0.4% |     19,979.00 |      0.2% | Darwin |  0.06x | yes |
| 112 | BenchmarkMicroWakeupSyscall_Burst/Baseline |       1,321.80 |       1.4% |     19,795.00 |      0.3% | Darwin |  0.07x | yes |
| 113 | BenchmarkMicroWakeupSyscall_Burst/Main |       1,239.00 |       0.2% |     19,897.20 |      0.4% | Darwin |  0.06x | yes |
| 114 | BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateThree |          71.12 |       1.4% |        139.88 |      4.4% | Darwin |  0.51x | yes |
| 115 | BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateTwo |          94.47 |       4.4% |        108.06 |      4.3% | Darwin |  0.87x | yes |
| 116 | BenchmarkMicroWakeupSyscall_RapidSubmit/Baseline |         119.80 |       1.4% |         78.25 |      0.8% | Linux |  1.53x | yes |
| 117 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main |          53.41 |       0.6% |         47.97 |      0.4% | Linux |  1.11x | yes |
| 118 | BenchmarkMicroWakeupSyscall_Running/AlternateThree |          89.63 |       0.8% |      1,179.40 |      4.9% | Darwin |  0.08x | yes |
| 119 | BenchmarkMicroWakeupSyscall_Running/AlternateTwo |         211.54 |       3.8% |        145.56 |      3.9% | Linux |  1.45x | yes |
| 120 | BenchmarkMicroWakeupSyscall_Running/Baseline |         144.46 |       2.5% |         87.95 |      2.1% | Linux |  1.64x | yes |
| 121 | BenchmarkMicroWakeupSyscall_Running/Main |          83.24 |       1.4% |         47.94 |      0.5% | Linux |  1.74x | yes |
| 122 | BenchmarkMicroWakeupSyscall_Sleeping/AlternateThree |          70.62 |       2.2% |        142.26 |      2.3% | Darwin |  0.50x | yes |
| 123 | BenchmarkMicroWakeupSyscall_Sleeping/AlternateTwo |          95.25 |       3.4% |        104.48 |      6.9% | Darwin |  0.91x |  |
| 124 | BenchmarkMicroWakeupSyscall_Sleeping/Baseline |         116.56 |       4.2% |         78.34 |      1.5% | Linux |  1.49x | yes |
| 125 | BenchmarkMicroWakeupSyscall_Sleeping/Main |          53.35 |       0.7% |         48.28 |      1.3% | Linux |  1.11x | yes |
| 126 | BenchmarkMultiProducer/AlternateThree |         145.08 |       1.1% |      1,051.80 |      8.9% | Darwin |  0.14x | yes |
| 127 | BenchmarkMultiProducer/AlternateTwo |         158.50 |       3.5% |        179.42 |      3.3% | Darwin |  0.88x | yes |
| 128 | BenchmarkMultiProducer/Baseline |         207.60 |       0.2% |        210.30 |      3.8% | Darwin |  0.99x |  |
| 129 | BenchmarkMultiProducer/Main |         128.72 |       0.4% |        101.54 |      5.1% | Linux |  1.27x | yes |
| 130 | BenchmarkMultiProducerContention/AlternateThree |         133.86 |       0.9% |        215.42 |      4.3% | Darwin |  0.62x | yes |
| 131 | BenchmarkMultiProducerContention/AlternateTwo |         138.06 |       3.3% |        141.90 |      5.2% | Darwin |  0.97x |  |
| 132 | BenchmarkMultiProducerContention/Baseline |         184.74 |       1.1% |        225.82 |      3.5% | Darwin |  0.82x | yes |
| 133 | BenchmarkMultiProducerContention/Main |         125.82 |       0.5% |        171.74 |      7.0% | Darwin |  0.73x | yes |
| 134 | BenchmarkPingPong/AlternateThree |         100.88 |       0.7% |        350.58 |      2.2% | Darwin |  0.29x | yes |
| 135 | BenchmarkPingPong/AlternateTwo |         152.16 |       2.8% |        200.20 |      3.9% | Darwin |  0.76x | yes |
| 136 | BenchmarkPingPong/Baseline |          98.66 |       2.5% |        145.88 |      2.5% | Darwin |  0.68x | yes |
| 137 | BenchmarkPingPong/Main |          99.01 |       1.8% |        108.10 |      5.8% | Darwin |  0.92x | yes |
| 138 | BenchmarkPingPongLatency/AlternateThree |      12,483.80 |       1.4% |     65,198.60 |     10.3% | Darwin |  0.19x | yes |
| 139 | BenchmarkPingPongLatency/AlternateTwo |      12,522.80 |       0.7% |     88,316.20 |     18.7% | Darwin |  0.14x | yes |
| 140 | BenchmarkPingPongLatency/Baseline |         533.02 |       2.3% |        695.00 |     23.6% | Darwin |  0.77x |  |
| 141 | BenchmarkPingPongLatency/Main |         434.38 |       0.4% |        529.94 |      5.3% | Darwin |  0.82x | yes |
| 142 | BenchmarkPromises/ChainedPromise/ChainCreation_Depth100 |       4,499.20 |       0.6% |      6,171.40 |      1.9% | Darwin |  0.73x | yes |
| 143 | BenchmarkPromises/ChainedPromise/CheckResolved_Overhead |         103.72 |       7.3% |        110.12 |      8.7% | Darwin |  0.94x |  |
| 144 | BenchmarkPromises/ChainedPromise/FanOut_100 |      12,295.00 |       7.9% |     20,773.80 |     13.3% | Darwin |  0.59x | yes |
| 145 | BenchmarkPromises/ChainedPromise/Race_100 |      38,576.60 |       2.5% |     38,865.60 |      3.8% | Darwin |  0.99x |  |
| 146 | BenchmarkPromises/PromiseAltFour/ChainCreation_Depth100 |      13,410.60 |       0.2% |     18,071.00 |      2.2% | Darwin |  0.74x | yes |
| 147 | BenchmarkPromises/PromiseAltFour/CheckResolved_Overhead |         201.16 |       4.0% |        202.88 |      4.4% | Darwin |  0.99x |  |
| 148 | BenchmarkPromises/PromiseAltFour/FanOut_100 |      18,927.80 |       5.5% |     21,301.20 |     16.8% | Darwin |  0.89x |  |
| 149 | BenchmarkPromises/PromiseAltFour/Race_100 |      23,912.40 |       0.4% |     33,750.20 |      1.4% | Darwin |  0.71x | yes |
| 150 | BenchmarkPromises/PromiseAltOne/ChainCreation_Depth100 |       4,567.00 |       0.6% |      6,210.20 |      0.9% | Darwin |  0.74x | yes |
| 151 | BenchmarkPromises/PromiseAltOne/CheckResolved_Overhead |         105.55 |       9.6% |        106.02 |      2.7% | Darwin |  1.00x |  |
| 152 | BenchmarkPromises/PromiseAltOne/FanOut_100 |      11,668.20 |       8.3% |     19,727.40 |      1.6% | Darwin |  0.59x | yes |
| 153 | BenchmarkPromises/PromiseAltOne/Race_100 |       3,799.40 |       0.4% |      5,156.00 |      0.5% | Darwin |  0.74x | yes |
| 154 | BenchmarkPromises/PromiseAltThree/ChainCreation_Depth10 |       8,396.40 |       0.3% |      9,672.00 |      0.5% | Darwin |  0.87x | yes |
| 155 | BenchmarkPromises/PromiseAltThree/CheckResolved_Overhea |         151.92 |       4.1% |        143.82 |      2.5% | Linux |  1.06x |  |
| 156 | BenchmarkPromises/PromiseAltThree/FanOut_100 |      10,888.80 |       7.1% |     11,327.80 |      4.1% | Darwin |  0.96x |  |
| 157 | BenchmarkPromises/PromiseAltThree/Race_100 |           0.00 |       8.0% |          0.00 |      9.2% | Linux |  1.06x |  |
| 158 | BenchmarkPromises/PromiseAltTwo/ChainCreation_Depth100 |       6,378.60 |       0.3% |      8,097.60 |      0.9% | Darwin |  0.79x | yes |
| 159 | BenchmarkPromises/PromiseAltTwo/CheckResolved_Overhead |         119.62 |       2.4% |        120.92 |      2.3% | Darwin |  0.99x |  |
| 160 | BenchmarkPromises/PromiseAltTwo/FanOut_100 |       9,596.40 |      13.3% |      9,372.20 |     10.2% | Linux |  1.02x |  |
| 161 | BenchmarkPromises/PromiseAltTwo/Race_100 |           0.00 |      16.3% |          0.00 |     17.7% | Linux |  1.02x |  |
| 162 | BenchmarkTournament/ChainedPromise |         324.44 |       3.3% |        346.28 |      2.3% | Darwin |  0.94x | yes |
| 163 | BenchmarkTournament/PromiseAltFive |         372.44 |       3.1% |        365.60 |      3.0% | Linux |  1.02x |  |
| 164 | BenchmarkTournament/PromiseAltFour |         668.50 |       4.9% |        621.00 |      3.9% | Linux |  1.08x |  |
| 165 | BenchmarkTournament/PromiseAltOne |         300.80 |       1.0% |        303.02 |      3.7% | Darwin |  0.99x |  |
| 166 | BenchmarkTournament/PromiseAltTwo |         339.08 |       4.0% |        332.64 |      2.9% | Linux |  1.02x |  |

## Performance by Category

### Concurrency (22 benchmarks)

- Darwin wins: 13/22
- Linux wins: 9/22
- Darwin category mean: 142.04 ns/op
- Linux category mean: 487.42 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkMicroCASContention/AlternateThree/N=04 |         143.72 |      2,081.20 | Darwin | 14.48x |
| BenchmarkMicroCASContention/AlternateThree/N=02 |         125.84 |      1,810.20 | Darwin | 14.38x |
| BenchmarkMicroCASContention/AlternateThree/N=08 |         139.94 |      1,743.80 | Darwin | 12.46x |
| BenchmarkMicroCASContention/AlternateThree/N=16 |         137.84 |      1,586.20 | Darwin | 11.51x |
| BenchmarkMicroCASContention/AlternateThree/N=32 |         126.96 |        970.36 | Darwin | 7.64x |
| BenchmarkMicroCASContention/AlternateThree/N=01 |          86.73 |        248.84 | Darwin | 2.87x |
| BenchmarkMultiProducerContention/AlternateThree |         133.86 |        215.42 | Darwin | 1.61x |
| BenchmarkMicroCASContention/Main/N=08 |         132.34 |         83.32 | Linux | 1.59x |
| BenchmarkMicroCASContention/Main/N=04 |         124.16 |         78.06 | Linux | 1.59x |
| BenchmarkMultiProducerContention/Main |         125.82 |        171.74 | Darwin | 1.36x |
| BenchmarkMicroCASContention/Baseline/N=04 |         202.00 |        159.86 | Linux | 1.26x |
| BenchmarkMicroCASContention/Main/N=02 |         114.20 |         72.93 | Linux | 1.57x |
| BenchmarkMultiProducerContention/Baseline |         184.74 |        225.82 | Darwin | 1.22x |
| BenchmarkMicroCASContention/Baseline/N=08 |         220.06 |        185.06 | Linux | 1.19x |
| BenchmarkMicroCASContention/Baseline/N=32 |         184.34 |        210.06 | Darwin | 1.14x |
| BenchmarkMicroCASContention/Main/N=16 |         124.32 |         98.96 | Linux | 1.26x |
| BenchmarkMicroCASContention/Baseline/N=01 |         138.10 |        113.10 | Linux | 1.22x |
| BenchmarkMicroCASContention/Baseline/N=02 |         156.72 |        138.24 | Linux | 1.13x |
| BenchmarkMicroCASContention/Baseline/N=16 |         198.18 |        203.46 | Darwin | 1.03x |
| BenchmarkMultiProducerContention/AlternateTwo |         138.06 |        141.90 | Darwin | 1.03x |
| BenchmarkMicroCASContention/Main/N=01 |          67.99 |         64.59 | Linux | 1.05x |
| BenchmarkMicroCASContention/Main/N=32 |         119.02 |        120.14 | Darwin | 1.01x |

### Latency & Primitives (32 benchmarks)

- Darwin wins: 29/32
- Linux wins: 3/32
- Darwin category mean: 917.40 ns/op
- Linux category mean: 5,066.38 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkPingPongLatency/AlternateTwo |      12,522.80 |     88,316.20 | Darwin | 7.05x |
| BenchmarkPingPongLatency/AlternateThree |      12,483.80 |     65,198.60 | Darwin | 5.22x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=64 |         261.96 |        996.36 | Darwin | 3.80x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         221.64 |        922.94 | Darwin | 4.16x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=12 |         186.34 |        563.82 | Darwin | 3.03x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         139.44 |        497.72 | Darwin | 3.57x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |          93.23 |        323.06 | Darwin | 3.47x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |          98.05 |        323.86 | Darwin | 3.30x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=25 |         207.66 |        389.74 | Darwin | 1.88x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         110.46 |        279.38 | Darwin | 2.53x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         127.84 |        293.42 | Darwin | 2.30x |
| BenchmarkPingPongLatency/Baseline |         533.02 |        695.00 | Darwin | 1.30x |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= |         104.02 |        265.04 | Darwin | 2.55x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=51 |         177.30 |        322.84 | Darwin | 1.82x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=10 |         160.36 |        288.38 | Darwin | 1.80x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=40 |         136.76 |        232.44 | Darwin | 1.70x |
| BenchmarkPingPongLatency/Main |         434.38 |        529.94 | Darwin | 1.22x |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=20 |         153.88 |        242.40 | Darwin | 1.58x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=512 |         102.28 |        144.18 | Darwin | 1.41x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=2048 |         101.05 |        140.48 | Darwin | 1.39x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=256 |         106.10 |        144.22 | Darwin | 1.36x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=64 |         109.30 |        147.28 | Darwin | 1.35x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=128 |         107.70 |        145.10 | Darwin | 1.35x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=1024 |         110.32 |        143.16 | Darwin | 1.30x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=4096 |         106.16 |        138.50 | Darwin | 1.30x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 |          89.34 |         61.53 | Linux | 1.45x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=2048 |          86.65 |         61.82 | Linux | 1.40x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=128 |          50.76 |         67.65 | Darwin | 1.33x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=64 |          49.89 |         65.37 | Darwin | 1.31x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=1024 |          75.05 |         60.41 | Linux | 1.24x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=512 |          52.82 |         61.28 | Darwin | 1.16x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=256 |          56.27 |         62.10 | Darwin | 1.10x |

### Other (69 benchmarks)

- Darwin wins: 57/69
- Linux wins: 12/69
- Darwin category mean: 977.17 ns/op
- Linux category mean: 6,323.40 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=10 |       4,587.60 |     35,832.80 | Darwin | 7.81x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=20 |       4,438.60 |     30,658.00 | Darwin | 6.91x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=10 |       4,436.20 |     30,573.60 | Darwin | 6.89x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=50 |       4,482.60 |     30,478.40 | Darwin | 6.80x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=50 |       4,430.80 |     29,622.40 | Darwin | 6.69x |
| BenchmarkMicroWakeupSyscall_Burst/AlternateTwo |       1,298.40 |     19,979.00 | Darwin | 15.39x |
| BenchmarkMicroWakeupSyscall_Burst/AlternateThree |       1,281.20 |     19,956.40 | Darwin | 15.58x |
| BenchmarkMicroWakeupSyscall_Burst/Main |       1,239.00 |     19,897.20 | Darwin | 16.06x |
| BenchmarkMicroWakeupSyscall_Burst/Baseline |       1,321.80 |     19,795.00 | Darwin | 14.98x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         375.98 |     18,791.20 | Darwin | 49.98x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         312.80 |     14,807.80 | Darwin | 47.34x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=100 |       2,848.40 |     17,126.60 | Darwin | 6.01x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=100 |       2,858.60 |     14,281.60 | Darwin | 5.00x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         194.00 |      8,701.60 | Darwin | 44.85x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         251.72 |      8,029.40 | Darwin | 31.90x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=500 |       2,825.00 |     10,408.00 | Darwin | 3.68x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 |         157.32 |      7,442.20 | Darwin | 47.31x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=500 |       2,775.80 |     10,053.00 | Darwin | 3.62x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=64 |         273.32 |      7,328.40 | Darwin | 26.81x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=2000 |       2,761.20 |      9,030.60 | Darwin | 3.27x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=1000 |       2,819.00 |      9,070.40 | Darwin | 3.22x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=5000 |       2,832.00 |      8,986.40 | Darwin | 3.17x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=5000 |       3,133.00 |      8,875.60 | Darwin | 2.83x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=2000 |       3,240.20 |      8,890.00 | Darwin | 2.74x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=1000 |       3,095.00 |      8,452.60 | Darwin | 2.73x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 |         102.32 |      4,916.00 | Darwin | 48.05x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         147.30 |      3,576.00 | Darwin | 24.28x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         241.08 |      2,917.80 | Darwin | 12.10x |
| BenchmarkGCPressure/AlternateThree |       2,403.36 |        889.00 | Linux | 2.70x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         130.30 |      1,633.80 | Darwin | 12.54x |
| BenchmarkMicroBatchBudget_Continuous/AlternateThree |         192.86 |      1,488.00 | Darwin | 7.72x |
| BenchmarkMicroWakeupSyscall_Running/AlternateThree |          89.63 |      1,179.40 | Darwin | 13.16x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         223.10 |      1,249.40 | Darwin | 5.60x |
| BenchmarkMultiProducer/AlternateThree |         145.08 |      1,051.80 | Darwin | 7.25x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         116.36 |        984.20 | Darwin | 8.46x |
| BenchmarkGCPressure/Baseline |         322.80 |      1,035.60 | Darwin | 3.21x |
| BenchmarkGCPressure/Main |         441.30 |      1,085.80 | Darwin | 2.46x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         111.02 |        639.26 | Darwin | 5.76x |
| BenchmarkGCPressure/AlternateTwo |         435.10 |        832.10 | Darwin | 1.91x |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur |         105.74 |        488.32 | Darwin | 4.62x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         213.12 |        577.84 | Darwin | 2.71x |
| BenchmarkPingPong/AlternateThree |         100.88 |        350.58 | Darwin | 3.48x |
| BenchmarkGCPressure_Allocations/AlternateThree |          99.44 |        341.02 | Darwin | 3.43x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=512 |          86.23 |        305.86 | Darwin | 3.55x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         201.48 |        339.74 | Darwin | 1.69x |
| BenchmarkMicroBatchBudget_Throughput/Baseline/Burst=102 |         111.72 |        196.56 | Darwin | 1.76x |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst |         185.70 |        266.78 | Darwin | 1.44x |
| BenchmarkMicroWakeupSyscall_Sleeping/AlternateThree |          70.62 |        142.26 | Darwin | 2.01x |
| BenchmarkMicroWakeupSyscall_Running/AlternateTwo |         211.54 |        145.56 | Linux | 1.45x |
| BenchmarkMicroBatchBudget_Continuous/Main |         191.26 |        126.80 | Linux | 1.51x |
| BenchmarkMicroWakeupSyscall_Running/Baseline |         144.46 |         87.95 | Linux | 1.64x |
| BenchmarkPingPong/AlternateTwo |         152.16 |        200.20 | Darwin | 1.32x |
| BenchmarkPingPong/Baseline |          98.66 |        145.88 | Darwin | 1.48x |
| BenchmarkGCPressure_Allocations/Baseline |          95.42 |        135.58 | Darwin | 1.42x |
| BenchmarkMicroWakeupSyscall_Sleeping/Baseline |         116.56 |         78.34 | Linux | 1.49x |
| BenchmarkMicroWakeupSyscall_Running/Main |          83.24 |         47.94 | Linux | 1.74x |
| BenchmarkMicroBatchBudget_Continuous/Baseline |         239.14 |        206.26 | Linux | 1.16x |
| BenchmarkGCPressure_Allocations/AlternateTwo |         175.64 |        204.42 | Darwin | 1.16x |
| BenchmarkMultiProducer/Main |         128.72 |        101.54 | Linux | 1.27x |
| BenchmarkMultiProducer/AlternateTwo |         158.50 |        179.42 | Darwin | 1.13x |
| BenchmarkMicroBatchBudget_Continuous/AlternateTwo |         224.88 |        239.02 | Darwin | 1.06x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=4096 |          99.19 |         86.71 | Linux | 1.14x |
| BenchmarkGCPressure_Allocations/Main |         100.98 |        112.66 | Darwin | 1.12x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=2048 |         102.87 |         92.17 | Linux | 1.12x |
| BenchmarkMicroWakeupSyscall_Sleeping/AlternateTwo |          95.25 |        104.48 | Darwin | 1.10x |
| BenchmarkPingPong/Main |          99.01 |        108.10 | Darwin | 1.09x |
| BenchmarkMicroWakeupSyscall_Sleeping/Main |          53.35 |         48.28 | Linux | 1.11x |
| BenchmarkMultiProducer/Baseline |         207.60 |        210.30 | Darwin | 1.01x |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=1024 |          99.92 |         97.58 | Linux | 1.02x |

### Promise Operations (35 benchmarks)

- Darwin wins: 28/35
- Linux wins: 7/35
- Darwin category mean: 7,403.53 ns/op
- Linux category mean: 15,111.31 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkChainDepth/PromiseAltFour/Depth=100 |      27,562.20 |     83,769.80 | Darwin | 3.04x |
| BenchmarkChainDepth/PromiseAltTwo/Depth=100 |      14,605.60 |     54,261.40 | Darwin | 3.72x |
| BenchmarkChainDepth/PromiseAltFive/Depth=100 |      10,516.00 |     42,403.40 | Darwin | 4.03x |
| BenchmarkChainDepth/ChainedPromise/Depth=100 |      10,787.40 |     42,142.60 | Darwin | 3.91x |
| BenchmarkChainDepth/PromiseAltOne/Depth=100 |      10,268.20 |     41,109.20 | Darwin | 4.00x |
| BenchmarkChainDepth/PromiseAltFour/Depth=10 |       4,610.40 |     15,663.40 | Darwin | 3.40x |
| BenchmarkPromises/PromiseAltFour/Race_100 |      23,912.40 |     33,750.20 | Darwin | 1.41x |
| BenchmarkPromises/ChainedPromise/FanOut_100 |      12,295.00 |     20,773.80 | Darwin | 1.69x |
| BenchmarkPromises/PromiseAltOne/FanOut_100 |      11,668.20 |     19,727.40 | Darwin | 1.69x |
| BenchmarkChainDepth/PromiseAltTwo/Depth=10 |       3,284.60 |     10,525.40 | Darwin | 3.20x |
| BenchmarkChainDepth/PromiseAltFive/Depth=10 |       2,740.00 |      9,831.80 | Darwin | 3.59x |
| BenchmarkChainDepth/PromiseAltOne/Depth=10 |       2,504.00 |      9,164.40 | Darwin | 3.66x |
| BenchmarkChainDepth/ChainedPromise/Depth=10 |       2,641.60 |      8,875.80 | Darwin | 3.36x |
| BenchmarkPromises/PromiseAltFour/ChainCreation_Depth100 |      13,410.60 |     18,071.00 | Darwin | 1.35x |
| BenchmarkPromises/PromiseAltFour/FanOut_100 |      18,927.80 |     21,301.20 | Darwin | 1.13x |
| BenchmarkPromises/PromiseAltTwo/ChainCreation_Depth100 |       6,378.60 |      8,097.60 | Darwin | 1.27x |
| BenchmarkPromises/ChainedPromise/ChainCreation_Depth100 |       4,499.20 |      6,171.40 | Darwin | 1.37x |
| BenchmarkPromises/PromiseAltOne/ChainCreation_Depth100 |       4,567.00 |      6,210.20 | Darwin | 1.36x |
| BenchmarkPromises/PromiseAltOne/Race_100 |       3,799.40 |      5,156.00 | Darwin | 1.36x |
| BenchmarkPromises/PromiseAltThree/ChainCreation_Depth10 |       8,396.40 |      9,672.00 | Darwin | 1.15x |
| BenchmarkPromises/PromiseAltThree/FanOut_100 |      10,888.80 |     11,327.80 | Darwin | 1.04x |
| BenchmarkPromises/ChainedPromise/Race_100 |      38,576.60 |     38,865.60 | Darwin | 1.01x |
| BenchmarkPromises/PromiseAltTwo/FanOut_100 |       9,596.40 |      9,372.20 | Linux | 1.02x |
| BenchmarkTournament/PromiseAltFour |         668.50 |        621.00 | Linux | 1.08x |
| BenchmarkTournament/ChainedPromise |         324.44 |        346.28 | Darwin | 1.07x |
| BenchmarkPromises/PromiseAltThree/CheckResolved_Overhea |         151.92 |        143.82 | Linux | 1.06x |
| BenchmarkTournament/PromiseAltFive |         372.44 |        365.60 | Linux | 1.02x |
| BenchmarkTournament/PromiseAltTwo |         339.08 |        332.64 | Linux | 1.02x |
| BenchmarkPromises/ChainedPromise/CheckResolved_Overhead |         103.72 |        110.12 | Darwin | 1.06x |
| BenchmarkTournament/PromiseAltOne |         300.80 |        303.02 | Darwin | 1.01x |
| BenchmarkPromises/PromiseAltFour/CheckResolved_Overhead |         201.16 |        202.88 | Darwin | 1.01x |
| BenchmarkPromises/PromiseAltTwo/CheckResolved_Overhead |         119.62 |        120.92 | Darwin | 1.01x |
| BenchmarkPromises/PromiseAltOne/CheckResolved_Overhead |         105.55 |        106.02 | Darwin | 1.00x |
| BenchmarkPromises/PromiseAltThree/Race_100 |           0.00 |          0.00 | Linux | 1.06x |
| BenchmarkPromises/PromiseAltTwo/Race_100 |           0.00 |          0.00 | Linux | 1.02x |

### Task Submission (8 benchmarks)

- Darwin wins: 6/8
- Linux wins: 2/8
- Darwin category mean: 94.83 ns/op
- Linux category mean: 167.22 ns/op

| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |
|-----------|----------------|---------------|--------|-------|
| BenchmarkBurstSubmit/AlternateThree |         105.48 |        418.58 | Darwin | 3.97x |
| BenchmarkBurstSubmit/AlternateTwo |         153.60 |        279.96 | Darwin | 1.82x |
| BenchmarkBurstSubmit/Baseline |         105.89 |        191.90 | Darwin | 1.81x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateThree |          71.12 |        139.88 | Darwin | 1.97x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Baseline |         119.80 |         78.25 | Linux | 1.53x |
| BenchmarkBurstSubmit/Main |          54.89 |         73.14 | Darwin | 1.33x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateTwo |          94.47 |        108.06 | Darwin | 1.14x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Main |          53.41 |         47.97 | Linux | 1.11x |

## Statistically Significant Differences

**142** out of 166 benchmarks show statistically significant
differences (Welch's t-test, p < 0.05).

- Darwin significantly faster: **119** benchmarks
- Linux significantly faster: **23** benchmarks

### Largest Significant Differences

| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |
|-----------|--------|---------|----------------|---------------|--------|
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/ | Darwin | 49.98x |         375.98 |     18,791.20 | 31.77 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=25 | Darwin | 48.05x |         102.32 |      4,916.00 | 27.81 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | Darwin | 47.34x |         312.80 |     14,807.80 | 66.00 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=12 | Darwin | 47.31x |         157.32 |      7,442.20 | 19.98 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | Darwin | 44.85x |         194.00 |      8,701.60 | 24.58 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/ | Darwin | 31.90x |         251.72 |      8,029.40 | 40.34 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=64 | Darwin | 26.81x |         273.32 |      7,328.40 | 12.50 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | Darwin | 24.28x |         147.30 |      3,576.00 | 44.97 |
| BenchmarkMicroWakeupSyscall_Burst/Main | Darwin | 16.06x |       1,239.00 |     19,897.20 | 500.05 |
| BenchmarkMicroWakeupSyscall_Burst/AlternateThree | Darwin | 15.58x |       1,281.20 |     19,956.40 | 2493.35 |
| BenchmarkMicroWakeupSyscall_Burst/AlternateTwo | Darwin | 15.39x |       1,298.40 |     19,979.00 | 967.82 |
| BenchmarkMicroWakeupSyscall_Burst/Baseline | Darwin | 14.98x |       1,321.80 |     19,795.00 | 595.99 |
| BenchmarkMicroCASContention/AlternateThree/N=04 | Darwin | 14.48x |         143.72 |      2,081.20 | 28.00 |
| BenchmarkMicroCASContention/AlternateThree/N=02 | Darwin | 14.38x |         125.84 |      1,810.20 | 14.43 |
| BenchmarkMicroWakeupSyscall_Running/AlternateThree | Darwin | 13.16x |          89.63 |      1,179.40 | 42.22 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | Darwin | 12.54x |         130.30 |      1,633.80 | 13.41 |
| BenchmarkMicroCASContention/AlternateThree/N=08 | Darwin | 12.46x |         139.94 |      1,743.80 | 13.77 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/ | Darwin | 12.10x |         241.08 |      2,917.80 | 45.26 |
| BenchmarkMicroCASContention/AlternateThree/N=16 | Darwin | 11.51x |         137.84 |      1,586.20 | 23.96 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThre | Darwin | 8.46x |         116.36 |        984.20 | 19.96 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | Darwin | 7.81x |       4,587.60 |     35,832.80 | 64.35 |
| BenchmarkMicroBatchBudget_Continuous/AlternateThre | Darwin | 7.72x |         192.86 |      1,488.00 | 44.07 |
| BenchmarkMicroCASContention/AlternateThree/N=32 | Darwin | 7.64x |         126.96 |        970.36 | 24.14 |
| BenchmarkMultiProducer/AlternateThree | Darwin | 7.25x |         145.08 |      1,051.80 | 21.75 |
| BenchmarkPingPongLatency/AlternateTwo | Darwin | 7.05x |      12,522.80 |     88,316.20 | 10.24 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | Darwin | 6.91x |       4,438.60 |     30,658.00 | 41.65 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | Darwin | 6.89x |       4,436.20 |     30,573.60 | 41.59 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | Darwin | 6.80x |       4,482.60 |     30,478.40 | 44.34 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | Darwin | 6.69x |       4,430.80 |     29,622.40 | 85.07 |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=100 | Darwin | 6.01x |       2,848.40 |     17,126.60 | 46.18 |

## Top 10 Fastest Benchmarks

### Darwin

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkPromises/PromiseAltTwo/Race_100 |       0.00 |    0 |         0 | 16.3% |
| 2 | BenchmarkPromises/PromiseAltThree/Race_100 |       0.00 |    0 |         0 | 8.0% |
| 3 | BenchmarkMicroBatchBudget_Latency/Main/Burst=64 |      49.89 |   16 |         1 | 0.6% |
| 4 | BenchmarkMicroBatchBudget_Latency/Main/Burst=128 |      50.76 |   16 |         1 | 1.3% |
| 5 | BenchmarkMicroBatchBudget_Latency/Main/Burst=512 |      52.82 |   16 |         1 | 0.9% |
| 6 | BenchmarkMicroWakeupSyscall_Sleeping/Main |      53.35 |    0 |         0 | 0.7% |
| 7 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main |      53.41 |    0 |         0 | 0.6% |
| 8 | BenchmarkBurstSubmit/Main |      54.89 |   24 |         0 | 1.3% |
| 9 | BenchmarkMicroBatchBudget_Latency/Main/Burst=256 |      56.27 |   16 |         1 | 0.4% |
| 10 | BenchmarkMicroCASContention/Main/N=01 |      67.99 |   16 |         1 | 0.8% |

### Linux

| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |
|------|-----------|-------|------|-----------|-----|
| 1 | BenchmarkPromises/PromiseAltThree/Race_100 |       0.00 |    0 |         0 | 9.2% |
| 2 | BenchmarkPromises/PromiseAltTwo/Race_100 |       0.00 |    0 |         0 | 17.7% |
| 3 | BenchmarkMicroWakeupSyscall_Running/Main |      47.94 |    0 |         0 | 0.5% |
| 4 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main |      47.97 |    0 |         0 | 0.4% |
| 5 | BenchmarkMicroWakeupSyscall_Sleeping/Main |      48.28 |    0 |         0 | 1.3% |
| 6 | BenchmarkMicroBatchBudget_Latency/Main/Burst=1024 |      60.41 |   16 |         1 | 2.0% |
| 7 | BenchmarkMicroBatchBudget_Latency/Main/Burst=512 |      61.28 |   16 |         1 | 2.7% |
| 8 | BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 |      61.53 |   16 |         1 | 6.8% |
| 9 | BenchmarkMicroBatchBudget_Latency/Main/Burst=2048 |      61.82 |   16 |         1 | 7.4% |
| 10 | BenchmarkMicroBatchBudget_Latency/Main/Burst=256 |      62.10 |   16 |         1 | 1.6% |

## Allocation Comparison

Since both platforms run the same Go code, allocations (allocs/op) and bytes (B/op)
should be identical. Differences indicate platform-specific runtime behavior.

- **Allocs/op match:** 150/166 (90.4%)
- **B/op match:** 110/166 (66.3%)
- **Zero-allocation benchmarks (both platforms):** 17

### Zero-Allocation Benchmarks

These benchmarks achieve zero allocations on both platforms — the gold standard
for hot-path performance:

- `BenchmarkBurstSubmit/Main` — Darwin: 54.89 ns/op, Linux: 73.14 ns/op (Darwin faster)
- `BenchmarkMicroBatchBudget_Throughput/Main/Burst=1024` — Darwin: 99.92 ns/op, Linux: 97.58 ns/op (Linux faster)
- `BenchmarkMicroBatchBudget_Throughput/Main/Burst=4096` — Darwin: 99.19 ns/op, Linux: 86.71 ns/op (Linux faster)
- `BenchmarkMicroWakeupSyscall_Burst/AlternateThree` — Darwin: 1281.20 ns/op, Linux: 19956.40 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_Burst/AlternateTwo` — Darwin: 1298.40 ns/op, Linux: 19979.00 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_Burst/Main` — Darwin: 1239.00 ns/op, Linux: 19897.20 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateThree` — Darwin: 71.12 ns/op, Linux: 139.88 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateTwo` — Darwin: 94.47 ns/op, Linux: 108.06 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/Main` — Darwin: 53.41 ns/op, Linux: 47.97 ns/op (Linux faster)
- `BenchmarkMicroWakeupSyscall_Running/AlternateThree` — Darwin: 89.63 ns/op, Linux: 1179.40 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_Running/AlternateTwo` — Darwin: 211.54 ns/op, Linux: 145.56 ns/op (Linux faster)
- `BenchmarkMicroWakeupSyscall_Running/Main` — Darwin: 83.24 ns/op, Linux: 47.94 ns/op (Linux faster)
- `BenchmarkMicroWakeupSyscall_Sleeping/AlternateThree` — Darwin: 70.62 ns/op, Linux: 142.26 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_Sleeping/AlternateTwo` — Darwin: 95.25 ns/op, Linux: 104.48 ns/op (Darwin faster)
- `BenchmarkMicroWakeupSyscall_Sleeping/Main` — Darwin: 53.35 ns/op, Linux: 48.28 ns/op (Linux faster)
- `BenchmarkPromises/PromiseAltThree/Race_100` — Darwin: 0.00 ns/op, Linux: 0.00 ns/op (Linux faster)
- `BenchmarkPromises/PromiseAltTwo/Race_100` — Darwin: 0.00 ns/op, Linux: 0.00 ns/op (Linux faster)

### Allocation Mismatches

| Benchmark | Darwin allocs | Linux allocs | Delta |
|-----------|---------------|--------------|-------|
| BenchmarkBurstSubmit/AlternateThree | 1 | 0 | 1 |
| BenchmarkBurstSubmit/Baseline | 2 | 2 | 0 |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= | 1 | 0 | 1 |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=4096 | 2 | 2 | 0 |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 | 1 | 1 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1 | 0 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1 | 0 | 1 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1 | 1 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1 | 1 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 0 | 0 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1 | 0 | 1 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 | 0 | 0 | 0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=2048 | 0 | 0 | 0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 | 0 | 0 | 0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=64 | 0 | 0 | 0 |
| BenchmarkMicroWakeupSyscall_Burst/Baseline | 1 | 1 | 0 |

### B/op Mismatches

| Benchmark | Darwin B/op | Linux B/op | Delta |
|-----------|-------------|------------|-------|
| BenchmarkBurstSubmit/AlternateTwo | 24 | 25 | 1 |
| BenchmarkBurstSubmit/Baseline | 64 | 64 | 0 |
| BenchmarkChainDepth/ChainedPromise/Depth=10 | 995 | 999 | 4 |
| BenchmarkChainDepth/ChainedPromise/Depth=100 | 8,193 | 8,198 | 5 |
| BenchmarkChainDepth/PromiseAltFive/Depth=10 | 971 | 974 | 3 |
| BenchmarkChainDepth/PromiseAltFive/Depth=100 | 8,169 | 8,174 | 5 |
| BenchmarkChainDepth/PromiseAltFour/Depth=10 | 2,699 | 2,702 | 3 |
| BenchmarkChainDepth/PromiseAltFour/Depth=100 | 24,298 | 24,301 | 3 |
| BenchmarkChainDepth/PromiseAltOne/Depth=10 | 979 | 982 | 3 |
| BenchmarkChainDepth/PromiseAltOne/Depth=100 | 8,177 | 8,182 | 5 |
| BenchmarkChainDepth/PromiseAltTwo/Depth=10 | 1,107 | 1,110 | 3 |
| BenchmarkChainDepth/PromiseAltTwo/Depth=100 | 9,746 | 9,749 | 3 |
| BenchmarkGCPressure/AlternateThree | 26 | 27 | 1 |
| BenchmarkGCPressure/AlternateTwo | 30 | 43 | 13 |
| BenchmarkGCPressure_Allocations/AlternateTwo | 24 | 28 | 4 |
| BenchmarkMicroBatchBudget_Continuous/AlternateTwo | 42 | 40 | 2 |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=10 | 16 | 17 | 1 |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=20 | 16 | 17 | 1 |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=40 | 16 | 17 | 1 |
| BenchmarkMicroBatchBudget_Mixed/AlternateThree/Burst=20 | 24 | 25 | 1 |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=2000 | 64 | 64 | 0 |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=5000 | 64 | 66 | 2 |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=2000 | 24 | 24 | 0 |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=5000 | 24 | 26 | 2 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 26 | 2 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 24 | 0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 26 | 2 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 25 | 1 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 26 | 2 |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 24 | 26 | 2 |
| BenchmarkMicroCASContention/AlternateThree/N=02 | 16 | 16 | 0 |
| BenchmarkMicroCASContention/AlternateThree/N=08 | 16 | 16 | 0 |
| BenchmarkMicroCASContention/AlternateThree/N=32 | 16 | 16 | 0 |
| BenchmarkMicroWakeupSyscall_Burst/Baseline | 40 | 40 | 0 |
| BenchmarkMicroWakeupSyscall_Running/Baseline | 41 | 40 | 1 |
| BenchmarkMicroWakeupSyscall_Sleeping/AlternateTwo | 0 | 0 | 0 |
| BenchmarkMultiProducer/AlternateTwo | 46 | 44 | 2 |
| BenchmarkMultiProducer/Baseline | 56 | 57 | 1 |
| BenchmarkMultiProducerContention/AlternateThree | 17 | 16 | 1 |
| BenchmarkMultiProducerContention/AlternateTwo | 29 | 29 | 0 |
| BenchmarkMultiProducerContention/Main | 16 | 16 | 0 |
| BenchmarkPingPong/AlternateTwo | 24 | 27 | 3 |
| BenchmarkPromises/ChainedPromise/CheckResolved_Overhead | 182 | 179 | 3 |
| BenchmarkPromises/ChainedPromise/FanOut_100 | 23,687 | 23,079 | 607 |
| BenchmarkPromises/ChainedPromise/Race_100 | 28,192 | 27,144 | 1,047 |
| BenchmarkPromises/PromiseAltFour/CheckResolved_Overhead | 322 | 325 | 2 |
| BenchmarkPromises/PromiseAltFour/FanOut_100 | 40,910 | 41,779 | 869 |
| BenchmarkPromises/PromiseAltOne/CheckResolved_Overhead | 181 | 180 | 1 |
| BenchmarkPromises/PromiseAltOne/FanOut_100 | 22,600 | 24,088 | 1,488 |
| BenchmarkPromises/PromiseAltThree/CheckResolved_Overhea | 183 | 178 | 5 |
| BenchmarkPromises/PromiseAltTwo/CheckResolved_Overhead | 179 | 181 | 1 |
| BenchmarkTournament/ChainedPromise | 426 | 427 | 2 |
| BenchmarkTournament/PromiseAltFive | 404 | 404 | 0 |
| BenchmarkTournament/PromiseAltFour | 853 | 852 | 1 |
| BenchmarkTournament/PromiseAltOne | 413 | 412 | 1 |
| BenchmarkTournament/PromiseAltTwo | 411 | 415 | 4 |

## Measurement Stability

Coefficient of variation (CV%) indicates measurement consistency. Lower is better.

- Benchmarks with CV < 2% on both platforms: **26**
- Darwin benchmarks with CV > 5%: **26**
- Linux benchmarks with CV > 5%: **49**

### High-Variance Benchmarks (CV > 5%)

| Benchmark | Darwin CV% | Linux CV% |
|-----------|------------|-----------|
| BenchmarkBurstSubmit/AlternateThree | 1.4% | 5.9% (high) |
| BenchmarkBurstSubmit/AlternateTwo | 2.4% | 11.8% (high) |
| BenchmarkBurstSubmit/Baseline | 7.2% (high) | 4.6% |
| BenchmarkChainDepth/ChainedPromise/Depth=10 | 13.8% (high) | 2.6% |
| BenchmarkChainDepth/ChainedPromise/Depth=100 | 9.0% (high) | 2.0% |
| BenchmarkChainDepth/PromiseAltFive/Depth=10 | 6.1% (high) | 4.2% |
| BenchmarkChainDepth/PromiseAltFour/Depth=10 | 5.9% (high) | 4.9% |
| BenchmarkChainDepth/PromiseAltFour/Depth=100 | 5.7% (high) | 1.0% |
| BenchmarkChainDepth/PromiseAltOne/Depth=10 | 14.8% (high) | 3.8% |
| BenchmarkChainDepth/PromiseAltTwo/Depth=10 | 12.6% (high) | 5.7% (high) |
| BenchmarkGCPressure/AlternateThree | 184.6% (high) | 6.1% (high) |
| BenchmarkGCPressure/AlternateTwo | 1.3% | 8.8% (high) |
| BenchmarkGCPressure_Allocations/AlternateTwo | 7.7% (high) | 9.1% (high) |
| BenchmarkGCPressure_Allocations/Baseline | 4.6% | 5.6% (high) |
| BenchmarkGCPressure_Allocations/Main | 3.6% | 5.1% (high) |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst= | 1.7% | 7.4% (high) |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=20 | 7.3% (high) | 2.7% |
| BenchmarkMicroBatchBudget_Latency/AlternateTwo/Burst=25 | 10.0% (high) | 1.8% |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=4096 | 7.6% (high) | 0.8% |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=2048 | 2.3% | 7.4% (high) |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=4096 | 1.4% | 6.8% (high) |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=1000 | 7.8% (high) | 2.6% |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burst=500 | 2.3% | 7.7% (high) |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=100 | 2.7% | 5.8% (high) |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=2000 | 1.4% | 5.9% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 1.5% | 9.9% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 0.6% | 8.9% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Bur | 0.9% | 15.3% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 3.0% | 6.6% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 0.7% | 5.4% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 6.9% (high) | 5.4% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 9.9% (high) | 2.1% |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 3.5% | 7.0% (high) |
| BenchmarkMicroBatchBudget_Throughput/AlternateTwo/Burst | 1.7% | 6.9% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=1024 | 3.8% | 7.7% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128 | 0.3% | 11.0% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=2048 | 3.8% | 9.3% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256 | 1.8% | 7.9% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=4096 | 1.6% | 5.6% (high) |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=512 | 8.1% (high) | 3.9% |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=64 | 0.5% | 17.2% (high) |
| BenchmarkMicroCASContention/AlternateThree/N=02 | 4.6% | 14.4% (high) |
| BenchmarkMicroCASContention/AlternateThree/N=04 | 0.9% | 7.4% (high) |
| BenchmarkMicroCASContention/AlternateThree/N=08 | 3.2% | 14.9% (high) |
| BenchmarkMicroCASContention/AlternateThree/N=16 | 1.4% | 8.5% (high) |
| BenchmarkMicroCASContention/AlternateThree/N=32 | 0.6% | 8.1% (high) |
| BenchmarkMicroWakeupSyscall_Sleeping/AlternateTwo | 3.4% | 6.9% (high) |
| BenchmarkMultiProducer/AlternateThree | 1.1% | 8.9% (high) |
| BenchmarkMultiProducer/Main | 0.4% | 5.1% (high) |
| BenchmarkMultiProducerContention/AlternateTwo | 3.3% | 5.2% (high) |
| BenchmarkMultiProducerContention/Main | 0.5% | 7.0% (high) |
| BenchmarkPingPong/Main | 1.8% | 5.8% (high) |
| BenchmarkPingPongLatency/AlternateThree | 1.4% | 10.3% (high) |
| BenchmarkPingPongLatency/AlternateTwo | 0.7% | 18.7% (high) |
| BenchmarkPingPongLatency/Baseline | 2.3% | 23.6% (high) |
| BenchmarkPingPongLatency/Main | 0.4% | 5.3% (high) |
| BenchmarkPromises/ChainedPromise/CheckResolved_Overhead | 7.3% (high) | 8.7% (high) |
| BenchmarkPromises/ChainedPromise/FanOut_100 | 7.9% (high) | 13.3% (high) |
| BenchmarkPromises/PromiseAltFour/FanOut_100 | 5.5% (high) | 16.8% (high) |
| BenchmarkPromises/PromiseAltOne/CheckResolved_Overhead | 9.6% (high) | 2.7% |
| BenchmarkPromises/PromiseAltOne/FanOut_100 | 8.3% (high) | 1.6% |
| BenchmarkPromises/PromiseAltThree/FanOut_100 | 7.1% (high) | 4.1% |
| BenchmarkPromises/PromiseAltThree/Race_100 | 8.0% (high) | 9.2% (high) |
| BenchmarkPromises/PromiseAltTwo/FanOut_100 | 13.3% (high) | 10.2% (high) |
| BenchmarkPromises/PromiseAltTwo/Race_100 | 16.3% (high) | 17.7% (high) |

## Key Findings

### 1. Architecture Parity

Both platforms run ARM64, eliminating architectural differences. Performance gaps
are attributable to:
- **OS kernel scheduling** (macOS Mach scheduler vs Linux CFS)
- **Memory management** (macOS memory pressure vs Linux cgroups in container)
- **Syscall overhead** differences
- **Go runtime behavior** variations between `darwin/arm64` and `linux/arm64`

### 2. Performance Distribution

- Darwin significantly faster (ratio < 0.9): **117** benchmarks
- Roughly equal (0.9–1.1x): **25** benchmarks
- Linux significantly faster (ratio > 1.1): **24** benchmarks

### 3. Timer Operations


### 4. Concurrency & Contention

- `BenchmarkBurstSubmit/AlternateThree`: Darwin (3.97x)
- `BenchmarkBurstSubmit/AlternateTwo`: Darwin (1.82x)
- `BenchmarkBurstSubmit/Baseline`: Darwin (1.81x)
- `BenchmarkBurstSubmit/Main`: Darwin (1.33x)
- `BenchmarkMicroCASContention/AlternateThree/N=01`: Darwin (2.87x)
- `BenchmarkMicroCASContention/AlternateThree/N=02`: Darwin (14.38x)
- `BenchmarkMicroCASContention/AlternateThree/N=04`: Darwin (14.48x)
- `BenchmarkMicroCASContention/AlternateThree/N=08`: Darwin (12.46x)
- `BenchmarkMicroCASContention/AlternateThree/N=16`: Darwin (11.51x)
- `BenchmarkMicroCASContention/AlternateThree/N=32`: Darwin (7.64x)
- `BenchmarkMicroCASContention/Baseline/N=01`: Linux (1.22x)
- `BenchmarkMicroCASContention/Baseline/N=02`: Linux (1.13x)
- `BenchmarkMicroCASContention/Baseline/N=04`: Linux (1.26x)
- `BenchmarkMicroCASContention/Baseline/N=08`: Linux (1.19x)
- `BenchmarkMicroCASContention/Baseline/N=16`: Darwin (1.03x)
- `BenchmarkMicroCASContention/Baseline/N=32`: Darwin (1.14x)
- `BenchmarkMicroCASContention/Main/N=01`: Linux (1.05x)
- `BenchmarkMicroCASContention/Main/N=02`: Linux (1.57x)
- `BenchmarkMicroCASContention/Main/N=04`: Linux (1.59x)
- `BenchmarkMicroCASContention/Main/N=08`: Linux (1.59x)
- `BenchmarkMicroCASContention/Main/N=16`: Linux (1.26x)
- `BenchmarkMicroCASContention/Main/N=32`: Darwin (1.01x)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateThree`: Darwin (1.97x)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateTwo`: Darwin (1.14x)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/Baseline`: Linux (1.53x)
- `BenchmarkMicroWakeupSyscall_RapidSubmit/Main`: Linux (1.11x)
- `BenchmarkMultiProducerContention/AlternateThree`: Darwin (1.61x)
- `BenchmarkMultiProducerContention/AlternateTwo`: Darwin (1.03x)
- `BenchmarkMultiProducerContention/Baseline`: Darwin (1.22x)
- `BenchmarkMultiProducerContention/Main`: Darwin (1.36x)

### 5. Summary

**Darwin wins overall** with 133/166 benchmarks faster.

The mean performance ratio of 0.640x (Darwin/Linux) indicates
Darwin is systematically faster across the board.

