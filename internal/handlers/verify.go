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

	writeJSON(w, http.StatusOK, domain.VerifyResponse{
		Allowed: true,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
