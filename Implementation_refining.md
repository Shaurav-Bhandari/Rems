# Implementation Refining — Process Documentation

Detailed record of how the implementation plan was developed: research, findings, decisions.

---

## Phase 1: Error Audit

### Backend (Go)
**Method:** `go build ./...` in `c:\projects\ReMS\backend`

**Result: 3 compilation errors**, all in [handlers/auth.go](file:///c:/projects/ReMS/backend/handlers/auth.go):

| # | Line | Error | Root Cause |
|---|---|---|---|
| 1 | 157 | `h.authService.RefreshToken undefined` | Handler calls `RefreshToken()` but service method is `RefreshAccessToken()`. Also, handler passes `(*DTO.RefreshTokenRequest, ip, ua, fingerprint)` but service expects `(refreshTokenString, ip, ua)` |
| 2 | 194 | `not enough arguments in call to ChangePassword` | Handler passes `(ctx, userID, *DTO.ChangePasswordRequest, ip, ua)` but service expects `(ctx, userID, oldPassword, newPassword, ip, ua)` |
| 3 | 282 | `h.authService.ResetPassword undefined` | `ResetPassword` method does not exist on `AuthService` — needs to be created |

### Frontend (SvelteKit)
**Method:** `npx svelte-check --threshold warning` in `c:\projects\ReMS\frontend`

**Result: Clean** — zero errors, zero warnings.

---

## Phase 2: Stub Audit

Searched the entire backend with multiple patterns:
- Empty function bodies: `func.*\{\s*\}$`
- Hardcoded return values: `return "Unknown`, `return false, nil`, `return 0.0`
- TODO/FIXME/STUB markers: `TODO|FIXME|HACK|STUB|placeholder|not implemented`

### Findings

| Stub | File | Impact | Action |
|---|---|---|---|
| **EmailService** (4 no-ops) | `services/core/auth.go:757-762` | All email notifications silently drop | **Replace now** |
| **GeoIPService** (3 hardcoded) | `services/core/auth.go:768-780` | All locations = "Unknown" | Defer (needs MaxMind DB) |
| **CheckPasswordBreach** | `services/core/auth.go:427-433` | Breached passwords never detected | Defer (needs HIBP API) |
| **InventoryNotifier** (no-op) | `services/business/Order.go:269-280` | Stock never deducted on order | Defer (needs DB migration) |
| RBAC `maxRolesPerUser` | `services/core/rbac.go:440` | Config hardcoded to 10 | Low priority |
| DTO validation | `DTO/restaurant.go:340` | Empty validation method | Low priority |

Full details in [stubs.md](file:///C:/Users/wannaquit/.gemini/antigravity/brain/65ab0360-309b-4224-a77b-8a059a21fbb4/stubs.md).

---

## Phase 3: Printer Research

### User Input
- **Model:** Winpal WP230W
- **Requirements:** Print from both wired (USB) connection and over the internet (network)

### WP230W Specifications (from web research)
- **Paper width:** 80mm (48 printable characters per line)
- **Protocol:** ESC/POS compatible
- **Connectivity:** USB + WiFi (2.4GHz)
- **Network port:** TCP 9100 (standard raw printing port)
- **Auto-cutter:** Yes (partial/full cut supported)

### Technology Decision
**Direct ESC/POS over raw TCP socket** — no external library needed.

Rationale:
- ESC/POS is a simple byte-level protocol; a small set of command constants handles all formatting
- Go's `net.Dial("tcp", "ip:9100")` provides the network connection
- Go's `os.OpenFile` provides USB device file access
- Libraries like `hennedo/escpos` wrap this but add unnecessary dependencies for our simple receipt layout
- Building our own gives full control over Nepali tax formatting and currency display

---

## Phase 4: Email Service Research

### Technology Decision
**`wneessen/go-mail`** — actively maintained Go email library.

Rationale:
- Original `go-gomail/gomail` is unmaintained (author inactive)
- `wneessen/go-mail` is the recommended modern replacement
- Supports: TLS/STARTTLS, attachments, HTML templates, context support, connection pooling
- Used in production by Shopify and others
- Simple API: `mail.NewMsg()` → set headers → set body → `client.DialAndSend()`

### Email Config Design
```
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=noreply@yourdomain.com
SMTP_TLS=true
```

Supports Gmail (app passwords), SendGrid, Mailgun, Amazon SES, or any standard SMTP server.

---

## Phase 5: Final Plan

### Execution Order
1. **Fix 3 compile errors** (handlers/auth.go + add ResetPassword to AuthService)
2. **Add email service** (new `services/core/email.go`, config, update main.go)
3. **Add receipt printing** (new `services/printing/` package, handler, routes, config)
4. **Verify** (`go build ./...` must pass)

### Files to Create
| File | Purpose |
|---|---|
| `services/core/email.go` | Real EmailService with SMTP sending + HTML templates |
| `config/email_config.go` | Email config loader |
| `services/printing/printer.go` | PrinterService with USB/network support |
| `services/printing/receipt.go` | Receipt struct + formatting |
| `services/printing/escpos.go` | ESC/POS command constants |
| `handlers/receipt.go` | HTTP handler for print endpoints |
| `config/printer_config.go` | Printer config loader |

### Files to Modify
| File | Changes |
|---|---|
| `handlers/auth.go` | Fix 3 method call mismatches |
| `services/core/auth.go` | Remove EmailService stub, add ResetPassword method |
| `main.go` | Wire email service + printer service |
| `routes/routes.go` | Add printer routes + PrinterService to Dependencies |
| `.env` | Add SMTP_* and PRINTER_* variables |
