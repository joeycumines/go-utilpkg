#!/bin/bash
set -euo pipefail

echo "=== Check 1: Inlining analysis for logiface-slog ==="
echo "Running: go build -gcflags=-m ./... 2>&1 | grep -E '(Level|AddField|toSlogLevel)' | head -20"
go build -gcflags=-m ./... 2>&1 | grep -E '(Level|AddField|toSlogLevel)' | head -20 || echo "No matches found"

echo ""
echo "=== Check 2: Struct alignment with betteralign ==="
if command -v betteralign &>/dev/null; then
    echo "Running: betteralign ./..."
    betteralign ./...
elif command -v fieldalignment &>/dev/null; then
    echo "Running: fieldalignment -fix=false ./..."
    fieldalignment -fix=false ./...
else
    echo "No alignment tool found - skipping"
fi

echo ""
echo "Done."
