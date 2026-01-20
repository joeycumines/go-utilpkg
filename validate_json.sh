#!/bin/bash
# Validate JSON blueprint.json
python3 -c "import json; json.load(open('blueprint.json'))" 2>&1
echo "Exit code: $?"
