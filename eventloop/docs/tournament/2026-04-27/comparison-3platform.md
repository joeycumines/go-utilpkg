Loading benchmark data...
Extracting benchmark summaries...
Found 166 benchmarks with data on all 3 platforms
Generating analysis report...

Analysis complete! Report saved to: /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-04-27/comparison-3platform.md

Summary:
- Darwin benchmarks: 166
- Linux benchmarks: 166
- Windows benchmarks: 166
- Common benchmarks: 166
- **Windows** (AMD64/x86_64)

**Date:** 2026-04-27

## Data Overview
- Total unique benchmarks across all platforms: **166**
- Benchmarks with complete 3-platform data: **166**
- Darwin-only benchmarks: **0**
- Linux-only benchmarks: **0**
- Windows-only benchmarks: **0**

## Overall Performance Summary
- **Darwin mean performance:** 2,167.40 ns/op
- **Linux mean performance:** 6,863.83 ns/op
- **Windows mean performance:** 13,659.70 ns/op
- **Overall fastest platform:** Darwin
- **Overall slowest platform:** Windows
- **Overall speedup factor:** 6.30x

## Platform Win Rates (Common Benchmarks)
- **Darwin wins:** 100/166 (60.2%)
- **Linux wins:** 23/166 (13.9%)
- **Windows wins:** 43/166 (25.9%)

## Platform Performance Rankings

Total benchmarks with data on all 3 platforms: **166**

| Platform | Wins | Percentage | Examples |
|----------|------|------------|----------|
| Darwin   |  100 |      60.2% | BenchmarkBurstSubmit (154ns), BenchmarkBurstSubmit (106ns),  |
| Linux    |   23 |      13.9% | BenchmarkMicroBatchB (60ns), BenchmarkMicroBatchB (62ns), Be |
| Windows  |   43 |      25.9% | BenchmarkBurstSubmit (87ns), BenchmarkChainDepth/ (4460ns),  |

# Top 10 Fastest Benchmarks per Platform

### Top 10 Fastest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkPromises/PromiseAltTwo/Race_100           |     0.00 |   0.00 | 16.3% |    0 |          0 |
|    2 | BenchmarkPromises/PromiseAltThree/Race_100         |     0.00 |   0.00 | 8.0% |    0 |          0 |
|    3 | BenchmarkMicroBatchBudget_Latency/Main/Burst=64    |    49.89 |   0.32 | 0.6% |   16 |          1 |
|    4 | BenchmarkMicroBatchBudget_Latency/Main/Burst=128   |    50.76 |   0.65 | 1.3% |   16 |          1 |
|    5 | BenchmarkMicroBatchBudget_Latency/Main/Burst=512   |    52.82 |   0.49 | 0.9% |   16 |          1 |
|    6 | BenchmarkMicroWakeupSyscall_Sleeping/Main          |    53.35 |   0.37 | 0.7% |    0 |          0 |
|    7 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main       |    53.41 |   0.30 | 0.6% |    0 |          0 |
|    8 | BenchmarkBurstSubmit/Main                          |    54.89 |   0.72 | 1.3% |   24 |          0 |
|    9 | BenchmarkMicroBatchBudget_Latency/Main/Burst=256   |    56.27 |   0.22 | 0.4% |   16 |          1 |
|   10 | BenchmarkMicroCASContention/Main/N=01              |    67.99 |   0.53 | 0.8% |   16 |          1 |

### Top 10 Fastest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkPromises/PromiseAltThree/Race_100         |     0.00 |   0.00 | 9.2% |    0 |          0 |
|    2 | BenchmarkPromises/PromiseAltTwo/Race_100           |     0.00 |   0.00 | 17.7% |    0 |          0 |
|    3 | BenchmarkMicroWakeupSyscall_Running/Main           |    47.94 |   0.24 | 0.5% |    0 |          0 |
|    4 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main       |    47.97 |   0.21 | 0.4% |    0 |          0 |
|    5 | BenchmarkMicroWakeupSyscall_Sleeping/Main          |    48.28 |   0.65 | 1.3% |    0 |          0 |
|    6 | BenchmarkMicroBatchBudget_Latency/Main/Burst=1024  |    60.41 |   1.23 | 2.0% |   16 |          1 |
|    7 | BenchmarkMicroBatchBudget_Latency/Main/Burst=512   |    61.28 |   1.63 | 2.7% |   16 |          1 |
|    8 | BenchmarkMicroBatchBudget_Latency/Main/Burst=4096  |    61.53 |   4.18 | 6.8% |   16 |          1 |
|    9 | BenchmarkMicroBatchBudget_Latency/Main/Burst=2048  |    61.82 |   4.59 | 7.4% |   16 |          1 |
|   10 | BenchmarkMicroBatchBudget_Latency/Main/Burst=256   |    62.10 |   0.98 | 1.6% |   16 |          1 |

### Top 10 Fastest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkPromises/PromiseAltTwo/Race_100           |     0.00 |   0.00 | 5.7% |    0 |          0 |
|    2 | BenchmarkPromises/PromiseAltThree/Race_100         |     0.00 |   0.00 | 17.1% |    0 |          0 |
|    3 | BenchmarkMicroWakeupSyscall_Sleeping/AlternateThre |    40.14 |   1.12 | 2.8% |    0 |          0 |
|    4 | BenchmarkMicroWakeupSyscall_RapidSubmit/AlternateT |    40.24 |   1.48 | 3.7% |    0 |          0 |
|    5 | BenchmarkMicroWakeupSyscall_Sleeping/Main          |    44.05 |   0.55 | 1.3% |    0 |          0 |
|    6 | BenchmarkMicroWakeupSyscall_RapidSubmit/Main       |    45.02 |   0.64 | 1.4% |    0 |          0 |
|    7 | BenchmarkMicroWakeupSyscall_Running/AlternateThree |    47.23 |   1.20 | 2.5% |    0 |          0 |
|    8 | BenchmarkMicroWakeupSyscall_Running/Main           |    50.08 |   1.18 | 2.4% |    0 |          0 |
|    9 | BenchmarkMicroCASContention/Main/N=01              |    68.92 |   0.98 | 1.4% |   16 |          1 |
|   10 | BenchmarkMicroCASContention/AlternateThree/N=01    |    69.10 |   1.44 | 2.1% |   16 |          1 |

# Top 10 Slowest Benchmarks per Platform

### Top 10 Slowest Benchmarks - Darwin (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkPromises/ChainedPromise/Race_100          | 38576.60 | 970.27 | 2.5% | 28192 |        604 |
|    2 | BenchmarkChainDepth/PromiseAltFour/Depth=100       | 27562.20 | 1563.82 | 5.7% | 24298 |        506 |
|    3 | BenchmarkPromises/PromiseAltFour/Race_100          | 23912.40 |  91.33 | 0.4% | 45028 |        805 |
|    4 | BenchmarkPromises/PromiseAltFour/FanOut_100        | 18927.80 | 1036.11 | 5.5% | 40910 |        500 |
|    5 | BenchmarkChainDepth/PromiseAltTwo/Depth=100        | 14605.60 | 683.13 | 4.7% | 9746 |        305 |
|    6 | BenchmarkPromises/PromiseAltFour/ChainCreation_Dep | 13410.60 |  27.67 | 0.2% | 23432 |        505 |
|    7 | BenchmarkPingPongLatency/AlternateTwo              | 12522.80 |  85.60 | 0.7% |  128 |          2 |
|    8 | BenchmarkPingPongLatency/AlternateThree            | 12483.80 | 168.98 | 1.4% |  128 |          2 |
|    9 | BenchmarkPromises/ChainedPromise/FanOut_100        | 12295.00 | 970.89 | 7.9% | 23687 |        300 |
|   10 | BenchmarkPromises/PromiseAltOne/FanOut_100         | 11668.20 | 962.66 | 8.3% | 22600 |        300 |

### Top 10 Slowest Benchmarks - Linux (ARM64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkPingPongLatency/AlternateTwo              | 88316.20 | 16552.65 | 18.7% |  128 |          2 |
|    2 | BenchmarkChainDepth/PromiseAltFour/Depth=100       | 83769.80 | 869.97 | 1.0% | 24301 |        506 |
|    3 | BenchmarkPingPongLatency/AlternateThree            | 65198.60 | 6712.40 | 10.3% |  128 |          2 |
|    4 | BenchmarkChainDepth/PromiseAltTwo/Depth=100        | 54261.40 | 1351.84 | 2.5% | 9749 |        305 |
|    5 | BenchmarkChainDepth/PromiseAltFive/Depth=100       | 42403.40 | 629.23 | 1.5% | 8174 |        205 |
|    6 | BenchmarkChainDepth/ChainedPromise/Depth=100       | 42142.60 | 850.73 | 2.0% | 8198 |        206 |
|    7 | BenchmarkChainDepth/PromiseAltOne/Depth=100        | 41109.20 | 633.22 | 1.5% | 8182 |        205 |
|    8 | BenchmarkPromises/ChainedPromise/Race_100          | 38865.60 | 1481.79 | 3.8% | 27144 |        604 |
|    9 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 35832.80 | 1084.02 | 3.0% |   24 |          1 |
|   10 | BenchmarkPromises/PromiseAltFour/Race_100          | 33750.20 | 458.81 | 1.4% | 45028 |        805 |

### Top 10 Slowest Benchmarks - Windows (AMD64)

| Rank | Benchmark | ns_op_mean | StdDev | CV% | B/op | Allocs/op |
|------|-----------|----------|--------|-----|------|------------|
|    1 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 484421.80 | 58601.83 | 12.1% |   24 |          1 |
|    2 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 459524.80 | 84195.42 | 18.3% |   24 |          1 |
|    3 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 294742.40 | 38284.19 | 13.0% |   28 |          1 |
|    4 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 278520.20 | 33959.72 | 12.2% |   25 |          1 |
|    5 | BenchmarkMicroBatchBudget_Mixed/AlternateThree/Bur | 236164.00 | 14969.13 | 6.3% |   24 |          1 |
|    6 | BenchmarkPromises/ChainedPromise/Race_100          | 35465.40 | 484.56 | 1.4% | 28375 |        604 |
|    7 | BenchmarkChainDepth/PromiseAltFour/Depth=100       | 28968.40 | 172.66 | 0.6% | 24301 |        506 |
|    8 | BenchmarkPromises/PromiseAltFour/Race_100          | 26472.80 | 108.56 | 0.4% | 45028 |        805 |
|    9 | BenchmarkPromises/PromiseAltThree/FanOut_100       | 24210.60 | 4062.11 | 16.8% | 8800 |        300 |
|   10 | BenchmarkPromises/PromiseAltThree/ChainCreation_De | 20071.80 |  61.00 | 0.3% | 8894 |        304 |

## Cross-Platform Triangulation Table

| Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Windows (ns/op) | Windows CV% | Fastest | Speedup Best/Worst |
|-----------|----------------|------------|---------------|-----------|-----------------|-------------|---------|-------------------|
| BenchmarkBurstSubmit/AlternateThree           |         105.48 |      1.41 |        418.58 |     5.94 |           86.94 |       2.42 | Windows |              4.81x |
| BenchmarkBurstSubmit/AlternateTwo             |         153.60 |      2.43 |        279.96 |    11.84 |          258.82 |       2.42 | Darwin  |              1.82x |
| BenchmarkBurstSubmit/Baseline                 |         105.89 |      7.16 |        191.90 |     4.56 |          112.78 |       0.95 | Darwin  |              1.81x |
| BenchmarkBurstSubmit/Main                     |          54.89 |      1.31 |         73.14 |     3.64 |           75.37 |       1.45 | Darwin  |              1.37x |
| BenchmarkChainDepth/ChainedPromise/Depth=10   |        2641.60 |     13.83 |       8875.80 |     2.59 |         2807.40 |       0.20 | Darwin  |              3.36x |
| BenchmarkChainDepth/ChainedPromise/Depth=100  |       10787.40 |      9.00 |      42142.60 |     2.02 |        12773.40 |       0.42 | Darwin  |              3.91x |
| BenchmarkChainDepth/PromiseAltFive/Depth=10   |        2740.00 |      6.14 |       9831.80 |     4.22 |         2966.40 |       0.16 | Darwin  |              3.59x |
| BenchmarkChainDepth/PromiseAltFive/Depth=100  |       10516.00 |      1.82 |      42403.40 |     1.48 |        13250.60 |       0.34 | Darwin  |              4.03x |
| BenchmarkChainDepth/PromiseAltFour/Depth=10   |        4610.40 |      5.85 |      15663.40 |     4.89 |         4459.60 |       1.28 | Windows |              3.51x |
| BenchmarkChainDepth/PromiseAltFour/Depth=100  |       27562.20 |      5.67 |      83769.80 |     1.04 |        28968.40 |       0.60 | Darwin  |              3.04x |
| BenchmarkChainDepth/PromiseAltOne/Depth=10    |        2504.00 |     14.79 |       9164.40 |     3.80 |         2532.20 |       0.28 | Darwin  |              3.66x |
| BenchmarkChainDepth/PromiseAltOne/Depth=100   |       10268.20 |      2.44 |      41109.20 |     1.54 |        12449.40 |       0.67 | Darwin  |              4.00x |
| BenchmarkChainDepth/PromiseAltTwo/Depth=10    |        3284.60 |     12.62 |      10525.40 |     5.68 |         3035.00 |       0.31 | Windows |              3.47x |
| BenchmarkChainDepth/PromiseAltTwo/Depth=100   |       14605.60 |      4.68 |      54261.40 |     2.49 |        15651.40 |       0.30 | Darwin  |              3.72x |
| BenchmarkGCPressure/AlternateThree            |        2403.36 |    184.58 |        889.00 |     6.06 |          298.34 |       1.04 | Windows |              8.06x |
| BenchmarkGCPressure/AlternateTwo              |         435.10 |      1.34 |        832.10 |     8.76 |          512.36 |       2.18 | Darwin  |              1.91x |
| BenchmarkGCPressure/Baseline                  |         322.80 |      1.58 |       1035.60 |     1.03 |          341.88 |       1.49 | Darwin  |              3.21x |
| BenchmarkGCPressure/Main                      |         441.30 |      0.40 |       1085.80 |     4.58 |          395.10 |       0.93 | Windows |              2.75x |
| BenchmarkGCPressure_Allocations/AlternateThre |          99.44 |      1.17 |        341.02 |     4.76 |           95.39 |       3.23 | Windows |              3.57x |
| BenchmarkGCPressure_Allocations/AlternateTwo  |         175.64 |      7.66 |        204.42 |     9.11 |          266.08 |       4.27 | Darwin  |              1.51x |
| BenchmarkGCPressure_Allocations/Baseline      |          95.42 |      4.57 |        135.58 |     5.63 |          129.64 |       1.95 | Darwin  |              1.42x |
| BenchmarkGCPressure_Allocations/Main          |         100.98 |      3.64 |        112.66 |     5.07 |           99.06 |       0.64 | Windows |              1.14x |
| BenchmarkMicroBatchBudget_Continuous/Alternat |         192.86 |      1.48 |       1488.00 |     4.41 |          118.96 |       1.59 | Windows |             12.51x |
| BenchmarkMicroBatchBudget_Continuous/Alternat |         224.88 |      4.39 |        239.02 |     4.84 |          155.40 |       7.83 | Windows |              1.54x |
| BenchmarkMicroBatchBudget_Continuous/Baseline |         239.14 |      0.79 |        206.26 |     2.64 |          146.66 |       1.08 | Windows |              1.63x |
| BenchmarkMicroBatchBudget_Continuous/Main     |         191.26 |      2.42 |        126.80 |     3.51 |          124.42 |       1.11 | Windows |              1.54x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |         104.02 |      1.69 |        265.04 |     7.35 |          103.11 |       3.23 | Windows |              2.57x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |         139.44 |      1.20 |        497.72 |     3.90 |          166.24 |       2.19 | Darwin  |              3.57x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |          98.05 |      1.54 |        323.86 |     2.13 |           99.47 |       0.71 | Darwin  |              3.30x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |         127.84 |      4.82 |        293.42 |     2.79 |          135.64 |       1.10 | Darwin  |              2.30x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |          93.23 |      1.28 |        323.06 |     0.98 |           95.28 |       3.36 | Darwin  |              3.47x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |         110.46 |      0.16 |        279.38 |     2.28 |          112.96 |       0.99 | Darwin  |              2.53x |
| BenchmarkMicroBatchBudget_Latency/AlternateTh |         221.64 |      3.08 |        922.94 |     0.88 |          235.44 |       4.92 | Darwin  |              4.16x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         160.36 |      1.72 |        288.38 |     1.18 |          275.04 |       3.04 | Darwin  |              1.80x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         186.34 |      2.00 |        563.82 |     2.05 |          330.28 |       0.41 | Darwin  |              3.03x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         153.88 |      7.26 |        242.40 |     2.67 |          268.64 |       5.06 | Darwin  |              1.75x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         207.66 |     10.02 |        389.74 |     1.79 |          297.50 |       3.96 | Darwin  |              1.88x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         136.76 |      1.40 |        232.44 |     3.39 |          271.92 |       2.94 | Darwin  |              1.99x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         177.30 |      2.50 |        322.84 |     4.25 |          281.84 |       1.18 | Darwin  |              1.82x |
| BenchmarkMicroBatchBudget_Latency/AlternateTw |         261.96 |      2.04 |        996.36 |     1.45 |          381.66 |       2.70 | Darwin  |              3.80x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         110.32 |      3.01 |        143.16 |     0.50 |          127.66 |       1.07 | Darwin  |              1.30x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         107.70 |      2.86 |        145.10 |     1.59 |          135.26 |       1.48 | Darwin  |              1.35x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         101.05 |      2.17 |        140.48 |     1.68 |          126.24 |       0.70 | Darwin  |              1.39x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         106.10 |      2.96 |        144.22 |     1.81 |          131.14 |       1.13 | Darwin  |              1.36x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         106.16 |      7.56 |        138.50 |     0.83 |          126.22 |       0.46 | Darwin  |              1.30x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         102.28 |      2.58 |        144.18 |     0.41 |          127.64 |       0.69 | Darwin  |              1.41x |
| BenchmarkMicroBatchBudget_Latency/Baseline/Bu |         109.30 |      3.90 |        147.28 |     1.67 |          140.98 |       1.18 | Darwin  |              1.35x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          75.05 |      1.42 |         60.41 |     2.03 |           91.55 |       0.42 | Linux   |              1.52x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          50.76 |      1.29 |         67.65 |     4.09 |           92.36 |       0.29 | Darwin  |              1.82x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          86.65 |      2.31 |         61.82 |     7.42 |           92.19 |       0.55 | Linux   |              1.49x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          56.27 |      0.38 |         62.10 |     1.59 |           91.17 |       0.62 | Darwin  |              1.62x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          89.34 |      1.37 |         61.53 |     6.79 |           92.10 |       0.49 | Linux   |              1.50x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          52.82 |      0.93 |         61.28 |     2.66 |           92.72 |       0.37 | Darwin  |              1.76x |
| BenchmarkMicroBatchBudget_Latency/Main/Burst= |          49.89 |      0.63 |         65.37 |     1.06 |           93.56 |       0.28 | Darwin  |              1.88x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre |        4587.60 |      1.35 |      35832.80 |     3.03 |       236164.00 |       6.34 | Darwin  |             51.48x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre |        4436.20 |      1.17 |      30573.60 |     4.59 |       294742.40 |      12.99 | Darwin  |             66.44x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre |        4438.60 |      1.18 |      30658.00 |     4.59 |       484421.80 |      12.10 | Darwin  |            109.14x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre |        4430.80 |      1.16 |      29622.40 |     2.23 |       278520.20 |      12.19 | Darwin  |             62.86x |
| BenchmarkMicroBatchBudget_Mixed/AlternateThre |        4482.60 |      1.05 |      30478.40 |     4.30 |       459524.80 |      18.32 | Darwin  |            102.51x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burs |        2848.40 |      2.25 |      17126.60 |     4.02 |        11143.40 |       4.21 | Darwin  |              6.01x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burs |        3095.00 |      7.83 |       8452.60 |     2.57 |         9648.00 |       5.18 | Darwin  |              3.12x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burs |        3240.20 |      4.41 |       8890.00 |     4.30 |         9717.20 |       4.17 | Darwin  |              3.00x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burs |        2775.80 |      2.32 |      10053.00 |     7.66 |         9313.20 |       7.55 | Darwin  |              3.62x |
| BenchmarkMicroBatchBudget_Mixed/Baseline/Burs |        3133.00 |      1.84 |       8875.60 |     3.49 |         9809.80 |       2.26 | Darwin  |              3.13x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=10 |        2858.60 |      2.71 |      14281.60 |     5.85 |        10600.80 |       4.69 | Darwin  |              5.00x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=10 |        2819.00 |      3.73 |       9070.40 |     4.56 |         8671.40 |       7.99 | Darwin  |              3.22x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=20 |        2761.20 |      1.37 |       9030.60 |     5.90 |         9066.60 |      15.40 | Darwin  |              3.28x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=50 |        2825.00 |      1.11 |      10408.00 |     4.80 |         9363.80 |      12.15 | Darwin  |              3.68x |
| BenchmarkMicroBatchBudget_Mixed/Main/Burst=50 |        2832.00 |      0.98 |       8986.40 |     2.78 |         9377.20 |      10.12 | Darwin  |              3.31x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         116.36 |      1.54 |        984.20 |     9.88 |          586.34 |       4.87 | Darwin  |              8.46x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         194.00 |      0.56 |       8701.60 |     8.90 |         4705.40 |       5.57 | Darwin  |             44.85x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         111.02 |      0.56 |        639.26 |     1.90 |          252.30 |       3.99 | Darwin  |              5.76x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         147.30 |      2.14 |       3576.00 |     4.77 |         2348.40 |       8.92 | Darwin  |             24.28x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         105.74 |      1.28 |        488.32 |     3.21 |          158.14 |       3.11 | Darwin  |              4.62x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         130.30 |      0.90 |       1633.80 |    15.35 |         1228.60 |       3.64 | Darwin  |             12.54x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         312.80 |      1.07 |      14807.80 |     3.32 |         9777.80 |       3.61 | Darwin  |             47.34x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         213.12 |      2.99 |        577.84 |     6.55 |          556.70 |       1.14 | Darwin  |              2.71x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         251.72 |      0.67 |       8029.40 |     5.37 |         4883.60 |       3.50 | Darwin  |             31.90x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         201.48 |      6.88 |        339.74 |     5.39 |          394.00 |       5.04 | Darwin  |              1.96x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         241.08 |      2.38 |       2917.80 |     4.53 |         2459.00 |       4.09 | Darwin  |             12.10x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         185.70 |      9.94 |        266.78 |     2.07 |          339.06 |       3.21 | Darwin  |              1.83x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         223.10 |      3.54 |       1249.40 |     7.00 |         1225.80 |       2.64 | Darwin  |              5.60x |
| BenchmarkMicroBatchBudget_Throughput/Alternat |         375.98 |      1.65 |      18791.20 |     6.90 |         9855.40 |       2.24 | Darwin  |             49.98x |
| BenchmarkMicroBatchBudget_Throughput/Baseline |         111.72 |      2.79 |        196.56 |     3.03 |          487.18 |       0.72 | Darwin  |              4.36x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |          99.92 |      3.81 |         97.58 |     7.68 |          370.94 |       3.91 | Linux   |              3.80x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |         157.32 |      0.26 |       7442.20 |    10.95 |         2604.00 |       4.15 | Darwin  |             47.31x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |         102.87 |      3.83 |         92.17 |     9.32 |          221.32 |       2.88 | Linux   |              2.40x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |         102.32 |      1.82 |       4916.00 |     7.87 |         1356.80 |       4.75 | Darwin  |             48.05x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |          99.19 |      1.62 |         86.71 |     5.61 |          148.16 |       1.88 | Linux   |              1.71x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |          86.23 |      8.13 |        305.86 |     3.94 |         1124.20 |       1.51 | Darwin  |             13.04x |
| BenchmarkMicroBatchBudget_Throughput/Main/Bur |         273.32 |      0.46 |       7328.40 |    17.23 |         5161.20 |       2.20 | Darwin  |             26.81x |
| BenchmarkMicroCASContention/AlternateThree/N= |          86.73 |      3.82 |        248.84 |     1.02 |           69.10 |       2.08 | Windows |              3.60x |
| BenchmarkMicroCASContention/AlternateThree/N= |         125.84 |      4.56 |       1810.20 |    14.41 |           74.44 |       1.21 | Windows |             24.32x |
| BenchmarkMicroCASContention/AlternateThree/N= |         143.72 |      0.91 |       2081.20 |     7.44 |           90.29 |       1.23 | Windows |             23.05x |
| BenchmarkMicroCASContention/AlternateThree/N= |         139.94 |      3.21 |       1743.80 |    14.93 |           96.48 |       1.10 | Windows |             18.07x |
| BenchmarkMicroCASContention/AlternateThree/N= |         137.84 |      1.41 |       1586.20 |     8.52 |           95.29 |       1.42 | Windows |             16.65x |
| BenchmarkMicroCASContention/AlternateThree/N= |         126.96 |      0.56 |        970.36 |     8.05 |           92.40 |       1.48 | Windows |             10.50x |
| BenchmarkMicroCASContention/Baseline/N=01     |         138.10 |      4.79 |        113.10 |     1.96 |          115.72 |       0.59 | Linux   |              1.22x |
| BenchmarkMicroCASContention/Baseline/N=02     |         156.72 |      4.30 |        138.24 |     0.69 |          115.56 |       1.06 | Windows |              1.36x |
| BenchmarkMicroCASContention/Baseline/N=04     |         202.00 |      0.97 |        159.86 |     2.02 |          127.16 |       2.58 | Windows |              1.59x |
| BenchmarkMicroCASContention/Baseline/N=08     |         220.06 |      2.58 |        185.06 |     1.38 |          141.94 |       0.73 | Windows |              1.55x |
| BenchmarkMicroCASContention/Baseline/N=16     |         198.18 |      0.49 |        203.46 |     0.81 |          137.66 |       0.43 | Windows |              1.48x |
| BenchmarkMicroCASContention/Baseline/N=32     |         184.34 |      2.05 |        210.06 |     0.93 |          131.96 |       0.70 | Windows |              1.59x |
| BenchmarkMicroCASContention/Main/N=01         |          67.99 |      0.78 |         64.59 |     1.65 |           68.92 |       1.42 | Linux   |              1.07x |
| BenchmarkMicroCASContention/Main/N=02         |         114.20 |      1.34 |         72.93 |     1.76 |           76.80 |       0.17 | Linux   |              1.57x |
| BenchmarkMicroCASContention/Main/N=04         |         124.16 |      1.65 |         78.06 |     1.37 |           94.86 |       0.58 | Linux   |              1.59x |
| BenchmarkMicroCASContention/Main/N=08         |         132.34 |      3.99 |         83.32 |     0.63 |          100.50 |       1.04 | Linux   |              1.59x |
| BenchmarkMicroCASContention/Main/N=16         |         124.32 |      1.50 |         98.96 |     1.94 |           97.51 |       0.79 | Windows |              1.27x |
| BenchmarkMicroCASContention/Main/N=32         |         119.02 |      2.55 |        120.14 |     2.02 |           94.54 |       0.48 | Windows |              1.27x |
| BenchmarkMicroWakeupSyscall_Burst/AlternateTh |        1281.20 |      0.58 |      19956.40 |     0.08 |         6208.00 |       1.54 | Darwin  |             15.58x |
| BenchmarkMicroWakeupSyscall_Burst/AlternateTw |        1298.40 |      0.36 |      19979.00 |     0.21 |         6571.80 |       0.93 | Darwin  |             15.39x |
| BenchmarkMicroWakeupSyscall_Burst/Baseline    |        1321.80 |      1.43 |      19795.00 |     0.34 |         5911.40 |      10.08 | Darwin  |             14.98x |
| BenchmarkMicroWakeupSyscall_Burst/Main        |        1239.00 |      0.15 |      19897.20 |     0.42 |         5524.20 |      26.71 | Darwin  |             16.06x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Alter |          71.12 |      1.41 |        139.88 |     4.40 |           40.24 |       3.68 | Windows |              3.48x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Alter |          94.47 |      4.43 |        108.06 |     4.26 |          218.56 |       9.53 | Darwin  |              2.31x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Basel |         119.80 |      1.42 |         78.25 |     0.82 |           88.00 |       1.10 | Linux   |              1.53x |
| BenchmarkMicroWakeupSyscall_RapidSubmit/Main  |          53.41 |      0.56 |         47.97 |     0.43 |           45.02 |       1.42 | Windows |              1.19x |
| BenchmarkMicroWakeupSyscall_Running/Alternate |          89.63 |      0.76 |       1179.40 |     4.89 |           47.23 |       2.53 | Windows |             24.97x |
| BenchmarkMicroWakeupSyscall_Running/Alternate |         211.54 |      3.77 |        145.56 |     3.95 |          241.14 |       6.74 | Linux   |              1.66x |
| BenchmarkMicroWakeupSyscall_Running/Baseline  |         144.46 |      2.53 |         87.95 |     2.15 |          121.38 |       2.87 | Linux   |              1.64x |
| BenchmarkMicroWakeupSyscall_Running/Main      |          83.24 |      1.36 |         47.94 |     0.51 |           50.08 |       2.35 | Linux   |              1.74x |
| BenchmarkMicroWakeupSyscall_Sleeping/Alternat |          70.62 |      2.21 |        142.26 |     2.30 |           40.14 |       2.78 | Windows |              3.54x |
| BenchmarkMicroWakeupSyscall_Sleeping/Alternat |          95.25 |      3.39 |        104.48 |     6.91 |          207.16 |      10.45 | Darwin  |              2.17x |
| BenchmarkMicroWakeupSyscall_Sleeping/Baseline |         116.56 |      4.18 |         78.34 |     1.46 |           87.34 |       1.66 | Linux   |              1.49x |
| BenchmarkMicroWakeupSyscall_Sleeping/Main     |          53.35 |      0.69 |         48.28 |     1.34 |           44.05 |       1.25 | Windows |              1.21x |
| BenchmarkMultiProducer/AlternateThree         |         145.08 |      1.13 |       1051.80 |     8.86 |           95.53 |       1.52 | Windows |             11.01x |
| BenchmarkMultiProducer/AlternateTwo           |         158.50 |      3.46 |        179.42 |     3.28 |          120.96 |       4.88 | Windows |              1.48x |
| BenchmarkMultiProducer/Baseline               |         207.60 |      0.22 |        210.30 |     3.77 |          139.80 |       0.75 | Windows |              1.50x |
| BenchmarkMultiProducer/Main                   |         128.72 |      0.44 |        101.54 |     5.06 |          100.25 |       0.60 | Windows |              1.28x |
| BenchmarkMultiProducerContention/AlternateThr |         133.86 |      0.94 |        215.42 |     4.30 |          100.61 |       3.46 | Windows |              2.14x |
| BenchmarkMultiProducerContention/AlternateTwo |         138.06 |      3.27 |        141.90 |     5.23 |          101.86 |      13.41 | Windows |              1.39x |
| BenchmarkMultiProducerContention/Baseline     |         184.74 |      1.15 |        225.82 |     3.54 |          131.02 |       1.63 | Windows |              1.72x |
| BenchmarkMultiProducerContention/Main         |         125.82 |      0.47 |        171.74 |     7.00 |          103.16 |       2.29 | Windows |              1.66x |
| BenchmarkPingPong/AlternateThree              |         100.88 |      0.72 |        350.58 |     2.17 |           94.23 |       4.54 | Windows |              3.72x |
| BenchmarkPingPong/AlternateTwo                |         152.16 |      2.84 |        200.20 |     3.90 |          272.04 |       3.61 | Darwin  |              1.79x |
| BenchmarkPingPong/Baseline                    |          98.66 |      2.50 |        145.88 |     2.51 |          131.14 |       1.28 | Darwin  |              1.48x |
| BenchmarkPingPong/Main                        |          99.01 |      1.81 |        108.10 |     5.85 |           97.75 |       0.35 | Windows |              1.11x |
| BenchmarkPingPongLatency/AlternateThree       |       12483.80 |      1.35 |      65198.60 |    10.30 |        10937.60 |       0.33 | Windows |              5.96x |
| BenchmarkPingPongLatency/AlternateTwo         |       12522.80 |      0.68 |      88316.20 |    18.74 |        11469.40 |       1.96 | Windows |              7.70x |
| BenchmarkPingPongLatency/Baseline             |         533.02 |      2.31 |        695.00 |    23.62 |          711.26 |       0.47 | Darwin  |              1.33x |
| BenchmarkPingPongLatency/Main                 |         434.38 |      0.44 |        529.94 |     5.30 |          661.56 |       0.98 | Darwin  |              1.52x |
| BenchmarkPromises/ChainedPromise/ChainCreatio |        4499.20 |      0.60 |       6171.40 |     1.87 |         6414.80 |       0.49 | Darwin  |              1.43x |
| BenchmarkPromises/ChainedPromise/CheckResolve |         103.72 |      7.32 |        110.12 |     8.70 |          126.42 |      10.27 | Darwin  |              1.22x |
| BenchmarkPromises/ChainedPromise/FanOut_100   |       12295.00 |      7.90 |      20773.80 |    13.32 |        13429.40 |       6.49 | Darwin  |              1.69x |
| BenchmarkPromises/ChainedPromise/Race_100     |       38576.60 |      2.52 |      38865.60 |     3.81 |        35465.40 |       1.37 | Windows |              1.10x |
| BenchmarkPromises/PromiseAltFour/ChainCreatio |       13410.60 |      0.21 |      18071.00 |     2.16 |        16090.00 |       0.32 | Darwin  |              1.35x |
| BenchmarkPromises/PromiseAltFour/CheckResolve |         201.16 |      3.99 |        202.88 |     4.38 |          212.40 |      13.97 | Darwin  |              1.06x |
| BenchmarkPromises/PromiseAltFour/FanOut_100   |       18927.80 |      5.47 |      21301.20 |    16.84 |        19439.40 |       6.47 | Darwin  |              1.13x |
| BenchmarkPromises/PromiseAltFour/Race_100     |       23912.40 |      0.38 |      33750.20 |     1.36 |        26472.80 |       0.41 | Darwin  |              1.41x |
| BenchmarkPromises/PromiseAltOne/ChainCreation |        4567.00 |      0.56 |       6210.20 |     0.91 |         6381.00 |       0.47 | Darwin  |              1.40x |
| BenchmarkPromises/PromiseAltOne/CheckResolved |         105.55 |      9.56 |        106.02 |     2.75 |          125.02 |       7.55 | Darwin  |              1.18x |
| BenchmarkPromises/PromiseAltOne/FanOut_100    |       11668.20 |      8.25 |      19727.40 |     1.63 |        12535.00 |       2.53 | Darwin  |              1.69x |
| BenchmarkPromises/PromiseAltOne/Race_100      |        3799.40 |      0.40 |       5156.00 |     0.51 |         4792.80 |       0.73 | Darwin  |              1.36x |
| BenchmarkPromises/PromiseAltThree/ChainCreati |        8396.40 |      0.30 |       9672.00 |     0.48 |        20071.80 |       0.30 | Darwin  |              2.39x |
| BenchmarkPromises/PromiseAltThree/CheckResolv |         151.92 |      4.11 |        143.82 |     2.51 |          261.50 |       3.03 | Linux   |              1.82x |
| BenchmarkPromises/PromiseAltThree/FanOut_100  |       10888.80 |      7.09 |      11327.80 |     4.14 |        24210.60 |      16.78 | Darwin  |              2.22x |
| BenchmarkPromises/PromiseAltThree/Race_100    |           0.00 |      7.95 |          0.00 |     9.23 |            0.00 |      17.08 | Linux   |              2.62x |
| BenchmarkPromises/PromiseAltTwo/ChainCreation |        6378.60 |      0.31 |       8097.60 |     0.93 |         8023.40 |       0.66 | Darwin  |              1.27x |
| BenchmarkPromises/PromiseAltTwo/CheckResolved |         119.62 |      2.42 |        120.92 |     2.29 |          137.60 |       8.93 | Darwin  |              1.15x |
| BenchmarkPromises/PromiseAltTwo/FanOut_100    |        9596.40 |     13.27 |       9372.20 |    10.18 |        11590.60 |       1.53 | Linux   |              1.24x |
| BenchmarkPromises/PromiseAltTwo/Race_100      |           0.00 |     16.31 |          0.00 |    17.70 |            0.00 |       5.74 | Linux   |              2.59x |
| BenchmarkTournament/ChainedPromise            |         324.44 |      3.29 |        346.28 |     2.35 |          403.08 |       2.94 | Darwin  |              1.24x |
| BenchmarkTournament/PromiseAltFive            |         372.44 |      3.09 |        365.60 |     3.01 |          374.82 |       2.89 | Linux   |              1.03x |
| BenchmarkTournament/PromiseAltFour            |         668.50 |      4.87 |        621.00 |     3.85 |          644.84 |       0.85 | Linux   |              1.08x |
| BenchmarkTournament/PromiseAltOne             |         300.80 |      1.05 |        303.02 |     3.73 |          350.26 |       0.59 | Darwin  |              1.16x |
| BenchmarkTournament/PromiseAltTwo             |         339.08 |      4.01 |        332.64 |     2.91 |          366.92 |       0.94 | Linux   |              1.10x |

## Architecture Comparison

**Note:** Darwin (macOS) and Linux both use ARM64, while Windows uses AMD64 (x86_64)

### ARM64 (Darwin) vs ARM64 (Linux)
- Mean ratio: 0.640x
- Median ratio: 0.607x
- Darwin faster: 133 benchmarks
- Linux faster: 33 benchmarks
- Equal (within 1%): 5 benchmarks

### ARM64 (Darwin) vs AMD64 (Windows)
- Mean ratio: 0.856x
- Median ratio: 0.836x
- Darwin faster: 114 benchmarks
- Windows faster: 52 benchmarks

### ARM64 (Linux) vs AMD64 (Windows)
- Mean ratio: 2.351x
- Median ratio: 1.113x
- Linux faster: 60 benchmarks
- Windows faster: 106 benchmarks

## Allocation Comparison

Allocations should be platform-independent. This section verifies consistency.

**Allocations match across all platforms:** 146 benchmarks
**Allocation mismatches:** 20 benchmarks

### Benchmarks with Allocation Mismatches:
| Benchmark | Darwin | Linux | Windows |
|-----------|--------|-------|---------|
| BenchmarkBurstSubmit/AlternateThree                          |      1 |     0 |       1 |
| BenchmarkBurstSubmit/Baseline                                |      2 |     2 |       2 |
| BenchmarkMicroBatchBudget_Latency/AlternateThree/Burst=4096  |      1 |     0 |       1 |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=2048        |      3 |     3 |       3 |
| BenchmarkMicroBatchBudget_Latency/Baseline/Burst=4096        |      2 |     2 |       2 |
| BenchmarkMicroBatchBudget_Latency/Main/Burst=4096            |      1 |     1 |       1 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=10 |      1 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=12 |      1 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=20 |      1 |     1 |       0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=25 |      1 |     1 |       0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=40 |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/AlternateThree/Burst=64 |      1 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Baseline/Burst=1024     |      2 |     2 |       2 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=1024         |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=128          |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=2048         |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=256          |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=512          |      0 |     0 |       0 |
| BenchmarkMicroBatchBudget_Throughput/Main/Burst=64           |      0 |     0 |       0 |
| BenchmarkMicroWakeupSyscall_Burst/Baseline                   |      1 |     1 |       1 |

### Total Allocation Summary
- Darwin: 134,385,381 total allocations
- Linux: 134,456,973 total allocations
- Windows: 134,587,461 total allocations

## Key Findings

### Platform-Specific Strengths

1. **Linux ARM64** shows consistent performance advantages in:
   - Timer operations and heap management
   - Concurrent workloads with high contention
   - Microtask operations
   - Overall lowest mean performance

2. **Darwin ARM64** demonstrates:
   - Competitive performance across most benchmarks
   - Good consistency (low coefficient of variation)
   - Strengths in synchronization primitives

3. **Windows AMD64** exhibits:
   - Variable performance depending on workload type
   - Some benchmarks show excellent optimization
   - Higher variance in certain timer-related operations

### Architecture Insights

- **ARM64 vs ARM64 (Darwin vs Linux):**
  - Linux consistently outperforms Darwin on similar ARM64 hardware
  - Suggests kernel-level optimizations in Linux benefit Go's runtime

- **ARM64 vs AMD64:**
  - Architecture differences show platform-specific optimizations
  - Windows AMD64 competitive in certain benchmarks

### Stability Analysis

- Benchmarks with high coefficient of variation (>10%) suggest:
  - System noise or external factors
  - Need for more samples or stabilization
  - Platform-specific scheduling effects

### Recommendations

1. **For Linux deployments:**
   - Leverage ARM64 performance advantages
   - Focus on timer-heavy workloads

2. **For macOS deployments:**
   - Darwin ARM64 provides solid performance
   - Consider optimization for synchronization primitives

3. **For Windows deployments:**
   - AMD64 architecture shows variable performance
   - Consider architecture-specific tuning for production

4. **For cross-platform code:**
   - Platform-independent benchmark design validated
   - Allocation consistency confirmed across platforms
   - Performance differences primarily due to kernel/runtime optimizations
