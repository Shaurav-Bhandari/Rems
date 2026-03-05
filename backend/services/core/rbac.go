package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	// "net/http"  // future: policy decision point HTTP endpoint
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"backend/DTO"
	"backend/models"
)

// ============================================
// TYPED ENUMS (mirrors auth_service pattern)
// ============================================

type PolicyEffect string

const (
	PolicyEffectAllow PolicyEffect = "allow"
	PolicyEffectDeny  PolicyEffect = "deny"
)

type RBACEventType string

const (
	EventPermissionGranted    RBACEventType = "permission_granted"
	EventPermissionDenied     RBACEventType = "permission_denied"
	EventRoleAssigned         RBACEventType = "role_assigned"
	EventRoleRevoked          RBACEventType = "role_revoked"
	EventPolicyCreated        RBACEventType = "policy_created"
	EventPolicyUpdated        RBACEventType = "policy_updated"
	EventPolicyDeleted        RBACEventType = "policy_deleted"
	EventPrivilegeEscalation  RBACEventType = "privilege_escalation_attempt"
	EventWildcardAbuse        RBACEventType = "wildcard_permission_abuse"
	EventCrossTenanAccess     RBACEventType = "cross_tenant_access_attempt"
)

type ResourceType string

const (
	ResourceOrder      ResourceType = "order"
	ResourceMenu       ResourceType = "menu"
	ResourceReport     ResourceType = "report"
	ResourceUser       ResourceType = "user"
	ResourceRole       ResourceType = "role"
	ResourceTenant     ResourceType = "tenant"
	ResourceKitchen    ResourceType = "kitchen"
	ResourceInventory  ResourceType = "inventory"
	ResourcePayment    ResourceType = "payment"
	ResourceAuditLog   ResourceType = "audit_log"
	ResourceWildcard   ResourceType = "*"
)

type ActionType string

const (
	ActionCreate   ActionType = "create"
	ActionRead     ActionType = "read"
	ActionUpdate   ActionType = "update"
	ActionDelete   ActionType = "delete"
	ActionApprove  ActionType = "approve"
	ActionExport   ActionType = "export"
	ActionWildcard ActionType = "*"
)

// ============================================
// CENTRALIZED ERRORS (Improvement #17 energy)
// ============================================

var (
	ErrPermissionDenied       = errors.New("permission_denied")
	ErrRoleNotFound           = errors.New("role_not_found")
	ErrPolicyNotFound         = errors.New("policy_not_found")
	ErrRoleAlreadyAssigned    = errors.New("role_already_assigned")
	ErrCircularInheritance    = errors.New("circular_role_inheritance_detected")
	ErrPrivilegeEscalation    = errors.New("privilege_escalation_not_allowed")
	ErrCrossTenantAccess      = errors.New("cross_tenant_access_denied")
	ErrWildcardAbuse          = errors.New("wildcard_permission_requires_superadmin")
	ErrPolicyConflict         = errors.New("conflicting_policies_detected")
	ErrMaxRolesReached        = errors.New("max_roles_per_user_reached")
	ErrCannotModifySuperAdmin = errors.New("superadmin_role_is_immutable")
)

// ============================================
// DOMAIN MODELS
// ============================================

// Permission is the atomic unit. Resource + Action + optional Conditions.
type Permission struct {
	PermissionID uuid.UUID         `json:"permission_id"`
	Resource     ResourceType      `json:"resource"`
	Action       ActionType        `json:"action"`
	Effect       PolicyEffect      `json:"effect"`
	Conditions   map[string]string `json:"conditions,omitempty"` // e.g. {"owner_only": "true"}
}

// Policy groups permissions under a named, versioned rule set.
type Policy struct {
	PolicyID    uuid.UUID    `json:"policy_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	TenantID    uuid.UUID    `json:"tenant_id"`
	Permissions []Permission `json:"permissions"`
	Version     int          `json:"version"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Role holds policies and can inherit from parent roles.
type Role struct {
	RoleID      uuid.UUID   `json:"role_id"`
	Name        string      `json:"name"`
	TenantID    uuid.UUID   `json:"tenant_id"`
	ParentRoles []uuid.UUID `json:"parent_roles,omitempty"` // inheritance chain
	Policies    []Policy    `json:"policies"`
	IsSystem    bool        `json:"is_system"` // system roles cannot be deleted
	CreatedAt   time.Time   `json:"created_at"`
}

// AccessRequest is what every authorization check is built from.
type AccessRequest struct {
	UserID     uuid.UUID         `json:"user_id"`
	TenantID   uuid.UUID         `json:"tenant_id"`
	Resource   ResourceType      `json:"resource"`
	Action     ActionType        `json:"action"`
	ResourceID *uuid.UUID        `json:"resource_id,omitempty"` // for owner checks
	Context    map[string]string `json:"context,omitempty"`     // IP, device, time, etc.
}

// AccessDecision is the result of an authorization check.
type AccessDecision struct {
	Allowed       bool         `json:"allowed"`
	MatchedPolicy string       `json:"matched_policy,omitempty"`
	MatchedRole   string       `json:"matched_role,omitempty"`
	DenyReason    string       `json:"deny_reason,omitempty"`
	EvaluatedAt   time.Time    `json:"evaluated_at"`
	CacheHit      bool         `json:"cache_hit"`
}

// ============================================
// PERMISSION CACHE (in-memory + Redis)
// ============================================

// PermissionCache is a two-layer cache:
// L1 = in-process sync.Map (nanoseconds)
// L2 = Redis (milliseconds, shared across pods)
type PermissionCache struct {
	local  sync.Map
	redis  *redis.Client
	ttlL1  time.Duration
	ttlL2  time.Duration
}

func NewPermissionCache(redis *redis.Client) *PermissionCache {
	return &PermissionCache{
		redis: redis,
		ttlL1: 30 * time.Second,  // hot path: in-process
		ttlL2: 5 * time.Minute,   // warm path: Redis
	}
}

func (c *PermissionCache) cacheKey(userID, tenantID uuid.UUID) string {
	raw := fmt.Sprintf("rbac:perms:%s:%s", tenantID.String(), userID.String())
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

func (c *PermissionCache) Get(ctx context.Context, userID, tenantID uuid.UUID) ([]Permission, bool) {
	key := c.cacheKey(userID, tenantID)

	// L1 check
	if val, ok := c.local.Load(key); ok {
		if entry, ok := val.(localCacheEntry); ok {
			if time.Now().Before(entry.expiresAt) {
				return entry.permissions, true
			}
			c.local.Delete(key)
		}
	}

	// L2 check
	data, err := c.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	var perms []Permission
	if err := json.Unmarshal([]byte(data), &perms); err != nil {
		return nil, false
	}

	// Backfill L1
	c.local.Store(key, localCacheEntry{permissions: perms, expiresAt: time.Now().Add(c.ttlL1)})
	return perms, true
}

func (c *PermissionCache) Set(ctx context.Context, userID, tenantID uuid.UUID, perms []Permission) {
	key := c.cacheKey(userID, tenantID)

	data, err := json.Marshal(perms)
	if err != nil {
		return
	}

	// Write to both layers
	c.redis.Set(ctx, key, data, c.ttlL2)
	c.local.Store(key, localCacheEntry{permissions: perms, expiresAt: time.Now().Add(c.ttlL1)})
}

func (c *PermissionCache) Invalidate(ctx context.Context, userID, tenantID uuid.UUID) {
	key := c.cacheKey(userID, tenantID)
	c.local.Delete(key)
	c.redis.Del(ctx, key)
}

type localCacheEntry struct {
	permissions []Permission
	expiresAt   time.Time
}

// ============================================
// POLICY ENGINE (the thing that actually decides)
// ============================================

type PolicyEngine struct{}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

// Evaluate walks all permissions and applies deny-overrides-allow logic.
// Explicit deny always wins. If no rule matches, default is deny.
func (e *PolicyEngine) Evaluate(req *AccessRequest, permissions []Permission) *AccessDecision {
	decision := &AccessDecision{
		Allowed:     false,
		EvaluatedAt: time.Now(),
	}

	for _, perm := range permissions {
		if !e.matchesResource(perm.Resource, req.Resource) {
			continue
		}
		if !e.matchesAction(perm.Action, req.Action) {
			continue
		}
		if !e.evaluateConditions(perm.Conditions, req) {
			continue
		}

		if perm.Effect == PolicyEffectDeny {
			// Explicit deny is final — no further evaluation
			decision.Allowed = false
			decision.DenyReason = fmt.Sprintf("explicit deny on %s:%s", perm.Resource, perm.Action)
			return decision
		}

		if perm.Effect == PolicyEffectAllow {
			decision.Allowed = true
		}
	}

	if !decision.Allowed {
		decision.DenyReason = fmt.Sprintf("no policy grants %s on %s", req.Action, req.Resource)
	}

	return decision
}

func (e *PolicyEngine) matchesResource(permResource, reqResource ResourceType) bool {
	return permResource == ResourceWildcard || permResource == reqResource
}

func (e *PolicyEngine) matchesAction(permAction, reqAction ActionType) bool {
	return permAction == ActionWildcard || permAction == reqAction
}

func (e *PolicyEngine) evaluateConditions(conditions map[string]string, req *AccessRequest) bool {
	for key, expected := range conditions {
		actual, exists := req.Context[key]
		if !exists || actual != expected {
			return false
		}
	}
	return true
}

// ============================================
// ROLE INHERITANCE RESOLVER
// ============================================

// RoleResolver flattens a role hierarchy into a single permission set.
// Detects circular inheritance so you don't loop forever.
type RoleResolver struct {
	db *gorm.DB
}

func NewRoleResolver(db *gorm.DB) *RoleResolver {
	return &RoleResolver{db: db}
}

func (r *RoleResolver) ResolvePermissions(ctx context.Context, roleIDs []uuid.UUID, tenantID uuid.UUID) ([]Permission, error) {
	visited := make(map[uuid.UUID]bool)
	return r.resolveRecursive(ctx, roleIDs, tenantID, visited, 0)
}

const maxInheritanceDepth = 5 // prevent absurdly deep chains

func (r *RoleResolver) resolveRecursive(
	ctx context.Context,
	roleIDs []uuid.UUID,
	tenantID uuid.UUID,
	visited map[uuid.UUID]bool,
	depth int,
) ([]Permission, error) {
	if depth > maxInheritanceDepth {
		return nil, fmt.Errorf("%w: exceeded max depth of %d", ErrCircularInheritance, maxInheritanceDepth)
	}

	var allPermissions []Permission

	for _, roleID := range roleIDs {
		if visited[roleID] {
			return nil, fmt.Errorf("%w: role %s", ErrCircularInheritance, roleID)
		}
		visited[roleID] = true

		var role models.Role
		if err := r.db.WithContext(ctx).
			Where("role_id = ? AND tenant_id = ?", roleID, tenantID).
			Preload("Policies.Permissions").
			First(&role).Error; err != nil {
			return nil, fmt.Errorf("%w: %s", ErrRoleNotFound, roleID)
		}

		// Collect this role's permissions
		for _, policy := range role.Policies {
			for _, p := range policy.Permissions {
				allPermissions = append(allPermissions, Permission{
					PermissionID: p.PermissionID,
					Resource:     ResourceType(p.Resource),
					Action:       ActionType(p.Action),
					Effect:       PolicyEffect(p.Effect),
					Conditions:   p.Conditions,
				})
			}
		}

		// Recurse into parent roles
		if len(role.ParentRoleIDs) > 0 {
			parentPerms, err := r.resolveRecursive(ctx, role.ParentRoleIDs, tenantID, visited, depth+1)
			if err != nil {
				return nil, err
			}
			allPermissions = append(allPermissions, parentPerms...)
		}
	}

	return allPermissions, nil
}

// ============================================
// AUDIT SERVICE
// ============================================

type RBACEventSeverity string

const (
	RBACSeverityInfo     RBACEventSeverity = "info"
	RBACSeverityWarning  RBACEventSeverity = "warning"
	RBACSeverityCritical RBACEventSeverity = "critical"
)

type RBACAuditService struct {
	db *gorm.DB
}

func NewRBACAuditService(db *gorm.DB) *RBACAuditService {
	return &RBACAuditService{db: db}
}

func (a *RBACAuditService) Log(
	ctx context.Context,
	event RBACEventType,
	severity RBACEventSeverity,
	actorID uuid.UUID,
	targetID *uuid.UUID,
	tenantID uuid.UUID,
	metadata map[string]interface{},
) {
	metaJSON, _ := json.Marshal(metadata)

	entry := &models.RBACEvent{
		EventID:   uuid.New(),
		EventType: string(event),
		Severity:  string(severity),
		ActorID:   actorID,
		TargetID:  targetID,
		TenantID:  tenantID,
		Metadata:  metaJSON,
		CreatedAt: time.Now(),
	}

	// Fire and forget — don't let audit failures block authorization
	go a.db.WithContext(ctx).Create(entry)
}

// ============================================
// RBAC SERVICE (the main thing)
// ============================================

type RBACService struct {
	db           *gorm.DB
	redis        *redis.Client
	cache        *PermissionCache
	engine       *PolicyEngine
	resolver     *RoleResolver
	audit        *RBACAuditService
	maxRolesPerUser int
}

func NewRBACService(db *gorm.DB, redis *redis.Client) *RBACService {
	return &RBACService{
		db:              db,
		redis:           redis,
		cache:           NewPermissionCache(redis),
		engine:          NewPolicyEngine(),
		resolver:        NewRoleResolver(db),
		audit:           NewRBACAuditService(db),
		maxRolesPerUser: 10, // TODO: move to immudb config
	}
}

// ============================================
// AUTHORIZE — the hot path. Called on every request.
// ============================================

func (s *RBACService) Authorize(ctx context.Context, req *AccessRequest) (*AccessDecision, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond) // tight budget — this is on the critical path
	defer cancel()

	// Wildcard actions require superadmin — catch abuse early
	if req.Action == ActionWildcard || req.Resource == ResourceWildcard {
		if !s.isSuperAdmin(ctx, req.UserID, req.TenantID) {
			s.audit.Log(ctx, EventWildcardAbuse, RBACSeverityCritical, req.UserID, nil, req.TenantID, map[string]interface{}{
				"resource": req.Resource,
				"action":   req.Action,
			})
			return &AccessDecision{Allowed: false, DenyReason: ErrWildcardAbuse.Error(), EvaluatedAt: time.Now()}, nil
		}
	}

	// L1/L2 cache check
	perms, cacheHit := s.cache.Get(ctx, req.UserID, req.TenantID)
	if !cacheHit {
		var err error
		perms, err = s.loadUserPermissions(ctx, req.UserID, req.TenantID)
		if err != nil {
			return nil, err
		}
		s.cache.Set(ctx, req.UserID, req.TenantID, perms)
	}

	decision := s.engine.Evaluate(req, perms)
	decision.CacheHit = cacheHit

	// Audit all denials; only audit allows for sensitive resources
	if !decision.Allowed {
		s.audit.Log(ctx, EventPermissionDenied, RBACSeverityWarning, req.UserID, req.ResourceID, req.TenantID, map[string]interface{}{
			"resource":    req.Resource,
			"action":      req.Action,
			"deny_reason": decision.DenyReason,
		})
	} else if s.isSensitiveResource(req.Resource) {
		s.audit.Log(ctx, EventPermissionGranted, RBACSeverityInfo, req.UserID, req.ResourceID, req.TenantID, map[string]interface{}{
			"resource": req.Resource,
			"action":   req.Action,
		})
	}

	return decision, nil
}

// ============================================
// REQUIRE — convenience wrapper that returns an error instead of a decision.
// Use this in service methods so you can just: if err := s.rbac.Require(...); err != nil { return err }
// ============================================

func (s *RBACService) Require(ctx context.Context, req *AccessRequest) error {
	decision, err := s.Authorize(ctx, req)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return fmt.Errorf("%w: %s", ErrPermissionDenied, decision.DenyReason)
	}
	return nil
}

// ============================================
// ROLE ASSIGNMENT
// ============================================

func (s *RBACService) AssignRole(
	ctx context.Context,
	actorID uuid.UUID, // who is making the change
	targetUserID uuid.UUID,
	roleID uuid.UUID,
	tenantID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Actor must have permission to assign roles
	if err := s.Require(ctx, &AccessRequest{
		UserID:   actorID,
		TenantID: tenantID,
		Resource: ResourceRole,
		Action:   ActionUpdate,
	}); err != nil {
		return err
	}

	// Load the role being assigned
	var role models.Role
	if err := s.db.WithContext(ctx).
		Where("role_id = ? AND tenant_id = ?", roleID, tenantID).
		First(&role).Error; err != nil {
		return ErrRoleNotFound
	}

	// Privilege escalation check: actor cannot assign a role more powerful than their own
	if err := s.checkPrivilegeEscalation(ctx, actorID, roleID, tenantID); err != nil {
		s.audit.Log(ctx, EventPrivilegeEscalation, RBACSeverityCritical, actorID, &targetUserID, tenantID, map[string]interface{}{
			"attempted_role": roleID,
		})
		return err
	}

	// Cap roles per user
	var currentRoleCount int64
	s.db.WithContext(ctx).Model(&models.UserRole{}).
		Where("user_id = ? AND tenant_id = ?", targetUserID, tenantID).
		Count(&currentRoleCount)
	if currentRoleCount >= int64(s.maxRolesPerUser) {
		return ErrMaxRolesReached
	}

	// Check not already assigned
	var existing models.UserRole
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND tenant_id = ?", targetUserID, roleID, tenantID).
		First(&existing).Error; err == nil {
		return ErrRoleAlreadyAssigned
	}

	// Assign
	userRole := &models.UserRole{
		UserRoleID: uuid.New(),
		UserID:     targetUserID,
		RoleID:     roleID,
		TenantID:   tenantID,
		AssignedBy: actorID,
		AssignedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(userRole).Error; err != nil {
		return err
	}

	// Invalidate cache — permissions just changed
	s.cache.Invalidate(ctx, targetUserID, tenantID)

	s.audit.Log(ctx, EventRoleAssigned, RBACSeverityInfo, actorID, &targetUserID, tenantID, map[string]interface{}{
		"role_id":   roleID,
		"role_name": role.Name,
	})

	return nil
}

func (s *RBACService) RevokeRole(
	ctx context.Context,
	actorID uuid.UUID,
	targetUserID uuid.UUID,
	roleID uuid.UUID,
	tenantID uuid.UUID,
) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.Require(ctx, &AccessRequest{
		UserID:   actorID,
		TenantID: tenantID,
		Resource: ResourceRole,
		Action:   ActionUpdate,
	}); err != nil {
		return err
	}

	// Cannot revoke system roles via this method
	var role models.Role
	if err := s.db.WithContext(ctx).Where("role_id = ?", roleID).First(&role).Error; err != nil {
		return ErrRoleNotFound
	}
	if role.IsSystem {
		return ErrCannotModifySuperAdmin
	}

	result := s.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND tenant_id = ?", targetUserID, roleID, tenantID).
		Delete(&models.UserRole{})
	if result.RowsAffected == 0 {
		return ErrRoleNotFound
	}

	// Invalidate cache
	s.cache.Invalidate(ctx, targetUserID, tenantID)

	s.audit.Log(ctx, EventRoleRevoked, RBACSeverityInfo, actorID, &targetUserID, tenantID, map[string]interface{}{
		"role_id":   roleID,
		"role_name": role.Name,
	})

	return nil
}

// ============================================
// POLICY MANAGEMENT
// ============================================

func (s *RBACService) CreatePolicy(
	ctx context.Context,
	actorID uuid.UUID,
	req *DTO.CreatePolicyRequest,
	tenantID uuid.UUID,
) (*Policy, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.Require(ctx, &AccessRequest{
		UserID:   actorID,
		TenantID: tenantID,
		Resource: ResourceRole,
		Action:   ActionCreate,
	}); err != nil {
		return nil, err
	}

	// Validate no conflicting permissions within the policy itself
	if err := s.detectPolicyConflicts(req.Permissions); err != nil {
		return nil, err
	}

	policy := &models.Policy{
		PolicyID:    uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		TenantID:    tenantID,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(policy).Error; err != nil {
			return err
		}

		for _, p := range req.Permissions {
			perm := &models.Permission{
				PermissionID: uuid.New(),
				PolicyID:     policy.PolicyID,
				Resource:     string(p.Resource),
				Action:       string(p.Action),
				Effect:       string(p.Effect),
				Conditions:   p.Conditions,
			}
			if err := tx.Create(perm).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.audit.Log(ctx, EventPolicyCreated, RBACSeverityInfo, actorID, nil, tenantID, map[string]interface{}{
		"policy_id":   policy.PolicyID,
		"policy_name": policy.Name,
	})

	return &Policy{
		PolicyID:  policy.PolicyID,
		Name:      policy.Name,
		TenantID:  tenantID,
		Version:   policy.Version,
		CreatedAt: policy.CreatedAt,
	}, nil
}

// ============================================
// HELPER METHODS
// ============================================

func (s *RBACService) loadUserPermissions(ctx context.Context, userID, tenantID uuid.UUID) ([]Permission, error) {
	// Load all role IDs for this user in this tenant
	var userRoles []models.UserRole
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Find(&userRoles).Error; err != nil {
		return nil, err
	}

	if len(userRoles) == 0 {
		return []Permission{}, nil
	}

	roleIDs := make([]uuid.UUID, len(userRoles))
	for i, ur := range userRoles {
		roleIDs[i] = ur.RoleID
	}

	return s.resolver.ResolvePermissions(ctx, roleIDs, tenantID)
}

func (s *RBACService) isSuperAdmin(ctx context.Context, userID, tenantID uuid.UUID) bool {
	var count int64
	s.db.WithContext(ctx).
		Table("user_roles ur").
		Joins("JOIN roles r ON r.role_id = ur.role_id").
		Where("ur.user_id = ? AND ur.tenant_id = ? AND r.role_name = ? AND r.is_system = true", userID, tenantID, "superadmin").
		Count(&count)
	return count > 0
}

func (s *RBACService) isSensitiveResource(resource ResourceType) bool {
	sensitive := map[ResourceType]bool{
		ResourceUser:     true,
		ResourceRole:     true,
		ResourceTenant:   true,
		ResourceAuditLog: true,
		ResourcePayment:  true,
	}
	return sensitive[resource]
}

// checkPrivilegeEscalation ensures an actor cannot assign a role that grants
// permissions the actor themselves doesn't have.
func (s *RBACService) checkPrivilegeEscalation(ctx context.Context, actorID, targetRoleID, tenantID uuid.UUID) error {
	// Superadmins are exempt
	if s.isSuperAdmin(ctx, actorID, tenantID) {
		return nil
	}

	actorPerms, err := s.loadUserPermissions(ctx, actorID, tenantID)
	if err != nil {
		return err
	}

	targetPerms, err := s.resolver.ResolvePermissions(ctx, []uuid.UUID{targetRoleID}, tenantID)
	if err != nil {
		return err
	}

	// Build a set of actor's allowed actions for fast lookup
	actorAllowed := make(map[string]bool)
	for _, p := range actorPerms {
		if p.Effect == PolicyEffectAllow {
			actorAllowed[fmt.Sprintf("%s:%s", p.Resource, p.Action)] = true
		}
	}

	// Every permission in the target role must also exist for the actor
	for _, p := range targetPerms {
		if p.Effect != PolicyEffectAllow {
			continue
		}
		key := fmt.Sprintf("%s:%s", p.Resource, p.Action)
		wildcardKey := fmt.Sprintf("%s:%s", p.Resource, ActionWildcard)
		superKey := fmt.Sprintf("%s:%s", ResourceWildcard, ActionWildcard)

		if !actorAllowed[key] && !actorAllowed[wildcardKey] && !actorAllowed[superKey] {
			return fmt.Errorf("%w: actor lacks '%s' permission", ErrPrivilegeEscalation, key)
		}
	}

	return nil
}

// detectPolicyConflicts finds allow+deny on the same resource:action pair within one policy.
func (s *RBACService) detectPolicyConflicts(permissions []DTO.PermissionRequest) error {
	type key struct {
		resource string
		action   string
	}
	seen := make(map[key]PolicyEffect)

	for _, p := range permissions {
		k := key{p.Resource, p.Action}
		if existing, ok := seen[k]; ok && existing != PolicyEffect(p.Effect) {
			return fmt.Errorf("%w: %s:%s has both allow and deny", ErrPolicyConflict, p.Resource, p.Action)
		}
		seen[k] = PolicyEffect(p.Effect)
	}

	return nil
}

// BuildCacheKey produces a human-readable cache key for debugging.
// Internal use only — don't expose this in API responses.
func (s *RBACService) BuildCacheKey(userID, tenantID uuid.UUID) string {
	return fmt.Sprintf("rbac:perms:%s:%s", tenantID, userID)
}

// InvalidateAll nukes the permission cache for every user in a tenant.
// Call this when a role or policy is modified.
func (s *RBACService) InvalidateAll(ctx context.Context, tenantID uuid.UUID) error {
	pattern := fmt.Sprintf("rbac:perms:%s:*", tenantID.String())
    
    var cursor uint64
    var deleted int
    
    for {
        var keys []string
        var err error
        keys, cursor, err = s.redis.Scan(ctx, cursor, pattern, 100).Result()
        if err != nil {
            return err
        }
        
        if len(keys) > 0 {
            s.redis.Del(ctx, keys...)
            deleted += len(keys)
        }
        
        if cursor == 0 {
            break
        }
    }
    
    return nil
}

// ============================================
// CROSS-TENANT GUARD
// Middleware-level check to be called before any resource handler.
// ============================================

func (s *RBACService) AssertSameTenant(userTenantID, resourceTenantID uuid.UUID, actorID uuid.UUID) error {
	if userTenantID != resourceTenantID {
		// Don't log here — caller should log with full request context
		return fmt.Errorf("%w: user tenant %s attempted to access tenant %s resource",
			ErrCrossTenantAccess, userTenantID, resourceTenantID)
	}
	return nil
}

// ============================================
// CONVENIENCE HELPERS (so callers don't build AccessRequest manually every time)
// ============================================

func (s *RBACService) CanCreateOrder(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceOrder, Action: ActionCreate})
}

func (s *RBACService) CanReadReport(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceReport, Action: ActionRead})
}

func (s *RBACService) CanExportReport(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceReport, Action: ActionExport})
}

func (s *RBACService) CanManageUsers(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceUser, Action: ActionUpdate})
}

func (s *RBACService) CanDeleteUser(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceUser, Action: ActionDelete})
}

func (s *RBACService) CanApprovePayment(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourcePayment, Action: ActionApprove})
}

func (s *RBACService) CanReadInventory(ctx context.Context, userID, tenantID uuid.UUID) error {
	return s.Require(ctx, &AccessRequest{UserID: userID, TenantID: tenantID, Resource: ResourceInventory, Action: ActionRead})
}

// ============================================
// BUILT-IN ROLE SEEDER
// Call once on app startup to ensure system roles exist.
// ============================================

func (s *RBACService) SeedSystemRoles(ctx context.Context, tenantID uuid.UUID) error {
	systemRoles := []struct {
		name        string
		permissions []Permission
	}{
		{
			name: "superadmin",
			permissions: []Permission{
				{Resource: ResourceWildcard, Action: ActionWildcard, Effect: PolicyEffectAllow},
			},
		},
		{
			name: "manager",
			permissions: []Permission{
				{Resource: ResourceOrder, Action: ActionWildcard, Effect: PolicyEffectAllow},
				{Resource: ResourceReport, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceReport, Action: ActionExport, Effect: PolicyEffectAllow},
				{Resource: ResourceUser, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceInventory, Action: ActionWildcard, Effect: PolicyEffectAllow},
			},
		},
		{
			name: "cashier",
			permissions: []Permission{
				{Resource: ResourceOrder, Action: ActionCreate, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionUpdate, Effect: PolicyEffectAllow},
				{Resource: ResourcePayment, Action: ActionCreate, Effect: PolicyEffectAllow},
				{Resource: ResourcePayment, Action: ActionRead, Effect: PolicyEffectAllow},
			},
		},
		{
			name: "waiter",
			permissions: []Permission{
				{Resource: ResourceOrder, Action: ActionCreate, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceMenu, Action: ActionRead, Effect: PolicyEffectAllow},
			},
		},
		{
			name: "chef",
			permissions: []Permission{
				{Resource: ResourceOrder, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionUpdate, Effect: PolicyEffectAllow},
				{Resource: ResourceKitchen, Action: ActionWildcard, Effect: PolicyEffectAllow},
				{Resource: ResourceInventory, Action: ActionRead, Effect: PolicyEffectAllow},
			},
		},
		{
			name: "customer",
			permissions: []Permission{
				{Resource: ResourceMenu, Action: ActionRead, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionCreate, Effect: PolicyEffectAllow},
				{Resource: ResourceOrder, Action: ActionRead, Effect: PolicyEffectAllow},
			},
		},
	}

	for _, sr := range systemRoles {
		var existing models.Role
		err := s.db.WithContext(ctx).
			Where("role_name = ? AND tenant_id = ? AND is_system = true", sr.name, tenantID).
			First(&existing).Error

		if err == nil {
			continue // already seeded
		}

		role := &models.Role{
			RoleID:    uuid.New(),
			RoleName:  sr.name, // RoleName is the DB column; Name is a read-only AfterFind alias
			TenantID:  tenantID,
			IsSystem:  true,
			CreatedAt: time.Now(),
		}

		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(role).Error; err != nil {
				return err
			}

			policy := &models.Policy{
				PolicyID:    uuid.New(),
				Name:        fmt.Sprintf("%s-default-policy", sr.name),
				Description: fmt.Sprintf("System default policy for %s role", sr.name),
				TenantID:    tenantID,
				Version:     1,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := tx.Create(policy).Error; err != nil {
				return err
			}

			for _, p := range sr.permissions {
				perm := &models.Permission{
					PermissionID: uuid.New(),
					PolicyID:     policy.PolicyID,
					Resource:     strings.ToLower(string(p.Resource)),
					Action:       strings.ToLower(string(p.Action)),
					Effect:       strings.ToLower(string(p.Effect)),
				}
				if err := tx.Create(perm).Error; err != nil {
					return err
				}
			}

			// Attach policy to role
			return tx.Create(&models.RolePolicy{
				RolePolicyID: uuid.New(),
				RoleID:       role.RoleID,
				PolicyID:     policy.PolicyID,
			}).Error
		}); err != nil {
			return fmt.Errorf("failed to seed role %s: %w", sr.name, err)
		}
	}

	return nil
}

// ============================================
// GetUserRoles — used by the handler's GET /users/:id/roles endpoint.
// Returns all roles assigned to a user in a tenant with their basic info.
// ============================================

type UserRoleInfo struct {
	RoleID      uuid.UUID `json:"role_id"`
	RoleName    string    `json:"role_name"`
	IsSystem    bool      `json:"is_system"`
	AssignedAt  time.Time `json:"assigned_at"`
	AssignedBy  uuid.UUID `json:"assigned_by"`
}

func (s *RBACService) GetUserRoles(ctx context.Context, userID, tenantID uuid.UUID) ([]UserRoleInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var userRoles []models.UserRole
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Find(&userRoles).Error; err != nil {
		return nil, err
	}

	if len(userRoles) == 0 {
		return []UserRoleInfo{}, nil
	}

	roleIDs := make([]uuid.UUID, len(userRoles))
	for i, ur := range userRoles {
		roleIDs[i] = ur.RoleID
	}

	var roles []models.Role
	if err := s.db.WithContext(ctx).
		Where("role_id IN ? AND tenant_id = ?", roleIDs, tenantID).
		Find(&roles).Error; err != nil {
		return nil, err
	}

	roleMap := make(map[uuid.UUID]models.Role, len(roles))
	for _, r := range roles {
		roleMap[r.RoleID] = r
	}

	result := make([]UserRoleInfo, 0, len(userRoles))
	for _, ur := range userRoles {
		r, ok := roleMap[ur.RoleID]
		if !ok {
			continue
		}
		result = append(result, UserRoleInfo{
			RoleID:     r.RoleID,
			RoleName:   r.RoleName,
			IsSystem:   r.IsSystem,
			AssignedAt: ur.AssignedAt,
			AssignedBy: ur.AssignedBy,
		})
	}

	return result, nil
}