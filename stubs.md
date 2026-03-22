# Backend Stub/Placeholder Audit

All stub structs, no-op functions, hardcoded returns, and TODO markers found in the ReMS backend.

---

## 1. EmailService (Stub — 4 no-op methods)

**File:** [auth.go](file:///c:/projects/ReMS/backend/services/core/auth.go#L757-L762)

```go
type EmailService struct{}

func (e *EmailService) SendVerificationEmail(email, name, token string)                   {}
func (e *EmailService) SendPasswordChangedNotification(email, name, ip string)            {}
func (e *EmailService) SendAccountLockedNotification(email, name string, until time.Time) {}
func (e *EmailService) SendSuspiciousLoginAlert(email, name, ip, location string)         {}
```

**Impact:** All email notifications silently do nothing. Verification emails, password change alerts, account lockout notices, and suspicious login alerts are never sent.

**Status:** 🔧 Being replaced in this task.

---

## 2. GeoIPService (Stub — 3 hardcoded methods)

**File:** [auth.go](file:///c:/projects/ReMS/backend/services/core/auth.go#L768-L780)

```go
type GeoIPService struct{}

func (g *GeoIPService) Lookup(ip string) string              { return "Unknown Location" }
func (g *GeoIPService) GetCountry(ip string) string          { return "Unknown" }
func (g *GeoIPService) GetCoordinates(ip string) (float64, float64) { return 0.0, 0.0 }
```

**Impact:** All login events log "Unknown Location" and "Unknown" country. Risk scoring based on location will be inaccurate. Suspicious login detection (geo-anomaly) is non-functional.

**Status:** ⏳ Not addressed in this task — requires a GeoIP database (MaxMind GeoLite2 or similar).

---

## 3. CheckPasswordBreach (Stub — returns false always)

**File:** [auth.go](file:///c:/projects/ReMS/backend/services/core/auth.go#L427-L433)

```go
// IMPROVEMENT #32: Password breach check stub
func (s *PasswordService) CheckPasswordBreach(password string) (bool, error) {
    // Stub - implement HaveIBeenPwned API
    hash := sha256.Sum256([]byte(password))
    _ = fmt.Sprintf("%X", hash)
    return false, nil
}
```

**Impact:** Breached/leaked passwords are never detected. Users can register or change to passwords that have appeared in data breaches. The SHA256 hash is computed but discarded — the HaveIBeenPwned k-anonymity API call is not implemented.

**Status:** ⏳ Not addressed in this task — requires HIBP API integration.

---

## 4. InventoryNotifier in Order.go (Stub — no-op)

**File:** [Order.go](file:///c:/projects/ReMS/backend/services/business/Order.go#L269-L280)

```go
// InventoryNotifier deducts stock after an order is placed.
// No-op until menu_item_inventory_links table is built.
type InventoryNotifier struct{ db *gorm.DB }

func (n *InventoryNotifier) Notify(_ context.Context, _ models.OrderCreatedPayload) error {
    // TODO: implement once menu_item_inventory_links exists in menu.go
    return nil
}
```

**Impact:** Inventory stock is never deducted when orders are placed. Stock levels will be inaccurate.

> [!NOTE]
> A real implementation exists in [inventoryNotif.go](file:///c:/projects/ReMS/backend/services/inventory/inventoryNotif.go) but it's in the `inventory` package and not yet wired in. The stub in [Order.go](file:///c:/projects/ReMS/backend/services/business/Order.go) needs to be removed and replaced with the real one once `menu_item_inventory_links` table is created.

**Status:** ⏳ Not addressed in this task — requires DB migration for `menu_item_inventory_links`.

---

## 5. TODOs and Incomplete Items

| Location | Line | Note |
|---|---|---|
| [rbac.go](file:///c:/projects/ReMS/backend/services/core/rbac.go#L440) | 440 | `maxRolesPerUser: 10, // TODO: move to immudb config` |
| [Order.go](file:///c:/projects/ReMS/backend/services/business/Order.go#L278) | 278 | `// TODO: implement once menu_item_inventory_links exists` |
| [restaurant.go](file:///c:/projects/ReMS/backend/DTO/restaurant.go#L340) | 340 | `// TODO: Add custom validation logic here if needed` |

---

## Summary

| Stub | Type | Severity | This Task? |
|---|---|---|---|
| [EmailService](file:///c:/projects/ReMS/backend/services/core/auth.go#757-758) | 4 no-op functions | 🔴 High | ✅ Yes |
| [GeoIPService](file:///c:/projects/ReMS/backend/services/core/auth.go#768-769) | 3 hardcoded returns | 🟡 Medium | ❌ No |
| [CheckPasswordBreach](file:///c:/projects/ReMS/backend/services/core/auth.go#427-434) | Always returns `false` | 🟡 Medium | ❌ No |
| [InventoryNotifier](file:///c:/projects/ReMS/backend/services/business/Order.go#271-272) (Order.go) | No-op [Notify](file:///c:/projects/ReMS/backend/services/business/Order.go#217-268) | 🟡 Medium | ❌ No |
| RBAC `maxRolesPerUser` | Hardcoded `10` | 🟢 Low | ❌ No |
| DTO validation | Empty validation method | 🟢 Low | ❌ No |
