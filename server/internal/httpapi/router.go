package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Router assembles the HTTP API.
func Router(cfg config.Config, jm *auth.Manager, h *Handlers, pingDB func(context.Context) error) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestID) // X-Request-Id (del front o generado), trazabilidad
	r.Use(middleware.Recoverer)
	r.Use(RequestLogger) // captura request/response (redactado) en todos los ambientes
	r.Use(cors(cfg.CORSOrigin))

	// Ops (unversioned)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := pingDB(ctx); err != nil {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down"})
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", h.Login)
			r.Post("/refresh", h.Refresh)
			r.Post("/logout", h.Logout)
			r.Group(func(r chi.Router) {
				r.Use(RequireAuth(jm))
				r.Post("/pin-switch", h.PinSwitch)
				r.Get("/me", h.Me)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(jm))

			r.Get("/pos/menu", h.PosMenu)
			r.Get("/pos/popular", h.PosPopular)
			r.Get("/pos/modifier-defaults", h.ModifierDefaults)
			r.Get("/products/{id}/costing", h.ProductCosting)

			r.Route("/orders", func(r chi.Router) {
				r.Post("/", h.CreateOrder)
				r.Get("/", h.ListOrders)
				r.Get("/{id}", h.GetOrder)
				r.Post("/{id}/status", h.SetOrderStatus)
				r.Post("/{id}/cancel", h.CancelOrder)
			})

			r.Get("/events", h.Events)

			// Backoffice
			r.Get("/payment-methods", h.PaymentMethods)
			r.Route("/cash-sessions", func(r chi.Router) {
				r.Post("/", h.OpenCashSession)
				r.Get("/current", h.CurrentCashSession)
				r.Post("/close", h.CloseCashSession)
			})
			r.Get("/expense-categories", h.ExpenseCategories)
			r.Route("/expenses", func(r chi.Router) {
				r.Get("/", h.ListExpenses)
				r.Post("/", h.CreateExpense)
			})
			r.Route("/stock", func(r chi.Router) {
				r.Get("/levels", h.StockLevels)
				r.Get("/movements", h.StockMovements)
				r.Post("/movements", h.CreateStockMovement)
			})
			r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/reports", func(r chi.Router) {
				r.Get("/sales", h.ReportSales)
				r.Get("/margins", h.ReportMargins)
			})

			r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/admin/products", func(r chi.Router) {
				r.Get("/", h.AdminListProducts)
				r.Patch("/{id}", h.AdminUpdateProduct)
				// grupos de modificadores asignados a un producto (min/max/obligatorio por producto)
				r.Get("/{id}/groups", h.AdminProductGroups)
				r.Post("/{id}/groups", h.AdminAttachProductGroup)
				r.Delete("/{id}/groups/{groupId}", h.AdminDetachProductGroup)
			})

			r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/admin/modifier-options", func(r chi.Router) {
				r.Get("/", h.AdminListModifierOptions)
				r.Patch("/{id}", h.AdminUpdateOption)
			})

			// catálogo global de grupos + sus opciones
			r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/admin/groups", func(r chi.Router) {
				r.Get("/", h.AdminListGroups)
				r.Post("/", h.AdminCreateGroup)
				r.Patch("/{id}", h.AdminUpdateGroup)
				r.Get("/{id}/options", h.AdminGroupOptions)
				r.Post("/{id}/options", h.AdminCreateOption)
				r.Post("/{id}/options/reorder", h.AdminReorderOptions)
				r.Get("/{id}/products", h.AdminGroupProducts)
			})

			// Recarga cachés en memoria/Redis sin reiniciar (menú, popular, recomendador).
			r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Post("/admin/reload", h.AdminReload)

			r.With(RequireRole(domain.RoleAdmin)).Route("/users", func(r chi.Router) {
				r.Get("/", h.ListUsers)
				r.Post("/", h.CreateUser)
			})
		})
	})

	return r
}

// cors: con credentials NO se puede usar "*" (el browser lo rechaza). Si CORS_ORIGIN
// es "*" reflejamos el Origin del request; si es específico, se usa tal cual.
func cors(allowed string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allow := allowed
			if allowed == "*" && origin != "" {
				allow = origin
			}
			if allow != "" {
				w.Header().Set("Access-Control-Allow-Origin", allow)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,"+RequestIDHeader)
				w.Header().Set("Access-Control-Expose-Headers", RequestIDHeader)
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
