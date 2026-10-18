package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"tierguard/internal/domain"
	"tierguard/internal/service"
)

type AdminHandler struct {
	entitlementService *service.EntitlementService
	tmpl               *template.Template
}

func NewAdminHandler(entitlementService *service.EntitlementService, tmplPath string) (*AdminHandler, error) {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template file %s: %w", tmplPath, err)
	}

	return &AdminHandler{
		entitlementService: entitlementService,
		tmpl:               tmpl,
	}, nil
}

type DashboardPageData struct {
	Tenants           []domain.TenantDashboardView
	Tiers             []domain.Tier
	SuccessMsg        string
	ErrorMsg          string
	LastUpdated       string
	PostgresConnected bool
	RedisConnected    bool
}

func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenants, tiers, err := h.entitlementService.GetDashboardData(r.Context())
	if err != nil {
		log.Printf("[AdminHandler] Error fetching dashboard data: %v", err)
		http.Error(w, "Failed to load dashboard data", http.StatusInternalServerError)
		return
	}

	data := DashboardPageData{
		Tenants:           tenants,
		Tiers:             tiers,
		LastUpdated:       time.Now().UTC().Format("15:04:05 UTC"),
		PostgresConnected: true,
		RedisConnected:    true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// If requested via HTMX partial refresh (e.g. Refresh button), return ONLY the tenant-table partial
	if r.Header.Get("HX-Request") == "true" {
		if err := h.tmpl.ExecuteTemplate(w, "tenant-table", data); err != nil {
			log.Printf("[AdminHandler] Error rendering tenant-table partial: %v", err)
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
		}
		return
	}

	if err := h.tmpl.ExecuteTemplate(w, "dashboard", data); err != nil {
		log.Printf("[AdminHandler] Error rendering dashboard template: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func (h *AdminHandler) UpdateTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	tenantIDStr := strings.TrimSpace(r.FormValue("tenant_id"))
	tierIDStr := strings.TrimSpace(r.FormValue("tier_id"))

	if tenantIDStr == "" || tierIDStr == "" {
		http.Error(w, "Both tenant_id and tier_id are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	tenants, tiers, err := h.entitlementService.GetDashboardData(ctx)
	if err != nil {
		log.Printf("[AdminHandler] Failed to load dashboard data: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// 1. Resolve Tenant UUID (supports either UUID or Company Name)
	var tenantUUID uuid.UUID
	var companyName string
	parsedTenantUUID, err := uuid.Parse(tenantIDStr)
	if err == nil {
		tenantUUID = parsedTenantUUID
		for _, t := range tenants {
			if t.TenantID == tenantUUID {
				companyName = t.CompanyName
				break
			}
		}
	} else {
		// Search by company name
		for _, t := range tenants {
			if strings.EqualFold(t.CompanyName, tenantIDStr) {
				tenantUUID = t.TenantID
				companyName = t.CompanyName
				break
			}
		}
	}

	if tenantUUID == uuid.Nil {
		http.Error(w, fmt.Sprintf("Tenant '%s' not found", tenantIDStr), http.StatusNotFound)
		return
	}

	// 2. Resolve Tier UUID (supports either UUID or Tier Name like "Free", "Pro", "Enterprise")
	var tierUUID uuid.UUID
	var newTierName string
	parsedTierUUID, err := uuid.Parse(tierIDStr)
	if err == nil {
		tierUUID = parsedTierUUID
		for _, t := range tiers {
			if t.ID == tierUUID {
				newTierName = t.Name
				break
			}
		}
	} else {
		// Search by tier name
		for _, t := range tiers {
			if strings.EqualFold(t.Name, tierIDStr) {
				tierUUID = t.ID
				newTierName = t.Name
				break
			}
		}
	}

	if tierUUID == uuid.Nil {
		http.Error(w, fmt.Sprintf("Tier '%s' not found", tierIDStr), http.StatusNotFound)
		return
	}

	// 3. Perform atomic update in PostgreSQL AND Redis
	if err := h.entitlementService.UpdateTenantTier(ctx, tenantUUID, tierUUID); err != nil {
		log.Printf("[AdminHandler] Failed to update tenant tier: %v", err)
		http.Error(w, "Failed to update tier in database/cache", http.StatusInternalServerError)
		return
	}

	log.Printf("[AdminHandler] Tenant '%s' (%s) updated to tier '%s' (%s) in Postgres & Redis",
		companyName, tenantUUID, newTierName, tierUUID)

	// 4. Reload updated tenant records
	updatedTenants, updatedTiers, err := h.entitlementService.GetDashboardData(ctx)
	if err != nil {
		http.Error(w, "Failed to reload updated records", http.StatusInternalServerError)
		return
	}

	data := DashboardPageData{
		Tenants:           updatedTenants,
		Tiers:             updatedTiers,
		SuccessMsg:        fmt.Sprintf("%s subscription tier updated to %s.", companyName, newTierName),
		LastUpdated:       time.Now().UTC().Format("15:04:05 UTC"),
		PostgresConnected: true,
		RedisConnected:    true,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// If request came from HTMX, render the tenant-table partial
	if r.Header.Get("HX-Request") == "true" {
		if err := h.tmpl.ExecuteTemplate(w, "tenant-table", data); err != nil {
			log.Printf("[AdminHandler] Error executing tenant-table partial: %v", err)
			http.Error(w, "Template execution error", http.StatusInternalServerError)
		}
		return
	}

	// Fallback to full page redirect if non-HTMX
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
