package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestRateLimiter_BlocksAfterMax(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(3, time.Minute)
	rl.now = func() time.Time { return now }
	const key = "1.2.3.4"

	for i := 0; i < 3; i++ {
		if rl.blocked(key) {
			t.Fatalf("attempt %d should be allowed", i)
		}
		rl.record(key)
	}
	if !rl.blocked(key) {
		t.Fatal("expected key to be blocked after reaching max attempts")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(2, time.Minute)
	rl.now = func() time.Time { return now }
	const key = "user:kate"

	rl.record(key)
	rl.record(key)
	if !rl.blocked(key) {
		t.Fatal("expected blocked at limit")
	}
	now = now.Add(time.Minute + time.Second) // window elapsed
	if rl.blocked(key) {
		t.Fatal("expected counter to reset after the window elapsed")
	}
}

func TestRateLimiter_ResetClearsCounter(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	const key = "login:admin"
	rl.record(key)
	if !rl.blocked(key) {
		t.Fatal("expected blocked")
	}
	rl.reset(key) // e.g. after a successful login
	if rl.blocked(key) {
		t.Fatal("reset must clear the counter so a legit user is not penalized")
	}
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	h := rateLimit(rl, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.9:5555" // distinto puerto no debe crear otra clave
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if do() != http.StatusOK || do() != http.StatusOK {
		t.Fatal("first two requests should pass")
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("third request should be rate-limited, got %d", code)
	}
}

func TestRateKeyIP(t *testing.T) {
	// sin proxy: usa el host de RemoteAddr, sin puerto → misma clave por cliente
	r1 := httptest.NewRequest(http.MethodPost, "/", nil)
	r1.RemoteAddr = "203.0.113.7:40001"
	r2 := httptest.NewRequest(http.MethodPost, "/", nil)
	r2.RemoteAddr = "203.0.113.7:59999"
	if rateKeyIP(r1, false) != "203.0.113.7" || rateKeyIP(r1, false) != rateKeyIP(r2, false) {
		t.Fatal("direct: key must be the port-less host, stable across ports")
	}
	// detrás de proxy: el cliente NO puede rotar la clave con un XFF falso — se toma
	// la última entrada (la que agregó Caddy = peer real).
	rp := httptest.NewRequest(http.MethodPost, "/", nil)
	rp.RemoteAddr = "10.0.0.2:8080" // Caddy
	rp.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9")
	if got := rateKeyIP(rp, true); got != "198.51.100.9" {
		t.Fatalf("proxied: expected the real (last) client IP, got %q", got)
	}
}

func TestRateLimiter_MapStaysBounded(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(5, time.Minute)
	rl.now = func() time.Time { return now }
	// floodea claves distintas y ya vencidas; el sweep debe podarlas
	for i := 0; i < rlSweepEvery*2; i++ {
		rl.record("k" + strconv.Itoa(i))
		now = now.Add(2 * time.Minute) // cada entrada vence de inmediato
	}
	rl.mu.Lock()
	n := len(rl.hits)
	rl.mu.Unlock()
	if n > rlSweepEvery {
		t.Fatalf("map should be swept, has %d entries", n)
	}
}
