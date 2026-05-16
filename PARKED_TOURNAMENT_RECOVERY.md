# Incomplete eventloop tournament parking checkpoint

This artifact preserves the tournament state captured during the 2026-07-21
live-product pivot. It is a recovery checkpoint only. The corpus is incomplete,
not exhaustively inventoried, not fully correctness-qualified, may omit major
variants, and is not a current live baseline. Nothing in this ref supports a
correctness, winner, baseline, or longitudinal-performance claim.

The final REC001 forced-interleaving audit reported unresolved proof gaps after
the last code change. No current-identity broad normal/race, fuzz, platform,
Linux, coverage, freeze, or strict-review qualification was run. Restored work
beyond REC001 likewise has no final completeness census or qualification.

## Captured identity

- Base and pivot HEAD: `469fd952ed251edc7ea1d2bb0faf4e04fc94dd88`
- Preservation commit: `1448706eb9f8ebecf5410147a671a562193a48e0`
- Preservation tree: `f0cca8bcdecb4f8ba1a0f54bea06afdf7f22503d`
- Full mixed staged tree: `9dc24780a841d9965d228788dabe1621e34bfc73`
- Full mixed safety ref: `refs/parking/mixed-index-2026-07-21` at
  `61df44ab8d0063c0c7eedd34a9fc14788de9a795`
- Raw no-renames changed-path records: 916
- Rename-detected diff summary: 890 files changed, 201553 added lines,
  109871 deleted lines
- Historical REC001 selected-index SHA-256:
  `1fafa64028a8de7eea701450545dcbeac8cf9222ce882aac7ae26157a84aef69`.
  The hashed byte stream was not retained, so this value is provenance only,
  not independently reproducible recovery evidence.
- Protected local `config.mk` SHA-256: `6c4bc4ce3c43a2e5f33894c4152f1e8eb56ba6eeb735ca4c6e29fe349146cfff`

The immutable preservation commit contains every pivot change under
`eventloop/internal/` and `eventloop/docs/tournament/`, plus whole-file copies
of the mixed supporting paths `project.mk`, `go.mod`, and `go.sum`. The latter
are deliberately over-inclusive so tournament targets and benchstat dependency
bytes remain recoverable even though `project.mk` also contains candidate-module
work owned by a later live-product task.

The sibling ref `refs/parking/mixed-index-2026-07-21` resolves to commit
`61df44ab8d0063c0c7eedd34a9fc14788de9a795` and preserves the complete mixed
tree corresponding to all 916 no-renames changed-path records as a
classification-error safety net. It is a recovery snapshot, not a qualified
product commit.

## Embedded manifests

- `PARKING/full-index.raw`: exact old/new mode and blob records for the complete
  mixed index; SHA-256
  `9dc7aace34d70153787fd7e47f81702631c5148ad121c57d8d89d48573deeff0`.
- `PARKING/parked-primary.raw`: exact records for `eventloop/internal/` and
  `eventloop/docs/tournament/`; SHA-256
  `73ed04776516d6bf70499f195126e0b879f4dfac5d8c43b79ad7c7e02e693a76`.
- `PARKING/parked-support.raw`: exact records for the primary set plus
  `project.mk`, `go.mod`, and `go.sum`; SHA-256
  `296cd03f352598edcdc8eec6a37e8e3221596528b35318f528d384e45b7e4954`.

Each manifest is `git diff --cached --raw --no-abbrev --no-renames` output
captured before foreground removal. The commit tree itself is the authoritative
new-mode/new-blob recovery source; the raw manifests also retain deletions and
their old blobs.

## Recovery

Inspect without touching an existing checkout:

```sh
git worktree add --detach /tmp/go-utilpkg-tournament-recovery \
  1448706eb9f8ebecf5410147a671a562193a48e0
```

Recover the entire pre-partition mixed index instead when classification is in
doubt:

```sh
git worktree add --detach /tmp/go-utilpkg-mixed-recovery \
  refs/parking/mixed-index-2026-07-21
```

Future recovery begins by authenticating these commits, refs, and manifests
against the then-current repository. Their existence preserves bytes but does
not establish current qualification.
