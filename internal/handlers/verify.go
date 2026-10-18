package handlers

import (
	"encoding/json"
	"net/http"

	"tierguard/internal/domain"
	"tierguard/internal/service"
)

type VerifyHandler struct {
	redisSvc *service.RedisService
}

func NewVerifyHandler(redisSvc *service.RedisService) *VerifyHandler {
	return &VerifyHandler{
		redisSvc: redisSvc,
	}
}

func (h *VerifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, domain.VerifyResponse{
			Allowed: false,
			Reason:  "invalid_request_payload",
		})
		return
	}

	if req.TenantID == "" || req.RequestedFeature == "" {
		writeJSON(w, http.StatusBadRequest, domain.VerifyResponse{
			Allowed: false,
			Reason:  "missing_tenant_id_or_feature",
		})
		return
	}

	ctx := r.Context()

	// 1. Check feature required tier level from Redis cache (NOT Postgres)
	reqLevel, found, err := h.redisSvc.Cache().GetFeature(ctx, req.RequestedFeature)
	if err != nil || !found {
		writeJSON(w, http.StatusForbidden, domain.VerifyResponse{
			Allowed: false,
			Reason:  "quota_exceeded",
		})
		return
	}

	// 2. Check tenant and assigned tier from Redis cache (NOT Postgres)
	cachedTenant, err := h.redisSvc.Cache().GetTenant(ctx, req.TenantID)
	if err != nil || cachedTenant == nil {
		writeJSON(w, http.StatusForbidden, domain.VerifyResponse{
			Allowed: false,
			Reason:  "quota_exceeded",
		})
		return
	}

	// 3. Verify tenant tier level meets required feature tier level
	if cachedTenant.HierarchyLevel < reqLevel {
		writeJSON(w, http.StatusForbidden, domain.VerifyResponse{
			Allowed: false,
			Reason:  "quota_exceeded",
		})
		return
	}

	// 4. Verify tenant has available quota in current month in Redis cache
	month := domain.CurrentMonth()
	currentUsage, err := h.redisSvc.Cache().GetUsage(ctx, req.TenantID, month)
	if err != nil || currentUsage >= cachedTenant.MaxMonthlyRequests {
		writeJSON(w, http.StatusForbidden, domain.VerifyResponse{
			Allowed: false,
			Reason:  "quota_exceeded",
		})
		return
	}

	writeJSON(w, http.StatusOK, domain.VerifyResponse{
		Allowed: true,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
