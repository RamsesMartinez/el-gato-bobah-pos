package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
)

// EL LOCKOUT POR CUENTA NO SE ESQUIVA CAMBIANDO MAYÚSCULAS.
//
// `users.username` y `companies.slug` son citext: "admin" y "ADMIN" son EL MISMO usuario para
// autenticar. La llave del limitador se armaba con el cuerpo crudo, así que cada grafía abría su
// propio contador — `admin` × `gatobobah` da 2^5 × 2^9 = 16,384 llaves para la misma credencial, y
// el lockout que existe contra la adivinación distribuida (B1 de docs/security-owasp.md) dejaba de
// morder. Lo encontró una revisión adversarial, no producción.
//
// El test agota el limitador con una grafía y vuelve con OTRA: si la llave no está normalizada, el
// segundo intento pasa el filtro y sigue hacia la autenticación.
func TestElLockoutDelLoginNoDistingueMayusculas(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})

	ctx := context.Background()
	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, "login:gatobobah:admin")
	}

	// Sin `h.auth`, pasar el filtro del lockout termina en un nil pointer: eso ES el fallo, y este
	// recover lo traduce a la frase que explica qué se rompió.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("la llave del lockout distingue mayúsculas: 'ADMIN@GATOBOBAH' abrió un contador "+
				"nuevo y el intento siguió hasta la autenticación (%v)", r)
		}
	}()

	cuerpo, _ := json.Marshal(map[string]string{"username": "ADMIN", "slug": "GATOBOBAH", "password": "loquesea"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(cuerpo))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: el bloqueo de 'admin@gatobobah' tiene que alcanzar a 'ADMIN@GATOBOBAH'", w.Code)
	}
}

// UN USUARIO ABSURDO SE RECHAZA EN LA FRONTERA, Y SE RECHAZA IGUAL QUE UNO CUALQUIERA.
//
// Sin cota, el usuario del cuerpo viaja entero a dos lugares que no lo esperan:
//
//   - a la llave del limitador, que vive en Redis con 128 MB y política allkeys-lru: unas 145
//     peticiones de ~900 KB llenan la instancia y empiezan a desalojarse los contadores del POS
//     —incluido el throttle del propio atacante—, y el limitador falla ABIERTO por diseño, así que
//     vaciarlo no se ve como un error.
//   - al evento de seguridad, que rota a 1 MB con 10 respaldos: once peticiones se llevan toda la
//     bitácora anterior de intentos fallidos.
//
// El rechazo NO puede ser un oráculo: mismo 401 y misma latencia que una credencial cualquiera, con
// el bcrypt de descarte corrido igual.
func TestUnUsuarioAbsurdoSeRechazaSinLlegarAlLimitador(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("un usuario de 900 KB llegó hasta la autenticación en vez de rechazarse en la frontera (%v)", r)
		}
	}()

	enorme := strings.Repeat("a", 900_000)
	cuerpo, _ := json.Marshal(map[string]string{"username": enorme, "slug": "gatobobah", "password": "loquesea"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(cuerpo))
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quiere 401: el usuario absurdo se rechaza como cualquier credencial equivocada", w.Code)
	}
	// Y no dejó rastro en el limitador: si la llave enorme se registró, el ataque a Redis sigue
	// disponible aunque la respuesta sea 401.
	if h.authFails.blocked(context.Background(), "login:gatobobah:"+enorme) {
		t.Fatal("el usuario absurdo alcanzó el limitador")
	}
}

// Y lo mismo en la consola, que tiene su propio handler y por lo tanto su propia frontera.
func TestUnUsuarioAbsurdoEnLaConsolaSeRechazaEnLaFrontera(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("un usuario de 900 KB llegó hasta el servicio de plataforma (%v)", r)
		}
	}()

	enorme := strings.Repeat("a", 900_000)
	cuerpo, _ := json.Marshal(map[string]string{"username": enorme, "password": "loquesea"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/platform/auth/login", bytes.NewReader(cuerpo))
	w := httptest.NewRecorder()
	h.PlatformLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, quiere 401", w.Code)
	}
	if h.authFails.blocked(context.Background(), "plataforma:"+enorme) {
		t.Fatal("el usuario absurdo alcanzó el limitador de la consola")
	}
}

// EL LOCKOUT DE LA CONSOLA EXISTE, Y NO COMPARTE CONTADOR CON EL DEL NEGOCIO.
//
// Las dos mitades importan y ninguna tenía check:
//
//   - Que muerda: sin él, la consola es un login público sin bloqueo por cuenta, en un subdominio
//     que se puede tocar desde cualquier parte.
//   - Que NO se pise con el del negocio: el comentario del handler dice que agotar los intentos de
//     "soporte" en una superficie no puede bloquear al "soporte" de la otra —y de paso, que un 429
//     donde no lo hay diría que ese nombre existe en la otra superficie—. Un comportamiento escrito
//     en un comentario y sin test es una intención.
func TestElLockoutDeLaConsolaMuerdeYNoSePisaConElDelNegocio(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})
	ctx := context.Background()

	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, "plataforma:soporte")
	}

	cuerpo, _ := json.Marshal(map[string]string{"username": "soporte", "password": "loquesea"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/platform/auth/login", bytes.NewReader(cuerpo))
	w := httptest.NewRecorder()
	h.PlatformLogin(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: la consola se quedó sin bloqueo por cuenta", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("un 429 sin Retry-After deja a quien opera adivinando cuándo reintentar")
	}

	// Y el mismo nombre en el negocio sigue con su presupuesto intacto: si estuviera bloqueado,
	// el intento no llegaría a la autenticación y no habría pánico que atrapar.
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("bloquear a 'soporte' en la consola también bloqueó a 'soporte' en el negocio: comparten contador, y un 429 ahí delata que el nombre existe en la otra superficie")
		}
	}()
	cuerpoNegocio, _ := json.Marshal(map[string]string{"username": "soporte", "slug": "gatobobah", "password": "loquesea"})
	reqNegocio := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(cuerpoNegocio))
	h.Login(httptest.NewRecorder(), reqNegocio)
}

// Y que el throttle por IP siga CABLEADO en la ruta de la consola.
//
// Es lo que el test de arriba no ve: quitar el `.With(rateLimit(...))` del router no rompe nada
// visible, y deja el login de la consola aceptando todas las peticiones que quepan en un minuto —
// que es el presupuesto con el que se adivina una contraseña, no con el del lockout por cuenta.
func TestElLoginDeLaConsolaEstaDetrasDelThrottlePorIP(t *testing.T) {
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo", nil)
	h := NewHandlers(Deps{Cfg: config.Config{}, JWT: jm, PlatformJWT: auth.NewManagerDePlataforma("otro-secreto-de-pruebas-largo-igual", nil), Platform: &app.PlatformService{}})
	r := Router(config.Config{}, jm, h, nil)

	// El throttle cuenta TODA petición, así que no hace falta un cuerpo válido ni un servicio vivo:
	// el 429 tiene que llegar antes de que el handler corra.
	var ultimo int
	for i := 0; i < authIPMax+2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/platform/auth/login", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		ultimo = w.Code
		if ultimo == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatalf("tras %d peticiones seguidas el login de la consola sigue respondiendo %d: no está detrás del throttle por IP", authIPMax+2, ultimo)
}
