package service

import (
	"context"
	"fmt"
	"log"
	"sync"

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
