package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tierguard/internal/domain"
	"tierguard/internal/handlers"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/service"
)

func TestAdminHandler_DashboardAndTierUpdate(t *testing.T) {
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pgRepo, err := postgres.NewRepository(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgRepo.Close()

	redisSvc, err := service.NewRedisService(redisURL)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisSvc.Close()

	entitlementSvc := service.NewEntitlementService(pgRepo, redisSvc.Cache())
	_ = entitlementSvc.WarmUpCache(ctx)

	adminHandler, err := handlers.NewAdminHandler(entitlementSvc, "../../web/templates/dashboard.html")
	if err != nil {
		t.Fatalf("Failed to initialize AdminHandler: %v", err)
	}

	// 1. Test GET /admin renders complete dashboard HTML
	t.Run("GET_Admin_Renders_Dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()

		adminHandler.Dashboard(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected HTTP 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "TierGuard") {
			t.Errorf("Dashboard missing header title")
		}
		if !strings.Contains(body, "hx-post=\"/admin/update-tier\"") {
			t.Errorf("Dashboard missing HTMX update tier form")
		}
	})

	// 2. Test POST /admin/update-tier with HTMX updates Postgres and Redis
	t.Run("POST_UpdateTier_Updates_Postgres_And_Redis", func(t *testing.T) {
		// Find a tenant on Free tier (e.g. Starlight Labs)
		tenants, err := pgRepo.GetAllTenants(ctx)
		if err != nil || len(tenants) == 0 {
			t.Fatalf("No tenants found in Postgres: %v", err)
		}

		freeTier, err := pgRepo.GetTierByName(ctx, "Free")
		if err != nil || freeTier == nil {
			t.Fatalf("Free tier not found")
		}
		proTier, err := pgRepo.GetTierByName(ctx, "Pro")
		if err != nil || proTier == nil {
			t.Fatalf("Pro tier not found")
		}

		var targetTenant domain.Tenant
		for _, tenant := range tenants {
			if tenant.TierID == freeTier.ID {
				targetTenant = tenant
				break
			}
		}

		// If no Free tenant, set the first tenant to Free initially
		if targetTenant.ID == uuid.Nil {
			targetTenant = tenants[0]
			_ = pgRepo.UpdateTenantTier(ctx, targetTenant.ID, freeTier.ID)
		}

		// Perform HTMX Tier Update: Free -> Pro
		formData := url.Values{}
		formData.Set("tenant_id", targetTenant.ID.String())
		formData.Set("tier_id", "Pro") // tests name resolution ("Pro")

		req := httptest.NewRequest(http.MethodPost, "/admin/update-tier", strings.NewReader(formData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		rr := httptest.NewRecorder()

		adminHandler.UpdateTier(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected HTTP 200 for HTMX update, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		respBody := rr.Body.String()
		if !strings.Contains(respBody, "Database and cache synced") {
			t.Errorf("Expected success notice in HTMX partial response, got: %s", respBody)
		}

		// VERIFY 1: Postgres was updated
		_, updatedTierInDB, err := pgRepo.GetTenantWithTier(ctx, targetTenant.ID)
		if err != nil || updatedTierInDB == nil {
			t.Fatalf("Failed to fetch tenant from DB: %v", err)
		}
		if updatedTierInDB.Name != "Pro" {
			t.Errorf("Expected tier in PostgreSQL to be 'Pro', got '%s'", updatedTierInDB.Name)
		}

		// VERIFY 2: Redis cache was updated
		cachedInRedis, err := redisSvc.Cache().GetTenant(ctx, targetTenant.ID.String())
		if err != nil || cachedInRedis == nil {
			t.Fatalf("Failed to fetch tenant from Redis cache: %v", err)
		}
		if cachedInRedis.TierName != "Pro" {
			t.Errorf("Expected tier in Redis to be 'Pro', got '%s'", cachedInRedis.TierName)
		}
		if cachedInRedis.HierarchyLevel != 2 {
			t.Errorf("Expected hierarchy level 2 in Redis, got %d", cachedInRedis.HierarchyLevel)
		}
		if cachedInRedis.MaxMonthlyRequests != proTier.MaxMonthlyRequests {
			t.Errorf("Expected max monthly requests %d in Redis, got %d", proTier.MaxMonthlyRequests, cachedInRedis.MaxMonthlyRequests)
		}

		t.Logf("✔ Successfully verified Postgres and Redis both updated to Pro tier for %s", targetTenant.CompanyName)
	})
}
