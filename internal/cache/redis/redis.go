package rediscache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"crypto/sha256"

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
	val, err := c.client.Get(ctx, c.key(prompt)).Bytes()
	if err != nil {
		return nil, err
	}
	var cr models.CachedResponse
	if err := json.Unmarshal(val, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

func (c *Cache) Set(ctx context.Context, prompt string, cr *models.CachedResponse) error {
	b, err := json.Marshal(cr)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(prompt), b, c.ttl).Err()
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
