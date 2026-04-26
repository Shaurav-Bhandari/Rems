# AUTH_IMPLEMENTATION.md — ReMS Authentication System

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Stub → Production Changes](#stub--production-changes)
- [Email Service (SMTP)](#email-service-smtp)
- [GeoIP Service](#geoip-service)
- [Password Security & Breach Checking](#password-security--breach-checking)
- [Token System](#token-system)
- [OAuth Setup Guide](#oauth-setup-guide)
- [Security Features](#security-features)
- [How to Modify / Extend](#how-to-modify--extend)
- [Scalability Recommendations](#scalability-recommendations)

---

## Overview

The ReMS auth module is a production-grade authentication system built on:

- **Argon2id** password hashing (memory-hard, side-channel resistant)
- **JWT access tokens** (short-lived, role-based TTL)
- **Opaque refresh tokens** (stored as SHA-256 hashes in PostgreSQL)
- **Redis-backed sessions** with per-user session limits
- **ImmuDB vault** for tamper-proof config (JWT secret, TTLs, rate limits)
- **Google OAuth2** for third-party login
- **SMTP email notifications** for security events
- **GeoIP resolution** for suspicious login detection
- **HaveIBeenPwned** integration for password breach checking

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        HTTP Layer (Fiber v3)                        │
│  handlers/auth.go   handlers/oauth.go                              │
├─────────────────────────────────────────────────────────────────────┤
│                        Middleware                                    │
│  Auth (JWT verify)  RateLimit  RBAC  SecurityHeaders                │
├─────────────────────────────────────────────────────────────────────┤
│                        Service Layer                                │
│  AuthService ─── PasswordService ─── TokenService                   │
│  SessionService ─── SecurityService ─── OAuthService                │
│  EmailService ─── GeoIPService                                      │
├─────────────────────────────────────────────────────────────────────┤
│                        Data / External                               │
│  PostgreSQL   Redis   ImmuDB   Google APIs   ip-api.com   HIBP API  │
└─────────────────────────────────────────────────────────────────────┘
```

### Service Dependency Graph

```
AuthService
├── PasswordService     (validation, hashing, breach check, history)
├── TokenService        (JWT generation, refresh token hashing)
├── SessionService      (Redis session CRUD, per-user limits)
├── SecurityService     (event logging, suspicious login detection)
├── EmailService        (SMTP notifications)
└── GeoIPService        (IP → location resolution)

OAuthService
├── TokenService
├── SessionService
└── GeoIPService
```

---

## Stub → Production Changes

All stubs have been replaced with production implementations:

| Stub | Was | Now | File |
|------|-----|-----|------|
| `EmailService.SendVerificationEmail` | Empty `{}` | SMTP with HTML template | `services/core/email.go` |
| `EmailService.SendPasswordChangedNotification` | Empty `{}` | SMTP with HTML template | `services/core/email.go` |
| `EmailService.SendAccountLockedNotification` | Empty `{}` | SMTP with HTML template | `services/core/email.go` |
| `EmailService.SendSuspiciousLoginAlert` | Empty `{}` | SMTP with HTML template | `services/core/email.go` |
| `GeoIPService.Lookup` | Returns `"Unknown Location"` | ip-api.com with caching | `services/core/geoip.go` |
| `GeoIPService.GetCountry` | Returns `"Unknown"` | ip-api.com with caching | `services/core/geoip.go` |
| `GeoIPService.GetCoordinates` | Returns `0.0, 0.0` | ip-api.com with caching | `services/core/geoip.go` |
| `PasswordService.CheckPasswordBreach` | Returns `false, nil` | HaveIBeenPwned k-anonymity | `services/core/auth.go` |
| `AuthService.ResetPassword` | Did not exist | Full implementation | `services/core/auth.go` |

---

## Email Service (SMTP)

### Location
`services/core/email.go`

### Design
- **Interface-based**: `EmailSender` interface with 4 methods
- **Graceful degradation**: When SMTP is not configured, all emails are logged to stdout
- **HTML templates**: Inline templates with responsive design, dark-mode compatible
- **TLS support**: Automatic TLS for port 465, STARTTLS for port 587

### Configuration

Set these environment variables (or add to `.env`):

```env
SMTP_HOST=smtp.gmail.com          # SMTP server hostname
SMTP_PORT=587                     # 587 (STARTTLS) or 465 (TLS)
SMTP_USERNAME=your@gmail.com      # SMTP login
SMTP_PASSWORD=your-app-password   # SMTP password or app-specific password
SMTP_FROM=noreply@yourapp.com     # From address
SMTP_FROM_NAME=ReMS               # From display name
APP_BASE_URL=http://localhost:8080 # Base URL for email links
```

### Email Templates

| Template | Triggered By | Content |
|----------|-------------|---------|
| `templateVerifyEmail` | User registration | Verification link, 24h expiry |
| `templatePasswordChanged` | Password change/reset | Timestamp, IP, "wasn't you?" alert |
| `templateAccountLocked` | Too many failed logins | Lock duration, unlock time |
| `templateSuspiciousLogin` | Login from new IP/location | IP, location, timestamp, action steps |

### Swapping Providers

To use SendGrid, AWS SES, or another provider:

1. Create a new struct implementing `EmailSender`
2. Replace `NewEmailService(smtpCfg)` in `main.go` with your implementation
3. No other changes needed — the interface is the contract

```go
// Example: SendGrid implementation
type SendGridEmailService struct { apiKey string }
func (s *SendGridEmailService) SendVerificationEmail(email, name, token string) { /* ... */ }
// ... implement all 4 methods
```

---

## GeoIP Service

### Location
`services/core/geoip.go`

### Design
- **Provider**: [ip-api.com](http://ip-api.com) (free tier: 45 req/min, no API key)
- **In-memory cache**: 10,000 entries, 1-hour TTL, 25% eviction on overflow
- **Private IP detection**: RFC 1918, loopback, link-local, IPv6 private ranges
- **Fail-open**: API errors return `nil` — never blocks auth flow

### Methods

```go
Lookup(ip string) string                    // "San Francisco, California, US"
GetCountry(ip string) string                // "US"
GetCoordinates(ip string) (float64, float64) // 37.7749, -122.4194
```

### Swapping Providers

To use MaxMind GeoIP2:

1. Download the GeoLite2-City database from [MaxMind](https://www.maxmind.com/)
2. Replace `fetchFromAPI()` in `geoip.go` with a local database lookup:

```go
import "github.com/oschwald/geoip2-golang"

func (g *GeoIPService) fetchFromAPI(ip string) (*GeoIPResult, error) {
    db, _ := geoip2.Open("/path/to/GeoLite2-City.mmdb")
    defer db.Close()
    record, _ := db.City(net.ParseIP(ip))
    return &GeoIPResult{
        City:        record.City.Names["en"],
        Country:     record.Country.Names["en"],
        CountryCode: record.Country.IsoCode,
        Latitude:    record.Location.Latitude,
        Longitude:   record.Location.Longitude,
        Status:      "success",
    }, nil
}
```

---

## Password Security & Breach Checking

### Hashing: Argon2id

```
Algorithm:   argon2id (memory-hard, resistant to GPU/ASIC attacks)
Memory:      64 MB
Iterations:  3
Parallelism: 2
Salt:        16 bytes (crypto/rand)
Key Length:  32 bytes
Format:      $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
```

### Breach Check: HaveIBeenPwned k-Anonymity

**How it works** (user's password never leaves the server):

1. SHA-1 hash the password: `5BAA6...` → 40 hex chars
2. Send only the **first 5 characters** to HIBP: `GET /range/5BAA6`
3. HIBP returns ~500 suffixes matching that prefix
4. Check locally if any suffix matches the remaining 35 characters
5. If match found → password appeared in a data breach

**Configuration**:
```env
HIBP_ENABLED=true   # Set to "false" to disable (e.g., in air-gapped environments)
```

**Behavior**:
- Fail-open: If HIBP API is unreachable, the check is skipped (never blocks users)
- Padding enabled: `Add-Padding: true` header prevents response-length side-channel

### Password Policy

| Rule | Default |
|------|---------|
| Minimum length | 12 characters |
| Uppercase required | Yes |
| Lowercase required | Yes |
| Number required | Yes |
| Special character required | Yes |
| Password history | Last 5 passwords checked |
| Breach check | Enabled (HIBP k-anonymity) |

---

## Token System

### Access Tokens (JWT)

```
Type:        JWT (HS256)
Payload:     user_id, tenant_id, session_id, role, exp, iat, iss, aud
Lifetime:    Role-dependent (15min–7days, configured in ImmuDB)
Storage:     Client-side only (Authorization: Bearer header)
Validation:  Middleware checks signature + expiry + Redis session existence
```

**Role-based TTLs** (configured in ImmuDB vault):

| Role | Access Token | Refresh Token |
|------|-------------|---------------|
| admin | 15 min | 2 hours |
| manager | 20 min | 4 hours |
| cashier | 2 hours | 8 hours |
| chef | 8 hours | 24 hours |
| waiter | 4 hours | 12 hours |
| customer | 7 days | 90 days |

### Refresh Tokens (Opaque)

```
Type:        64-byte crypto/rand, base64-encoded
Storage:     SHA-256 hash stored in PostgreSQL (refresh_tokens table)
Rotation:    Old token revoked + new token issued on each refresh
Revocation:  By token ID, by user (all), by device
Detection:   Reuse of revoked token → revoke entire token family
```

### Token Rotation Flow

```
Client                     Server
  |                          |
  |── POST /auth/refresh ──→ |
  |   { refresh_token }      |
  |                          |── Validate hash against DB
  |                          |── Check not revoked, not expired
  |                          |── Mark old token as rotated
  |                          |── Generate new access + refresh tokens
  |                          |── Store new refresh token hash
  |←── { new tokens } ──────|
```

### Session Management

- Sessions stored in Redis with user-scoped keys: `session:{userId}:{sessionId}`
- Max 5 concurrent sessions per user (configurable in ImmuDB)
- Oldest session evicted when limit exceeded
- Session tracks: IP, user agent, device type, login time
- All sessions revoked on password change/reset

---

## OAuth Setup Guide

### Prerequisites

1. A Google Cloud project with OAuth 2.0 credentials
2. The callback URL added as an authorized redirect URI

### Step-by-Step Setup

1. Go to [Google Cloud Console → APIs & Services → Credentials](https://console.cloud.google.com/apis/credentials)
2. Click **Create Credentials → OAuth 2.0 Client IDs**
3. Select **Web application**
4. Add authorized redirect URI:
   - Development: `http://localhost:8080/api/v1/auth/google/callback`
   - Production: `https://yourdomain.com/api/v1/auth/google/callback`
5. Copy the Client ID and Client Secret

### Environment Configuration

```env
GOOGLE_CLIENT_ID=123456789-abcdef.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-your-secret-here
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
```

### Auth Code Flow

```
 Browser                    Backend                         Google
   |                          |                               |
   |── GET /auth/google ────→ |                               |
   |                          |── Generate CSRF state token    |
   |                          |── Store state in Redis (10min) |
   |←── 307 redirect ────────|                               |
   |                          |                               |
   |──────────── Consent screen ────────────────────────────→ |
   |←──────────── Redirect with ?code=...&state=... ─────────|
   |                          |                               |
   |── GET /callback ────────→|                               |
   |                          |── Validate state against Redis |
   |                          |── Exchange code for token ───→ |
   |                          |←── Access token ──────────────|
   |                          |── Fetch userinfo ────────────→ |
   |                          |←── Email, name, picture ──────|
   |                          |── Find/create user in DB       |
   |                          |── Issue JWT access + refresh   |
   |←── { tokens, user } ────|                               |
```

### User Resolution Logic

1. **Find by `google_id`** → User already linked → login
2. **Find by `email`** → User exists, link Google account → login
3. **No match** → Create new user + tenant → login

New OAuth users are created with:
- `password_hash: ""` (OAuth-only, no password login)
- `is_email_verified: true` (Google verified)
- `primary_role: "owner"` (gets their own tenant)

### Token Handling

OAuth users receive the same JWT tokens as password-authenticated users:
- Access token (role-based TTL from ImmuDB)
- Refresh token (stored in PostgreSQL)
- Redis session created

---

## Security Features

### Already Implemented

| Feature | Location | Details |
|---------|----------|---------|
| Rate limiting (login) | `SecurityService.CheckIPRateLimit` | 10 attempts per 60s window (configurable) |
| Account locking | `AuthService.handleFailedLogin` | Escalating lockouts: 15min → 30min → 1hr → 24hr |
| Suspicious login detection | `AuthService.Login` | New device/IP triggers alert email |
| Security event logging | `SecurityService.LogSecurityEvent` | All auth events logged with IP, UA, device, location |
| Device registry | `DeviceRegistry` model | Track trusted devices per user |
| CSRF protection (OAuth) | `OAuthHandler.GoogleLogin` | Random state token stored in Redis |
| Password breach check | `PasswordService.CheckPasswordBreach` | HIBP k-anonymity API |
| Password history | `PasswordService.CheckPasswordHistory` | Last N passwords blocked |
| Token rotation | `AuthService.RefreshAccessToken` | Old refresh token revoked on use |
| Session limits | `SessionService` | Max N concurrent sessions per user |
| Argon2id hashing | `HashPwd` / `ComparePwd` | Memory-hard, 64MB, 3 iterations |
| 2FA (TOTP) | `AuthService.Verify2FA` | Google Authenticator compatible |

### Event Types Logged

```
login_success, login_failed, logout, password_changed,
password_reset, account_locked, suspicious_login,
session_revoked, token_refreshed, 2fa_verified
```

---

## How to Modify / Extend

### Add a new email notification

1. Add a new template constant in `email.go`
2. Add a method to `EmailSender` interface
3. Implement the method on `EmailService`
4. Call it from the relevant service

### Change the GeoIP provider

1. Replace the `fetchFromAPI()` method body in `geoip.go`
2. The cache and private IP detection remain unchanged

### Add a new OAuth provider (e.g., GitHub)

1. Add `GithubOAuth2Config()` to `config/oauth.go`
2. Add `HandleGithubCallback()` to `services/core/oauth.go`
3. Add handler methods in `handlers/oauth.go`
4. Register routes in `routes/routes.go`

### Modify password policy

Update the ImmuDB vault config:
```
auth.password_min_length    → Minimum characters
auth.password_history_count → Number of old passwords to check
```

Or change `PasswordService.ValidatePasswordStrength()` for custom rules.

### Add API key authentication

1. Uncomment the `APIKey` model in `models/user.go`
2. Create `services/core/apikey.go` with generation/validation
3. Add middleware to check `X-API-Key` header
4. Register routes in `routes.go`

---

## Scalability Recommendations

### Short-term (current stage)

- ✅ Connection pooling (GORM + Redis already configured)
- ✅ Redis-backed sessions (horizontally scalable)
- ✅ GeoIP caching (reduces external API calls)

### Medium-term (10k+ users)

- **Move email sending to a queue**: Replace direct SMTP calls with a Redis/RabbitMQ queue + worker
- **Use MaxMind local DB**: Replace ip-api.com HTTP calls with a local `.mmdb` file (~100x faster)
- **Add Redis cluster**: For session storage across multiple backend instances
- **Implement token blacklisting**: Use Redis SET for revoked JWTs instead of checking DB on every request

### Long-term (100k+ users)

- **Dedicated auth microservice**: Extract auth into its own service with gRPC API
- **Event-driven architecture**: Publish auth events to Kafka/NATS for downstream consumers
- **Hardware security modules**: Store JWT signing keys in AWS CloudHSM or HashiCorp Vault
- **WebAuthn/Passkeys**: Add FIDO2 support alongside password + OAuth
- **Distributed rate limiting**: Move from per-instance to Redis-based sliding window
