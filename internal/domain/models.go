package domain

import (
	"time"

	"github.com/google/uuid"
)

// Tier represents a subscription level with hierarchy and request limits.
type Tier struct {
	ID                 uuid.UUID `json:"id" db:"id"`
	Name               string    `json:"name" db:"name"`
	HierarchyLevel     int       `json:"hierarchy_level" db:"hierarchy_level"`
	MaxMonthlyRequests int       `json:"max_monthly_requests" db:"max_monthly_requests"`
}

// Feature represents a system capability requiring a minimum tier hierarchy level.
type Feature struct {
	ID                uuid.UUID `json:"id" db:"id"`
	Name              string    `json:"name" db:"name"`
	RequiredTierLevel int       `json:"required_tier_level" db:"required_tier_level"`
}

// Tenant represents a customer company assigned to a specific tier.
type Tenant struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CompanyName string    `json:"company_name" db:"company_name"`
	TierID      uuid.UUID `json:"tier_id" db:"tier_id"`
}

// UsageLog records monthly request count per tenant.
type UsageLog struct {
	ID           uuid.UUID `json:"id" db:"id"`
	TenantID     uuid.UUID `json:"tenant_id" db:"tenant_id"`
	RequestCount int       `json:"request_count" db:"request_count"`
	Month        string    `json:"month" db:"month"` // Format: YYYY-MM
}

// TenantDashboardView represents aggregated tenant details for the admin view.
type TenantDashboardView struct {
	TenantID           uuid.UUID
	CompanyName        string
	TierName           string
	HierarchyLevel     int
	MaxMonthlyRequests int
	CurrentUsage       int
	UsagePercentage    float64
}

// VerifyRequest is the payload sent to POST /api/verify.
type VerifyRequest struct {
	TenantID         string `json:"tenant_id"`
	RequestedFeature string `json:"requested_feature"`
}

// VerifyResponse is the result returned by POST /api/verify.
type VerifyResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// CachedTenantData stores tenant and tier data in Redis for sub-5ms lookups.
type CachedTenantData struct {
	TenantID           string `json:"tenant_id"`
	TierID             string `json:"tier_id"`
	TierName           string `json:"tier_name"`
	HierarchyLevel     int    `json:"hierarchy_level"`
	MaxMonthlyRequests int    `json:"max_monthly_requests"`
}

// CurrentMonth returns current year and month formatted as YYYY-MM.
func CurrentMonth() string {
	return time.Now().UTC().Format("2006-01")
}
