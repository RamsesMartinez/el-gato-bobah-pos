//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/secrets"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// cifradorDePrueba es el respaldo local, con una llave fija por "ambiente". Dos llaves distintas
// simulan un respaldo de producción restaurado en otra máquina.
func testCipher(t *testing.T, env byte) *secrets.Local {
	t.Helper()
	l, err := secrets.NewLocal(bytes.Repeat([]byte{env}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// clienteFalso hace de la plataforma: acepta o rechaza al verificar, y cuenta cuántas veces se armó.
type fakeClient struct {
	decisorFalso
	lectorFalso
	rejection error
	built     *atomic.Int32
	id        string
}

func (c *fakeClient) Verify(context.Context) error { return c.rejection }
func (c *fakeClient) ClaseDeFallo(error) domain.ClaseDeFallo {
	return domain.FalloRespuestaInvalida
}

// fabricaFalsa arma clientes que recuerdan con qué credenciales nacieron. `rechazo` es lo que la
// plataforma respondería al verificar.
func fakeFactory(built *atomic.Int32, rejection *error) app.PlatformClientFactory {
	return func(c domain.AppCredentials) (app.PlatformClient, error) {
		built.Add(1)
		return &fakeClient{rejection: *rejection, built: built, id: c.ClientID}, nil
	}
}

func testCredentialsService(t *testing.T, st *store.Store, cif app.Cipher, rejection *error) (*app.PlatformCredentialsService, *atomic.Int32) {
	t.Helper()
	var built atomic.Int32
	return app.NewPlatformCredentialsService(st, cif,
		map[string]app.PlatformClientFactory{"Uber Eats": fakeFactory(&built, rejection)}, "sandbox"), &built
}

const (
	testClientID     = "client-id-de-la-empresa"
	testClientSecret = "secreto-de-la-app-0123456789"
)

// LA MIGRACIÓN 0075, BAJO EL ROL DE LA APLICACIÓN. Lo que un unitario no ve: el grant (sin él,
// 42501 en el primer request de producción), la política de RLS (no existe para el owner) y que la
// FK compuesta no deje apuntar a la plataforma de otra empresa (la integridad referencial salta
// RLS). Y que la llave de firma ya no tenga dónde guardarse en claro.
func TestTheCredentialsMigrationIsolatesAndStoresNoPlaintext(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "cred-a")
	b := makeCompany(t, st, "cred-b")
	userA := makeUserIn(t, st, a, "admin-cred-a", "admin")
	platA := platformID(t, st, a, "Uber Eats")
	platB := platformID(t, st, b, "Uber Eats")

	appSt := appRoleStore(t)
	// Conexiones con el tenant fijado EN ellas: `st.Pool` a secas consultaría como la empresa por
	// defecto y el test pasaría con la política borrada.
	connA := conexionDeEmpresa(t, appSt, a)
	insert := func(plat int16) error {
		_, err := connA.Exec(ctx,
			`insert into platform_credentials (delivery_platform_id, client_id, client_secret_encrypted, updated_by)
			 values ($1, 'un-client-id', '\x0102030405060708090a0b0c0d0e0f10', $2)`, plat, userA)
		return err
	}
	if err := insert(platA); err != nil {
		t.Fatalf("el rol de la app no puede escribir su credencial (¿falta el GRANT?): %v", err)
	}
	// La plataforma de OTRA empresa, desde la sesión de A: la FK compuesta lo rechaza.
	exigeViolacionDeRestriccion(t, insert(platB), "una credencial de A colgada de la plataforma de B")

	var n int
	if err := conexionDeEmpresa(t, appSt, b).QueryRow(ctx, `select count(*) from platform_credentials`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("la empresa B ve %d credenciales de la A: RLS no aísla", n)
	}

	// Ninguna columna de secreto en texto: las que quedan son bytea.
	rows, err := st.Pool.Query(ctx, `
		select table_name, column_name, data_type from information_schema.columns
		 where table_name in ('platform_webhook_keys','platform_credentials')
		   and (column_name like 'key_%' or column_name like 'client_secret%')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var table, col, tipo string
		_ = rows.Scan(&table, &col, &tipo)
		seen++
		if tipo != "bytea" {
			t.Errorf("%s.%s es %s: un secreto de un tercero quedaría en claro en cada respaldo", table, col, tipo)
		}
	}
	if seen != 3 {
		t.Fatalf("esperaba 3 columnas de secreto (dos llaves y el client secret), hay %d", seen)
	}
}

// SOLO SE GUARDA LO QUE LA PLATAFORMA ACEPTA, Y NADA DE LO GUARDADO VUELVE A LA PANTALLA.
func TestCredentialsAreSavedOnlyIfThePlatformAcceptsThem(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	company := makeCompany(t, st, "cred-guardar")
	user := makeUserIn(t, st, company, "admin-cred-guardar", "admin")
	plat := platformID(t, st, company, "Uber Eats")

	appSt := appRoleStore(t)
	var rejection error = domain.ErrCredentialsRejected
	svc, _ := testCredentialsService(t, appSt, testCipher(t, 1), &rejection)
	ctxT, release, err := appSt.AcquireTenant(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	for _, r := range []error{domain.ErrCredentialsRejected, domain.ErrCredentialsMissingScopes, domain.ErrPlatformUnavailable} {
		rejection = r
		if err := svc.Save(ctxT, company, user, plat, testClientID, testClientSecret); !errors.Is(err, r) {
			t.Fatalf("con la plataforma respondiendo %v, Guardar dio %v", r, err)
		}
		if e, _ := svc.State(ctxT, plat); e.Configured {
			t.Fatalf("tras un rechazo (%v) quedó guardada: guardar sin comprobar es lo que la comprobación evita", r)
		}
	}

	rejection = nil
	if err := svc.Save(ctxT, company, user, plat, " "+testClientID+"\n", testClientSecret+"\n"); err != nil {
		t.Fatalf("guardar credenciales buenas: %v", err)
	}
	e, err := svc.State(ctxT, plat)
	if err != nil || !e.Configured || e.ClientID != testClientID || e.NeedsRecapture || e.UpdatedBy == "" || e.Environment != "sandbox" {
		t.Fatalf("estado tras guardar = %+v, %v", e, err)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), testClientSecret) || strings.Contains(string(raw), "secreto") {
		t.Fatalf("el estado que va a la pantalla trae el secreto: %s", raw)
	}

	// En la base, el secreto NO aparece en claro.
	var saved []byte
	if err := st.Pool.QueryRow(ctx, `select client_secret_encrypted from platform_credentials where company_id = $1`, company).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte(testClientSecret)) {
		t.Fatal("el client secret quedó en claro en la base: cualquier respaldo lo trae")
	}
}

// UN RESPALDO DE OTRO AMBIENTE NO TUMBA LA PANTALLA: DICE «VUELVE A CAPTURARLAS».
//
// Es lo que pasa cada vez que se restaura producción en local (`make db-restaurar`): las
// credenciales llegan, pero cifradas con una llave que aquí no existe. Eso es la protección, y la
// respuesta correcta es pedir recaptura — no un 500, y no «sin configurar», que haría creer que
// nunca se capturaron.
func TestABackupFromAnotherEnvironmentNeedsRecapture(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	company := makeCompany(t, st, "cred-otro-ambiente")
	user := makeUserIn(t, st, company, "admin-cred-amb", "admin")
	plat := platformID(t, st, company, "Uber Eats")
	appSt := appRoleStore(t)
	ctxT, release, err := appSt.AcquireTenant(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	var noError error
	prod, _ := testCredentialsService(t, appSt, testCipher(t, 1), &noError)
	if err := prod.Save(ctxT, company, user, plat, testClientID, testClientSecret); err != nil {
		t.Fatal(err)
	}

	local, _ := testCredentialsService(t, appSt, testCipher(t, 2), &noError)
	e, err := local.State(ctxT, plat)
	if err != nil {
		t.Fatalf("el estado con credenciales de otro ambiente dio error (sería un 500): %v", err)
	}
	if !e.Configured || !e.NeedsRecapture {
		t.Fatalf("esperaba configurada y por recapturar, fue %+v", e)
	}
	if _, err := local.MenuReaderFor(ctxT, "Uber Eats"); !errors.Is(err, domain.ErrCredentialsUnreadable) {
		t.Fatalf("leer menús con credenciales ilegibles: esperaba ErrCredencialesPorRecapturar, fue %v", err)
	}
	// Recapturar lo arregla sin tocar nada más.
	if err := local.Save(ctxT, company, user, plat, testClientID, testClientSecret); err != nil {
		t.Fatal(err)
	}
	if e, _ := local.State(ctxT, plat); e.NeedsRecapture {
		t.Fatalf("tras recapturar sigue pidiendo recaptura: %+v", e)
	}
}

// LA EMPRESA B NO HABLA CON LA PLATAFORMA CON LA APP DE LA A. Era el riesgo que dejó escrito el
// plan de la 021: el `accept_pos_order` de una empresa saliendo con la identidad de otra.
//
// Tres caminos para que pasara, y los tres se cierran aquí: RLS (B no lee la fila de A), la memoria
// de clientes (armada por A, pedida por B) y el cifrado copiado a mano a la fila de B (AAD).
func TestCompanyBDoesNotUseCompanyAsApp(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "cred-app-a")
	b := makeCompany(t, st, "cred-app-b")
	userA := makeUserIn(t, st, a, "admin-app-a", "admin")
	userB := makeUserIn(t, st, b, "admin-app-b", "admin")
	platA := platformID(t, st, a, "Uber Eats")
	platB := platformID(t, st, b, "Uber Eats")
	appSt := appRoleStore(t)

	var noError error
	svc, _ := testCredentialsService(t, appSt, testCipher(t, 1), &noError)
	ctxA, releaseA, _ := appSt.AcquireTenant(ctx, a)
	defer releaseA()
	if err := svc.Save(ctxA, a, userA, platA, "client-id-de-a", testClientSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MenuReaderFor(ctxA, "Uber Eats"); err != nil {
		t.Fatalf("la empresa A no puede usar su propia app: %v", err)
	}

	ctxB, releaseB, _ := appSt.AcquireTenant(ctx, b)
	defer releaseB()
	if _, err := svc.OrderDeciderFor(ctxB, "Uber Eats"); !errors.Is(err, domain.ErrPlataformaSinCredenciales) {
		t.Fatalf("la empresa B, sin credenciales propias, obtuvo un cliente (¿el de A?): %v", err)
	}

	// Alguien copia a mano el cifrado de A a la fila de B.
	if _, err := st.Pool.Exec(ctx, `
		insert into platform_credentials (company_id, delivery_platform_id, client_id, client_secret_encrypted, updated_by)
		select $1, $2, client_id, client_secret_encrypted, $3 from platform_credentials where company_id = $4`,
		b, platB, userB, a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OrderDeciderFor(ctxB, "Uber Eats"); !errors.Is(err, domain.ErrCredentialsUnreadable) {
		t.Fatalf("el secreto de A copiado a la fila de B se pudo usar: %v", err)
	}
}

// Recapturar cambia el cliente con el que se habla; el cliente viejo, con su token, no se queda
// atendiendo. Y mientras nada cambie, se reusa: armar uno por aviso gastaría un token por aviso.
func TestTheClientIsReusedUntilCredentialsChange(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	company := makeCompany(t, st, "cred-reuso")
	user := makeUserIn(t, st, company, "admin-reuso", "admin")
	plat := platformID(t, st, company, "Uber Eats")
	appSt := appRoleStore(t)
	var noError error
	svc, built := testCredentialsService(t, appSt, testCipher(t, 1), &noError)
	ctxT, release, _ := appSt.AcquireTenant(ctx, company)
	defer release()

	if err := svc.Save(ctxT, company, user, plat, "client-id-primero", testClientSecret); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.OrderDeciderFor(ctxT, "Uber Eats"); err != nil {
			t.Fatal(err)
		}
	}
	if n := built.Load(); n != 1 {
		t.Fatalf("se armaron %d clientes para una sola credencial: el que se verificó al guardar es el que atiende", n)
	}
	if err := svc.Save(ctxT, company, user, plat, "client-id-segundo", testClientSecret+"-2"); err != nil {
		t.Fatal(err)
	}
	d, err := svc.OrderDeciderFor(ctxT, "Uber Eats")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.(*fakeClient).id; got != "client-id-segundo" {
		t.Fatalf("tras recapturar se sigue hablando con el cliente de %q", got)
	}
}

// LA LLAVE DE FIRMA TAMPOCO QUEDA EN CLARO, y cifrada sigue validando.
func TestTheSigningKeyIsStoredEncrypted(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	company := makeCompany(t, st, "llave-cifrada")
	_, plat := tiendaSinLlave(t, st, company, "tienda-cifrada")
	appSt := appRoleStore(t)
	svc := servicioDePedidos(appSt)
	ctxT, release, _ := appSt.AcquireTenant(ctx, company)
	defer release()

	const key = "whsec_una-llave-que-no-se-ve-01"
	if err := svc.GuardarLlave(ctxT, company, plat, key); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := st.Pool.QueryRow(ctx, `select key_primary_encrypted from platform_webhook_keys where company_id = $1`, company).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("whsec_")) {
		t.Fatal("la llave de firma quedó en claro")
	}
	// Con la llave de OTRO ambiente la llave no se puede leer: el estado lo dice.
	otherEnv := app.NewPedidosDePlataformaService(appSt, fixedClients{}, testCipher(t, 9), "sandbox", clock)
	e, err := otherEnv.EstadoDeLlave(ctxT, plat)
	if err != nil || !e.Configurada || !e.NeedsRecapture {
		t.Fatalf("estado con la llave de otro ambiente = %+v, %v", e, err)
	}
}

// SOLO EL ADMINISTRADOR CAMBIA LO QUE DEJA ENTRAR O ACEPTAR PEDIDOS. El gerente ve el estado, pero
// no captura ni retira: con la llave de firma se decide qué entra a la cocina, y con dos PUT seguidos
// un gerente dejaba fuera la llave real y ningún pedido volvía a validar.
//
// Por el ROUTER real: el gate vive en el cableado de rutas, y moverlo no rompe ningún test de
// servicio.
func TestOnlyTheAdminChangesKeysAndCredentials(t *testing.T) {
	st := newTestStore(t)
	company := makeCompany(t, st, "gate-llaves")
	_, plat := tiendaSinLlave(t, st, company, "tienda-gate")
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	var noError error
	creds, _ := testCredentialsService(t, st, testCipher(t, 1), &noError)
	h := httpapi.NewHandlers(httpapi.Deps{
		Cfg: config.Config{}, JWT: jm,
		PedidosPlataforma: servicioDePedidos(st),
		Credentials:       creds,
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUserIn(t, st, company, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: company, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	manager, admin := token("gate-gerente", "gerente"), token("gate-admin", "admin")
	key, _ := json.Marshal(map[string]string{"key": "whsec_llave-del-gerente-0001"})
	credsBody, _ := json.Marshal(map[string]string{"clientId": testClientID, "clientSecret": testClientSecret})
	base := "/api/v1/admin/platform-menus"
	p := itoa(int(plat))

	for _, c := range []struct {
		method, path string
		body         []byte
	}{
		{http.MethodPut, base + "/webhook-keys/" + p, key},
		{http.MethodDelete, base + "/webhook-keys/" + p + "/previous", nil},
		{http.MethodPut, base + "/credentials/" + p, credsBody},
	} {
		if code := do(t, r, c.method, c.path, manager, c.body, "application/json").Code; code != http.StatusForbidden {
			t.Errorf("gerente %s %s: %d, se esperaba 403", c.method, c.path, code)
		}
	}
	// Ver el estado sí puede: es lo que le dice a quién pedírselo.
	for _, path := range []string{base + "/webhook-keys/" + p, base + "/credentials/" + p} {
		if code := do(t, r, http.MethodGet, path, manager, nil, "").Code; code != http.StatusOK {
			t.Errorf("gerente GET %s: %d, se esperaba 200", path, code)
		}
	}
	// Y el administrador sí captura las dos cosas.
	if code := do(t, r, http.MethodPut, base+"/webhook-keys/"+p, admin, key, "application/json").Code; code != http.StatusNoContent {
		t.Errorf("admin PUT llave: %d, se esperaba 204", code)
	}
	if code := do(t, r, http.MethodPut, base+"/credentials/"+p, admin, credsBody, "application/json").Code; code != http.StatusNoContent {
		t.Errorf("admin PUT credenciales: %d, se esperaba 204", code)
	}
}

// LAS CREDENCIALES DE A NO LAS ALCANZA NADIE MÁS, en los tres contextos donde RLS ya falló.
func TestCredentialsAreUnreachableInTheThreeCases(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "tres-casos-a")
	b := makeCompany(t, st, "tres-casos-b")
	userA := makeUserIn(t, st, a, "admin-tres-a", "admin")
	platA := platformID(t, st, a, "Uber Eats")
	cif := testCipher(t, 1)
	var noError error

	ownerStore := appRoleStore(t)
	svcA, _ := testCredentialsService(t, ownerStore, cif, &noError)
	ctxA, releaseA, _ := ownerStore.AcquireTenant(ctx, a)
	if err := svcA.Save(ctxA, a, userA, platA, testClientID, testClientSecret); err != nil {
		t.Fatal(err)
	}
	releaseA()

	inTheThreeCases(t, a, b, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc, _ := testCredentialsService(t, st, cif, &noError)
		if c, err := svc.OrderDeciderFor(ctx, "Uber Eats"); err == nil {
			t.Fatalf("se obtuvo un cliente (%T) con las credenciales de otra empresa", c)
		}
		if e, err := svc.State(ctx, platA); err == nil && e.Configured {
			t.Fatalf("se vio el estado de las credenciales de otra empresa: %+v", e)
		}
	})
}

// CON RLS ROTO, EL CIFRADO SIGUE CERRANDO. Se simula con el owner, que salta RLS: la sesión de B
// lee la fila de A. Si la AAD se arma con la empresa DE LA FILA, descifra y B usa la app de A; con
// la empresa de la SESIÓN, no descifra. Es la segunda barrera que la documentación promete, y sin
// este test la promesa no tiene quién la vigile.
func TestWithBrokenRLSCompanyBCannotDecryptCompanyAsCredential(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "rls-roto-a")
	b := makeCompany(t, st, "rls-roto-b")
	userA := makeUserIn(t, st, a, "admin-rls-a", "admin")
	platA := platformID(t, st, a, "Uber Eats")
	cif := testCipher(t, 1)
	var noError error

	svc, _ := testCredentialsService(t, st, cif, &noError) // owner: RLS no aplica
	ctxA, releaseA, _ := st.AcquireTenant(ctx, a)
	if err := svc.Save(ctxA, a, userA, platA, testClientID, testClientSecret); err != nil {
		t.Fatal(err)
	}
	releaseA()

	ctxB, releaseB, _ := st.AcquireTenant(ctx, b)
	defer releaseB()
	if c, err := svc.OrderDeciderFor(ctxB, "Uber Eats"); err == nil {
		t.Fatalf("con RLS roto, la empresa B obtuvo el cliente de A (%T): la AAD sale de la fila, no de la sesión", c)
	}
	// Y sin empresa en el contexto no se adivina: se niega.
	if _, err := svc.OrderDeciderFor(ctx, "Uber Eats"); err == nil {
		t.Fatal("sin empresa en el contexto se obtuvo un cliente")
	}
}

// downCipher simula KMS sin responder: ni cifra ni descifra.
type downCipher struct{}

func (downCipher) Encrypt(context.Context, []byte, string) ([]byte, error) {
	return nil, secrets.ErrUnavailable
}
func (downCipher) Decrypt(context.Context, []byte, string) ([]byte, error) {
	return nil, secrets.ErrUnavailable
}

// CON KMS CAÍDO AL GUARDAR, NO SE GUARDA NADA Y EL ERROR DICE QUÉ FALLÓ. Uber ya aceptó las
// credenciales, pero sin cifrarlas no hay cómo guardarlas: se responde que el servicio de llaves no
// respondió —para que la pantalla diga «intenta de nuevo»— y no un error interno sin nombre.
func TestKeyServiceDownOnSaveStoresNothingAndSaysSo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	company := makeCompany(t, st, "kms-caido")
	user := makeUserIn(t, st, company, "admin-kms-caido", "admin")
	plat := platformID(t, st, company, "Uber Eats")
	appSt := appRoleStore(t)
	var noError error
	svc, _ := testCredentialsService(t, appSt, downCipher{}, &noError)
	ctxT, release, _ := appSt.AcquireTenant(ctx, company)
	defer release()

	if err := svc.Save(ctxT, company, user, plat, testClientID, testClientSecret); !errors.Is(err, domain.ErrKeyServiceUnavailable) {
		t.Fatalf("con KMS caído, Save dio %v; se esperaba ErrKeyServiceUnavailable", err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from platform_credentials where company_id = $1`, company).Scan(&n); err != nil || n != 0 {
		t.Fatalf("quedaron %d filas guardadas (%v)", n, err)
	}
}
