//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LÍMITES DEL CONTEO POR EMPRESA (spec 044).

// countLimitsVersion es la migración de los límites del conteo.
const countLimitsVersion = 98

func limitsEqual(a, b domain.CountLimits) bool {
	for _, k := range []string{"masa", "volumen", "longitud", "pieza"} {
		pa, _ := a.ForKind(k)
		pb, _ := b.ForKind(k)
		if !pa.Max.Equal(pb.Max) || !pa.Confirm.Equal(pb.Confirm) {
			return false
		}
	}
	return true
}

// La fila que siembra CreateCompany (con los defaults de la columna) es la de domain: si alguien
// mueve uno sin el otro, una empresa nueva y una sin fila contarían con límites distintos.
func TestCreateCompanySeedsTheDefaultCountLimits(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "limites-siembra")
	svc, ctx := asTenant(t, co)
	got, err := svc.CountLimits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !limitsEqual(got, domain.DefaultCountLimits()) {
		t.Fatalf("sembrados = %+v; quiere domain.DefaultCountLimits", got)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from count_limits where company_id = $1`, co).Scan(&n); err != nil || n != 1 {
		t.Fatalf("filas = %d %v; quiere 1", n, err)
	}
	t.Run("los checks con nombre rechazan cero y máximo ≤ confirmar", func(t *testing.T) {
		for stmt, constraint := range map[string]string{
			`update count_limits set confirm_piece = 0 where company_id = $1`:        "count_limits_positive",
			`update count_limits set max_mass = 50000 where company_id = $1`:         "count_limits_max_over_confirm",
			`update count_limits set max_length = 1000000001 where company_id = $1`:  "count_limits_ceiling",
			`update count_limits set confirm_volume = 2000000 where company_id = $1`: "count_limits_max_over_confirm",
		} {
			_, err := st.Pool.Exec(ctx, stmt, co)
			if err == nil || !strings.Contains(err.Error(), constraint) {
				t.Errorf("%s = %v; quiere %s", stmt, err, constraint)
			}
		}
	})
}

// E1 y E3: leer y guardar bajo el rol de la aplicación; sin fila caen los defaults sin escribir; otra
// empresa no ve ni cambia los de la dueña.
func TestCountLimitsReadAndSaveIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "limites-duena")
	other := makeCompany(t, st, "limites-otra")
	user := makeUserIn(t, st, co, "limites_admin", "admin")
	svc, ctx := asTenant(t, co)

	custom := domain.DefaultCountLimits()
	custom.Mass.Max = dd("2000000")
	custom.Piece.Confirm = dd("100")
	if err := svc.SetCountLimits(ctx, custom, user); err != nil {
		t.Fatal(err)
	}
	got, err := svc.CountLimits(ctx)
	if err != nil || !limitsEqual(got, custom) {
		t.Fatalf("leídos = %+v %v; quiere los guardados", got, err)
	}
	t.Run("el cambio deja su evento con antes y después", func(t *testing.T) {
		var before, after string
		if err := st.Pool.QueryRow(ctx, `select before::text, after::text from audit_events
			where company_id = $1 and action = 'settings.count_limits_changed'`, co).Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(before, `"1000000"`) || !strings.Contains(after, `"2000000"`) {
			t.Fatalf("antes = %s; después = %s", before, after)
		}
	})
	t.Run("límites inválidos no se guardan", func(t *testing.T) {
		bad := custom
		bad.Volume.Max = bad.Volume.Confirm
		if err := svc.SetCountLimits(ctx, bad, user); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v; quiere ErrValidation", err)
		}
	})
	t.Run("sin fila caen los defaults y la lectura no escribe", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `delete from count_limits where company_id = $1`, other); err != nil {
			t.Fatal(err)
		}
		osvc, octx := asTenant(t, other)
		got, err := osvc.CountLimits(octx)
		if err != nil || !limitsEqual(got, domain.DefaultCountLimits()) {
			t.Fatalf("= %+v %v; quiere los defaults", got, err)
		}
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from count_limits where company_id = $1`, other).Scan(&n); err != nil || n != 0 {
			t.Fatalf("la lectura escribió una fila (%d, %v)", n, err)
		}
	})
	inTheThreeCases(t, co, other, func(t *testing.T, s *store.Store, c context.Context) {
		svc := app.NewBackofficeService(s, nil)
		if l, err := svc.CountLimits(c); err == nil && limitsEqual(l, custom) {
			t.Fatal("otra sesión ve los límites de la dueña")
		}
		_ = svc.SetCountLimits(c, domain.DefaultCountLimits(), user)
		var max string
		if err := st.Pool.QueryRow(context.Background(), `select max_mass::text from count_limits where company_id = $1`, co).Scan(&max); err != nil || max != "2000000.0000" {
			t.Fatalf("máximo de la dueña = %s %v; otra sesión lo cambió", max, err)
		}
	})
}

// US4: el máximo de la empresa decide qué se rechaza, con el mensaje en la unidad grande.
func TestTheCountRejectsOverTheCompanyMax(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	_, err = k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: dd("1500000")}, k.user)
	if !errors.Is(err, domain.ErrValidation) || !strings.HasSuffix(err.Error(), "Máximo 1,000 kg por conteo") {
		t.Fatalf("1,500 kg = %v; quiere «Máximo 1,000 kg por conteo»", err)
	}
	limits := domain.DefaultCountLimits()
	limits.Mass.Max = dd("2000000")
	if err := k.svc.SetCountLimits(k.ctx, limits, k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: dd("1500000")}, k.user); err != nil {
		t.Fatalf("1,500 kg con el máximo en 2,000 kg = %v", err)
	}
	t.Run("bajar el máximo no toca lo ya capturado (E4)", func(t *testing.T) {
		limits.Mass.Max = dd("100000")
		if err := k.svc.SetCountLimits(k.ctx, limits, k.user); err != nil {
			t.Fatal(err)
		}
		var counted string
		if err := k.st.Pool.QueryRow(context.Background(), `select counted_qty::text from stock_count_lines where count_id = $1 and ingredient_id = $2`,
			c.ID, k.azucar).Scan(&counted); err != nil || counted != "1500000.0000" {
			t.Fatalf("lo capturado = %s %v", counted, err)
		}
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.leche, app.CountCapture{Qty: dd("200000")}, k.user); err != nil {
			t.Fatalf("la leche (otra clase) = %v", err)
		}
		_, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.azucar, app.CountCapture{Qty: dd("150000")}, k.user)
		if err == nil || !strings.HasSuffix(err.Error(), "Máximo 100 kg por conteo") {
			t.Fatalf("la siguiente captura = %v; quiere el máximo nuevo", err)
		}
	})
	t.Run("lo pesado convertido a piezas usa el máximo de piezas (E7)", func(t *testing.T) {
		popotes := ingredientIn(t, k.st, defaultCompanyID, "Popotes límite", "pieza")
		kg := unitID(t, k.st, "kg")
		if _, err := k.st.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate)
			values ($1, $2, 'Kilo', 400, $3, true)`, defaultCompanyID, popotes, kg); err != nil {
			t.Fatal(err)
		}
		limits.Piece.Max = dd("10000")
		if err := k.svc.SetCountLimits(k.ctx, limits, k.user); err != nil {
			t.Fatal(err)
		}
		// 30 kg × 400 = 12,000 piezas: el peso es poco, las piezas pasan el máximo.
		_, err := k.svc.CaptureCountChecked(k.ctx, c.ID, popotes, app.CountCapture{Weight: new(dd("30")), WeightUnitID: &kg}, k.user)
		if err == nil || !strings.HasSuffix(err.Error(), "Máximo 10,000 piezas por conteo") {
			t.Fatalf("= %v; quiere el máximo de piezas", err)
		}
	})
}

// El aviso sin otra referencia usa la cifra para confirmar de la empresa.
func TestTheCountConfirmUsesTheCompanyFigure(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	vasos := ingredientIn(t, k.st, defaultCompanyID, "Vasos límite", "pieza")
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}
	limits := domain.DefaultCountLimits()
	limits.Piece.Confirm = dd("100")
	if err := k.svc.SetCountLimits(k.ctx, limits, k.user); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, vasos, app.CountCapture{Qty: dd("150"), Confirmable: true}, k.user); !errors.Is(err, domain.ErrCountNeedsConfirm) {
		t.Fatalf("150 piezas con confirmar en 100 = %v; quiere el aviso", err)
	}
}

func newLimitsAPI(t *testing.T) (http.Handler, *store.Store, func(username, role string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	// Quien solo cuenta: un rol con inventory.count y sin inventory.configure (roles propios).
	perms := func(r domain.Role) []domain.Permission {
		if r == domain.RoleCajero {
			return []domain.Permission{domain.PermInventoryCount}
		}
		return domain.PermissionsFor(r)
	}
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), MenuCache: cache.NewMenuCache(""),
		Broker: realtime.NewBroker(), Permissions: perms,
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	return r, st, token
}

// E10: quien cuenta lee los límites (los necesita el aviso) y no los cambia; quien no cuenta ni
// configura no los ve.
func TestCountLimitsHTTP(t *testing.T) {
	t.Parallel()
	r, _, token := newLimitsAPI(t)
	admin := token("limites_http_admin", "admin")
	counter := token("limites_http_cuenta", "cajero")
	waiter := token("limites_http_mesero", "mesero")
	const path = "/api/v1/settings/count-limits"

	w := do(t, r, http.MethodGet, path, counter, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("quien cuenta lee = %d %s", w.Code, w.Body.String())
	}
	var got map[string]map[string]json.Number
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	if got["mass"]["max"] != "1000000" || got["length"]["confirm"] != "5000" || got["piece"]["confirm"] != "5000" {
		t.Fatalf("GET = %s; quiere los defaults como números", w.Body.String())
	}
	if w := do(t, r, http.MethodGet, path, waiter, nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("sin contar ni configurar = %d; quiere 403", w.Code)
	}
	body := `{"mass":{"max":2000000,"confirm":50000},"volume":{"max":1000000,"confirm":50000},"length":{"max":1000000,"confirm":5000},"piece":{"max":1000000,"confirm":5000}}`
	if w := do(t, r, http.MethodPut, path, counter, []byte(body), "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("quien cuenta cambia = %d; quiere 403", w.Code)
	}
	bad := strings.Replace(body, `"max":2000000,"confirm":50000`, `"max":50000,"confirm":50000`, 1)
	if w := do(t, r, http.MethodPut, path, admin, []byte(bad), "application/json"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "el máximo tiene que ser mayor") {
		t.Fatalf("máximo = confirmar = %d %s; quiere 400 con el porqué", w.Code, w.Body.String())
	}
	// Lo absurdo en la frontera es 400, nunca 500 (constitución V): cada caso reemplaza el par de peso.
	// Las cifras se redondean a 4 decimales antes de validar: 1000000000.00001 se guarda como el techo.
	massPair := `"mass":{"max":2000000,"confirm":50000}`
	for name, mass := range map[string]string{
		"NaN entre comillas":                 `"mass":{"max":"NaN","confirm":50000}`,
		"1e30":                               `"mass":{"max":1e30,"confirm":50000}`,
		"sobre el techo por un diezmilésimo": `"mass":{"max":1000000000.0001,"confirm":50000}`,
		"campo ausente":                      `"mass":{"max":2000000}`,
		"campo null":                         `"mass":{"max":2000000,"confirm":null}`,
		"campo vacío":                        `"mass":{"max":2000000,"confirm":""}`,
		"negativo":                           `"mass":{"max":2000000,"confirm":-1}`,
		"más de 4 decimales redondea a cero": `"mass":{"max":0.00002,"confirm":0.00001}`,
	} {
		b := strings.Replace(body, massPair, mass, 1)
		if w := do(t, r, http.MethodPut, path, admin, []byte(b), "application/json"); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s; quiere 400", name, w.Code, w.Body.String())
		}
	}
	w = do(t, r, http.MethodPut, path, admin, []byte(body), "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"max":2000000`) {
		t.Fatalf("admin guarda = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, path, counter, nil, ""); !strings.Contains(w.Body.String(), `"max":2000000`) {
		t.Fatalf("tras guardar = %s", w.Body.String())
	}
}

// LA 0098 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV): cada empresa queda con su fila y
// los defaults de domain, y el Down vuelve a la 0097 limpio.
func TestMigrationCountLimitsOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	start := versionDeEsquema(t, st)
	if start < countLimitsVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, countLimitsVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	var exists bool
	if err := st.Pool.QueryRow(ctx, `select to_regclass('count_limits') is not null`).Scan(&exists); err != nil || exists {
		t.Fatalf("tras el Down la tabla sigue (%v, %v)", exists, err)
	}
	migrarArriba(t, st.Pool)
	var companies, rows, wrong int
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from companies), (select count(*) from count_limits),
		(select count(*) from count_limits where max_mass <> 1000000 or max_volume <> 1000000 or max_length <> 1000000 or max_piece <> 1000000
		   or confirm_mass <> 50000 or confirm_volume <> 50000 or confirm_length <> 5000 or confirm_piece <> 5000)`).Scan(&companies, &rows, &wrong); err != nil {
		t.Fatal(err)
	}
	if companies < 2 || rows != companies || wrong != 0 {
		t.Fatalf("empresas %d, filas %d, distintas del default %d", companies, rows, wrong)
	}
	d := domain.DefaultCountLimits()
	if !d.Mass.Max.Equal(dd("1000000")) || !d.Piece.Confirm.Equal(dd("5000")) {
		t.Fatal("domain.DefaultCountLimits se separó de los defaults de la 0098")
	}
}
