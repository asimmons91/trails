package cache

import (
	"context"
	"time"
)

type Store interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exist(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
}
