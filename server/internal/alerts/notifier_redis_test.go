package alerts

// Prueba el agrupamiento compartido contra un Redis REAL (TEST_REDIS_URL; CI lo define). Sin la
// variable se omite: los tests en memoria no ven lo que pasa entre dos procesos.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func testRedis(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_REDIS_URL")
	if u == "" {
		t.Skip("TEST_REDIS_URL no definido; omitiendo el agrupamiento compartido en Redis")
	}
	opt, err := redis.ParseURL(u)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	defer func() { _ = rdb.Close() }()
	ctx := context.Background()
	keys, _ := rdb.Keys(ctx, "pos:alert:*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	return u
}

// Azul/verde: dos procesos vivos ven el mismo error. Un solo aviso inmediato, y el resumen del
// dueño de la ventana suma las repeticiones de los dos.
func TestTwoProcessesShareTheWindow(t *testing.T) {
	u := testRedis(t)
	blue, green := &inbox{}, &inbox{}
	nb := newTestNotifier(t, blue, Options{RedisURL: u, Window: 400 * time.Millisecond})
	ng := newTestNotifier(t, green, Options{RedisURL: u, Window: 400 * time.Millisecond})
	a := alertFor("/api/v1/x", "pg:XX000")
	nb.Report(a)
	waitFor(t, "aviso de azul", time.Second, func() bool { return len(blue.all()) == 1 })
	for range 3 {
		ng.Report(a)
		nb.Report(a)
	}
	time.Sleep(100 * time.Millisecond)
	if n := len(green.all()); n != 0 {
		t.Fatalf("verde mandó %d avisos de una ventana que ganó azul", n)
	}
	waitFor(t, "resumen de azul", 2*time.Second, func() bool { return len(blue.all()) == 2 })
	if s := blue.all()[1].subject; !strings.Contains(s, "repetido 6 veces") {
		t.Fatalf("el resumen no suma las repeticiones de los dos procesos: %q", s)
	}
	if n := len(green.all()); n != 0 {
		t.Fatalf("verde mandó %d correos", n)
	}
}

// Un proceso que murió sin cerrar su ventana deja repeticiones en Redis; el siguiente aviso de ese
// error las reporta en vez de perderlas.
func TestOrphanedRepeatsAreReportedByTheNextWindow(t *testing.T) {
	u := testRedis(t)
	a := alertFor("/api/v1/y", "pg:XX001")
	opt, _ := redis.ParseURL(u)
	rdb := redis.NewClient(opt)
	defer func() { _ = rdb.Close() }()
	rdb.Set(context.Background(), countKey(domain.AlertKey(a)), 7, time.Hour)

	b := &inbox{}
	n := newTestNotifier(t, b, Options{RedisURL: u, Window: time.Hour})
	n.Report(a)
	waitFor(t, "aviso", time.Second, func() bool { return len(b.all()) == 1 })
	if !strings.Contains(b.all()[0].body, "7 anteriores sin reportar") {
		t.Fatalf("no reportó las repeticiones huérfanas: %s", b.all()[0].body)
	}
}
