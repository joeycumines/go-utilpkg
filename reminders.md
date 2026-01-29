# Reminders for Takumi

These are permanent reminders that are never cleared:

1. Spend the next four hours finding improvements and enhancements. Record start time in four_hours_start.txt, verify with time-tracker-check make target.

2. Utilize "peer review" extensively with #runSubagent - do not take their word at face value; find local maximum first, then use subagent to validate.

3. Commit only after two contiguous issue-free #runSubagent reviews with specific guarantee instructions.

4. Use #runSubagent in parallel where appropriate.

5. Prove it works in production, including known pre-existing deadlocks on stop.

6. Track ALL tasks exhaustively in ./blueprint.json.

7. Endless cycle: distill improvements, perfect, review, review, rinse, repeat.

8. Use custom targets in config.mk for complex commands, pipe to build.log.

9. Do not stop before four hours elapsed.

10. Expand definition of done to include highest value next step, e.g. integration tests for retry behavior.