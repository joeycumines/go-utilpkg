# Security Guide for logiface-slog

This guide covers security best practices for logging with `logiface-slog` (islog) adapter.

## Overview

Logging production applications requires careful handling of sensitive data:

- **Personally Identifiable Information (PII)**: Names, emails, phone numbers, addresses
- **Credentials**: Passwords, API keys, tokens, certificates
- **Financial Data**: Credit cards, bank account numbers
- **Health Information**: Medical records, diagnoses, treatments
- **Security Events**: Authentication attempts, authorization failures

## PII Handling Guidance

### Do Not Log PII by Default

**AVOID:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("user login successful")
event.AddField("user_name", "John Smith")      // PII
event.AddField("email", "john@example.com")       // PII
event.AddField("phone", "+1-555-123-4567")    // PII
event.AddField("address", "123 Main St, Anytown")// PII
logger.Write(logger.ReleaseEvent(event))
```

**RECOMMENDED:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("user login successful")
event.AddField("user_id", "user_123")         // Non-PII identifier
event.AddField("email_hash", sha256Hash(email))   // Hashed, reversible only if needed
event.AddField("login_method", "password")     // Non-PII context
logger.Write(logger.ReleaseEvent(event))
```

### Use Non-Reversible Identifiers

**RECOMMENDED:**
```go
event.AddField("user_id", generateUserID())           // Application-generated ID
event.AddField("account_id", account.ID)           // Internal account number
event.AddField("session_id", session.ID)           // Session token (non-sensitive)
```

**AVOID:**
```go
event.AddField("ssn", "123-45-6789")           // Social Security Number
event.AddField("credit_card", "4111-1111-1111-1111") // Full credit card number
event.AddField("passport", "A12345678")          // Passport number
```

### When PII Logging is Required

If business requirements mandate PII logging:

1. **Document explicitly** in security/approval records
2. **Encrypt logs at rest** (disk encryption, encrypted storage)
3. **Restrict access** (role-based access control, audit trail)
4. **Consider hashing** (if lookup is needed, use salted hashes)

**RECOMMENDED (PII required):**
```go
event := logger.NewEvent(logiface.LevelWarning)
event.AddMessage("compliance: PII logged per legal requirement")
event.AddField("ticket_id", "LEGAL-REQ-20240118")
event.AddField("data_type", "user_name")
event.AddField("encryption_method", "AES-256-GCM")
event.AddField("authorized_by", "security-team-123")
logger.Write(logger.ReleaseEvent(event))

// Log the PII separately with restricted access
piLogger := logger.WithHandler(restrictedHandler)  // Logs to encrypted file
event := piLogger.NewEvent(logiface.LevelInformational)
event.AddMessage("user registration")
event.AddField("user_name", "Jane Doe") // PII - restricted access
piLogger.Write(piLogger.ReleaseEvent(event))
```

## Credential Redaction Patterns

### Use ReplaceAttr to Redact Sensitive Fields

Configure `slog.Handler` with `ReplaceAttr` to automatically redact sensitive data:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        // Redact password fields
        switch a.Key {
        case "password", "passwd", "pwd", "secret":
            return slog.String(a.Key, "********")
        case "api_key", "apikey", "token", "authorization":
            // Show first 4 characters for debugging
            return slog.String(a.Key, maskToken(a.Value.String()))
        case "credit_card", "card_number":
            // Show last 4 digits
            return slog.String(a.Key, maskCreditCard(a.Value.String()))
        default:
            return a
        }
    },
})

logger := logiface.New[*islog.Event](islog.WithSlogHandler(handler))
```

**Helper functions:**
```go
func maskToken(token string) string {
    if len(token) <= 8 {
        return "********"
    }
    return token[:4] + "****" // Show first 4 characters for debugging
}

func maskCreditCard(card string) string {
    if len(card) < 16 {
        return "****"
    }
    return "****" + card[len(card)-4:] // Show last 4 for identification
}
```

### Redact Nested Fields

```go
ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
    // Handle nested fields under "user" group
    if len(groups) > 0 && groups[0] == "user" {
        switch a.Key {
        case "password", "email", "phone":
            return slog.String(a.Key, "***REDACTED***")
        default:
            return a
        }
    }
    return a
}
```

### Field Naming Patterns for Redaction

Use consistent suffixes to identify sensitive fields:

- `*_password`: user_password, admin_password
- `*_key`: api_key, secret_key, encryption_key
- `*_token`: access_token, refresh_token, csrf_token
- `*_secret`: webhook_secret, jwt_secret
- `*_header`: authorization_header, x-api-key

**RECOMMENDED:**
```go
// Consistent naming enables automatic redact detection
event.AddField("database_password", "secret123")  // Auto-redacted by pattern
event.AddField("api_access_key", "sk_live_...") // Auto-redacted by pattern
event.AddField("jwt_token", "eyJhbGc...")         // Auto-redacted by pattern
```

### Manual Redaction (When ReplaceAttr Not Available)

```go
func logUserLogin(logger *logiface.Logger[*Event], user User, password string) {
    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("user login attempt")
    event.AddField("username", user.Username)  // Username may be non-PII
    // NEVER log password
    event.AddField("password_provided", password != "") // Boolean only
    event.AddField("login_method", "form")
    logger.Write(logger.ReleaseEvent(event))
}
```

## Audit Logging Requirements

### Separate Audit Trail

Create dedicated logger for audit events (compliance, forensics):

```go
// Audit logger writes to immutable, append-only storage
auditHandler := slog.NewJSONHandler(auditWriter, &slog.HandlerOptions{
    Level: slog.LevelInfo,  // All audit logs emitted
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        // Audit logs may have stricter redact rules
        return sanitizeForAudit(a)
    },
})

auditLogger := logiface.New[*islog.Event](islog.WithSlogHandler(auditHandler))

func logAuthenticationEvent(user User, success bool, ip string) {
    event := auditLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("authentication event")
    event.AddField("timestamp", time.Now().UTC().Format(time.RFC3339))
    event.AddField("user_id", user.ID)           // Non-PII identifier
    event.AddField("success", success)
    event.AddField("ip_address", ip)
    event.AddField("user_agent", "")                // Never log PII in audit
    auditLogger.Write(auditLogger.ReleaseEvent(event))
}
```

### Audit Log Fields

**REQUIRED for compliance audits:**
- `timestamp`: Exact UTC timestamp with timezone
- `event_type`: Category of event (authentication, data_access, config_change)
- `actor_id`: Who performed the action (user ID, service account)
- `target_id`: What was affected (resource ID, record ID)
- `action`: What was done (create, read, update, delete)
- `result`: Success/failure, error details
- `ip_address`: Source IP (for security incident correlation)

**RECOMMENDED:**
```go
func logDataAccessEvent(actorID, resourceID, action, result string) {
    event := auditLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("data access event")
    event.AddField("timestamp", time.Now().UTC().Format(time.RFC3339))
    event.AddField("event_type", "data_access")
    event.AddField("actor_id", actorID)
    event.AddField("target_id", resourceID)
    event.AddField("action", action)
    event.AddField("result", result)
    auditLogger.Write(auditLogger.ReleaseEvent(event))
}
```

### Audit Log Immutability

Audit logs must be append-only and tamper-evident:

```go
// AuditWriter ensures append-only, sequential writes
type AuditWriter struct {
    file           *os.File
    filePath       string
    sequenceNumber int64
}

func NewAuditWriter(filePath string) (*AuditWriter, error) {
    file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil {
        return nil, err
    }
    return &AuditWriter{
        file:     file,
        filePath: filePath,
    }, nil
}

func (aw *AuditWriter) Write(p []byte) (n int, err error) {
    // Prepend sequence number (detects tampering if gaps)
    aw.sequenceNumber++
    record := fmt.Sprintf("seq=%d %s", aw.sequenceNumber, string(p))
    return aw.file.Write([]byte(record))
}

func (aw *AuditWriter) Sync() error {
    return aw.file.Sync()
}
```

## Error Message Security

### Do Not Log Sensitive Error Details

**AVOID:**
```go
err := db.Query("SELECT * FROM users WHERE email = '" + email + "'")
if err != nil {
    // ERROR: May log email in SQL syntax error message!
    event := logger.NewEvent(logiface.LevelError)
    event.AddMessage("database query failed")
    event.AddError(err) // err.Error() might contain email!
    logger.Write(logger.ReleaseEvent(event))
}
```

**RECOMMENDED:**
```go
err := db.Query("SELECT * FROM users WHERE email = ?", email)
if err != nil {
    event := logger.NewEvent(logiface.LevelError)
    event.AddMessage("database query failed")
    event.AddField("operation", "select_users")
    event.AddField("error_code", classifyDatabaseError(err))
    // Do NOT include user input in error!
    logger.Write(logger.ReleaseEvent(event))
}
```

### Custom Error Types

Wrap sensitive errors with non-sensitive messages:

```go
type AuthenticationError struct {
    code    string
    message string
}

func (e *AuthenticationError) Error() string {
    // Safe for logging - no PII
    return fmt.Sprintf("authentication error: %s", e.code)
}

func authenticateUser(username, password string) error {
    if !checkPassword(username, password) {
        return &AuthenticationError{
            code:    "INVALID_CREDENTIALS",
            message: "authentication failed", // Non-specific
        }
        // NOT: "authentication failed for user " + username + " with invalid password"
    }
    return nil
}
```

## Log Access Control

### File Permissions

Set restrictive permissions on log files:

```bash
# Production logs: owner read/write only
chmod 600 /var/log/app/application.log

# Audit logs: append-only for root/auditors
chmod 200 /var/log/app/audit.log  # Write-only
chown audit:audit /var/log/app/audit.log

# Log directories: no execute for non-owners
chmod 750 /var/log/app
```

### Log Rotation Security

Configure log rotation to prevent unauthorized retention:

```go
// Use lumberjack or logrotate with secure settings
logFile := &lumberjack.Logger{
    Filename:   "/var/log/app/application.log",
    MaxSize:    100, // megabytes
    MaxBackups: 7,  // Keep 7 days
    MaxAge:     7,  // days
    Compress:   true, // Compress rotated files
    LocalTime:  true,
}

handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})

// Ensure rotated files have correct permissions
os.Chmod("/var/log/app/application.log", 0600)
```

## Network Request Logging

### Do Not Log Full Request Bodies

**AVOID:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("HTTP POST received")
event.AddField("body", string(r.Body)) // May contain PII, passwords!
logger.Write(logger.ReleaseEvent(event))
```

**RECOMMENDED:**
```go
event := logger.NewEvent(logiface.LevelInformational)
event.AddMessage("HTTP POST received")
event.AddField("method", r.Method)
event.AddField("path", r.URL.Path)
event.AddField("content_length", r.ContentLength)
event.AddField("content_type", r.Header.Get("Content-Type"))
// Do NOT log body - may be sensitive
logger.Write(logger.ReleaseEvent(event))
```

### Sanitize URL Parameters

```go
func logHTTPRequest(logger *logiface.Logger[*Event], r *http.Request) {
    // Sanitize URL to remove sensitive query parameters
    cleanURL := sanitizeURL(r.URL.String())

    event := logger.NewEvent(logiface.LevelInformational)
    event.AddMessage("HTTP request")
    event.AddField("method", r.Method)
    event.AddField("url", cleanURL)
    logger.Write(logger.ReleaseEvent(event))
}

func sanitizeURL(rawURL string) string {
    // Remove known sensitive parameters
    sensitiveParams := []string{"password", "token", "api_key", "secret"}

    for _, param := range sensitiveParams {
        // Replace parameter value with ***REDACTED***
        re := regexp.MustCompile(`(?i)(\b|&?)` + param + `=[^&\s]*`)
        rawURL = re.ReplaceAllString(rawURL, `$1`+param+`=***REDACTED***`)
    }

    return rawURL
}
```

## Regulatory Compliance

### GDPR (EU General Data Protection)

- **Right to be forgotten:** Implement log deletion procedures
- **Data portability:** Export logs in structured format on request
- **Consent:** Log user consent status changes

**RECOMMENDED:**
```go
func logConsentChange(userID string, hasConsented bool, timestamp time.Time) {
    event := auditLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("user consent changed")
    event.AddField("timestamp", timestamp.UTC().Format(time.RFC3339))
    event.AddField("user_id", userID)
    event.AddField("consent_given", hasConsented)
    event.AddField("consent_version", "2024.01_GDPR")
    event.AddField("legal_basis", "legitimate_interest")
    auditLogger.Write(auditLogger.ReleaseEvent(event))
}
```

### HIPAA (US Health Insurance Portability)

- **PHI (Protected Health Information):** Minimum necessary logging
- **Access logging:** Every PHI access must be logged with who/what/when/why

**RECOMMENDED:**
```go
func logPHIAccess(actorID, patientID, recordType, reason string) {
    event := auditLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("PHI access event")
    event.AddField("timestamp", time.Now().UTC().Format(time.RFC3339))
    event.AddField("actor_id", actorID)
    event.AddField("patient_id", patientID) // Non-PII identifier
    event.AddField("record_type", recordType)
    event.AddField("access_reason", reason)
    event.AddField("compliance_framework", "HIPAA")
    auditLogger.Write(auditLogger.ReleaseEvent(event))
}
```

### PCI-DSS (Payment Card Industry)

- **Do not log full card numbers:** Only show last 4 digits
- **Do not log CVV/CVC:** Never log card verification codes
- **Do not log PINs:** Never log card PINs

**RECOMMENDED:**
```go
func logPaymentEvent(transactionID, maskedCard, result string) {
    event := auditLogger.NewEvent(logiface.LevelInformational)
    event.AddMessage("payment processed")
    event.AddField("timestamp", time.Now().UTC().Format(time.RFC3339))
    event.AddField("transaction_id", transactionID)
    event.AddField("card_last4", maskedCard[len(maskedCard)-4:])
    event.AddField("result", result)
    event.AddField("compliance_framework", "PCI-DSS")
    event.AddField("pci_level", "PCI-DSS-Level-1")
    auditLogger.Write(auditLogger.ReleaseEvent(event))
}
```

## Security Incident Logging

### Failed Authentication Attempts

```go
func logFailedAuth(username, ip, userAgent string) {
    event := logger.NewEvent(logiface.LevelWarning)
    event.AddMessage("authentication failed")
    event.AddField("username_provided", username != "") // Boolean only
    event.AddField("ip_address", ip)
    event.AddField("user_agent", userAgent)
    event.AddField("blocked", isIPBlocked(ip)) // Security response
    logger.Write(logger.ReleaseEvent(event))
}
```

### Authorization Failures

```go
func logAuthorizationFailure(userID, resource, action string) {
    event := logger.NewEvent(logiface.LevelWarning)
    event.AddMessage("authorization denied")
    event.AddField("actor_id", userID)
    event.AddField("target_resource", resource)
    event.AddField("requested_action", action)
    event.AddField("result", "DENIED")
    event.AddField("reason", "insufficient_privileges")
    logger.Write(logger.ReleaseEvent(event))
}
```

### Anomaly Detection

```go
func logSecurityAnomaly(anomalyType, severity, details map[string]string) {
    event := logger.NewEvent(logiface.LevelWarning)
    event.AddMessage("security anomaly detected")
    event.AddField("anomaly_type", anomalyType)
    event.AddField("severity", severity)
    event.AddField("timestamp", time.Now().UTC().Format(time.RFC3339))

    for key, value := range details {
        event.AddField("detail_"+key, value)
    }

    logger.Write(logger.ReleaseEvent(event))
}
```

## Checklist

- [ ] **PII:** Never log names, emails, phone numbers, addresses, SSNs, credit cards
- [ ] **Credentials:** Use `ReplaceAttr` to redact passwords, API keys, tokens
- [ ] **Audit logs:** Create dedicated audit logger with immutable append-only storage
- [ ] **Audit fields:** Always include timestamp, actor_id, target_id, action, result
- [ ] **Error messages:** Do not include user input in error messages
- [ ] **File permissions:** Set 600 for logs, 200 for audit logs
- [ ] **Log rotation:** Configure with secure compression and backup limits
- [ ] **HTTP requests:** Do not log request bodies, sanitize URL parameters
- [ ] **Compliance:** Follow GDPR/HIPAA/PCI-DSS requirements for regulated data

---

For operational advice, see [BEST_PRACTICES.md](BEST_PRACTICES.md).
