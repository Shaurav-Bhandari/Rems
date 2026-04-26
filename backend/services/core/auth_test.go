package services

import (
	"testing"
)

// ============================================================================
// GeoIP Tests
// ============================================================================

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.50", false},
		{"::1", true},
		{"", true},        // empty → private (fail safe)
		{"invalid", true}, // unparseable → private (fail safe)
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := isPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestGeoIPService_Lookup_PrivateIP(t *testing.T) {
	svc := NewGeoIPService()

	result := svc.Lookup("192.168.1.1")
	if result != "Local Network, LO" {
		t.Errorf("Lookup(192.168.1.1) = %q, want %q", result, "Local Network, LO")
	}
}

func TestGeoIPService_Lookup_EmptyIP(t *testing.T) {
	svc := NewGeoIPService()

	result := svc.Lookup("")
	if result != "Unknown Location" {
		t.Errorf("Lookup(\"\") = %q, want %q", result, "Unknown Location")
	}
}

func TestGeoIPService_Lookup_ForwardedIP(t *testing.T) {
	svc := NewGeoIPService()

	// X-Forwarded-For with multiple IPs — should use first
	result := svc.Lookup("192.168.1.1, 10.0.0.1")
	if result != "Local Network, LO" {
		t.Errorf("Lookup with forwarded IPs = %q, want %q", result, "Local Network, LO")
	}
}

func TestGeoIPService_GetCountry_PrivateIP(t *testing.T) {
	svc := NewGeoIPService()

	result := svc.GetCountry("10.0.0.1")
	if result != "LO" {
		t.Errorf("GetCountry(10.0.0.1) = %q, want %q", result, "LO")
	}
}

func TestGeoIPService_GetCoordinates_PrivateIP(t *testing.T) {
	svc := NewGeoIPService()

	lat, lon := svc.GetCoordinates("127.0.0.1")
	if lat != 0.0 || lon != 0.0 {
		t.Errorf("GetCoordinates(127.0.0.1) = (%f, %f), want (0, 0)", lat, lon)
	}
}

// ============================================================================
// Email Template Tests
// ============================================================================

func TestRenderTemplate_VerifyEmail(t *testing.T) {
	data := map[string]interface{}{
		"Name":      "John Doe",
		"VerifyURL": "https://example.com/verify?token=abc123",
		"Year":      2026,
		"AppName":   "ReMS",
	}

	result, err := renderTemplate(templateVerifyEmail, data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	// Check key content is present
	checks := []string{
		"John Doe",
		"https://example.com/verify?token=abc123",
		"Verify Email Address",
		"ReMS",
	}
	for _, check := range checks {
		if !contains(result, check) {
			t.Errorf("Template output missing %q", check)
		}
	}
}

func TestRenderTemplate_PasswordChanged(t *testing.T) {
	data := map[string]interface{}{
		"Name":      "Jane",
		"IPAddress": "203.0.113.50",
		"Timestamp": "April 24, 2026 at 3:00 PM IST",
		"Year":      2026,
		"AppName":   "ReMS",
	}

	result, err := renderTemplate(templatePasswordChanged, data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	if !contains(result, "203.0.113.50") {
		t.Error("Password changed template missing IP address")
	}
	if !contains(result, "password was successfully changed") {
		t.Error("Password changed template missing confirmation text")
	}
}

func TestRenderTemplate_AccountLocked(t *testing.T) {
	data := map[string]interface{}{
		"Name":        "Admin",
		"LockedUntil": "April 24, 2026 at 4:00 PM IST",
		"Duration":    "30m0s",
		"Year":        2026,
		"AppName":     "ReMS",
	}

	result, err := renderTemplate(templateAccountLocked, data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	if !contains(result, "temporarily locked") {
		t.Error("Account locked template missing lockout text")
	}
}

func TestRenderTemplate_SuspiciousLogin(t *testing.T) {
	data := map[string]interface{}{
		"Name":      "User",
		"IPAddress": "8.8.8.8",
		"Location":  "Mountain View, California, US",
		"Timestamp": "April 24, 2026 at 5:00 PM IST",
		"Year":      2026,
		"AppName":   "ReMS",
	}

	result, err := renderTemplate(templateSuspiciousLogin, data)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}

	if !contains(result, "Suspicious Login Detected") {
		t.Error("Suspicious login template missing alert text")
	}
	if !contains(result, "Mountain View") {
		t.Error("Suspicious login template missing location")
	}
}

// ============================================================================
// Password Validation Tests
// ============================================================================

func TestPasswordValidation_TooShort(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("Ab1!")
	if err == nil {
		t.Error("Expected error for short password")
	}
}

func TestPasswordValidation_NoUppercase(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("abcdefghijk1!")
	if err == nil {
		t.Error("Expected error for missing uppercase")
	}
}

func TestPasswordValidation_NoLowercase(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("ABCDEFGHIJK1!")
	if err == nil {
		t.Error("Expected error for missing lowercase")
	}
}

func TestPasswordValidation_NoNumber(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("Abcdefghijk!!")
	if err == nil {
		t.Error("Expected error for missing number")
	}
}

func TestPasswordValidation_NoSpecial(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("Abcdefghijk12")
	if err == nil {
		t.Error("Expected error for missing special character")
	}
}

func TestPasswordValidation_Valid(t *testing.T) {
	svc := NewPasswordService(nil, 12, 5)
	err := svc.ValidatePasswordStrength("Str0ngP@ssw0rd!")
	if err != nil {
		t.Errorf("Expected no error for valid password, got: %v", err)
	}
}

// ============================================================================
// Argon2id Hash Tests
// ============================================================================

func TestHashPwd_And_ComparePwd(t *testing.T) {
	password := "TestP@ssw0rd123"

	hash, err := HashPwd(password)
	if err != nil {
		t.Fatalf("HashPwd failed: %v", err)
	}

	// Verify hash format: $argon2id$v=...
	if !contains(hash, "$argon2id$") {
		t.Errorf("Hash doesn't have argon2id format: %s", hash)
	}

	// Correct password should match
	match, err := ComparePwd(password, hash)
	if err != nil {
		t.Fatalf("ComparePwd failed: %v", err)
	}
	if !match {
		t.Error("Correct password should match hash")
	}

	// Wrong password should not match
	match, err = ComparePwd("WrongPassword123!", hash)
	if err != nil {
		t.Fatalf("ComparePwd failed: %v", err)
	}
	if match {
		t.Error("Wrong password should not match hash")
	}
}

func TestHashPwd_UniqueHashes(t *testing.T) {
	password := "SamePassword1!"

	hash1, _ := HashPwd(password)
	hash2, _ := HashPwd(password)

	if hash1 == hash2 {
		t.Error("Two hashes of the same password should differ (different salts)")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
