package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
)

// FR-015. El evento de seguridad del desbloqueo lleva a QUIÉN se intentó desbloquear y desde dónde,
// nunca el PIN.
//
// Un secreto en un log es peor que no tener el log: sobrevive a la rotación, viaja a donde sea que
// se envíen los registros, y lo lee gente que nunca tuvo por qué conocerlo. El principio V no deja
// mergear un control de seguridad sin su test, y este es el que lo cubre.
//
// Se ejercita el camino del LOCKOUT porque es el único que emite sin tocar la base: basta agotar el
// limitador para que el siguiente intento registre `auth_lockout` con el PIN en el cuerpo.
func TestElEventoDeDesbloqueoNoLlevaElPin(t *testing.T) {
	const secreto = "913571"
	const objetivo = 42

	h := NewHandlers(Deps{Cfg: config.Config{}})

	// Se agota el limitador del usuario objetivo para forzar la rama que registra el evento.
	ctx := context.Background()
	key := "pin:42"
	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, key)
	}

	var log bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(anterior)

	cuerpo, _ := json.Marshal(map[string]any{"userId": objetivo, "pin": secreto})
	req := httptest.NewRequest(http.MethodPost, "/auth/pin-switch", bytes.NewReader(cuerpo))
	// La sesión del dispositivo: el handler exige el token de acceso Y la cookie de refresh, porque
	// el relevo hereda el reloj de ESA estación.
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	req.AddCookie(&http.Cookie{Name: refreshCookie, Value: "1.token-de-la-estacion"})
	w := httptest.NewRecorder()
	h.PinSwitch(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: el test necesita la rama que registra el evento", w.Code)
	}
	registrado := log.String()
	if !strings.Contains(registrado, "auth_lockout") {
		t.Fatalf("no se registró el evento de seguridad: %s", registrado)
	}
	if strings.Contains(registrado, secreto) {
		t.Fatalf("el PIN acabó en el log: %s", registrado)
	}
	// Y sí lleva a quién se intentó desbloquear, que es lo que vuelve accionable el evento.
	if !strings.Contains(registrado, "target_user_id") {
		t.Errorf("el evento no dice a quién se intentó desbloquear: %s", registrado)
	}
}

// FR-010 en el camino de SOLO-PIN, que nació sin ninguna protección.
//
// La llave del lockout se construía con el `userId` que en ese modo NO existe, y no inventé otra:
// la rama llamaba a PinSwitchSoloPin sin `blocked`, sin `record`, y /pin-switch tampoco tiene
// throttle per-IP. Cualquier empleado autenticado podía probar PINs sin límite — y como ahí el PIN
// IDENTIFICA, cada intento se prueba contra toda la plantilla a la vez: con 8 personas la
// esperanza baja a ~62,500 intentos, y si cae el del admin, el atacante recibe rol de admin.
//
// La llave cuelga de QUIEN PIDE, no de a quién se busca, porque en este modo no hay a quién buscar.
func TestElDesbloqueoSoloPinTambienSeFrena(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})
	ctx := context.Background()

	// Se agota el limitador del dispositivo que pide.
	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, "pinsolo:7")
	}

	cuerpo, _ := json.Marshal(map[string]any{"pin": "482715"})
	req := httptest.NewRequest(http.MethodPost, "/auth/pin-switch", bytes.NewReader(cuerpo))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	req.AddCookie(&http.Cookie{Name: refreshCookie, Value: "1.token-de-la-estacion"})
	w := httptest.NewRecorder()
	h.PinSwitch(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: sin límite se puede recorrer el espacio de PINs entero", w.Code)
	}
}

// Y sin la cookie de la estación el relevo se RECHAZA, sin tocar la base.
//
// Es lo que impide que el relevo arranque un turno nuevo: si al no encontrar sesión de estación se
// emitiera un plazo completo, bastaría borrar la cookie para renovar ocho horas con un PIN, tantas
// veces como se quisiera. El límite del turno sería decorativo.
func TestElRelevoSinSesionDeEstacionSeRechaza(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})

	cuerpo, _ := json.Marshal(map[string]any{"userId": 42, "pin": "4827"})
	req := httptest.NewRequest(http.MethodPost, "/auth/pin-switch", bytes.NewReader(cuerpo))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	w := httptest.NewRecorder()
	h.PinSwitch(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quiere 401: sin cookie de estación no hay reloj que heredar", w.Code)
	}
}

// FR-008 por el lado que no se veía: `POST /me/pin` era un ORÁCULO de existencia.
//
// El rechazo por PIN repetido no dice de quién es, pero decir "alguien lo usa" ya basta: cualquier
// autenticado recorre el espacio con su propio formulario, recolecta PINs válidos, y después los
// usa contra el desbloqueo. Cambiar el PIN legítimamente pasa dos o tres veces al año, así que el
// límite no estorba a nadie que lo use como se debe.
func TestCambiarElPropioPinTambienSeFrena(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})
	ctx := context.Background()

	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, "setpin:7")
	}

	cuerpo, _ := json.Marshal(map[string]any{"pin": "482715"})
	req := httptest.NewRequest(http.MethodPost, "/me/pin", bytes.NewReader(cuerpo))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	w := httptest.NewRecorder()
	h.SetOwnPIN(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: sin límite el formulario es un oráculo de PINs", w.Code)
	}
}

// UN ID ABSURDO EN LA RUTA ES 400, NO 500.
//
// `GetOrder` devolvía el error crudo de `strconv.ParseInt` a `Error`, que no lo reconoce y lo
// convierte en 500. Un 500 dice "el servidor se rompió" y manda a revisar logs; un 400 dice "esa
// petición no vale". La constitución lo pide explícitamente: la entrada absurda se rechaza como
// 400, no como un 500 opaco.
//
// Se volvió visible al quitar la ruta `/orders/unpaid`: "unpaid" pasó a caer en `/orders/{id}` y
// cualquier cliente viejo que la llamara recibía un 500 en vez de un error que se entiende.
func TestUnIdDePedidoQueNoEsNumeroSeRechazaComo400(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})

	req := httptest.NewRequest(http.MethodGet, "/orders/unpaid", nil)
	req = req.WithContext(conIDDeRuta(req.Context(), "unpaid"))
	w := httptest.NewRecorder()
	h.GetOrder(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, quiere 400: un 500 manda a revisar logs por una petición que nunca valió", w.Code)
	}
}

// conIDDeRuta pone el parámetro {id} como lo haría chi al enrutar.
func conIDDeRuta(ctx context.Context, id string) context.Context {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", id)
	return context.WithValue(ctx, chi.RouteCtxKey, rc)
}
