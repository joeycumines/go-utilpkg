#!/bin/bash
# Delete validate_json.go and run make build

set -o pipefail

echo "=== Deleting validate_json.go ===" 
cd /Users/joeyc/dev/go-utilpkg
if [ -f validate_json.go ]; then
    rm -f validate_json.go
    echo "✓ Deleted validate_json.go"
else
    echo "⊙ validate_json.go already does not exist"
fi

echo ""
echo "=== Running make build ==="
make build 2>&1 | fold -w 200 | tee /Users/joeyc/dev/go-utilpkg/build_after_delete.log
EXIT_CODE=${PIPESTATUS[0]}

echo ""
echo "=== EXIT CODE: $EXIT_CODE ==="
echo "Full output logged to: /Users/joeyc/dev/go-utilpkg/build_after_delete.log"

exit $EXIT_CODE
