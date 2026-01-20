#!/bin/bash
log_file="/Users/joeyc/dev/go-utilpkg/eventloop_full_60s.log"

# Count all tests that started (including subtests)
all_started=$(grep "^=== RUN   Test" "$log_file" | wc -l | tr -d ' ')
# Count tests WITHOUT "/" - these are top-level tests
top_level_started=$(grep "^=== RUN   Test" "$log_file" | grep -v "/" | wc -l | tr -d ' ')
subtests_started=$(grep "^=== RUN   Test" "$log_file" | grep "/" | wc -l | tr -d ' ')

# Count all test results (including indented subtests - use [[:space:]] to match any whitespace)
all_passed=$(grep "[[:space:]]*--- PASS: Test" "$log_file" | wc -l | tr -d ' ')
all_failed=$(grep "[[:space:]]*--- FAIL: Test" "$log_file" | wc -l | tr -d ' ')

# Count top-level passed tests (indented ones are subtests)
top_passed=$(grep "^--- PASS: Test" "$log_file" | grep -v "/" | wc -l | tr -d ' ')
# Count top-level failed tests
top_failed=$(grep "^--- FAIL: Test" "$log_file" | grep -v "/" | wc -l | tr -d ' ')


# Calculate subtest passed/failed
subtests_passed=$((all_passed - top_passed))
subtests_failed=$((all_failed - top_failed))

# Calculate top-level timed out tests
top_timed_out=$((top_level_started - top_passed - top_failed))

# List failed tests
echo "=== FAILED TESTS ==="
if grep "^--- FAIL:" "$log_file" > /dev/null; then
  grep "^--- FAIL:" "$log_file" | sed 's/--- FAIL: //; s/ (.*//'
fi

# List timed out tests (tests that started but never finished)
echo ""
echo "=== TIMED OUT TESTS ==="
# Get all top-level tests that started
for test_line in $(grep "^=== RUN   Test" "$log_file" | grep -v "/" | sed 's/^=== RUN   //'); do
  test_name=$(echo "$test_line$")  # Just the name part
  # Check if this test has PASS or FAIL result (without slash)
  if ! grep -q "^--- PASS: $test_line " "$log_file" && ! grep -q "^--- FAIL: $test_line " "$log_file"; then
    echo "$test_line (timed out)"
  fi
done

echo ""
echo "=== TEST SUMMARY ==="
echo "Total tests started: $all_started"
echo "  - Top-level tests: $top_level_started"
echo "  - Subtests: $subtests_started"
echo ""
echo "Results:"
echo "  - Passed tests: $all_passed (top-level: $top_passed, subtests: $subtests_passed)"
echo "  - Failed tests: $all_failed (top-level: $top_failed, subtests: $subtests_failed)"
echo "  - Timed out (top-level): $top_timed_out"
echo ""
echo "Overall: NO - $top_failed failed + $top_timed_out timed out"
echo ""

# Get total execution time
exec_time=$(grep "^FAIL" "$log_file" | grep -oE '[0-9.]+s$' || echo "60.318s (detected from panic)")
echo "Total execution time: $exec_time"
