package alerts

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Las ventanas viven en Redis para que los dos procesos de una subida azul/verde no avisen dos veces
// del mismo error. Redis nunca es fuente de verdad aquí (constitución I): si falla, el notifier
// sigue en memoria.

const (
	// windowGrace: la llave vive un poco más que el timer del dueño, para que nadie más gane la
	// ventana entre que el timer dispara y el dueño decide si la renueva.
	windowGrace = 30 * time.Second
	countTTL    = time.Hour
	redisOpWait = time.Second
)

var errNoRedis = errors.New("redis no configurado")

// releaseScript borra la ventana solo si sigue siendo de este proceso.
var releaseScript = redis.NewScript(`if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) end return 0`)

func winKey(key string) string   { return "pos:alert:win:" + key }
func countKey(key string) string { return "pos:alert:count:" + key }

func newRedis(url string) *redis.Client {
	if url == "" {
		return nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil
	}
	// Plazos cortos: un Redis colgado atrasa al goroutine de avisos, nunca a un request, pero
	// mientras espera la cola se llena y descarta.
	opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = 500*time.Millisecond, 500*time.Millisecond, 500*time.Millisecond
	opt.MaxRetries = 0
	return redis.NewClient(opt)
}

func (n *Notifier) op() (context.Context, context.CancelFunc) {
	return context.WithTimeout(n.ctx, redisOpWait)
}

// claim intenta ganar la ventana. Si la gana, se lleva las repeticiones que dejó un proceso que
// murió sin reportarlas.
func (n *Notifier) claim(key string) (won bool, orphaned int64, err error) {
	if n.rdb == nil {
		return false, 0, errNoRedis
	}
	ctx, cancel := n.op()
	defer cancel()
	won, err = n.rdb.SetNX(ctx, winKey(key), n.procID, n.o.Window+windowGrace).Result()
	if err != nil {
		return false, 0, err
	}
	if !won {
		return false, 0, nil
	}
	orphaned, err = n.rdb.GetDel(ctx, countKey(key)).Int64()
	if errors.Is(err, redis.Nil) {
		err = nil
	}
	return true, orphaned, err
}

func (n *Notifier) incr(key string) error {
	if n.rdb == nil {
		return errNoRedis
	}
	ctx, cancel := n.op()
	defer cancel()
	pipe := n.rdb.TxPipeline()
	pipe.Incr(ctx, countKey(key))
	pipe.Expire(ctx, countKey(key), countTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (n *Notifier) takeCount(key string) (int64, error) {
	ctx, cancel := n.op()
	defer cancel()
	c, err := n.rdb.GetDel(ctx, countKey(key)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return c, err
}

// extend renueva la ventana solo si la llave sigue existiendo; si ya venció, otro proceso pudo
// ganarla y este deja de ser dueño.
func (n *Notifier) extend(key string) (bool, error) {
	ctx, cancel := n.op()
	defer cancel()
	return n.rdb.SetXX(ctx, winKey(key), n.procID, n.o.Window+windowGrace).Result()
}

func (n *Notifier) release(key string) {
	ctx, cancel := n.op()
	defer cancel()
	_ = releaseScript.Run(ctx, n.rdb, []string{winKey(key)}, n.procID).Err()
}
