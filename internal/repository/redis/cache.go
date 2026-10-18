package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
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
