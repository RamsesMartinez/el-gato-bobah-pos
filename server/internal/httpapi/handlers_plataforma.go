package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// Handlers de la CONSOLA DE PLATAFORMA (spec 016). Todo lo de aquí cuelga de /api/v1/platform y
// corre con la conexión del rol de plataforma; ni una de estas rutas vive en el grupo del negocio.

// PlatformLogin: POST /api/v1/platform/auth/login
func (h *Handlers) PlatformLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	// Se normaliza AQUÍ y no solo en el servicio, y no es redundante: es esta cadena la que arma la
	// llave del limitador, y sin normalizar "Soporte" y "soporte" tendrían presupuestos de intentos
	// separados para la misma credencial.
	usuario := domain.NormalizarUsuario(body.Username)

	// La cota va ANTES del limitador, que es lo único que la hace servir: un usuario de 900 KB en
	// la llave llena un Redis de 128 MB en dos minutos —desalojando los contadores del POS— y en el
	// evento de seguridad se lleva la bitácora entera al rotar. Se rechaza como CUALQUIER
	// credencial equivocada, con el bcrypt de descarte corrido: la forma del usuario no puede
	// decir cuáles existen.
	if !domain.UsuarioValido(usuario) {
		auth.CheckDummySecret(body.Password)
		logging.SecurityEvent(r.Context(), "platform_login_failed", "username", "(inválido)", "ip", clientIP(r))
		Error(w, domain.ErrCredencialDePlataforma)
		return
	}

	// Lockout por cuenta, con su propio prefijo: compartir la llave con el login del negocio haría
	// que agotar los intentos de "soporte" en una superficie bloqueara al "soporte" de la otra —y
	// de paso diría que el nombre existe en las dos.
	key := "plataforma:" + usuario
	if h.authFails.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "platform_login", "username", usuario, "ip", clientIP(r))
		tooManyRequests(w, h.authFails.retryAfter(r.Context(), key))
		return
	}

	s, err := h.platform.Login(r.Context(), usuario, body.Password)
	if err != nil {
		h.authFails.record(r.Context(), key)
		if errors.Is(err, domain.ErrCredencialDePlataforma) {
			// FR-012: queda el intento con el usuario tecleado, NUNCA la contraseña. Y un solo
			// evento para los tres rechazos: partirlo en "no existe" y "apagado" convertiría la
			// bitácora en el oráculo que la respuesta se cuida de no ser.
			logging.SecurityEvent(r.Context(), "platform_login_failed", "username", usuario, "ip", clientIP(r))
			Error(w, err)
			return
		}
		Error(w, err)
		return
	}
	h.authFails.reset(r.Context(), key)
	logging.SecurityEvent(r.Context(), "platform_login", "operator", s.Operador.Username, "ip", clientIP(r))
	JSON(w, http.StatusOK, s)
}

// PlatformCompanies: GET /api/v1/platform/companies
func (h *Handlers) PlatformCompanies(w http.ResponseWriter, r *http.Request) {
	empresas, err := h.platform.Empresas(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, empresas)
}

// PlatformUsage: GET /api/v1/platform/usage
//
// El mapa de calor de uso (spec 017). Lee con la conexión de la CONSOLA, que solo alcanza el
// agregado: el grano fino —donde mañana van las coordenadas del toque— le está negado por grants.
func (h *Handlers) PlatformUsage(w http.ResponseWriter, r *http.Request) {
	desde, err := parseFechaDeUso(r.URL.Query().Get("desde"))
	if err != nil {
		Error(w, err)
		return
	}
	hasta, err := parseFechaDeUso(r.URL.Query().Get("hasta"))
	if err != nil {
		Error(w, err)
		return
	}
	// `empresa` ausente = todas juntas (FR-008).
	empresa, err := empresaDeLaConsulta(r)
	if err != nil {
		Error(w, err)
		return
	}

	mapa, err := h.usageConsola.Mapa(r.Context(), desde, hasta, empresa)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, mapa)
}

// PlatformTouches: GET /api/v1/platform/touches
//
// La rejilla de toques (spec 019). Misma conexión y mismos límites que el mapa de la 017: la
// consola solo alcanza el conteo por zona, y el toque suelto no existe en ningún lado del cual
// pudiera alcanzarlo.
func (h *Handlers) PlatformTouches(w http.ResponseWriter, r *http.Request) {
	desde, err := parseFechaDeUso(r.URL.Query().Get("desde"))
	if err != nil {
		Error(w, err)
		return
	}
	hasta, err := parseFechaDeUso(r.URL.Query().Get("hasta"))
	if err != nil {
		Error(w, err)
		return
	}
	empresa, err := empresaDeLaConsulta(r)
	if err != nil {
		Error(w, err)
		return
	}

	// La pantalla y la orientación las valida el servicio: las dos son listas del dominio, y
	// repetirlas aquí sería tener la regla en dos lugares que se desincronizan.
	rejilla, err := h.usageConsola.Rejilla(r.Context(),
		r.URL.Query().Get("pantalla"), r.URL.Query().Get("orientacion"), desde, hasta, empresa)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, rejilla)
}

// empresaDeLaConsulta lee el filtro opcional por empresa.
//
// Ausente = todas juntas. Presente y mal escrita se RECHAZA: caer a "todas" en silencio devolvería
// un número que nadie pidió, en una pantalla que se ve correcta y que por eso nadie audita.
func empresaDeLaConsulta(r *http.Request) (*int64, error) {
	v := r.URL.Query().Get("empresa")
	if v == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: empresa inválida", domain.ErrValidation)
	}
	return &id, nil
}

// parseFechaDeUso exige AAAA-MM-DD. Una fecha mal escrita se rechaza en vez de caer a un default:
// un `desde` que se convierte en "los últimos 30 días" devuelve una pantalla que se ve bien y
// reporta un periodo que nadie pidió.
func parseFechaDeUso(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, fmt.Errorf("%w: falta la fecha", domain.ErrValidation)
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: la fecha va como AAAA-MM-DD", domain.ErrValidation)
	}
	return t, nil
}
