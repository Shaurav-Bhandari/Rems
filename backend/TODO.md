# TODO: Migrate Hardcoded Config to immudb

These values are **intentionally hardcoded for now** but must be moved to immudb before production.

---

## Hardcoded Values to Migrate

### 1. `NewPasswordService` call — line 826

```go
passwordService := NewPasswordService(db, 12, 5)
```

- `12` → minimum password length
- `5` → password history count

---

### 2. `AuthService` struct initialization — lines 842–846

```go
maxSessions:       5,
maxFailedLogins:   5,
lockoutDuration:   30 * time.Minute,
passwordMinLength: 12,
passwordHistory:   5,
```

All of these belong in a config struct sourced from immudb at startup.

---

### 3. IP rate limit — line 961

```go
if err := s.securityService.CheckIPRateLimit(ctx, ipAddress, 10, time.Minute); err != nil {
```

- `10` → max requests
- `time.Minute` → window duration

---

## Migration Plan

1. Create an `AuthConfig` struct
2. On app startup, read config values from immudb
3. Pass the populated `AuthConfig` into `NewAuthService`
4. Remove all magic numbers from service code

```go
type AuthConfig struct {
    MaxSessions       int
    MaxFailedLogins   int
    LockoutDuration   time.Duration
    PasswordMinLength int
    PasswordHistory   int
    RateLimitCount    int
    RateLimitWindow   time.Duration
}
```

---

> **File:** `services/auth_service_integrated.go`  
> **Status:** Hardcoded values left in place temporarily — do not forget to migrate before prod.