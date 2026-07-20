package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// rlSweepEvery bounds the maps: every N writes we drop expired windows, so a flood
// of distinct keys (random usernames/IPs) can't grow memory without limit.
const rlSweepEvery = 1024

// rateLimiter is a fixed-window counter keyed by an arbitrary string (IP, username,
// or user id). In-memory only — fine for the single-instance MVP.
// ponytail: per-instance; if the API ever runs >1 replica, back this with Redis
// (already wired for the menu cache) so counters are shared.
type rateLimiter struct {
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

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string]*rlWindow),
		max:    max,
		window: window,
		now:    time.Now,
	}
}

// blocked reports whether key has reached the limit within the current window.
// It also lazily expires stale windows.
func (rl *rateLimiter) blocked(key string) bool {
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

// record counts one attempt against key, sweeping expired windows periodically so
// the map stays bounded under a flood of distinct keys.
func (rl *rateLimiter) record(key string) {
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
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.hits, key)
}

// retryAfter returns whole seconds until key's window resets (for the header).
func (rl *rateLimiter) retryAfter(key string) int {
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
// the sensitive endpoints (login/refresh) so routine ops (pin-switch, me) aren't
// throttled; account-targeted guessing is caught separately by the per-account limiter.
func rateLimit(rl *rateLimiter, behindProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := rateKeyIP(r, behindProxy)
			if rl.blocked(ip) {
				tooManyRequests(w, rl.retryAfter(ip))
				return
			}
			rl.record(ip)
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
