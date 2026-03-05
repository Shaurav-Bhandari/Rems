package DTO

import (
	"github.com/google/uuid"
)

// ============================================
// POLICY DTOs
// These are what rbac_service.go expects.
// CreatePolicyRequest and PermissionRequest are
// referenced directly in rbac_service.CreatePolicy
// and rbac_service.detectPolicyConflicts.
// ============================================

// PermissionRequest is the per-permission entry inside a CreatePolicyRequest.
// rbac_service reads: p.Resource, p.Action, p.Effect, p.Conditions
type PermissionRequest struct {
	Resource   string            `json:"resource" binding:"required"`           // matches ResourceType: "order", "menu", "*", etc.
	Action     string            `json:"action" binding:"required"`             // matches ActionType: "create", "read", "delete", "*", etc.
	Effect     string            `json:"effect" binding:"required,oneof=allow deny"` // PolicyEffectAllow or PolicyEffectDeny
	Conditions map[string]string `json:"conditions,omitempty"`                  // optional context conditions e.g. {"owner_only": "true"}
}

// CreatePolicyRequest is the body for POST /rbac/policies.
// rbac_service reads: req.Name, req.Description, req.Permissions
type CreatePolicyRequest struct {
	Name        string              `json:"name" binding:"required,min=2,max=255"`
	Description string              `json:"description" binding:"omitempty,max=1000"`
	Permissions []PermissionRequest `json:"permissions" binding:"required,min=1"`
}

// UpdatePolicyRequest is the body for PATCH /rbac/policies/:id.
// Version is auto-incremented by the service — don't accept it from client.
type UpdatePolicyRequest struct {
	Name        *string             `json:"name" binding:"omitempty,min=2,max=255"`
	Description *string             `json:"description" binding:"omitempty,max=1000"`
	Permissions *[]PermissionRequest `json:"permissions" binding:"omitempty,min=1"`
}

// PolicyResponse is what the API returns after creating or fetching a policy.
type PolicyResponse struct {
	PolicyID    uuid.UUID           `json:"policy_id"`
	TenantID    uuid.UUID           `json:"tenant_id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Permissions []PermissionRequest `json:"permissions,omitempty"`
}

// ============================================
// RBAC-SPECIFIC ROLE DTOs
// These extend the existing role.go DTOs with
// fields needed for the full RBAC system.
// ============================================

// CreateRBACRoleRequest creates a role with policies attached (not just permission name strings).
// Use this instead of CreateRoleRequest when working with the RBACService.
type CreateRBACRoleRequest struct {
	RoleName      string      `json:"role_name" binding:"required,min=2,max=255"`
	Description   string      `json:"description" binding:"omitempty,max=1000"`
	ParentRoleIDs []uuid.UUID `json:"parent_role_ids" binding:"omitempty"` // for role inheritance
	PolicyIDs     []uuid.UUID `json:"policy_ids" binding:"omitempty"`      // attach existing policies
}

// AssignRoleRequest is the body for POST /rbac/users/:id/roles.
// Matches what rbac_service.AssignRole expects.
type AssignRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// RevokeRoleRequest is the body for DELETE /rbac/users/:id/roles.
type RevokeRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// AccessCheckRequest lets you test if a user can perform an action.
// Useful for debugging and admin tooling.
type AccessCheckRequest struct {
	UserID     uuid.UUID         `json:"user_id" binding:"required"`
	Resource   string            `json:"resource" binding:"required"`
	Action     string            `json:"action" binding:"required"`
	ResourceID *uuid.UUID        `json:"resource_id,omitempty"`
	Context    map[string]string `json:"context,omitempty"`
}

// AccessCheckResponse is the result of an access check.
type AccessCheckResponse struct {
	Allowed       bool   `json:"allowed"`
	MatchedPolicy string `json:"matched_policy,omitempty"`
	MatchedRole   string `json:"matched_role,omitempty"`
	DenyReason    string `json:"deny_reason,omitempty"`
	CacheHit      bool   `json:"cache_hit"`
}