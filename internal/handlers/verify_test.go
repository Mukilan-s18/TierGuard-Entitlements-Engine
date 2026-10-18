package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"tierguard/internal/domain"
	"tierguard/internal/handlers"
	"tierguard/internal/service"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func setupTestRedis(t testing.TB) *service.RedisService {
	t.Helper()
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	redisSvc, err := service.NewRedisService(redisURL)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	return redisSvc
}

func TestVerifyHandler_ValidAndQuota(t *testing.T) {
	redisSvc := setupTestRedis(t)
	defer redisSvc.Close()

	ctx := context.Background()
	handler := handlers.NewVerifyHandler(redisSvc)

	// Preload Redis memory
	tenantID := "test-tenant-alpha"
	featureName := "export_reports"
	month := domain.CurrentMonth()

	_ = redisSvc.Cache().SetFeature(ctx, featureName, 2)
	_ = redisSvc.Cache().SetTenant(ctx, domain.CachedTenantData{
		TenantID:           tenantID,
		TierID:             "tier-pro-uuid",
		TierName:           "Pro",
		HierarchyLevel:     2,
		MaxMonthlyRequests: 500,
	})
	_ = redisSvc.Cache().SetUsage(ctx, tenantID, month, 10)

	// 1. Valid Request Test
	t.Run("ValidRequest_Returns200_And_StreamsAsync", func(t *testing.T) {
		body, _ := json.Marshal(domain.VerifyRequest{
			TenantID:         tenantID,
			RequestedFeature: featureName,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		start := time.Now()
		handler.ServeHTTP(rr, req)
		elapsed := time.Since(start)

		if rr.Code != http.StatusOK {
			t.Fatalf("Expected HTTP 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res domain.VerifyResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		if !res.Allowed {
			t.Errorf("Expected allowed: true, got false")
		}

		t.Logf("Valid request executed in: %v", elapsed)

		// Wait briefly for asynchronous goroutine to complete Redis Stream write
		time.Sleep(50 * time.Millisecond)

		// Verify usage incremented in Redis memory
		updatedUsage, err := redisSvc.Cache().GetUsage(ctx, tenantID, month)
		if err != nil {
			t.Fatalf("Failed to check updated usage: %v", err)
		}
		if updatedUsage != 11 {
			t.Errorf("Expected usage 11 after async stream increment, got %d", updatedUsage)
		}

		// Verify event was appended to Redis Stream
		entries, err := redisSvc.Cache().ReadStreamEntries(ctx, service.DefaultUsageStreamName, 5)
		if err != nil || len(entries) == 0 {
			t.Fatalf("Expected stream entries in %s, got none: %v", service.DefaultUsageStreamName, err)
		}
		found := false
		for _, e := range entries {
			if e.Values["tenant_id"] == tenantID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Did not find tenant_id in recent stream entries")
		}
	})

	// 2. Quota Exceeded Test
	t.Run("QuotaExceeded_Returns403", func(t *testing.T) {
		exceededTenantID := "test-tenant-exceeded"
		_ = redisSvc.Cache().SetTenant(ctx, domain.CachedTenantData{
			TenantID:           exceededTenantID,
			TierName:           "Free",
			HierarchyLevel:     2,
			MaxMonthlyRequests: 100,
		})
		_ = redisSvc.Cache().SetUsage(ctx, exceededTenantID, month, 100) // already at max quota

		body, _ := json.Marshal(domain.VerifyRequest{
			TenantID:         exceededTenantID,
			RequestedFeature: featureName,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("Expected HTTP 403, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res domain.VerifyResponse
		_ = json.Unmarshal(rr.Body.Bytes(), &res)
		if res.Allowed {
			t.Errorf("Expected allowed: false, got true")
		}
		if res.Reason != "quota_exceeded" {
			t.Errorf("Expected reason: quota_exceeded, got '%s'", res.Reason)
		}
	})

	// 3. Insufficient Tier Level Test
	t.Run("InsufficientTier_Returns403", func(t *testing.T) {
		lowTierTenantID := "test-tenant-lowtier"
		_ = redisSvc.Cache().SetTenant(ctx, domain.CachedTenantData{
			TenantID:           lowTierTenantID,
			TierName:           "Free",
			HierarchyLevel:     1, // feature requires 2
			MaxMonthlyRequests: 1000,
		})
		_ = redisSvc.Cache().SetUsage(ctx, lowTierTenantID, month, 0)

		body, _ := json.Marshal(domain.VerifyRequest{
			TenantID:         lowTierTenantID,
			RequestedFeature: featureName,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("Expected HTTP 403, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res domain.VerifyResponse
		_ = json.Unmarshal(rr.Body.Bytes(), &res)
		if res.Allowed {
			t.Errorf("Expected allowed: false, got true")
		}
		if res.Reason != "quota_exceeded" {
			t.Errorf("Expected reason: quota_exceeded, got '%s'", res.Reason)
		}
	})
}

// TestVerifyHandler_PureExecutionSpeed measures handler execution time directly across 100 requests
func TestVerifyHandler_PureExecutionSpeed(t *testing.T) {
	redisSvc := setupTestRedis(t)
	defer redisSvc.Close()

	ctx := context.Background()
	handler := handlers.NewVerifyHandler(redisSvc)

	tenantID := "speed-tenant"
	featureName := "export_reports"
	month := domain.CurrentMonth()

	_ = redisSvc.Cache().SetFeature(ctx, featureName, 1)
	_ = redisSvc.Cache().SetTenant(ctx, domain.CachedTenantData{
		TenantID:           tenantID,
		TierName:           "Pro",
		HierarchyLevel:     2,
		MaxMonthlyRequests: 1000000,
	})
	_ = redisSvc.Cache().SetUsage(ctx, tenantID, month, 0)

	payload, _ := json.Marshal(domain.VerifyRequest{
		TenantID:         tenantID,
		RequestedFeature: featureName,
	})

	const iterations = 100
	var totalTime time.Duration
	var maxTime time.Duration

	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		start := time.Now()
		handler.ServeHTTP(rr, req)
		elapsed := time.Since(start)

		if rr.Code != http.StatusOK {
			t.Fatalf("Request failed with status %d", rr.Code)
		}

		totalTime += elapsed
		if elapsed > maxTime {
			maxTime = elapsed
		}
	}

	avgTime := totalTime / iterations
	t.Logf("=== Pure Handler Speed Test (%d Requests) ===", iterations)
	t.Logf("  Average Handler Time: %v (Target: < 5ms)", avgTime)
	t.Logf("  Max Handler Time:     %v", maxTime)

	if avgTime >= 5*time.Millisecond {
		t.Errorf("Average handler time %v exceeded 5ms SLA", avgTime)
	}
	if maxTime >= 5*time.Millisecond {
		t.Errorf("Max handler time %v exceeded 5ms SLA", maxTime)
	}

	// Give async streaming goroutines time to finish before closing connection
	time.Sleep(100 * time.Millisecond)
}

// TestVerifyHandler_HammerSub5ms sends concurrent rapid requests and verifies latency SLA (<5ms).
func TestVerifyHandler_HammerSub5ms(t *testing.T) {
	redisSvc := setupTestRedis(t)
	defer redisSvc.Close()

	ctx := context.Background()
	handler := handlers.NewVerifyHandler(redisSvc)

	tenantID := "hammer-tenant"
	featureName := "basic_analytics"
	month := domain.CurrentMonth()

	_ = redisSvc.Cache().SetFeature(ctx, featureName, 1)
	_ = redisSvc.Cache().SetTenant(ctx, domain.CachedTenantData{
		TenantID:           tenantID,
		TierName:           "Enterprise",
		HierarchyLevel:     3,
		MaxMonthlyRequests: 10000000,
	})
	_ = redisSvc.Cache().SetUsage(ctx, tenantID, month, 0)

	const totalRequests = 200
	const concurrentWorkers = 20

	reqBody, _ := json.Marshal(domain.VerifyRequest{
		TenantID:         tenantID,
		RequestedFeature: featureName,
	})

	var wg sync.WaitGroup
	reqChan := make(chan struct{}, totalRequests)
	for i := 0; i < totalRequests; i++ {
		reqChan <- struct{}{}
	}
	close(reqChan)

	latencies := make([]time.Duration, totalRequests)
	var mu sync.Mutex
	idx := 0

	server := httptest.NewServer(handler)
	defer server.Close()

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: concurrentWorkers,
		},
	}

	for w := 0; w < concurrentWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqChan {
				req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/verify", bytes.NewReader(reqBody))
				req.Header.Set("Content-Type", "application/json")

				start := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(start)

				if err != nil {
					t.Errorf("HTTP request failed: %v", err)
					return
				}
				_ = resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Errorf("Expected status 200, got %d", resp.StatusCode)
				}

				mu.Lock()
				latencies[idx] = elapsed
				idx++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Calculate statistics
	var totalDuration time.Duration
	var maxDuration time.Duration
	var minDuration time.Duration = 10 * time.Minute
	exceeded5msCount := 0

	for _, lat := range latencies {
		totalDuration += lat
		if lat > maxDuration {
			maxDuration = lat
		}
		if lat < minDuration {
			minDuration = lat
		}
		if lat >= 5*time.Millisecond {
			exceeded5msCount++
		}
	}

	avgDuration := totalDuration / time.Duration(totalRequests)

	t.Logf("=== Hammer Test Summary (%d Requests, %d Workers) ===", totalRequests, concurrentWorkers)
	t.Logf("  Average Latency: %v", avgDuration)
	t.Logf("  Min Latency:     %v", minDuration)
	t.Logf("  Max Latency:     %v", maxDuration)
	t.Logf("  Requests >= 5ms: %d / %d", exceeded5msCount, totalRequests)

	if avgDuration >= 5*time.Millisecond {
		t.Fatalf("FAILED: Average latency %v exceeded 5ms SLA", avgDuration)
	}

	// Give goroutines time to stream
	time.Sleep(100 * time.Millisecond)

	finalUsage, _ := redisSvc.Cache().GetUsage(ctx, tenantID, month)
	t.Logf("  Final stream incremented usage in Redis: %d / %d requests", finalUsage, totalRequests)
}
