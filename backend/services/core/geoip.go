package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// GEOIP SERVICE — Production Implementation
// Uses ip-api.com (free tier: 45 req/min, no key needed).
// Includes in-memory LRU cache and private IP detection.
// Swap provider by replacing fetchFromAPI().
// ============================================================================

// GeoIPResult holds the resolved geolocation data for an IP.
type GeoIPResult struct {
	IP        string  `json:"query"`
	Country   string  `json:"country"`
	CountryCode string `json:"countryCode"`
	Region    string  `json:"regionName"`
	City      string  `json:"city"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	ISP       string  `json:"isp"`
	Timezone  string  `json:"timezone"`
	Status    string  `json:"status"` // "success" or "fail"
}

// GeoIPService resolves IP addresses to geographic locations.
type GeoIPService struct {
	client   *http.Client
	cache    map[string]*geoIPCacheEntry
	cacheMu  sync.RWMutex
	cacheTTL time.Duration
	maxCache int
}

type geoIPCacheEntry struct {
	result    *GeoIPResult
	expiresAt time.Time
}

// NewGeoIPService creates a production GeoIPService with caching.
func NewGeoIPService() *GeoIPService {
	return &GeoIPService{
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
		cache:    make(map[string]*geoIPCacheEntry),
		cacheTTL: 1 * time.Hour,
		maxCache: 10000,
	}
}

// Lookup returns a human-readable location string for an IP address.
// Example: "San Francisco, California, US"
func (g *GeoIPService) Lookup(ip string) string {
	result := g.resolve(ip)
	if result == nil {
		return "Unknown Location"
	}

	parts := make([]string, 0, 3)
	if result.City != "" {
		parts = append(parts, result.City)
	}
	if result.Region != "" {
		parts = append(parts, result.Region)
	}
	if result.CountryCode != "" {
		parts = append(parts, result.CountryCode)
	}

	if len(parts) == 0 {
		return "Unknown Location"
	}
	return strings.Join(parts, ", ")
}

// GetCountry returns the country code (e.g. "US") for an IP address.
func (g *GeoIPService) GetCountry(ip string) string {
	result := g.resolve(ip)
	if result == nil || result.CountryCode == "" {
		return "Unknown"
	}
	return result.CountryCode
}

// GetCoordinates returns latitude and longitude for an IP address.
func (g *GeoIPService) GetCoordinates(ip string) (float64, float64) {
	result := g.resolve(ip)
	if result == nil {
		return 0.0, 0.0
	}
	return result.Latitude, result.Longitude
}

// ────────────────────────────────────────────────────────────────────────────
// INTERNAL
// ────────────────────────────────────────────────────────────────────────────

// resolve looks up the IP, checking cache first, then API.
func (g *GeoIPService) resolve(ip string) *GeoIPResult {
	// Normalize
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil
	}

	// Handle X-Forwarded-For (take first IP)
	if idx := strings.Index(ip, ","); idx != -1 {
		ip = strings.TrimSpace(ip[:idx])
	}

	// Private/loopback IPs can't be geolocated
	if isPrivateIP(ip) {
		return &GeoIPResult{
			IP:          ip,
			City:        "Local Network",
			Country:     "Local",
			CountryCode: "LO",
			Status:      "success",
		}
	}

	// Check cache
	if entry := g.getFromCache(ip); entry != nil {
		return entry
	}

	// Fetch from API
	result, err := g.fetchFromAPI(ip)
	if err != nil {
		log.Printf("[GEOIP] API error for %s: %v", ip, err)
		return nil
	}

	// Cache result
	g.putInCache(ip, result)
	return result
}

// fetchFromAPI calls ip-api.com to resolve an IP.
// Swap this method to use MaxMind, ipinfo.io, etc.
func (g *GeoIPService) fetchFromAPI(ip string) (*GeoIPResult, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,regionName,city,lat,lon,isp,timezone,query", ip)

	resp, err := g.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by ip-api.com")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result GeoIPResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("lookup failed for %s", ip)
	}

	return &result, nil
}

// ────────────────────────────────────────────────────────────────────────────
// CACHE
// ────────────────────────────────────────────────────────────────────────────

func (g *GeoIPService) getFromCache(ip string) *GeoIPResult {
	g.cacheMu.RLock()
	defer g.cacheMu.RUnlock()

	entry, exists := g.cache[ip]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.result
}

func (g *GeoIPService) putInCache(ip string, result *GeoIPResult) {
	g.cacheMu.Lock()
	defer g.cacheMu.Unlock()

	// Evict oldest entries if cache is full (simple strategy)
	if len(g.cache) >= g.maxCache {
		count := 0
		for k := range g.cache {
			delete(g.cache, k)
			count++
			if count >= g.maxCache/4 { // evict 25%
				break
			}
		}
	}

	g.cache[ip] = &geoIPCacheEntry{
		result:    result,
		expiresAt: time.Now().Add(g.cacheTTL),
	}
}

// ────────────────────────────────────────────────────────────────────────────
// HELPERS
// ────────────────────────────────────────────────────────────────────────────

// isPrivateIP checks if an IP is private, loopback, or link-local.
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true // unparseable → treat as private (fail safe)
	}

	// Loopback
	if ip.IsLoopback() {
		return true
	}

	// Link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// RFC 1918 private ranges
	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"fc00::/7"}, // IPv6 private
	}

	for _, r := range privateRanges {
		_, cidr, err := net.ParseCIDR(r.network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}
