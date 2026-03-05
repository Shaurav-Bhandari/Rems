package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================
// PERMISSION
// Atomic unit: Resource + Action + Effect + optional Conditions.
// Belongs to a Policy. Loaded via Preload("Policies.Permissions")
// in rbac_service.go RoleResolver.
// ============================================

type Permission struct {
	PermissionID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"permission_id"`
	PolicyID     uuid.UUID `gorm:"type:uuid;not null;index" json:"policy_id"`

	// Resource matches ResourceType constants in rbac_service.go
	// e.g. "order", "menu", "user", "payment", "*"
	Resource string `gorm:"type:varchar(100);not null" json:"resource"`

	// Action matches ActionType constants in rbac_service.go
	// e.g. "create", "read", "update", "delete", "approve", "export", "*"
	Action string `gorm:"type:varchar(100);not null" json:"action"`

	// Effect is "allow" or "deny" — deny always overrides allow in PolicyEngine
	Effect string `gorm:"type:varchar(10);not null;default:'allow'" json:"effect"`

	// Conditions is an optional map evaluated by PolicyEngine.evaluateConditions.
	// e.g. {"owner_only": "true"} means permission only applies when
	// req.Context["owner_only"] == "true"
	Conditions map[string]string `gorm:"type:jsonb;serializer:json" json:"conditions,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Policy Policy `gorm:"foreignKey:PolicyID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.PermissionID == uuid.Nil {
		p.PermissionID = uuid.New()
	}
	return nil
}

// ============================================
// POLICY
// Named, versioned group of Permissions.
// Roles receive permissions by having Policies attached via RolePolicy.
// rbac_service.go preloads: Preload("Policies.Permissions")
// ============================================

type Policy struct {
	PolicyID    uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"policy_id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`

	// Version auto-increments on updates — used for cache invalidation
	Version int `gorm:"not null;default:1" json:"version"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Tenant      Tenant       `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Permissions []Permission `gorm:"foreignKey:PolicyID" json:"permissions,omitempty"`
	Roles       []Role       `gorm:"many2many:role_policies;" json:"-"`
}

func (p *Policy) BeforeCreate(tx *gorm.DB) error {
	if p.PolicyID == uuid.Nil {
		p.PolicyID = uuid.New()
	}
	return nil
}

// ============================================
// ROLE POLICY (JOIN TABLE)
// Many-to-many between Role and Policy.
// Created by rbac_service.go in SeedSystemRoles and CreatePolicy.
// ============================================

type RolePolicy struct {
	RolePolicyID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"role_policy_id"`
	RoleID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_role_policy" json:"role_id"`
	PolicyID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_role_policy" json:"policy_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Role   Role   `gorm:"foreignKey:RoleID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Policy Policy `gorm:"foreignKey:PolicyID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (rp *RolePolicy) BeforeCreate(tx *gorm.DB) error {
	if rp.RolePolicyID == uuid.Nil {
		rp.RolePolicyID = uuid.New()
	}
	return nil
}

// ============================================
// RBAC EVENT
// Audit log written by rbac_service.go RBACAuditService.
// Captures grants, denials, privilege escalation attempts,
// wildcard abuse, policy changes, and role assignments.
// ============================================

type RBACEvent struct {
	EventID  uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"event_id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`

	// ActorID is the user performing the action
	ActorID uuid.UUID `gorm:"type:uuid;not null;index" json:"actor_id"`

	// TargetID is the user being acted upon — nil for policy/role-level events
	TargetID *uuid.UUID `gorm:"type:uuid;index" json:"target_id,omitempty"`

	// EventType matches RBACEventType constants in rbac_service.go
	// e.g. "permission_denied", "role_assigned", "privilege_escalation_attempt"
	EventType string `gorm:"type:varchar(100);not null;index" json:"event_type"`

	// Severity matches RBACEventSeverity: "info", "warning", "critical"
	Severity string `gorm:"type:varchar(50);not null" json:"severity"`

	// Metadata holds event-specific detail map serialized as jsonb
	// e.g. {"resource": "payment", "action": "approve", "deny_reason": "..."}
	Metadata []byte `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Actor  User   `gorm:"foreignKey:ActorID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Target *User  `gorm:"foreignKey:TargetID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"-"`
}

func (e *RBACEvent) BeforeCreate(tx *gorm.DB) error {
	if e.EventID == uuid.Nil {
		e.EventID = uuid.New()
	}
	return nil
}