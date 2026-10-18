package service_test

import (
	"context"
	"os"
	"testing"
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

func TestRedisService_ConnectionAndStreams(t *testing.T) {
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Connection Test
	redisSvc, err := service.NewRedisService(redisURL)
	if err != nil {
		t.Fatalf("Failed to initialize RedisService: %v", err)
	}
	defer redisSvc.Close()

	latency, err := redisSvc.TestConnection(ctx)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if latency > 5*time.Millisecond {
		t.Logf("Warning: latency %v is near 5ms threshold", latency)
	}

	// 2. Load from Postgres
	pgRepo, err := postgres.NewRepository(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgRepo.Close()

	tenantsCount, _, err := redisSvc.LoadTenantsAndQuotasFromPostgres(ctx, pgRepo)
	if err != nil {
		t.Fatalf("LoadTenantsAndQuotasFromPostgres failed: %v", err)
	}
	if tenantsCount == 0 {
		t.Fatalf("Expected at least 1 tenant loaded, got 0")
	}

	// 3. Test Redis Streams Increment
	tenants, err := pgRepo.GetAllTenants(ctx)
	if err != nil || len(tenants) == 0 {
		t.Fatalf("No tenants available for stream test")
	}

	testTenantID := tenants[0].ID.String()
	streamMsgID, newCount, err := redisSvc.IncrementTenantUsageStream(ctx, testTenantID, 1)
	if err != nil {
		t.Fatalf("IncrementTenantUsageStream failed: %v", err)
	}
	if streamMsgID == "" {
		t.Fatalf("Expected non-empty stream message ID")
	}
	if newCount <= 0 {
		t.Fatalf("Expected positive usage count, got %d", newCount)
	}

	// 4. Verify Stream Entry
	entries, err := redisSvc.Cache().ReadStreamEntries(ctx, service.DefaultUsageStreamName, 1)
	if err != nil {
		t.Fatalf("Failed to read stream entries: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("Expected stream entries, got none")
	}
	if entries[0].Values["tenant_id"] != testTenantID {
		t.Errorf("Stream tenant_id mismatch: expected %s, got %v", testTenantID, entries[0].Values["tenant_id"])
	}

	// 5. Verify Sub-5ms Authorization Latency
	start := time.Now()
	cached, err := redisSvc.Cache().GetTenant(ctx, testTenantID)
	elapsed := time.Since(start)
	if err != nil || cached == nil {
		t.Fatalf("Failed to get cached tenant: %v", err)
	}
	if elapsed >= 5*time.Millisecond {
		t.Errorf("Authorization retrieval took %v, exceeded 5ms SLA", elapsed)
	}
	t.Logf("Sub-5ms check passed: retrieved tenant in %v", elapsed)

	// Verify month format helper
	if domain.CurrentMonth() == "" {
		t.Errorf("CurrentMonth returned empty string")
	}
}
