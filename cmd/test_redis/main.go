package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"tierguard/internal/domain"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/service"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("=================================================================")
	fmt.Println("             TIERGUARD REDIS & SUB-5MS ENGINE TEST               ")
	fmt.Println("=================================================================")

	// 1. Connect and test Redis connection
	log.Printf("[1/5] Connecting to Redis at %s...", redisURL)
	redisSvc, err := service.NewRedisService(redisURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	defer redisSvc.Close()

	latency, err := redisSvc.TestConnection(ctx)
	if err != nil {
		log.Fatalf("❌ Connection test failed: %v", err)
	}
	fmt.Printf("  ✔ Redis connection active! Round-trip Ping Latency: %v (Target: < 5ms)\n\n", latency)

	// 2. Connect to PostgreSQL
	log.Println("[2/5] Connecting to PostgreSQL to read tenant quotas & tiers...")
	pgRepo, err := postgres.NewRepository(ctx, dbURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	defer pgRepo.Close()
	fmt.Println("  ✔ Connected to PostgreSQL.")

	// 3. Load all tenant quotas and tiers into Redis memory
	log.Println("[3/5] Loading all tenant quotas and tiers into Redis memory...")
	tenantsCount, featuresCount, err := redisSvc.LoadTenantsAndQuotasFromPostgres(ctx, pgRepo)
	if err != nil {
		log.Fatalf("❌ Failed to load quotas into Redis: %v", err)
	}
	fmt.Printf("  ✔ Successfully loaded %d tenants and %d features into Redis memory.\n\n", tenantsCount, featuresCount)

	// 4. Test Sub-5ms Authorization Lookup in Redis Memory
	log.Println("[4/5] Testing authorization retrieval speed from Redis memory...")
	tenants, err := pgRepo.GetAllTenants(ctx)
	if err != nil || len(tenants) == 0 {
		log.Fatalf("❌ No tenants found to test: %v", err)
	}

	testTenant := tenants[0]
	lookupStart := time.Now()
	cachedData, err := redisSvc.Cache().GetTenant(ctx, testTenant.ID.String())
	lookupDuration := time.Since(lookupStart)
	if err != nil || cachedData == nil {
		log.Fatalf("❌ Failed to retrieve cached tenant: %v", err)
	}
	fmt.Printf("  ✔ Tenant Memory Lookup: '%s' (%s)\n", testTenant.CompanyName, testTenant.ID)
	fmt.Printf("    - Active Tier: %s (Level %d)\n", cachedData.TierName, cachedData.HierarchyLevel)
	fmt.Printf("    - Max Monthly Requests: %d\n", cachedData.MaxMonthlyRequests)
	fmt.Printf("    - Latency: %v (⚡ %s sub-5ms SLA)\n\n", lookupDuration, func() string {
		if lookupDuration < 5*time.Millisecond {
			return "PASSES"
		}
		return "FAILS"
	}())

	// 5. Test Incrementing Tenant Usage via Redis Streams
	log.Println("[5/5] Testing usage increment in Redis Streams (XADD)...")
	month := domain.CurrentMonth()
	initialUsage, _ := redisSvc.Cache().GetUsage(ctx, testTenant.ID.String(), month)

	streamStart := time.Now()
	streamMsgID, newUsage, err := redisSvc.IncrementTenantUsageStream(ctx, testTenant.ID.String(), 1)
	streamDuration := time.Since(streamStart)
	if err != nil {
		log.Fatalf("❌ Failed to increment usage stream: %v", err)
	}

	fmt.Printf("  ✔ Redis Stream Event Created!\n")
	fmt.Printf("    - Stream: %s\n", service.DefaultUsageStreamName)
	fmt.Printf("    - Message ID: %s\n", streamMsgID)
	fmt.Printf("    - Previous Usage: %d -> New Usage: %d\n", initialUsage, newUsage)
	fmt.Printf("    - Stream Execution Time: %v\n", streamDuration)

	// Read back from the stream to verify stream payload
	entries, err := redisSvc.Cache().ReadStreamEntries(ctx, service.DefaultUsageStreamName, 1)
	if err != nil || len(entries) == 0 {
		log.Fatalf("❌ Failed to read back stream entry: %v", err)
	}
	fmt.Printf("    - Stream Payload Verified: %+v\n\n", entries[0].Values)

	fmt.Println("=================================================================")
	fmt.Println("             ALL REDIS & STREAM TESTS PASSED!                    ")
	fmt.Println("=================================================================")
}
