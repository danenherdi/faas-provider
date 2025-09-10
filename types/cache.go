package types

import (
	"context"
	"time"

	paperClient "github.com/PaperCache/paper-client-go"
	"github.com/redis/go-redis/v9"
)

// CacheClient defines the required methods for our caching implementation.
type CacheClient interface {
	Get(ctx context.Context, key string) ([]byte, error)
	SetEx(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// RedisClientWrapper implements CacheClient for redis.Client.
type RedisClientWrapper struct {
	client *redis.Client
}

func (r *RedisClientWrapper) Get(ctx context.Context, key string) ([]byte, error) {
	return r.client.Get(ctx, key).Bytes()
}

func (r *RedisClientWrapper) SetEx(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.SetEx(ctx, key, value, ttl).Err()
}

// PaperCacheClientWrapper implements CacheClient for paperClient.PaperClient.
type PaperCacheClientWrapper struct {
	client *paperClient.PaperClient
}

func (p *PaperCacheClientWrapper) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := p.client.Get(key)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (p *PaperCacheClientWrapper) SetEx(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return p.client.Set(key, string(value), uint32(ttl.Seconds()))
}
