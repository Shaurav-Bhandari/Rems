// services/inventory/Vendor.go
//
// Shared types for the inventory package — mirrors the RBACAuthorizer,
// AccessRequest, ResourceType and ActionType definitions from the business
// package so the inventory package can perform RBAC checks without an import
// cycle. *core.RBACService satisfies both interfaces.
package inventory

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"backend/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// RBAC INTERFACE (mirrors business.RBACAuthorizer)
// ─────────────────────────────────────────────────────────────────────────────

// RBACAuthorizer is the subset of RBACService that inventory services need.
type RBACAuthorizer interface {
	Require(ctx context.Context, req *AccessRequest) error
}

// AccessRequest is the authorization query.
type AccessRequest struct {
	UserID     uuid.UUID
	TenantID   uuid.UUID
	Resource   ResourceType
	Action     ActionType
	ResourceID *uuid.UUID
}

// ─────────────────────────────────────────────────────────────────────────────
// RESOURCE & ACTION TYPES
// ─────────────────────────────────────────────────────────────────────────────

type ResourceType string
type ActionType string

const (
	ActionCreate ActionType = "create"
	ActionRead   ActionType = "read"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
)

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

// mustMarshalJSONB converts any value to models.JSONB (map[string]interface{}).
// models.JSONB is NOT []byte — it is map[string]interface{} with its own
// driver.Valuer that marshals to JSON when GORM writes to Postgres.
func mustMarshalJSONB(v interface{}) models.JSONB {
	b, err := json.Marshal(v)
	if err != nil {
		return models.JSONB{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return models.JSONB{}
	}
	return models.JSONB(m)
}
