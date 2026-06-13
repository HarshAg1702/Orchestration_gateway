package rediscache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/harshagae/orchestration-gateway/internal/models"
)

type Cache struct {
	client *goredis.Client
	ttl    time.Duration
}

func New(addr string, ttlHours int) *Cache {
	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	return &Cache{client: rdb, ttl: time.Duration(ttlHours) * time.Hour}
}

func (c *Cache) key(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("prompt:%x", h)
}

func (c *Cache) Get(ctx context.Context, prompt string) (*models.CachedResponse, error) {
	key := c.key(prompt)
	slog.Info("[redis] looking up cache", "key", key, "prompt", prompt)

	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		slog.Info("[redis] cache miss", "key", key)
		return nil, err
	}
	var cr models.CachedResponse
	if err := json.Unmarshal(val, &cr); err != nil {
		slog.Error("[redis] failed to unmarshal cached value", "key", key, "err", err)
		return nil, err
	}
	slog.Info("[redis] cache hit", "key", key, "model", cr.Model)
	return &cr, nil
}

func (c *Cache) Set(ctx context.Context, prompt string, cr *models.CachedResponse) error {
	key := c.key(prompt)
	b, err := json.Marshal(cr)
	if err != nil {
		return err
	}
	slog.Info("[redis] storing response", "key", key, "model", cr.Model, "ttl", c.ttl)
	return c.client.Set(ctx, key, b, c.ttl).Err()
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
