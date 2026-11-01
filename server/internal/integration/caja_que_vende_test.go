//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// cajaQueVendeYCierra deja la caja principal con un turno que vendió $80 en efectivo y ya cerró.
// Devuelve la caja y el turno.
func cajaQueVendeYCierra(t *testing.T, st *store.Store, cajero int64) (reg, sess int64) {
	t.Helper()
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	reg = principalRegister(t, st)
	abierta, err := back.OpenSession(ctx, reg, app.AperturaCmd{}, cajero)
	if err != nil {
		t.Fatalf("abrir: %v", err)
	}
	prod := makeProduct(t, st, "Café 049", decimal.RequireFromString("80"), false)
	if _, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: paymentMethodID(t, st, "Efectivo"), Amount: decimal.RequireFromString("80")}},
	}); err != nil {
		t.Fatalf("vender: %v", err)
	}
	entregarPendientes(t, st)
	if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`, abierta.ID, cajero); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	return reg, abierta.ID
}

// R1: QUÉ CAJA VENDIÓ UN TURNO PASADO NO SE LEE DE QUÉ CAJA VENDE HOY. A vende y cierra; se pasa la
// venta a B. «Hasta cuándo se ve un entregado» y el detalle del corte de A no pueden cambiar: si
// cambian, pasar la venta reescribe el pasado.
func TestMovingSalesDoesNotRewriteThePastOfTheOldRegister(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_049_pasado", "cajero")
	regA, sessA := cajaQueVendeYCierra(t, st, cajero)

	cerroAntes, err := st.Q.CerroLaCajaPrincipal(ctx)
	if err != nil {
		t.Fatalf("último cierre antes de pasar la venta: %v", err)
	}
	detalleAntes, err := back.SessionDetail(ctx, sessA)
	if err != nil {
		t.Fatal(err)
	}

	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (name) values ('Barra 049') returning id`).Scan(&regB); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update cash_registers set is_primary = false where id = $1`, regA); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update cash_registers set is_primary = true, counts_cash = true where id = $1`, regB); err != nil {
		t.Fatal(err)
	}

	cerroDespues, err := st.Q.CerroLaCajaPrincipal(ctx)
	if err != nil {
		t.Fatalf("último cierre tras pasar la venta a otra caja: %v (los entregados del día dejarían de tener corte)", err)
	}
	if !cerroDespues.Time.Equal(cerroAntes.Time) {
		t.Fatalf("el último cierre pasó de %v a %v solo por cambiar qué caja vende", cerroAntes.Time, cerroDespues.Time)
	}
	detalleDespues, err := back.SessionDetail(ctx, sessA)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(detalleAntes)
	b, _ := json.Marshal(detalleDespues)
	if string(a) != string(b) {
		t.Fatalf("el corte de la caja vieja cambió al pasar la venta:\nantes   %s\ndespués %s", a, b)
	}
}

// otraCaja da de alta una caja secundaria activa de la empresa por omisión.
func otraCaja(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `insert into cash_registers (name, counts_cash) values ($1, false) returning id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// primarias cuenta las cajas que venden en la empresa por omisión.
func primarias(t *testing.T, st *store.Store) []int64 {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(), `select id from cash_registers where company_id = $1 and is_primary order by id`, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

// PASAR LA VENTA (spec 049, historia 3): con las dos cerradas, de un solo golpe; la nueva lleva arqueo.
func TestMoveSales(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc, ctx := asTenant(t, defaultCompanyID)
	admin := makeUser(t, st, "admin_049_vende", "admin")
	regA := principalRegister(t, st)
	regB := otraCaja(t, st, "Barra 049 mover")

	if err := svc.MoveSales(ctx, regB, admin); err != nil {
		t.Fatalf("pasar la venta con las dos cerradas: %v", err)
	}
	if got := primarias(t, st); len(got) != 1 || got[0] != regB {
		t.Fatalf("cajas que venden = %v; quiere solo %d", got, regB)
	}
	var counts bool
	if err := st.Pool.QueryRow(context.Background(), `select counts_cash from cash_registers where id = $1`, regB).Scan(&counts); err != nil || !counts {
		t.Fatalf("la nueva caja que vende quedó sin arqueo (%v)", err)
	}
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = 'cash.selling_register_changed'
		and (before->>'registerId')::bigint = $1 and (after->>'registerId')::bigint = $2`, regA, regB).Scan(&n); err != nil || n != 1 {
		t.Errorf("bitácora cash.selling_register_changed = %d (%v); quiere 1 de %d a %d", n, err, regA, regB)
	}
	// La que ya vende: no cambia nada ni deja otra bitácora.
	if err := svc.MoveSales(ctx, regB, admin); err != nil {
		t.Fatalf("la que ya vende: %v", err)
	}
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = 'cash.selling_register_changed'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("un no-op dejó bitácora: %d eventos", n)
	}
	// Mientras la nueva no tiene turno, vender da el 409 de siempre.
	if open, err := svc.SellingRegisterOpen(ctx); err != nil || open {
		t.Errorf("con la nueva caja cerrada se puede cobrar = %v (%v)", open, err)
	}

	// Con turno abierto en cualquiera de las dos: 409, y nada cambia.
	cajero := makeUser(t, st, "cajero_049_vende", "cajero")
	back := app.NewBackofficeService(st, clock)
	if _, err := back.OpenSession(context.Background(), regB, app.AperturaCmd{}, cajero); err != nil {
		t.Fatalf("abrir la que vende: %v", err)
	}
	if err := svc.MoveSales(ctx, regA, admin); !errors.Is(err, domain.ErrRegistersMustBeClosed) {
		t.Fatalf("pasar la venta con la que vende abierta = %v; quiere ErrRegistersMustBeClosed", err)
	}
	if got := primarias(t, st); len(got) != 1 || got[0] != regB {
		t.Fatalf("un rechazo cambió la caja que vende: %v", got)
	}

	// Destino inactivo: 400.
	inactiva := otraCaja(t, st, "Vieja 049")
	if _, err := st.Pool.Exec(context.Background(), `update cash_registers set is_active = false where id = $1`, inactiva); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveSales(ctx, inactiva, admin); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("destino inactivo = %v; quiere ErrValidation", err)
	}
	// Una caja inexistente o de otra empresa: 404.
	if err := svc.MoveSales(ctx, 999999, admin); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("caja inexistente = %v; quiere ErrNotFound", err)
	}
}

// Otra sucursal: la caja de una sucursal no recibe las ventas de otra.
func TestMoveSalesRejectsAnotherBranch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc, ctx := asTenant(t, defaultCompanyID)
	admin := makeUser(t, st, "admin_049_sucursal", "admin")
	// Sucursal inactiva: «la sucursal» de la sesión sigue siendo la matriz, sin selector.
	var branch, reg int64
	if err := st.Pool.QueryRow(context.Background(), `insert into branches (company_id, name, code, is_active)
		values ($1, 'Norte 049', 'NORTE049', false) returning id`, defaultCompanyID).Scan(&branch); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(context.Background(), `insert into cash_registers (name, branch_id) values ('Caja norte', $1) returning id`, branch).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveSales(ctx, reg, admin); !errors.Is(err, domain.ErrOtherBranch) {
		t.Fatalf("caja de otra sucursal = %v; quiere ErrOtherBranch", err)
	}
}

// Pasar la venta y abrir turno a la vez: nunca dos ni ninguna, y si la apertura gana, el cambio se
// rechaza en vez de dejar un turno abierto en una caja que ya no vende.
func TestMoveSalesRacesOpeningAShift(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin := makeUser(t, st, "admin_049_carrera", "admin")
	cajero := makeUser(t, st, "cajero_049_carrera", "cajero")
	regA := principalRegister(t, st)
	regB := otraCaja(t, st, "Barra 049 carrera")
	var moveErr, openErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc, ctx := asTenant(t, defaultCompanyID)
		moveErr = svc.MoveSales(ctx, regB, admin)
	}()
	go func() {
		defer wg.Done()
		_, openErr = app.NewBackofficeService(st, clock).OpenSession(context.Background(), regA, app.AperturaCmd{}, cajero)
	}()
	wg.Wait()
	if openErr != nil {
		t.Fatalf("abrir: %v", openErr)
	}
	got := primarias(t, st)
	if len(got) != 1 {
		t.Fatalf("cajas que venden = %v; quiere una", got)
	}
	// Si el cambio ganó, A abrió ya como secundaria; si perdió, se rechazó por el turno abierto.
	if moveErr == nil && got[0] != regB || moveErr != nil && (!errors.Is(moveErr, domain.ErrRegistersMustBeClosed) || got[0] != regA) {
		t.Fatalf("cambio = %v con la venta en %d", moveErr, got[0])
	}
}

// Por el router: solo quien configura cajas, y la caja que vende viaja en el listado.
func TestMoveSalesHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, nil), Settings: app.NewSettingsService(st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
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
	admin, cajero := token("admin_049_http", "admin"), token("cajero_049_http", "cajero")
	regB := otraCaja(t, st, "Barra 049 http")
	path := "/api/v1/cash-registers/" + strconv.FormatInt(regB, 10) + "/sells"
	if w := do(t, r, http.MethodPost, path, cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("cajero pasa la venta = %d; quiere 403", w.Code)
	}
	if w := do(t, r, http.MethodPost, path, admin, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("admin pasa la venta = %d %s", w.Code, w.Body.String())
	}
	if got := primarias(t, st); len(got) != 1 || got[0] != regB {
		t.Fatalf("cajas que venden = %v", got)
	}
}

// Bajo el rol de la app en los tres casos: nadie fuera de la empresa le pasa la venta.
func TestMoveSalesIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "vende-otra")
	regA := principalRegister(t, st)
	regB := otraCaja(t, st, "Barra 049 aislada")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		if err := app.NewBackofficeService(as, nil).MoveSales(ctx, regB, 0); err == nil {
			t.Fatal("se pasó la venta a una caja de otra empresa")
		}
		if got := primarias(t, st); len(got) != 1 || got[0] != regA {
			t.Fatalf("la caja que vende de la dueña cambió: %v", got)
		}
	})
}
