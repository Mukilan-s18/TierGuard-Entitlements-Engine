package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"tierguard/internal/domain"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/repository/redis"
)

type EntitlementService struct {
	pgRepo     *postgres.Repository
	redisCache *redis.Cache
	syncMu     sync.RWMutex
}

func NewEntitlementService(pgRepo *postgres.Repository, redisCache *redis.Cache) *EntitlementService {
	return &EntitlementService{
		pgRepo:     pgRepo,
		redisCache: redisCache,
	}
}

// CheckHealth queries both PostgreSQL and Redis to verify active connectivity.
func (s *EntitlementService) CheckHealth(ctx context.Context) (bool, bool) {
	pgConnected := false
	redisConnected := false

	if s.pgRepo != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		if err := s.pgRepo.Ping(pingCtx); err == nil {
			pgConnected = true
		}
		cancel()
	}

	if s.redisCache != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		if _, err := s.redisCache.Ping(pingCtx); err == nil {
			redisConnected = true
		}
		cancel()
	}

	return pgConnected, redisConnected
}

// WarmUpCache loads tiers, features, tenants, and current month usage from Postgres into Redis on startup.
func (s *EntitlementService) WarmUpCache(ctx context.Context) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	log.Println("[Sync] Warming up Redis cache from PostgreSQL...")

	// 1. Cache features
	features, err := s.pgRepo.GetAllFeatures(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch features from postgres: %w", err)
	}
	for _, f := range features {
		if err := s.redisCache.SetFeature(ctx, f.Name, f.RequiredTierLevel); err != nil {
			return fmt.Errorf("failed to cache feature %s: %w", f.Name, err)
		}
	}

	// 2. Cache tenants with tier details
	tenants, err := s.pgRepo.GetAllTenants(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch tenants from postgres: %w", err)
	}

	currentMonth := domain.CurrentMonth()
	for _, t := range tenants {
		_, tier, err := s.pgRepo.GetTenantWithTier(ctx, t.ID)
		if err != nil || tier == nil {
			continue
		}

		cached := domain.CachedTenantData{
			TenantID:           t.ID.String(),
			TierID:             t.TierID.String(),
			TierName:           tier.Name,
			HierarchyLevel:     tier.HierarchyLevel,
			MaxMonthlyRequests: tier.MaxMonthlyRequests,
		}
		if err := s.redisCache.SetTenant(ctx, cached); err != nil {
			return fmt.Errorf("failed to cache tenant %s: %w", t.ID, err)
		}

		// Sync current month usage
		usage, err := s.pgRepo.GetUsage(ctx, t.ID, currentMonth)
		if err == nil {
			_ = s.redisCache.SetUsage(ctx, t.ID.String(), currentMonth, usage)
		}
	}

	log.Printf("[Sync] Cache warm-up complete: %d features, %d tenants loaded", len(features), len(tenants))
	return nil
}

// Verify evaluates entitlement and quota against Redis (sub-5ms target).
func (s *EntitlementService) Verify(ctx context.Context, tenantIDStr, featureName string) (bool, string, error) {
	// 1. Check feature requirement from Redis
	reqLevel, found, err := s.redisCache.GetFeature(ctx, featureName)
	if err != nil {
		return false, "cache_error", err
	}
	if !found {
		// Fallback to DB if missing in cache
		features, fErr := s.pgRepo.GetAllFeatures(ctx)
		if fErr == nil {
			for _, f := range features {
				_ = s.redisCache.SetFeature(ctx, f.Name, f.RequiredTierLevel)
				if f.Name == featureName {
					reqLevel = f.RequiredTierLevel
					found = true
				}
			}
		}
		if !found {
			return false, "feature_not_found", nil
		}
	}

	// 2. Check tenant and active tier from Redis
	cachedTenant, err := s.redisCache.GetTenant(ctx, tenantIDStr)
	if err != nil {
		return false, "cache_error", err
	}
	if cachedTenant == nil {
		// Fallback to DB
		tenantUUID, parseErr := uuid.Parse(tenantIDStr)
		if parseErr != nil {
			return false, "invalid_tenant_id", nil
		}
		t, tier, dbErr := s.pgRepo.GetTenantWithTier(ctx, tenantUUID)
		if dbErr != nil || t == nil || tier == nil {
			return false, "tenant_not_found", nil
		}
		cached := domain.CachedTenantData{
			TenantID:           t.ID.String(),
			TierID:             t.TierID.String(),
			TierName:           tier.Name,
			HierarchyLevel:     tier.HierarchyLevel,
			MaxMonthlyRequests: tier.MaxMonthlyRequests,
		}
		_ = s.redisCache.SetTenant(ctx, cached)
		cachedTenant = &cached
	}

	// 3. Hierarchy level check
	if cachedTenant.HierarchyLevel < reqLevel {
		return false, "tier_hierarchy_insufficient", nil
	}

	// 4. Monthly quota check
	month := domain.CurrentMonth()
	currentUsage, err := s.redisCache.GetUsage(ctx, tenantIDStr, month)
	if err != nil {
		return false, "cache_error", err
	}

	if currentUsage >= cachedTenant.MaxMonthlyRequests {
		return false, "quota_exceeded", nil
	}

	// 5. Fire async goroutine to increment usage count in Redis and asynchronously persist to Postgres
	go s.asyncIncrementUsage(tenantIDStr, month)

	return true, "", nil
}

func (s *EntitlementService) asyncIncrementUsage(tenantIDStr, month string) {
	// Increment in Redis immediately
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := s.redisCache.IncrementUsage(ctx, tenantIDStr, month)
	if err != nil {
		log.Printf("[AsyncUsage] Failed to increment Redis usage for %s: %v", tenantIDStr, err)
	}

	// Asynchronously record in PostgreSQL
	tenantUUID, err := uuid.Parse(tenantIDStr)
	if err == nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dbCancel()
		if dbErr := s.pgRepo.IncrementUsage(dbCtx, tenantUUID, month, 1); dbErr != nil {
			log.Printf("[AsyncUsage] Failed to persist usage in DB for %s: %v", tenantIDStr, dbErr)
		}
	}
}

// UpdateTenantTier updates PostgreSQL and pushes the new tier data to Redis.
func (s *EntitlementService) UpdateTenantTier(ctx context.Context, tenantID, tierID uuid.UUID) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	// Update PostgreSQL
	if err := s.pgRepo.UpdateTenantTier(ctx, tenantID, tierID); err != nil {
		return fmt.Errorf("failed to update tenant tier in postgres: %w", err)
	}

	// Fetch updated details from Postgres
	tenant, tier, err := s.pgRepo.GetTenantWithTier(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to reload tenant after update: %w", err)
	}

	// Push directly into Redis
	cached := domain.CachedTenantData{
		TenantID:           tenant.ID.String(),
		TierID:             tenant.TierID.String(),
		TierName:           tier.Name,
		HierarchyLevel:     tier.HierarchyLevel,
		MaxMonthlyRequests: tier.MaxMonthlyRequests,
	}
	if err := s.redisCache.SetTenant(ctx, cached); err != nil {
		return fmt.Errorf("failed to update tenant in redis: %w", err)
	}

	return nil
}

func (s *EntitlementService) GetDashboardData(ctx context.Context) ([]domain.TenantDashboardView, []domain.Tier, error) {
	month := domain.CurrentMonth()
	tenants, err := s.pgRepo.GetDashboardView(ctx, month)
	if err != nil {
		return nil, nil, err
	}

	// Ensure current usage reflects Redis if higher
	for i := range tenants {
		rUsage, rErr := s.redisCache.GetUsage(ctx, tenants[i].TenantID.String(), month)
		if rErr == nil && rUsage > tenants[i].CurrentUsage {
			tenants[i].CurrentUsage = rUsage
			if tenants[i].MaxMonthlyRequests > 0 {
				tenants[i].UsagePercentage = (float64(rUsage) / float64(tenants[i].MaxMonthlyRequests)) * 100.0
				if tenants[i].UsagePercentage > 100.0 {
					tenants[i].UsagePercentage = 100.0
				}
			}
		}
	}

	tiers, err := s.pgRepo.GetAllTiers(ctx)
	if err != nil {
		return nil, nil, err
	}

	return tenants, tiers, nil
}
