package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"tierguard/internal/domain"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/repository/redis"
)

const (
	DefaultUsageStreamName = "stream:usage_logs"
)

type RedisService struct {
	cache      *redis.Cache
	streamName string
}

// NewRedisService initializes a Redis client connection using go-redis.
func NewRedisService(redisURL string) (*RedisService, error) {
	cache, err := redis.NewCache(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis service: %w", err)
	}
	return &RedisService{
		cache:      cache,
		streamName: DefaultUsageStreamName,
	}, nil
}

func (s *RedisService) Close() error {
	return s.cache.Close()
}

func (s *RedisService) Cache() *redis.Cache {
	return s.cache
}

// TestConnection verifies connectivity to Redis and returns the round-trip latency.
func (s *RedisService) TestConnection(ctx context.Context) (time.Duration, error) {
	latency, err := s.cache.Ping(ctx)
	if err != nil {
		return 0, fmt.Errorf("redis connection test failed: %w", err)
	}
	return latency, nil
}

// LoadTenantsAndQuotasFromPostgres loads all tenant quotas and tiers from Postgres into Redis memory.
// This guarantees sub-5 millisecond authorization lookups without Postgres round-trips.
func (s *RedisService) LoadTenantsAndQuotasFromPostgres(ctx context.Context, pgRepo *postgres.Repository) (int, int, error) {
	startTime := time.Now()

	// 1. Fetch all features and cache in Redis memory
	features, err := pgRepo.GetAllFeatures(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch features from postgres: %w", err)
	}
	for _, f := range features {
		if err := s.cache.SetFeature(ctx, f.Name, f.RequiredTierLevel); err != nil {
			return 0, 0, fmt.Errorf("failed to cache feature %s: %w", f.Name, err)
		}
	}

	// 2. Fetch all tenants from Postgres
	tenants, err := pgRepo.GetAllTenants(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch tenants from postgres: %w", err)
	}

	currentMonth := domain.CurrentMonth()
	loadedCount := 0

	for _, t := range tenants {
		_, tier, err := pgRepo.GetTenantWithTier(ctx, t.ID)
		if err != nil || tier == nil {
			log.Printf("[RedisService] Skipping tenant %s: tier not found", t.ID)
			continue
		}

		cached := domain.CachedTenantData{
			TenantID:           t.ID.String(),
			TierID:             t.TierID.String(),
			TierName:           tier.Name,
			HierarchyLevel:     tier.HierarchyLevel,
			MaxMonthlyRequests: tier.MaxMonthlyRequests,
		}

		// Store in Redis memory
		if err := s.cache.SetTenant(ctx, cached); err != nil {
			return 0, 0, fmt.Errorf("failed to store tenant %s in redis memory: %w", t.ID, err)
		}

		// Load current month usage from Postgres into Redis memory
		usage, err := pgRepo.GetUsage(ctx, t.ID, currentMonth)
		if err == nil {
			_ = s.cache.SetUsage(ctx, t.ID.String(), currentMonth, usage)
		}

		loadedCount++
	}

	elapsed := time.Since(startTime)
	log.Printf("[RedisService] Successfully loaded %d tenants and %d features into Redis memory in %v",
		loadedCount, len(features), elapsed)

	return loadedCount, len(features), nil
}

// IncrementTenantUsageStream increments a specific tenant's usage counter in Redis Streams.
// It also updates the atomic memory counter in Redis so subsequent authorization checks resolve in <5ms.
func (s *RedisService) IncrementTenantUsageStream(ctx context.Context, tenantID string, delta int64) (string, int64, error) {
	month := domain.CurrentMonth()
	streamMsgID, newCount, err := s.cache.IncrementUsageWithStream(ctx, s.streamName, tenantID, month, delta)
	if err != nil {
		return "", 0, fmt.Errorf("failed to increment tenant usage in redis streams: %w", err)
	}
	return streamMsgID, newCount, nil
}
