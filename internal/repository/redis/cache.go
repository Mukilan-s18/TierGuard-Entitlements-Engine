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

// IncrementUsage increments the memory request counter in Redis.
func (c *Cache) IncrementUsage(ctx context.Context, tenantID, month string) (int64, error) {
	key := fmt.Sprintf("usage:%s:%s", tenantID, month)
	return c.client.Incr(ctx, key).Result()
}

// IncrementUsageBy increments memory counter by delta.
func (c *Cache) IncrementUsageBy(ctx context.Context, tenantID, month string, delta int64) (int64, error) {
	key := fmt.Sprintf("usage:%s:%s", tenantID, month)
	return c.client.IncrBy(ctx, key, delta).Result()
}

// GetUsage returns current monthly usage from Redis memory.
func (c *Cache) GetUsage(ctx context.Context, tenantID, month string) (int, error) {
	key := fmt.Sprintf("usage:%s:%s", tenantID, month)
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SetUsage sets the usage counter in Redis memory.
func (c *Cache) SetUsage(ctx context.Context, tenantID, month string, count int) error {
	key := fmt.Sprintf("usage:%s:%s", tenantID, month)
	return c.client.Set(ctx, key, count, 0).Err()
}

// AddUsageStreamEvent appends a usage increment event to a Redis Stream.
func (c *Cache) AddUsageStreamEvent(ctx context.Context, streamName string, tenantID, month string, delta int64) (string, error) {
	return c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]interface{}{
			"tenant_id": tenantID,
			"month":     month,
			"delta":     delta,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}).Result()
}

// IncrementUsageWithStream atomically increments the Redis memory counter and appends to Redis Streams.
func (c *Cache) IncrementUsageWithStream(ctx context.Context, streamName, tenantID, month string, delta int64) (string, int64, error) {
	pipe := c.client.TxPipeline()
	key := fmt.Sprintf("usage:%s:%s", tenantID, month)
	incrCmd := pipe.IncrBy(ctx, key, delta)
	xaddCmd := pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]interface{}{
			"tenant_id": tenantID,
			"month":     month,
			"delta":     delta,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})

	if _, err := pipe.Exec(ctx); err != nil {
		return "", 0, fmt.Errorf("redis stream pipeline failed: %w", err)
	}

	newCount := incrCmd.Val()
	streamMsgID := xaddCmd.Val()
	return streamMsgID, newCount, nil
}

// ReadStreamEntries reads recent entries from a Redis Stream for verification/consumption.
func (c *Cache) ReadStreamEntries(ctx context.Context, streamName string, count int64) ([]redis.XMessage, error) {
	streams, err := c.client.XRevRangeN(ctx, streamName, "+", "-", count).Result()
	if err != nil {
		return nil, err
	}
	return streams, nil
}
