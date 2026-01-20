#!/bin/bash
# Install betteralign if not already present
if ! command -v betteralign &> /dev/null; then
    echo "Installing betteralign..."
    go install github.com/dkorunic/betteralign/cmd/betteralign@latest
    echo "betteralign installed at $(go env GOPATH)/bin/betteralign"
else
    echo "betteralign is already installed: $(which betteralign)"
fi
