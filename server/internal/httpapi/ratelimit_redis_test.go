//go:build integration

// Correr: TEST_REDIS_URL="redis://localhost:6380/0" go test -tags=integration ./internal/httpapi/...
// Sin la env se omite (Skip). Prueba el backend REAL de Redis — lo que los tests planos (sin
// build tag) no pueden, porque ahí rl.rdb siempre es nil.
package httpapi

import (
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func testRedisURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("TEST_REDIS_URL no definido; omitiendo tests de integración de rate limit")
	}
	return u
}

// limiterKeyPrefix da un prefijo único por test (basado en el nombre) para que corran en
// paralelo sobre el mismo Redis sin pisarse claves entre sí.
func newRedisLimiter(t *testing.T, max int, window time.Duration) *rateLimiter {
	t.Helper()
	rl := newRateLimiter(testRedisURL(t), "test:"+t.Name()+":", max, window)
	if rl.rdb == nil {
		t.Fatal("se esperaba backend Redis (revisa TEST_REDIS_URL)")
	}
	t.Cleanup(func() {
		ctx := t.Context()
		keys, _ := rl.rdb.Keys(ctx, rl.prefix+"*").Result()
		if len(keys) > 0 {
			rl.rdb.Del(ctx, keys...)
		}
		rl.rdb.Close()
	})
	return rl
}

func TestRateLimiterRedis_BlocksAfterMax(t *testing.T) {
	rl := newRedisLimiter(t, 3, time.Minute)
	ctx := t.Context()
	const key = "1.2.3.4"

	for i := 0; i < 3; i++ {
		if rl.blocked(ctx, key) {
			t.Fatalf("attempt %d should be allowed", i)
		}
		rl.record(ctx, key)
	}
	if !rl.blocked(ctx, key) {
		t.Fatal("expected key to be blocked after reaching max attempts")
	}
}

func TestRateLimiterRedis_ResetClearsCounter(t *testing.T) {
	rl := newRedisLimiter(t, 1, time.Minute)
	ctx := t.Context()
	const key = "login:admin"

	rl.record(ctx, key)
	if !rl.blocked(ctx, key) {
		t.Fatal("expected blocked")
	}
	rl.reset(ctx, key)
	if rl.blocked(ctx, key) {
		t.Fatal("reset must clear the counter")
	}
}

func TestRateLimiterRedis_WindowExpiresViaTTL(t *testing.T) {
	rl := newRedisLimiter(t, 1, 2*time.Second)
	ctx := t.Context()
	const key = "user:kate"

	rl.record(ctx, key)
	if !rl.blocked(ctx, key) {
		t.Fatal("expected blocked at limit")
	}
	if ra := rl.retryAfter(ctx, key); ra <= 0 || ra > 3 {
		t.Fatalf("retryAfter debería estar entre 1 y 3s, dio %d", ra)
	}
	time.Sleep(2200 * time.Millisecond) // pasa la ventana real (TTL de Redis)
	if rl.blocked(ctx, key) {
		t.Fatal("expected the key to expire on its own via Redis TTL")
	}
}

// TestRateLimiterRedis_SharedAcrossInstances es la razón de ser de todo esto: dos
// *rateLimiter construidos por separado (= dos réplicas de la API) contra el MISMO Redis y
// prefijo deben ver el mismo contador. Con el fallback in-memory esto sería imposible de probar
// (cada instancia tendría su propio mapa) — así se distingue de un simple "funciona con Redis".
func TestRateLimiterRedis_SharedAcrossInstances(t *testing.T) {
	url := testRedisURL(t)
	prefix := "test:" + t.Name() + ":"
	t.Cleanup(func() {
		opt, _ := redis.ParseURL(url)
		rdb := redis.NewClient(opt)
		defer rdb.Close()
		ctx := t.Context()
		if keys, _ := rdb.Keys(ctx, prefix+"*").Result(); len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
	})

	replicaA := newRateLimiter(url, prefix, 3, time.Minute)
	replicaB := newRateLimiter(url, prefix, 3, time.Minute)
	defer replicaA.rdb.Close()
	defer replicaB.rdb.Close()
	ctx := t.Context()
	const key = "shared-ip"

	replicaA.record(ctx, key)
	replicaB.record(ctx, key)
	replicaA.record(ctx, key)
	// 3 intentos repartidos entre ambas réplicas → la SIGUIENTE (en cualquiera) debe bloquear.
	if !replicaB.blocked(ctx, key) {
		t.Fatal("replicaB debería ver bloqueado el key: el contador vive en Redis, no en memoria de A")
	}
	if !replicaA.blocked(ctx, key) {
		t.Fatal("replicaA también debe verlo bloqueado (mismo contador compartido)")
	}
}

func TestRateLimiterRedis_FailsOpenWhenDown(t *testing.T) {
	// Redis en un puerto que nadie escucha: blocked() no debe bloquear (fail-open), solo
	// registrar el error — nunca tumbar login/recuperación porque el cache esté caído.
	rl := newRateLimiter("redis://localhost:1/0", "test:"+t.Name()+":", 1, time.Minute)
	if rl.rdb == nil {
		t.Fatal("se esperaba un cliente redis (aunque no conecte) para probar el fail-open")
	}
	defer rl.rdb.Close()
	ctx := t.Context()
	if rl.blocked(ctx, "x") {
		t.Fatal("con Redis caído, blocked() debe fallar abierto (false), no bloquear a todos")
	}
}
