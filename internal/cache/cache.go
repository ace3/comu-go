package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client with typed get/set helpers.
type Cache struct {
	client *redis.Client
}

// New creates a Cache from a Redis URL, falling back to localhost if parsing fails.
func New(redisURL string) *Cache {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		opt = &redis.Options{Addr: "localhost:6379"}
	}
	return &Cache{client: redis.NewClient(opt)}
}

// Client returns the underlying Redis client for health checks and direct access.
func (c *Cache) Client() *redis.Client {
	return c.client
}

// TTLToMidnight returns the duration from now until midnight WIB (GMT+7).
func TTLToMidnight() time.Duration {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	return time.Until(midnight)
}

// Get unmarshals a cached JSON value into dest. Returns redis.Nil on cache miss.
func (c *Cache) Get(ctx context.Context, key string, dest any) error {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

// Set marshals value to JSON and stores it with the given TTL.
func (c *Cache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// IsNil reports whether err is a Redis cache miss.
func (c *Cache) IsNil(err error) bool {
	return err == redis.Nil
}

// InvalidateAll deletes all cached keys matching station:*, schedule:*, and route:*
// using SCAN + DEL. Returns the number of keys deleted.
func (c *Cache) InvalidateAll(ctx context.Context) int {
	patterns := []string{"station:*", "schedule:*", "route:*"}
	total := 0

	for _, pattern := range patterns {
		var cursor uint64
		for {
			keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				slog.Error("cache scan error", "pattern", pattern, "error", err)
				break
			}
			if len(keys) > 0 {
				if err := c.client.Del(ctx, keys...).Err(); err != nil {
					slog.Error("cache delete error", "error", err)
				} else {
					total += len(keys)
				}
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}

	slog.Info("cache invalidated", "keys", total)
	return total
}
