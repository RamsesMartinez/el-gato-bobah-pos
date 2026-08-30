package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestRateLimiter_BlocksAfterMax(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter("", "rl:", 3, time.Minute)
	rl.now = func() time.Time { return now }
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

func TestRateLimiter_WindowExpiry(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter("", "rl:", 2, time.Minute)
	rl.now = func() time.Time { return now }
	ctx := t.Context()
	const key = "user:kate"

	rl.record(ctx, key)
	rl.record(ctx, key)
	if !rl.blocked(ctx, key) {
		t.Fatal("expected blocked at limit")
	}
	now = now.Add(time.Minute + time.Second) // window elapsed
	if rl.blocked(ctx, key) {
		t.Fatal("expected counter to reset after the window elapsed")
	}
}

func TestRateLimiter_ResetClearsCounter(t *testing.T) {
	rl := newRateLimiter("", "rl:", 1, time.Minute)
	ctx := t.Context()
	const key = "login:admin"
	rl.record(ctx, key)
	if !rl.blocked(ctx, key) {
		t.Fatal("expected blocked")
	}
	rl.reset(ctx, key) // e.g. after a successful login
	if rl.blocked(ctx, key) {
		t.Fatal("reset must clear the counter so a legit user is not penalized")
	}
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	rl := newRateLimiter("", "rl:", 2, time.Minute)
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

	if c1, c2 := do(), do(); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("first two requests should pass, got %d and %d", c1, c2)
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
	// un solo salto: el XFF viene sin coma y la entrada única ES la que puso Caddy.
	one := httptest.NewRequest(http.MethodPost, "/", nil)
	one.RemoteAddr = "10.0.0.2:8080"
	one.Header.Set("X-Forwarded-For", "198.51.100.9")
	if got := rateKeyIP(one, true); got != "198.51.100.9" {
		t.Fatalf("proxied single hop: got %q, want 198.51.100.9", got)
	}
	// XFF presente pero vacío: no hay IP que usar, cae a RemoteAddr sin puerto.
	blank := httptest.NewRequest(http.MethodPost, "/", nil)
	blank.RemoteAddr = "10.0.0.2:8080"
	blank.Header.Set("X-Forwarded-For", " , ")
	if got := rateKeyIP(blank, true); got != "10.0.0.2" {
		t.Fatalf("proxied blank XFF: got %q, want the proxy host 10.0.0.2", got)
	}
}

func TestRateLimiter_MapStaysBounded(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter("", "rl:", 5, time.Minute)
	rl.now = func() time.Time { return now }
	ctx := t.Context()
	// floodea claves distintas y ya vencidas; el sweep debe podarlas
	for i := 0; i < rlSweepEvery*2; i++ {
		rl.record(ctx, "k"+strconv.Itoa(i))
		now = now.Add(2 * time.Minute) // cada entrada vence de inmediato
	}
	rl.mu.Lock()
	n := len(rl.hits)
	rl.mu.Unlock()
	if n > rlSweepEvery {
		t.Fatalf("map should be swept, has %d entries", n)
	}
}

// rateLimitUser cuenta por USUARIO, no por IP: todo el local sale por la misma dirección, así que
// un tope por IP castigaría al segundo cajero por lo que hizo el primero.
func TestRateLimitUsuario_CuentaPorUsuarioYNoPorIP(t *testing.T) {
	rl := newRateLimiter("", "rl-user:", 2, time.Minute)
	h := rateLimitUser(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(userID int64) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/platform-prices/product", nil)
		req.RemoteAddr = "10.0.0.9:5555" // la MISMA IP para los dos usuarios
		req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: userID}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if c1, c2 := do(7), do(7); c1 != http.StatusOK || c2 != http.StatusOK {
		t.Fatalf("las primeras dos del usuario 7 deben pasar, dieron %d y %d", c1, c2)
	}
	if code := do(7); code != http.StatusTooManyRequests {
		t.Fatalf("la tercera del usuario 7 debe frenarse, dio %d", code)
	}
	// Mismo equipo, otro cajero: su contador arranca en cero.
	if code := do(8); code != http.StatusOK {
		t.Fatalf("el usuario 8 no debe pagar el tope del 7, dio %d", code)
	}
}

// Sin usuario en el contexto no hay clave que contar. Devolver 401 y no "pasar de largo" evita que
// una ruta mal cableada —el limitador antes de RequireAuth— quede sin tope y sin que nada avise.
func TestRateLimitUsuario_SinUsuarioEs401(t *testing.T) {
	rl := newRateLimiter("", "rl-user:", 2, time.Minute)
	llamado := false
	h := rateLimitUser(rl)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		llamado = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/platform-prices/product", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin usuario debe ser 401, dio %d", rec.Code)
	}
	if llamado {
		t.Fatal("el handler no debe correr sin usuario")
	}
}
