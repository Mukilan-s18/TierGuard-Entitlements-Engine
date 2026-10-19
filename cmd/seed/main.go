package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"

	"tierguard/internal/domain"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/repository/redis"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("[Seed] Connecting to PostgreSQL...")
	repo, err := postgres.NewRepository(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer repo.Close()

	log.Println("[Seed] Inserting standard tiers (Free, Pro, Enterprise)...")
	mockTiers := []domain.Tier{
		{
			ID:                 uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Name:               "Free",
			HierarchyLevel:     1,
			MaxMonthlyRequests: 100,
		},
		{
			ID:                 uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Name:               "Pro",
			HierarchyLevel:     2,
			MaxMonthlyRequests: 10000,
		},
		{
			ID:                 uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			Name:               "Enterprise",
			HierarchyLevel:     3,
			MaxMonthlyRequests: 100000,
		},
	}

	tierMap := make(map[string]domain.Tier)
	for _, mt := range mockTiers {
		t, err := repo.UpsertTier(ctx, mt)
		if err != nil {
			log.Fatalf("Failed to upsert tier %s: %v", mt.Name, err)
		}
		tierMap[t.Name] = t
		fmt.Printf("  Tier: %s (Level %d, Max Requests: %d, ID: %s)\n", t.Name, t.HierarchyLevel, t.MaxMonthlyRequests, t.ID)
	}

	log.Println("[Seed] Inserting features...")
	mockFeatures := []domain.Feature{
		{Name: "basic_analytics", RequiredTierLevel: 1},
		{Name: "export_reports", RequiredTierLevel: 2},
		{Name: "custom_webhooks", RequiredTierLevel: 3},
	}
	for _, mf := range mockFeatures {
		f, err := repo.UpsertFeature(ctx, mf)
		if err != nil {
			log.Fatalf("Failed to upsert feature %s: %v", mf.Name, err)
		}
		fmt.Printf("  Feature: %s (Required Level: %d, ID: %s)\n", f.Name, f.RequiredTierLevel, f.ID)
	}

	log.Println("[Seed] Inserting mock tenants with real-world usage...")
	currentMonth := domain.CurrentMonth()

	type TenantSeed struct {
		ID          uuid.UUID
		CompanyName string
		TierName    string
		UsageCount  int
	}

	seeds := []TenantSeed{
		{
			ID:          uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
			CompanyName: "Tenant A",
			TierName:    "Free",
			UsageCount:  0, // Clean baseline for live demo simulation
		},
		{
			ID:          uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
			CompanyName: "Starlight Labs",
			TierName:    "Free",
			UsageCount:  85, // 85% Warning threshold
		},
		{
			ID:          uuid.MustParse("88888888-8888-8888-8888-888888888888"),
			CompanyName: "Startup Inc",
			TierName:    "Free",
			UsageCount:  105, // Quota exceeded threshold
		},
		{
			ID:          uuid.MustParse("77777777-7777-7777-7777-777777777777"),
			CompanyName: "Basecamp Digital",
			TierName:    "Pro",
			UsageCount:  3850, // 38% Normal healthy Pro tier
		},
		{
			ID:          uuid.MustParse("99999999-9999-9999-9999-999999999999"),
			CompanyName: "Mega Enterprises",
			TierName:    "Enterprise",
			UsageCount:  64280, // 64% Normal healthy Enterprise tier
		},
	}

	for _, s := range seeds {
		tier := tierMap[s.TierName]
		existing, _, _ := repo.GetTenantWithTier(ctx, s.ID)
		if existing == nil {
			t := domain.Tenant{
				ID:          s.ID,
				CompanyName: s.CompanyName,
				TierID:      tier.ID,
			}
			_, err := repo.CreateTenant(ctx, t)
			if err != nil {
				log.Fatalf("Failed to create tenant %s: %v", s.CompanyName, err)
			}
			fmt.Printf("  Created Tenant: %s (Tier: %s)\n", s.CompanyName, s.TierName)
		} else {
			_ = repo.UpdateTenant(ctx, s.ID, s.CompanyName, tier.ID)
			fmt.Printf("  Updated Tenant: %s (Tier: %s)\n", s.CompanyName, s.TierName)
		}

		// Reset and set current month usage in PostgreSQL
		_ = repo.SetUsage(ctx, s.ID, currentMonth, s.UsageCount)
	}

	// Directly synchronize Redis cache so database and memory counters are immediately aligned
	log.Println("[Seed] Syncing Redis in-memory cache...")
	cache, err := redis.NewCache(redisURL)
	if err != nil {
		log.Printf("[Seed] Warning: failed to connect to Redis: %v", err)
	} else {
		defer cache.Close()

		for _, f := range mockFeatures {
			_ = cache.SetFeature(ctx, f.Name, f.RequiredTierLevel)
		}

		for _, s := range seeds {
			tier := tierMap[s.TierName]
			cached := domain.CachedTenantData{
				TenantID:           s.ID.String(),
				TierID:             tier.ID.String(),
				TierName:           tier.Name,
				HierarchyLevel:     tier.HierarchyLevel,
				MaxMonthlyRequests: tier.MaxMonthlyRequests,
			}
			_ = cache.SetTenant(ctx, cached)
			_ = cache.SetUsage(ctx, s.ID.String(), currentMonth, s.UsageCount)
		}
		log.Println("[Seed] Redis cache synced successfully.")
	}

	log.Println("[Seed] Database seeding completed.")
}
