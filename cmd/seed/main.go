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
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")

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
			MaxMonthlyRequests: 1000,
		},
		{
			ID:                 uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Name:               "Pro",
			HierarchyLevel:     2,
			MaxMonthlyRequests: 50000,
		},
		{
			ID:                 uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			Name:               "Enterprise",
			HierarchyLevel:     3,
			MaxMonthlyRequests: 1000000,
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
			CompanyName: "Acme Corp",
			TierName:    "Pro",
			UsageCount:  38450,
		},
		{
			ID:          uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
			CompanyName: "Starlight Labs",
			TierName:    "Free",
			UsageCount:  850,
		},
		{
			ID:          uuid.MustParse("88888888-8888-8888-8888-888888888888"),
			CompanyName: "Startup Inc",
			TierName:    "Free",
			UsageCount:  420,
		},
		{
			ID:          uuid.MustParse("77777777-7777-7777-7777-777777777777"),
			CompanyName: "Basecamp Digital",
			TierName:    "Pro",
			UsageCount:  50000,
		},
		{
			ID:          uuid.MustParse("99999999-9999-9999-9999-999999999999"),
			CompanyName: "Mega Enterprises",
			TierName:    "Enterprise",
			UsageCount:  642800,
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
			_ = repo.UpdateTenantTier(ctx, s.ID, tier.ID)
			fmt.Printf("  Updated Tenant: %s (Tier: %s)\n", s.CompanyName, s.TierName)
		}

		// Reset and set current month usage
		_ = repo.SetUsage(ctx, s.ID, currentMonth, s.UsageCount)
	}

	log.Println("[Seed] Database seeding completed.")
}
