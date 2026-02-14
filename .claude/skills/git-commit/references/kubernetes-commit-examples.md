# Kubernetes-Style Commit Message Examples

Curated examples illustrating correct form. Study the pattern, not the content.

## Single-Line (No Body Needed)

```
Fix nil pointer dereference in token refresh
```

```
Remove deprecated --insecure-port flag
```

```
Update Go version to 1.22.0
```

## With Body

```
Add rate limiting to webhook admission handler

Prevents excessive API server load when external webhook
endpoints are slow or unresponsive. Limits concurrent
in-flight requests per webhook to 25 by default,
configurable via --webhook-max-in-flight flag.
```

```
Fix pod sandbox creation failure on cgroupv2

The container runtime was using the v1 cgroup path format
when the node runs cgroupv2. Detect the cgroup version at
startup and select the appropriate path format.
```

```
Replace status update polling with informer watch

Polling every 10s caused unnecessary API server load at
scale. An informer-based watch reduces steady-state QPS
by ~40% on clusters with >5000 pods while providing
faster convergence on status changes.
```

## Anti-Patterns (Do NOT Emulate)

```
# BAD: conventional commit format
feat(auth): add OAuth2 support

# BAD: vague
Update code

# BAD: past tense
Fixed the bug in scheduler

# BAD: ends with period
Add validation for node labels.

# BAD: GitHub keywords in commit
Fix crash on startup, fixes #4521

# BAD: exceeds 72 characters in subject
Add a new feature that allows users to configure the maximum number of retries for failed API calls
```
