# ReMS — Restaurant Management System

A full-stack Restaurant Management System backend built with **Go (Fiber v3)**, **PostgreSQL**, **Redis**, **ImmuDB**, and a comprehensive RBAC authorization engine.

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Tech Stack](#tech-stack)
- [Docker Setup](#docker-setup)
- [Google OAuth Integration](#google-oauth-integration)
- [API Routes](#api-routes)
- [Stub Functions & Pending Implementations](#stub-functions--pending-implementations)
- [Configuration](#configuration)
- [Development](#development)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Fiber v3 HTTP Layer                       │
│  handlers/ ── routes/ ── middleware/ (Auth, RBAC, Rate Limit)    │
├─────────────────────────────────────────────────────────────────┤
│                        Service Layer                             │
│  services/core/     ── Auth, RBAC, OAuth, Security, Sessions     │
│  services/business/ ── Orders, Menu, Taxes, Settings             │
│  services/inventory/── Inventory, Vendors, Purchase Orders       │
│  services/People/   ── HR & People Management                    │
├─────────────────────────────────────────────────────────────────┤
│                        Data Layer                                │
│  PostgreSQL (primary)  Redis (cache/sessions)  ImmuDB (vault)    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Tech Stack

| Component     | Technology              |
|---------------|-------------------------|
| Web Framework | Go Fiber v3             |
| Database      | PostgreSQL 18           |
| ORM           | GORM                    |
| Cache/Sessions| Redis 7.2               |
| Config Vault  | ImmuDB (immutable)      |
| Auth          | JWT + Argon2id + TOTP   |
| OAuth         | Google OAuth2           |
| Password Hash | Argon2id                |
| Containerization | Docker + Docker Compose |

---

## Docker Setup

All services are fully containerized. The `docker-compose.yml` runs:

| Service    | Container Name | Internal Host | External Port |
|------------|---------------|---------------|---------------|
| PostgreSQL | rms_postgres  | `postgres`    | 5432          |
| Redis      | rms_redis     | `redis`       | 6379          |
| ImmuDB     | rms_immudb    | `immudb`      | 3322 (gRPC), 9090 (Web UI) |
| Go Backend | rms_backend   | `backend`     | 8080          |

### Quick Start

```bash
# 1. Copy environment template
cp .env.docker .env

# 2. (Optional) Set your Google OAuth credentials in .env
#    GOOGLE_CLIENT_ID=your-client-id
#    GOOGLE_CLIENT_SECRET=your-client-secret

# 3. Build and start all services
docker-compose build
docker-compose up -d

# 4. Check health
docker-compose ps
curl http://localhost:8080/health
```

### Environment Variables

All environment variables are defined in `.env.docker` and injected via `docker-compose.yml`:

| Variable             | Default                          | Description                      |
|----------------------|----------------------------------|----------------------------------|
| `DB_HOST`            | `postgres`                       | PostgreSQL container hostname    |
| `DB_PORT`            | `5432`                           | PostgreSQL port                  |
| `DB_USER`            | `postgres`                       | PostgreSQL username              |
| `DB_PASSWORD`        | `root`                           | PostgreSQL password              |
| `DB_NAME`            | `restaurant_management_system`   | Database name                    |
| `DB_SSLMODE`         | `disable`                        | SSL mode for PostgreSQL          |
| `REDIS_HOST`         | `redis`                          | Redis container hostname         |
| `REDIS_PORT`         | `6379`                           | Redis port                       |
| `REDIS_PASSWORD`     | `roote`                          | Redis password                   |
| `IMMUDB_HOST`        | `immudb`                         | ImmuDB container hostname        |
| `IMMUDB_PORT`        | `3322`                           | ImmuDB gRPC port                 |
| `IMMUDB_PASSWORD`    | `rooter`                         | ImmuDB admin password            |
| `JWT_SECRET`         | `CHANGE_ME_IN_PRODUCTION`        | JWT signing key                  |
| `GOOGLE_CLIENT_ID`   | *(empty)*                        | Google OAuth client ID           |
| `GOOGLE_CLIENT_SECRET` | *(empty)*                      | Google OAuth client secret       |
| `GOOGLE_REDIRECT_URL`| `http://localhost:8080/api/v1/auth/google/callback` | OAuth redirect URI |
| `PORT`               | `8080`                           | Backend server port              |

> ⚠️ **IMPORTANT**: Change all default passwords and set a strong `JWT_SECRET` before deploying to production.

For full Docker documentation, see [DOCKER.md](DOCKER.md).

---

## Google OAuth Integration

Google OAuth2 is integrated for authentication. When credentials are configured, two additional endpoints become available:

| Method | Endpoint                             | Description                     |
|--------|--------------------------------------|---------------------------------|
| GET    | `/api/v1/auth/google`                | Redirects to Google consent screen |
| GET    | `/api/v1/auth/google/callback`       | Handles callback, returns JWT tokens |

### How It Works

1. Client redirects user to `GET /api/v1/auth/google`
2. Backend generates a CSRF state token (stored in Redis), redirects to Google
3. User consents on Google's page
4. Google redirects to `/api/v1/auth/google/callback?code=...&state=...`
5. Backend validates state token, exchanges code for Google access token
6. Backend fetches user profile from Google's userinfo API
7. User is found-or-created in the database:
   - If a user with matching `google_id` exists → login
   - If a user with matching `email` exists → link Google account, then login
   - If no user exists → create new user + tenant, then login
8. JWT access + refresh tokens are issued

### Setup

1. Create credentials at [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Add `http://localhost:8080/api/v1/auth/google/callback` as an authorized redirect URI
3. Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` in your `.env` file

### Files

| File | Purpose |
|------|---------|
| `config/oauth.go` | OAuth configuration loader |
| `DTO/oauth.go` | Google userinfo response DTO |
| `services/core/oauth.go` | OAuth service (code exchange, user resolution, token issuance) |
| `handlers/oauth.go` | HTTP handler (redirect + callback) |

---

## API Routes

### Public Endpoints
| Method | Path | Handler |
|--------|------|---------|
| GET | `/health` | `HealthCheck` |
| GET | `/health/detailed` | `DetailedHealthCheck` |
| POST | `/api/v1/auth/login` | `AuthHandler.Login` |
| POST | `/api/v1/auth/register` | `AuthHandler.Register` |
| POST | `/api/v1/auth/refresh` | `AuthHandler.RefreshToken` |
| POST | `/api/v1/auth/forgot-password` | `AuthHandler.ForgotPassword` |
| POST | `/api/v1/auth/reset-password` | `AuthHandler.ResetPassword` |
| POST | `/api/v1/auth/verify-2fa` | `AuthHandler.Verify2FA` |
| GET | `/api/v1/auth/google` | `OAuthHandler.GoogleLogin` |
| GET | `/api/v1/auth/google/callback` | `OAuthHandler.GoogleCallback` |

### Authenticated Endpoints
| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/v1/auth/logout` | `AuthHandler.Logout` |
| POST | `/api/v1/auth/change-password` | `AuthHandler.ChangePassword` |
| GET | `/api/v1/auth/sessions` | `AuthHandler.ListSessions` |
| POST | `/api/v1/auth/sessions/revoke` | `AuthHandler.RevokeSession` |
| POST | `/api/v1/auth/sessions/revoke-all` | `AuthHandler.RevokeAllSessions` |
| POST | `/api/v1/auth/verify-email` | `AuthHandler.VerifyEmail` |

### Resource Endpoints (Auth + RBAC Required)
| Group | Endpoints |
|-------|-----------|
| Restaurants | CRUD at `/api/v1/restaurants` |
| Menu | CRUD at `/api/v1/menu` |
| Orders | CRUD at `/api/v1/orders` |
| Inventory | CRUD at `/api/v1/inventory` |
| Users | CRUD at `/api/v1/users` |
| Analytics | GET at `/api/v1/analytics/*` |

---

## Stub Functions & Pending Implementations

The following functions are **currently stubs** or have **placeholder implementations** and need to be completed before production:

### 🔴 Empty Stub Functions (No-Op)

These functions have completely empty bodies `{}` and do nothing:

| # | File | Function | Signature | Purpose |
|---|------|----------|-----------|---------|
| 1 | `services/core/auth.go:759` | `EmailService.SendVerificationEmail` | `(email, name, token string)` | Send email verification link to new users |
| 2 | `services/core/auth.go:760` | `EmailService.SendPasswordChangedNotification` | `(email, name, ip string)` | Notify user their password was changed |
| 3 | `services/core/auth.go:761` | `EmailService.SendAccountLockedNotification` | `(email, name string, until time.Time)` | Notify user their account is locked |
| 4 | `services/core/auth.go:762` | `EmailService.SendSuspiciousLoginAlert` | `(email, name, ip, location string)` | Alert user of suspicious login from new location |

**Recommended integration**: SendGrid, AWS SES, or Resend.

---

### 🟡 Placeholder Implementations (Return Dummy Data)

These functions return hardcoded dummy values instead of real data:

| # | File | Function | Returns | Purpose |
|---|------|----------|---------|---------|
| 5 | `services/core/auth.go:770` | `GeoIPService.Lookup` | `"Unknown Location"` | Resolve IP address to city/region string |
| 6 | `services/core/auth.go:774` | `GeoIPService.GetCountry` | `"Unknown"` | Resolve IP address to country code |
| 7 | `services/core/auth.go:778` | `GeoIPService.GetCoordinates` | `0.0, 0.0` | Resolve IP address to lat/lng coordinates |

**Recommended integration**: MaxMind GeoIP2 database or ip-api.com.

---

### 🟡 Partial Implementations (Logic Present but Incomplete)

| # | File | Function | Current Behavior | What's Missing |
|---|------|----------|-----------------|----------------|
| 8 | `services/core/auth.go:428` | `PasswordService.CheckPasswordBreach` | Computes SHA-256 hash but always returns `false, nil` | Integration with [HaveIBeenPwned](https://haveibeenpwned.com/API/v3) k-Anonymity API |
| 9 | `services/business/Order.go:277` | `InventoryNotifier.Notify` | No-op `return nil` | Requires `menu_item_inventory_links` table to be built; replacement implementation exists in `services/inventory/inventoryNotif.go` |

---

### 🔵 TODOs in Code

| # | File | Line | TODO |
|---|------|------|------|
| 10 | `services/core/rbac.go:440` | L440 | `maxRolesPerUser: 10` — move to ImmuDB vault config |
| 11 | `DTO/restaurant.go:340` | L340 | Custom validation logic placeholder |

---

### Summary

| Category | Count | Priority |
|----------|-------|----------|
| 🔴 Empty stubs (no-op) | 4 | **High** — Email notifications are critical for security |
| 🟡 Placeholder returns | 3 | **Medium** — GeoIP enhances security logging |
| 🟡 Partial implementation | 2 | **Medium** — Password breach check + inventory deduction |
| 🔵 Config TODOs | 2 | **Low** — Move hardcoded values to ImmuDB vault |
| **Total** | **11** | |

---

## Configuration

### ImmuDB Vault

Application configuration is stored in ImmuDB (immutable, tamper-proof). The vault is seeded with defaults on first startup. Key config categories:

- **Auth**: `auth.max_sessions`, `auth.max_failed_logins`, `auth.lockout_duration_minutes`, etc.
- **JWT**: `jwt.secret`, `jwt.issuer`, `jwt.audience`
- **TTL**: Role-based and device-based token lifetimes
- **CORS**: `cors.allowed_origins`
- **App**: `app.port`

See `config/immudb_vault.go` for the full seeder and loader.

---

## Development

### Local Development (without Docker)

```bash
cd backend

# Set up .env with local DB/Redis/ImmuDB connection details
cp .env.example .env

# Run with hot reload
air
```

### Running with Docker

```bash
# Start all services
docker-compose up -d

# View backend logs
docker-compose logs -f backend

# Rebuild backend after code changes
docker-compose build --no-cache backend && docker-compose up -d backend

# Stop everything
docker-compose down

# Stop and delete all data
docker-compose down -v
```

### Project Structure

```
backend/
├── config/          # Configuration loaders (DB, Redis, ImmuDB, OAuth)
├── DB/              # Database initialization, scopes, migrations
├── DTO/             # Data Transfer Objects (request/response types)
├── handlers/        # HTTP handlers (auth, oauth, restaurant, menu, etc.)
├── middleware/       # Auth, RBAC, rate limiting, logging, recovery
├── models/          # GORM models (User, Tenant, Order, Menu, etc.)
├── routes/          # Route registration and dependency injection
├── services/
│   ├── core/        # Auth, RBAC, OAuth, Security, Token, Session services
│   ├── business/    # Order, Menu, Tax, Settings services
│   ├── inventory/   # Inventory, Vendor, Purchase Order services
│   └── People/      # HR/People services
├── utils/           # Shared utilities, FSM engine, API response helpers
├── Dockerfile       # Multi-stage Docker build
├── .air.toml        # Hot reload config
├── go.mod / go.sum  # Go module dependencies
└── main.go          # Application entry point
```
