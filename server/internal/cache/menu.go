package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const menuTTL = 24 * time.Hour

// Popularidad: read model aparte con TTL corto (read-through). Sin cron: refresca
// solo cuando expira y alguien la pide. El front además hace refetch periódico,
// así un POS abierto ve el "Top" al día en minutos.
const popularTTL = 5 * time.Minute

// menuSchema: versión de la FORMA del documento del menú. Se sube cuando el documento gana o
// cambia campos, y eso basta para que el caché viejo quede huérfano y expire solo.
//
// Sin esto, un deploy que agrega un campo sigue sirviendo el documento viejo hasta que venza el
// TTL — 24 horas de un POS sin el campo nuevo, sin nada que lo delate. Pasó al agregar las listas
// de precios por plataforma: el selector no aparecía y la base sí tenía las plataformas.
//
// v2: agrega platforms, platformPrices y platformModPrices (spec 002).
const menuSchema = "v3"

// Claves namespaced por empresa: el menú/popularidad de una empresa NUNCA se sirve a otra
// (aislamiento multi-tenant también en la caché). company_id viene del JWT, no del cliente.
func menuKey(companyID int64) string {
	return fmt.Sprintf("pos:menu:%s:%d", menuSchema, companyID)
}
func popularKey(companyID int64) string { return fmt.Sprintf("pos:popular:%d", companyID) }

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

func (c *MenuCache) Get(ctx context.Context, companyID int64) ([]byte, bool) {
	if c.rdb == nil {
		return nil, false
	}
	b, err := c.rdb.Get(ctx, menuKey(companyID)).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *MenuCache) Set(ctx context.Context, companyID int64, doc []byte) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Set(ctx, menuKey(companyID), doc, menuTTL).Err()
}

// Invalidate borra el menú cacheado de la empresa (llamar tras cualquier cambio de catálogo).
func (c *MenuCache) Invalidate(ctx context.Context, companyID int64) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, menuKey(companyID)).Err()
}

func (c *MenuCache) GetPopular(ctx context.Context, companyID int64) ([]byte, bool) {
	if c.rdb == nil {
		return nil, false
	}
	b, err := c.rdb.Get(ctx, popularKey(companyID)).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *MenuCache) SetPopular(ctx context.Context, companyID int64, doc []byte) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Set(ctx, popularKey(companyID), doc, popularTTL).Err()
}

// InvalidatePopular borra el read model de popularidad cacheado de la empresa.
func (c *MenuCache) InvalidatePopular(ctx context.Context, companyID int64) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, popularKey(companyID)).Err()
}
