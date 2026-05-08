# Eventloop Tournament — 2026-05-08

## Scope

- Monorepo context: `go-utilpkg`
- Focused target: `gmake eventloop-tournament-darwin` and `gmake eventloop-tournament-linux`
- Windows: not run; no `WINDOWS_HOST` runtime access was available in this session
- Raw root logs preserved at repository root:
  - `eventloop-tournament-darwin.log`
  - `eventloop-tournament-linux.log`

## Environment

- Repository HEAD: `802436f7fa69ff99842a58f5583d24b75c4b753e`
- Working diff at artifact creation: `124 files changed, 9475 insertions(+), 4680 deletions(-)`
- Darwin Go: `go version go1.26.4 darwin/arm64`
- Linux container image: `golang:1.26.2`
- Docker: `Docker version 29.6.1, build 8900f1d`

## Commands

```bash
gmake eventloop-tournament-darwin
gmake eventloop-tournament-linux
python3 eventloop/docs/tournament/2026-05-08/parse_benchmarks.py eventloop-tournament-darwin.log darwin unknown unknown > eventloop/docs/tournament/2026-05-08/darwin.json
python3 eventloop/docs/tournament/2026-05-08/parse_benchmarks.py eventloop-tournament-linux.log linux unknown unknown > eventloop/docs/tournament/2026-05-08/linux.json
python3 eventloop/docs/tournament/2026-05-08/analyze_2platform.py
```

## Raw-vs-parsed integrity

| Platform | Raw benchmark records | Parsed benchmark names | Parsed runs | Raw log PASS | Parsed goos/goarch |
|---|---:|---:|---:|---|---|
| Darwin | 190 | 38 | 190 | yes | `darwin/arm64` |
| Linux | 190 | 38 | 190 | yes | `linux/arm64` |

## Outputs

- `darwin.json`
- `linux.json`
- `comparison.md`
- Copied scripts: `parse_benchmarks.py`, `analyze_2platform.py`, `analyze_3platform.py`
