package httpapi

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// rlSweepEvery bounds the in-memory fallback map: every N writes we drop expired windows, so
// a flood of distinct keys (random usernames/IPs) can't grow memory without limit.
const rlSweepEvery = 1024

// rateLimiter is a fixed-window counter keyed by an arbitrary string (IP, username, or user
// id). Backed by Redis when REDIS_URL is set, so counters survive a restart and are shared
// across replicas — same degrade-gracefully pattern as cache.MenuCache: rdb == nil falls back
// to an in-memory map (dev without Redis, and what the plain unit tests exercise).
// Redis errors fail OPEN (never lock everyone out because the cache hiccuped) and log a
// security event so it's visible — same convention as the HIBP check.
type rateLimiter struct {
	rdb    *redis.Client
	prefix string // namespaces this limiter's keys in Redis (authFails vs authIPs share one instance)

	mu     sync.Mutex
	hits   map[string]*rlWindow
	max    int
	window time.Duration
	writes int
	now    func() time.Time
}

type rlWindow struct {
	count int
	reset time.Time
}

// newRateLimiter builds a limiter under prefix. redisURL empty or unparsable → in-memory only.
func newRateLimiter(redisURL, prefix string, max int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		prefix: prefix,
		hits:   make(map[string]*rlWindow),
		max:    max,
		window: window,
		now:    time.Now,
	}
	if redisURL == "" {
		return rl
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return rl
	}
	rl.rdb = redis.NewClient(opt)
	return rl
}

// blocked reports whether key has reached the limit within the current window. Read-only: it
// never creates or extends a window (record does that).
func (rl *rateLimiter) blocked(ctx context.Context, key string) bool {
	if rl.rdb != nil {
		n, err := rl.rdb.Get(ctx, rl.prefix+key).Int()
		if err != nil {
			if err != redis.Nil {
				logging.SecurityEvent(ctx, "ratelimit_redis_error", "op", "get", "error", err.Error())
			}
			return false // fail open: down or no-hits-yet ≠ blocked
		}
		return n >= rl.max
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	w := rl.hits[key]
	if w == nil {
		return false
	}
	if rl.now().After(w.reset) {
		delete(rl.hits, key)
		return false
	}
	return w.count >= rl.max
}

// record counts one attempt against key. Fixed window: the TTL is set only on the FIRST hit
// (ExpireNX) so a steady stream of attempts doesn't keep pushing the window forward forever.
func (rl *rateLimiter) record(ctx context.Context, key string) {
	if rl.rdb != nil {
		fullKey := rl.prefix + key
		pipe := rl.rdb.Pipeline()
		pipe.Incr(ctx, fullKey)
		pipe.ExpireNX(ctx, fullKey, rl.window)
		if _, err := pipe.Exec(ctx); err != nil {
			logging.SecurityEvent(ctx, "ratelimit_redis_error", "op", "record", "error", err.Error())
		}
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	if rl.writes++; rl.writes >= rlSweepEvery {
		rl.writes = 0
		for k, w := range rl.hits {
			if now.After(w.reset) {
				delete(rl.hits, k)
			}
		}
	}
	w := rl.hits[key]
	if w == nil || now.After(w.reset) {
		rl.hits[key] = &rlWindow{count: 1, reset: now.Add(rl.window)}
		return
	}
	w.count++
}

// reset clears a key — call after a successful auth so a legitimate user who
// mistyped once isn't punished, and to release an account lock on success.
func (rl *rateLimiter) reset(ctx context.Context, key string) {
	if rl.rdb != nil {
		if err := rl.rdb.Del(ctx, rl.prefix+key).Err(); err != nil {
			logging.SecurityEvent(ctx, "ratelimit_redis_error", "op", "reset", "error", err.Error())
		}
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.hits, key)
}

// retryAfter returns whole seconds until key's window resets (for the header).
func (rl *rateLimiter) retryAfter(ctx context.Context, key string) int {
	if rl.rdb != nil {
		d, err := rl.rdb.TTL(ctx, rl.prefix+key).Result()
		if err != nil || d <= 0 {
			return 0
		}
		return int(d.Seconds()) + 1
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	w := rl.hits[key]
	if w == nil {
		return 0
	}
	if d := w.reset.Sub(rl.now()); d > 0 {
		return int(d.Seconds()) + 1
	}
	return 0
}

// rateKeyIP resolves the client IP to key the per-IP throttle on. RemoteAddr always
// carries a port, so we strip it. Behind Caddy (behindProxy) RemoteAddr is the proxy,
// not the client — Caddy appends the real peer as the LAST X-Forwarded-For entry, so
// that's the spoof-resistant client IP (a client-sent XFF ends up to its left).
// ponytail: assumes exactly one trusted proxy (Caddy). If you add another hop, switch
// to a trusted-proxy allowlist.
func rateKeyIP(r *http.Request, behindProxy bool) string {
	if behindProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimit is a per-IP request throttle. It counts EVERY request so a flood (login
// spray, bcrypt-CPU DoS, refresh hammering) trips before the handler runs. Scope it to
// the sensitive endpoints (login/refresh/forgot/reset) so routine ops (pin-switch, me) aren't
// throttled; account-targeted guessing is caught separately by the per-account limiter.
func rateLimit(rl *rateLimiter, behindProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ip := rateKeyIP(r, behindProxy)
			if rl.blocked(ctx, ip) {
				tooManyRequests(w, rl.retryAfter(ctx, ip))
				return
			}
			rl.record(ctx, ip)
			next.ServeHTTP(w, r)
		})
	}
}

// maxBody caps every request body so a huge payload can't exhaust memory. SSE and
// other GETs carry no body, so this is transparent to them. 1 MiB is ample for POS
// orders (the biggest payload).
func maxBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func tooManyRequests(w http.ResponseWriter, retryAfterSec int) {
	if retryAfterSec > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	}
	Error(w, domain.ErrTooManyRequests)
}
