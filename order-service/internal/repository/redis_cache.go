package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"order-service/internal/domain"

	"github.com/redis/go-redis/v9"
)

// RedisOrderCache is a Redis-backed implementation of domain.OrderCache.
type RedisOrderCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisOrderCache(client *redis.Client, ttl time.Duration) *RedisOrderCache {
	return &RedisOrderCache{client: client, ttl: ttl}
}

func (r *RedisOrderCache) cacheKey(id string) string {
	return fmt.Sprintf("order:%s", id)
}

// Get retrieves an order from the cache.
// Returns redis.Nil wrapped in an error when the key is absent.
func (r *RedisOrderCache) Get(id string) (*domain.Order, error) {
	ctx := context.Background()
	data, err := r.client.Get(ctx, r.cacheKey(id)).Bytes()
	if err != nil {
		return nil, err // caller checks for redis.Nil
	}
	var order domain.Order
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("cache: unmarshal order %s: %w", id, err)
	}
	return &order, nil
}

// Set stores an order in the cache with the configured TTL.
func (r *RedisOrderCache) Set(order *domain.Order) error {
	ctx := context.Background()
	data, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("cache: marshal order %s: %w", order.ID, err)
	}
	return r.client.Set(ctx, r.cacheKey(order.ID), data, r.ttl).Err()
}

// Delete removes an order from the cache (invalidation on mutation).
func (r *RedisOrderCache) Delete(id string) error {
	ctx := context.Background()
	return r.client.Del(ctx, r.cacheKey(id)).Err()
}
