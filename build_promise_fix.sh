#!/bin/bash
# Build eventloop to verify promise.go changes compile correctly

cd /Users/joeyc/dev/go-utilpkg/eventloop

echo "=== Building eventloop package ==="
go build ./... 2>&1 | tee /tmp/promise_fix_build.log
EXIT_CODE=${PIPESTATUS[0]}

if [ $EXIT_CODE -ne 0 ]; then
    echo "❌ BUILD FAILED"
    echo ""
    echo "=== Build Errors ==="
    cat /tmp/promise_fix_build.log
    exit 1
else
    echo "✓ BUILD SUCCESSFUL"
    echo ""
    echo "=== Compiled Files ==="
    ls -lh /tmp/eventloop 2>/dev/null || echo "Binary location may vary"
    exit 0
fi
