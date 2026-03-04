package cache

import (
	"context"
	"encoding/json"
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
