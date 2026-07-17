package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const menuKey = "pos:menu"
const menuTTL = 24 * time.Hour

// Popularidad: read model aparte con TTL corto (read-through). Sin cron: refresca
// solo cuando expira y alguien la pide. El front además hace refetch periódico,
// así un POS abierto ve el "Top" al día en minutos.
const popularKey = "pos:popular"
const popularTTL = 5 * time.Minute

// MenuCache guarda el documento del menú ya serializado. Si rdb es nil (Redis no
// configurado), degrada a no-cache: Get siempre falla y el resto es no-op.
type MenuCache struct {
	rdb *redis.Client
}

func NewMenuCache(redisURL string) *MenuCache {
	if redisURL == "" {
		return &MenuCache{}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return &MenuCache{}
	}
	return &MenuCache{rdb: redis.NewClient(opt)}
}

func (c *MenuCache) Get(ctx context.Context) ([]byte, bool) {
	if c.rdb == nil {
		return nil, false
	}
	b, err := c.rdb.Get(ctx, menuKey).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *MenuCache) Set(ctx context.Context, doc []byte) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Set(ctx, menuKey, doc, menuTTL).Err()
}

// Invalidate borra el menú cacheado (llamar tras cualquier cambio de catálogo).
func (c *MenuCache) Invalidate(ctx context.Context) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, menuKey).Err()
}

func (c *MenuCache) GetPopular(ctx context.Context) ([]byte, bool) {
	if c.rdb == nil {
		return nil, false
	}
	b, err := c.rdb.Get(ctx, popularKey).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *MenuCache) SetPopular(ctx context.Context, doc []byte) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Set(ctx, popularKey, doc, popularTTL).Err()
}
