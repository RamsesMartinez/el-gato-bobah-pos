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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Router assembles the HTTP API. st es el store de SERVICIO (rol app, sujeto a RLS): lo usa el
// middleware WithTenant para atar la conexión del tenant a cada request autenticado.
func Router(cfg config.Config, jm *auth.Manager, h *Handlers, st *store.Store) http.Handler {
	r := chi.NewRouter()
	r.Use(RequestID) // X-Request-Id (del front o generado), trazabilidad
	r.Use(middleware.Recoverer)
	r.Use(maxBody(1 << 20)) // 1 MiB: acota el cuerpo antes de leerlo (anti-DoS)
	r.Use(RequestLogger)    // captura request/response (redactado) en todos los ambientes
	r.Use(cors(cfg.CORSOrigin, cfg.Env != "production"))

	// Ops (unversioned)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := st.Pool.Ping(ctx); err != nil {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_down"})
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			// per-IP throttle solo en los endpoints sensibles a flood; pin-switch/me
			// (frecuentes en el POS) no se limitan por IP — pin-switch ya está protegido
			// por el limiter per-usuario. behindProxy=true en prod (detrás de Caddy).
			ipThrottle := rateLimit(h.authIPs, cfg.Env == "production")
			r.With(ipThrottle).Post("/login", h.Login)
			r.With(ipThrottle).Post("/refresh", h.Refresh)
			r.Post("/logout", h.Logout)
			// Recuperación de contraseña (pública). Throttle per-IP: enviar correos y probar
			// tokens son superficies de abuso/flood.
			r.With(ipThrottle).Post("/forgot", h.ForgotPassword)
			r.With(ipThrottle).Post("/reset", h.ResetPassword)
			r.Group(func(r chi.Router) {
				r.Use(RequireAuth(jm))
				r.Use(WithTenant(st)) // pin-switch corre bajo el tenant del dispositivo
				r.Post("/pin-switch", h.PinSwitch)
				r.Get("/me", h.Me)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(jm))

			// SSE fuera del middleware de tenant: mantiene la respuesta abierta y acapararía una
			// conexión del pool. El aislamiento por empresa lo da el broker, que suscribe por
			// company_id del JWT (realtime.Broker + handlers_sse.go), no una conexión de tenant.
			r.Get("/events", h.Events)

			// Versión del backend (build SHA/fecha) para el pie de sistema del front. Autenticado
			// pero sin tenant: es info global de despliegue, no toca datos de empresa.
			r.Get("/version", h.Version)

			r.Group(func(r chi.Router) {
				r.Use(WithTenant(st)) // todo lo demás: conexión atada al tenant → RLS aísla cada query

				r.Get("/pos/menu", h.PosMenu)
				r.Get("/pos/popular", h.PosPopular)
				r.Get("/pos/modifier-defaults", h.ModifierDefaults)
				// costo/margen es información de gestión, no operativa del POS
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Get("/products/{id}/costing", h.ProductCosting)

				// preferencias del usuario autenticado (p. ej. orden de categorías del POS), sincronizadas entre tablets
				r.Get("/me/preferences/{key}", h.MeGetPreference)
				r.Put("/me/preferences/{key}", h.MeSetPreference)

				// Cuenta propia (cualquier empleado autenticado): su contraseña / PIN.
				r.Post("/me/password", h.ChangeOwnPassword)
				r.Post("/me/pin", h.SetOwnPIN)

				// Empresa (tenant): lectura para cualquier autenticado; editar nombre/slug solo admin/gerente.
				r.Get("/company", h.GetCompany)
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Patch("/company", h.UpdateCompany)

				r.Route("/orders", func(r chi.Router) {
					r.Post("/", h.CreateOrder)
					r.Get("/", h.ListOrders)
					r.Get("/{id}", h.GetOrder)
					r.Post("/{id}/status", h.SetOrderStatus)
					r.Post("/{id}/cancel", h.CancelOrder)
					// Entregadas del día + reembolso = salida de dinero → solo admin/gerente.
					r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Get("/delivered", h.DeliveredOrders)
					r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Post("/{id}/refund", h.RefundOrder)
				})

				// Backoffice. Role gates reflejan segregación de funciones; ajusta los
				// roles a tu operación real (p. ej. si un mesero también cobra).
				r.Get("/payment-methods", h.PaymentMethods)
				// auto_declare (config de negocio): solo admin/gerente elige qué métodos se
				// declaran solos al cerrar caja.
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Patch("/payment-methods/{id}", h.UpdatePaymentMethod)
				// Ajustes de negocio: GET lo necesita el cobro (costo de envío por defecto); PUT solo
				// admin/gerente (es dinero autoritativo del negocio).
				r.Get("/business-settings", h.BusinessSettings)
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Put("/business-settings", h.UpdateBusinessSettings)
				// ¿hay caja abierta? aviso del POS — cualquier rol autenticado (incl. mesero); no es dato sensible
				r.Get("/cash-status", h.CashStatus)
				// caja: la opera el cajero; excluye al mesero para que no cierre cortes ajenos
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente, domain.RoleCajero)).Route("/cash-sessions", func(r chi.Router) {
					r.Post("/", h.OpenCashSession)
					r.Get("/", h.CashHistory)
					r.Get("/current", h.CurrentCashSession)
					r.Get("/{id}", h.CashSessionDetail)
					r.Post("/close", h.CloseCashSession)
					r.Post("/movements", h.CreateCashMovement)
					r.Post("/transfer", h.CashTransfer) // traspaso entre dos cajas abiertas
				})
				// Listar cajas (para elegir dónde abrir/operar/pagar): el cajero la necesita.
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente, domain.RoleCajero)).Get("/cash-registers", h.CashRegisters)
				// Gestión del catálogo de cajas (alta/renombrar/activar) = configuración → admin/gerente.
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Group(func(r chi.Router) {
					r.Get("/cash-registers/all", h.AllCashRegisters)
					r.Post("/cash-registers", h.CreateCashRegister)
					r.Patch("/cash-registers/{id}", h.UpdateCashRegister)
				})
				// Lista de categorías (para el formulario de gasto): cualquier autenticado.
				r.Get("/expense-categories", h.ExpenseCategories)
				// Gastos, proveedores y catálogo de categorías = gestión/contabilidad → admin/gerente.
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Group(func(r chi.Router) {
					r.Post("/expense-categories", h.CreateExpenseCategory)
					r.Patch("/expense-categories/{id}", h.UpdateExpenseCategory)
					r.Get("/suppliers", h.Suppliers)
					r.Post("/suppliers", h.CreateSupplier)
					r.Patch("/suppliers/{id}", h.UpdateSupplier)
					r.Route("/expenses", func(r chi.Router) {
						r.Get("/", h.ListExpenses)
						r.Post("/", h.CreateExpense)
						r.Get("/{id}", h.ExpenseDetail)
						r.Post("/{id}/pay", h.PayExpense)         // agrega UN pago (el "+1 nuevo pago")
						r.Post("/{id}/receive", h.ReceiveExpense) // entra al almacén
						r.Post("/{id}/cancel", h.CancelExpense)
						// Extracción del documento (ticket/factura/pedido) → borrador. NO escribe nada:
						// devuelve las líneas para que el operador las confirme.
						r.Post("/parse-doc", h.ExtractPurchaseDoc)
					})
					// Catálogo de insumos y buscador de artículos: sin esto el detalle del gasto no
					// tiene contra qué buscar (los ingredientes solo entraban por el importador).
					r.Get("/units", h.Units)
					r.Get("/ingredients", h.ListIngredients)
					r.Post("/ingredients", h.CreateIngredient)
					r.Get("/articles", h.SearchArticles)
					// Sugerencias de mapeo proveedor→inventario. Solo sugiere.
					r.Get("/articles/suggest", h.SuggestArticles)
					// Catálogo aprendido: revisar qué mapeó el sistema y deshacer un mapeo equivocado.
					r.Get("/supplier-items", h.SupplierItems)
					r.Delete("/supplier-items/{id}", h.ForgetSupplierItem)
				})
				// inventario: ajustes/mermas los hace gerencia
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/stock", func(r chi.Router) {
					r.Get("/levels", h.StockLevels)
					r.Get("/movements", h.StockMovements)
					r.Post("/movements", h.CreateStockMovement)
				})
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/reports", func(r chi.Router) {
					r.Get("/sales", h.ReportSales)
					r.Get("/margins", h.ReportMargins)
					r.Get("/tips", h.ReportTips)
				})

				// Categorías (para filtro y alta de productos): admin/gerente.
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Get("/admin/categories", h.AdminCategories)
				r.With(RequireRole(domain.RoleAdmin, domain.RoleGerente)).Route("/admin/products", func(r chi.Router) {
					r.Get("/", h.AdminListProducts)
					r.Post("/", h.AdminCreateProduct)                  // alta de producto (gerente y admin)
					r.Post("/{id}/duplicate", h.AdminDuplicateProduct) // clon con todas sus relaciones
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
					r.Patch("/{id}", h.UpdateUser)               // nombre/rol/alta-baja/email de recuperación
					r.Post("/{id}/password", h.AdminSetPassword) // reset de contraseña por admin
					r.Post("/{id}/pin", h.AdminSetPIN)           // set/reset de PIN por admin
				})
			}) // cierra el grupo WithTenant
		})
	})

	return r
}

// cors resuelve el header Access-Control-Allow-Origin. Con credentials NO se puede
// usar "*" literal (el browser lo rechaza), así que:
//   - origen exacto (https://dominio) → se usa tal cual (recomendado en prod).
//   - "" (vacío) → NO se emiten headers CORS = solo mismo origen (fail-closed).
//   - "*" → SOLO en desarrollo reflejamos el Origin; en prod se ignora (config.Validate
//     ya rechaza "*" en producción, esto es defensa en profundidad).
func cors(allowed string, dev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allow := allowed
			if allowed == "*" {
				if dev && origin != "" {
					allow = origin // conveniencia de desarrollo
				} else {
					allow = "" // en prod no reflejamos orígenes arbitrarios
				}
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
