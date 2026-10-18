package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"tierguard/internal/domain"
)

type Cache struct {
	client *redis.Client
}

func NewCache(redisURL string) (*Cache, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		opt = &redis.Options{
			Addr: redisURL,
		}
	}

	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &Cache{client: client}, nil
}

func (c *Cache) Client() *redis.Client {
	return c.client
}

func (c *Cache) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	val, err := c.client.Ping(ctx).Result()
	duration := time.Since(start)
	if err != nil {
		return duration, err
	}
	if val != "PONG" {
		return duration, fmt.Errorf("unexpected ping response: %s", val)
	}
	return duration, nil
}

func (c *Cache) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// SetTenant caches tenant metadata for sub-5ms lookup.
func (c *Cache) SetTenant(ctx context.Context, tenant domain.CachedTenantData) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "tenant:"+tenant.TenantID, data, 0).Err()
}

// GetTenant retrieves cached tenant metadata.
func (c *Cache) GetTenant(ctx context.Context, tenantID string) (*domain.CachedTenantData, error) {
	val, err := c.client.Get(ctx, "tenant:"+tenantID).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var tenant domain.CachedTenantData
	if err := json.Unmarshal([]byte(val), &tenant); err != nil {
		return nil, err
	}
	return &tenant, nil
}

// SetFeature caches a feature's required tier level.
func (c *Cache) SetFeature(ctx context.Context, featureName string, requiredLevel int) error {
	return c.client.Set(ctx, "feature:"+featureName, requiredLevel, 0).Err()
}

// GetFeature retrieves a feature's required tier level.
func (c *Cache) GetFeature(ctx context.Context, featureName string) (int, bool, error) {
	val, err := c.client.Get(ctx, "feature:"+featureName).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}
	lvl, err := strconv.Atoi(val)
	if err != nil {
		return 0, false, err
	}
	return lvl, true, nil
}
