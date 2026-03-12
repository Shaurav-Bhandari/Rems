package middleware

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// ============================================================================
// SECURITY HEADERS & IP FINGERPRINTING MIDDLEWARE
// Sets OWASP-recommended security response headers and provides utilities
// for extracting the real client IP (behind proxies) and computing a
// client fingerprint hash.
// ============================================================================

// SecurityHeadersConfig configures the security headers middleware.
type SecurityHeadersConfig struct {
	// ContentSecurityPolicy directive.
	// Default: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'"
	ContentSecurityPolicy string

	// ReferrerPolicy directive. Default: "strict-origin-when-cross-origin"
	ReferrerPolicy string

	// PermissionsPolicy directive. Default: restrictive set
	PermissionsPolicy string

	// HSTSMaxAge in seconds. Default: 31536000 (1 year).
	// Set to 0 to disable HSTS.
	HSTSMaxAge int

	// HSTSIncludeSubdomains includes subdomains in HSTS. Default: true.
	HSTSIncludeSubdomains bool

	// FrameOptions value. Default: "DENY".
	FrameOptions string

	// TrustedProxies is the list of trusted proxy IPs.
	// When set, X-Forwarded-For is only trusted from these IPs.
	TrustedProxies []string

	// SuspiciousUserAgentPatterns are patterns that flag a request as
	// potentially automated. Matched case-insensitively.
	SuspiciousUserAgentPatterns []string
}

// DefaultSecurityHeadersConfig returns production-grade defaults.
func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		PermissionsPolicy:     "camera=(), microphone=(), geolocation=(), payment=(self)",
		HSTSMaxAge:            31536000,
		HSTSIncludeSubdomains: true,
		FrameOptions:          "DENY",
		TrustedProxies:        []string{},
		SuspiciousUserAgentPatterns: []string{
			"curl", "wget", "python-requests", "go-http-client",
			"postmanruntime", "insomnia", "httpie", "scanner",
			"bot", "crawler", "spider", "scraper",
		},
	}
}

// SecurityHeaders returns middleware that sets security response headers.
func SecurityHeaders() fiber.Handler {
	return SecurityHeadersWithConfig(DefaultSecurityHeadersConfig())
}

// SecurityHeadersWithConfig returns the security headers middleware with custom config.
func SecurityHeadersWithConfig(cfg SecurityHeadersConfig) fiber.Handler {
	// Build HSTS value once
	var hstsValue string
	if cfg.HSTSMaxAge > 0 {
		hstsValue = fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
		if cfg.HSTSIncludeSubdomains {
			hstsValue += "; includeSubDomains"
		}
	}

	// Build trusted proxy lookup set
	trustedSet := make(map[string]struct{}, len(cfg.TrustedProxies))
	for _, ip := range cfg.TrustedProxies {
		trustedSet[strings.TrimSpace(ip)] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		// ── OWASP Security Headers ─────────────────────────────
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", cfg.FrameOptions)
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("X-DNS-Prefetch-Control", "off")
		c.Set("X-Download-Options", "noopen")
		c.Set("X-Permitted-Cross-Domain-Policies", "none")
		c.Set("Referrer-Policy", cfg.ReferrerPolicy)
		c.Set("Permissions-Policy", cfg.PermissionsPolicy)

		if cfg.ContentSecurityPolicy != "" {
			c.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
		}

		if hstsValue != "" {
			c.Set("Strict-Transport-Security", hstsValue)
		}

		// Remove the Server header to avoid leaking server info
		c.Set("Server", "")

		// ── Extract real IP and store in locals ────────────────
		realIP := ExtractRealIP(c, trustedSet)
		c.Locals("realIP", realIP)

		// ── Compute client fingerprint ─────────────────────────
		fingerprint := ComputeFingerprint(c, realIP)
		c.Locals("fingerprint", fingerprint)

		// ── Flag suspicious requests ───────────────────────────
		ua := strings.ToLower(c.Get("User-Agent"))
		suspicious := false
		if ua == "" {
			suspicious = true
		} else {
			for _, pattern := range cfg.SuspiciousUserAgentPatterns {
				if strings.Contains(ua, strings.ToLower(pattern)) {
					suspicious = true
					break
				}
			}
		}
		c.Locals("suspiciousClient", suspicious)

		return c.Next()
	}
}

// ExtractRealIP resolves the client's true IP address from proxy headers.
// Priority: X-Real-IP → first untrusted IP in X-Forwarded-For → c.IP()
func ExtractRealIP(c fiber.Ctx, trusted map[string]struct{}) string {
	// X-Real-IP is typically set by the closest reverse proxy
	if xri := c.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// X-Forwarded-For: client, proxy1, proxy2
	if xff := c.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		// Walk from right to left, skip trusted proxies
		for i := len(parts) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(parts[i])
			if _, isTrusted := trusted[ip]; !isTrusted {
				return ip
			}
		}
		// If all are trusted, return leftmost
		return strings.TrimSpace(parts[0])
	}

	return c.IP()
}

// ComputeFingerprint generates a SHA-256 hash of the client's IP, User-Agent,
// and Accept-Language header for lightweight device fingerprinting.
func ComputeFingerprint(c fiber.Ctx, ip string) string {
	raw := fmt.Sprintf("%s|%s|%s", ip, c.Get("User-Agent"), c.Get("Accept-Language"))
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

// GetRealIP extracts the real IP stored in locals by the SecurityHeaders middleware.
func GetRealIP(c fiber.Ctx) string {
	if ip, ok := c.Locals("realIP").(string); ok {
		return ip
	}
	return c.IP()
}

// GetFingerprint extracts the client fingerprint from locals.
func GetFingerprint(c fiber.Ctx) string {
	if fp, ok := c.Locals("fingerprint").(string); ok {
		return fp
	}
	return ""
}

// IsSuspiciousClient checks if the SecurityHeaders middleware flagged this client.
func IsSuspiciousClient(c fiber.Ctx) bool {
	if s, ok := c.Locals("suspiciousClient").(bool); ok {
		return s
	}
	return false
}
