package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

type ctxKey int

const userCtxKey ctxKey = iota

// AuthUser is the authenticated principal attached to the request context.
type AuthUser struct {
	ID        int64
	CompanyID int64
	Name      string
	Role      domain.Role
}

func userFrom(ctx context.Context) (AuthUser, bool) {
	u, ok := ctx.Value(userCtxKey).(AuthUser)
	return u, ok
}

// RequireAuth verifies the Bearer access token and attaches the user to the context.
func RequireAuth(jm *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" || raw == r.Header.Get("Authorization") {
				// EventSource no permite headers: aceptar ?token= para SSE.
				raw = r.URL.Query().Get("token")
			}
			if raw == "" {
				Error(w, domain.ErrUnauthorized)
				return
			}
			claims, err := jm.Parse(raw)
			if err != nil {
				Error(w, domain.ErrUnauthorized)
				return
			}
			id, _ := strconv.ParseInt(claims.Subject, 10, 64)
			u := AuthUser{ID: id, CompanyID: claims.CompanyID, Name: claims.Name, Role: claims.Role}
			// alimenta la trazabilidad: el log del request sabrá quién lo hizo
			if ti := traceFrom(r.Context()); ti != nil {
				ti.userID = u.ID
				ti.role = string(u.Role)
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, u)))
		})
	}
}

// WithTenant toma una conexión con el GUC app.company_id fijado al tenant del JWT y la ata al
// ctx (store.QC/WithTx la usan → RLS aísla cada query). Debe correr DESPUÉS de RequireAuth.
// SSE (/events) se excluye: mantiene la respuesta abierta y acapararía una conexión del pool.
func WithTenant(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := userFrom(r.Context())
			if !ok || u.CompanyID == 0 {
				Error(w, domain.ErrUnauthorized)
				return
			}
			ctx, release, err := st.AcquireTenant(r.Context(), u.CompanyID)
			if err != nil {
				Error(w, err)
				return
			}
			defer release()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole gates a route to the given roles. Must run after RequireAuth.
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := userFrom(r.Context())
			if !ok || !u.Role.In(roles...) {
				// Evento de seguridad para detección: quién (id/rol) intentó qué ruta.
				logging.SecurityEvent(r.Context(), "forbidden",
					"user_id", u.ID, "role", string(u.Role),
					"method", r.Method, "path", r.URL.Path, "ip", clientIP(r))
				Error(w, domain.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
