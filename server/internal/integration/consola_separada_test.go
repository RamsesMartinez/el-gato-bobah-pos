//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LAS DOS SUPERFICIES SE RECHAZAN ENTRE SÍ (US1: FR-001, FR-003, FR-004).
//
// Por el ROUTER real y con las dos firmas de verdad, porque es cableado: los tests de servicio no
// verían que alguien montó una ruta de plataforma dentro del grupo del negocio, ni que el
// middleware de la consola quedó validando con el manager equivocado.

const secretoDelNegocioEnPruebas = "integration-test-secret-of-32+bytes-minimum"
const secretoDeLaConsolaEnPruebas = "otro-secreto-de-pruebas-de-32-o-mas-bytes"

// routerConConsola arma el router con las DOS superficies montadas, cada una con su firma y con
// su conexión: la consola sobre el rol de plataforma, el negocio sobre el de siempre.
func routerConConsola(t *testing.T, st *store.Store) (http.Handler, *auth.Manager, *auth.ManagerDePlataforma) {
	t.Helper()
	prepararRolDePlataforma(t, st)
	plataforma := platformRoleStore(t)

	// Los managers van con el reloj REAL, no con el fijo de las pruebas: `jwt` valida el
	// vencimiento contra time.Now, así que un token emitido en julio de 2026 nace caducado y todo
	// request autenticado responde 401 por una razón que no tiene nada que ver con lo que se prueba.
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	pjm := auth.NewManagerDePlataforma(secretoDeLaConsolaEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:         jm,
		Auth:        app.NewAuthService(st, jm, clock),
		PlatformJWT: pjm,
		Platform:    app.NewPlatformService(plataforma, pjm, clock),
		Sales:       app.NewSalesService(st, clock),
	})
	return httpapi.Router(config.Config{}, jm, h, st), jm, pjm
}

// crearOperador siembra un operador de plataforma. Lo hace el DUEÑO: el rol de la consola no
// puede escribir en su propia tabla, y esa es justo la barrera que la spec 018 tendrá que abrir a
// propósito.
func crearOperador(t *testing.T, st *store.Store, usuario, password string, activo bool) int64 {
	t.Helper()
	hash, err := auth.HashSecret(password)
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into platform_operators (username, name, password_hash, is_active)
		 values ($1, $2, $3, $4) returning id`,
		usuario, "Operador "+usuario, hash, activo).Scan(&id); err != nil {
		t.Fatalf("crear operador %s: %v", usuario, err)
	}
	return id
}

func loginDeConsola(t *testing.T, r http.Handler, usuario, password string) *http.Response {
	t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"username": usuario, "password": password})
	w := do(t, r, http.MethodPost, "/api/v1/platform/auth/login", "", cuerpo, "application/json")
	return w.Result()
}

func TestLaConsolaYElNegocioSeRechazanEntreSi(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, jm, pjm := routerConConsola(t, st)

	const passOperador = "Contrasena-De-Plataforma-1!"
	crearOperador(t, st, "soporte", passOperador, true)

	const passDelNegocio = "Contrasena-Larga-1!"
	hash, err := auth.HashSecret(passDelNegocio)
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	adminID := makeUser(t, st, "admin_del_cliente", "admin")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, adminID, hash); err != nil {
		t.Fatalf("fijar password del admin: %v", err)
	}

	t.Run("el operador entra a su consola", func(t *testing.T) {
		res := loginDeConsola(t, r, "soporte", passOperador)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("login de la consola = %d, quiere 200", res.StatusCode)
		}
	})

	// FR-003: la credencial de plataforma NO existe en `users`, así que el login del negocio la
	// rechaza sin ninguna lógica especial. Si algún día alguien "arregla" el login buscando
	// también en platform_operators, este test es el que truena.
	t.Run("la credencial de plataforma no entra al negocio", func(t *testing.T) {
		cuerpo, _ := json.Marshal(map[string]string{
			"username": "soporte@gatobobah", "password": passOperador,
		})
		w := do(t, r, http.MethodPost, "/api/v1/auth/login", "", cuerpo, "application/json")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("login del negocio con credencial de plataforma = %d, quiere 401", w.Code)
		}
	})

	// FR-004: y al revés. Ser admin de su empresa es el permiso más alto del producto y aun así no
	// alcanza: la consola es de quien vende el sistema, no de quien lo usa.
	t.Run("el admin de una empresa no entra a la consola", func(t *testing.T) {
		res := loginDeConsola(t, r, "admin_del_cliente", passDelNegocio)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("login de consola con credencial de negocio = %d, quiere 401", res.StatusCode)
		}
	})

	// Los tokens: lo que el diseño de dos firmas vuelve imposible de construir.
	t.Run("un token del negocio no abre una ruta de la consola", func(t *testing.T) {
		tok, err := jm.Issue(domain.User{ID: adminID, CompanyID: defaultCompanyID, Name: "Admin", Role: domain.RoleAdmin})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		// 401 y no 403: a quien no debe ver la consola no se le confirma que exista.
		if w := do(t, r, http.MethodGet, "/api/v1/platform/companies", tok, nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("token de negocio en la consola = %d, quiere 401", w.Code)
		}
	})

	t.Run("un token de la consola no abre una ruta del negocio", func(t *testing.T) {
		tok, err := pjm.Issue(domain.Operador{ID: 1, Username: "soporte", Name: "Soporte", Activo: true})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if w := do(t, r, http.MethodGet, "/api/v1/sales?preset=hoy", tok, nil, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("token de plataforma en el negocio = %d, quiere 401", w.Code)
		}
	})
}

// tokenDeOperador emite un acceso de consola para un operador que YA existe en la base, buscándolo
// por su usuario: un token con un id inventado pasaría la firma y moriría en el middleware, que
// vuelve a leer al operador en cada request.
func tokenDeOperador(t *testing.T, st *store.Store, pjm *auth.ManagerDePlataforma, usuario string) string {
	t.Helper()
	var op domain.Operador
	if err := st.Pool.QueryRow(context.Background(),
		`select id, username, name, is_active from platform_operators where username = $1`, usuario,
	).Scan(&op.ID, &op.Username, &op.Name, &op.Activo); err != nil {
		t.Fatalf("buscar al operador %s: %v", usuario, err)
	}
	tok, err := pjm.Issue(op)
	if err != nil {
		t.Fatalf("emitir token de %s: %v", usuario, err)
	}
	return tok
}

// EL OPERADOR APAGADO SE RECHAZA COMO UNO QUE NO EXISTE: misma respuesta y MISMA LATENCIA (FR-013).
//
// El cuerpo igual no basta. Si el camino del apagado se resolviera antes de bcrypt, respondería en
// microsegundos mientras el de "no existe" tarda decenas de milisegundos, y esa diferencia es una
// lista de qué operadores existen — legible desde fuera, sin credenciales.
func TestElOperadorApagadoSeRechazaComoUnoInexistente(t *testing.T) {
	st := newTestStore(t)
	r, _, _ := routerConConsola(t, st)

	const pass = "Contrasena-De-Plataforma-1!"
	crearOperador(t, st, "apagado", pass, false)

	mediana := func(usuario string) (int, string, time.Duration) {
		const n = 5
		var cuerpo string
		var code int
		ds := make([]time.Duration, n)
		for i := range ds {
			inicio := time.Now()
			cuerpoJSON, _ := json.Marshal(map[string]string{"username": usuario, "password": pass})
			w := do(t, r, http.MethodPost, "/api/v1/platform/auth/login", "", cuerpoJSON, "application/json")
			ds[i] = time.Since(inicio)
			code, cuerpo = w.Code, w.Body.String()
		}
		slices.Sort(ds)
		return code, cuerpo, ds[n/2]
	}

	// Usuarios distintos para no tropezar con el lockout por cuenta, que es por usuario.
	codeApagado, cuerpoApagado, tApagado := mediana("apagado")
	codeInexistente, cuerpoInexistente, tInexistente := mediana("no_existe_nadie_asi")

	if codeApagado != http.StatusUnauthorized || codeInexistente != http.StatusUnauthorized {
		t.Fatalf("códigos = apagado %d, inexistente %d; los dos deben ser 401", codeApagado, codeInexistente)
	}
	if cuerpoApagado != cuerpoInexistente {
		t.Fatalf("respuestas distintas:\n apagado    = %s\n inexistente= %s", cuerpoApagado, cuerpoInexistente)
	}
	// Cota holgada a propósito (mismo orden de magnitud): lo que atrapa es la ausencia del bcrypt,
	// que se nota por 100×, no una diferencia de microsegundos bajo carga de CI.
	if tApagado < tInexistente/2 || tApagado > tInexistente*2 {
		t.Fatalf("latencias desiguales (oráculo de enumeración): apagado=%v vs inexistente=%v", tApagado, tInexistente)
	}
}

// Y LO MISMO EN EL LOGIN DEL NEGOCIO (FR-003).
//
// Una credencial de plataforma tecleada en el POS tiene que verse como una cuenta que no existe,
// también en el tiempo. Es la fuga que `auth.CheckDummySecret` ya cierra para el resto del login;
// lo que se prueba aquí es que este camino nuevo no la reabrió.
func TestLaCredencialDePlataformaEnElPOSSeVeComoInexistente(t *testing.T) {
	st := newTestStore(t)
	r, _, _ := routerConConsola(t, st)

	const pass = "Contrasena-De-Plataforma-1!"
	crearOperador(t, st, "soporte_pos", pass, true)

	mediana := func(usuario string) (int, string, time.Duration) {
		const n = 5
		var cuerpo string
		var code int
		ds := make([]time.Duration, n)
		for i := range ds {
			cuerpoJSON, _ := json.Marshal(map[string]string{"username": usuario + "@gatobobah", "password": pass})
			inicio := time.Now()
			w := do(t, r, http.MethodPost, "/api/v1/auth/login", "", cuerpoJSON, "application/json")
			ds[i] = time.Since(inicio)
			code, cuerpo = w.Code, w.Body.String()
		}
		slices.Sort(ds)
		return code, cuerpo, ds[n/2]
	}

	codeOperador, cuerpoOperador, tOperador := mediana("soporte_pos")
	codeInexistente, cuerpoInexistente, tInexistente := mediana("no_existe_nadie_asi")

	if codeOperador != http.StatusUnauthorized || codeInexistente != http.StatusUnauthorized {
		t.Fatalf("códigos = operador %d, inexistente %d; los dos deben ser 401", codeOperador, codeInexistente)
	}
	if cuerpoOperador != cuerpoInexistente {
		t.Fatalf("el POS responde distinto a una credencial de plataforma:\n operador   = %s\n inexistente= %s", cuerpoOperador, cuerpoInexistente)
	}
	if tOperador < tInexistente/2 || tOperador > tInexistente*2 {
		t.Fatalf("latencias desiguales: el POS delata que ese usuario existe en otra superficie (operador=%v vs inexistente=%v)", tOperador, tInexistente)
	}
}

// DESACTIVAR CORTA EL ACCESO EN EL SIGUIENTE REQUEST, no cuando caduque el token (FR-013).
//
// "Retirar el acceso" no puede significar "en quince minutos". Con una sesión viva en la pantalla
// de alguien, apagar al operador tiene que bastar — sin reiniciar la API y sin esperar.
func TestDesactivarAlOperadorLeCortaElAccesoDeInmediato(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	r, _, pjm := routerConConsola(t, st)

	crearOperador(t, st, "soporte_vivo", "Contrasena-De-Plataforma-1!", true)
	tok := tokenDeOperador(t, st, pjm, "soporte_vivo")

	if w := do(t, r, http.MethodGet, "/api/v1/platform/companies", tok, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("con el operador activo = %d, quiere 200: %s", w.Code, w.Body.String())
	}

	if _, err := st.Pool.Exec(ctx,
		`update platform_operators set is_active = false where username = 'soporte_vivo'`); err != nil {
		t.Fatalf("desactivar al operador: %v", err)
	}

	// EL MISMO token, que sigue siendo criptográficamente válido y no ha caducado.
	if w := do(t, r, http.MethodGet, "/api/v1/platform/companies", tok, nil, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("tras desactivarlo = %d, quiere 401: el token sigue abriendo la consola y desactivar no sirvió de nada", w.Code)
	}
}
