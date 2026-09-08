//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La US-3: registrar lo que la plataforma se quedó.

// liquidacionReal son los números MEDIDOS del 2x1 de Uber Eats de septiembre (docs/plataformas-digitales.md
// §4-bis). No se re-derivan: se citan.
func liquidacionReal() domain.Settlement {
	pct := decimal.RequireFromString("30.00")
	return domain.Settlement{
		ReportedGross:    decimal.RequireFromString("220.00"),
		CommissionAmount: decimal.RequireFromString("33.00"),
		CommissionPct:    &pct,
		DiscountTotal:    decimal.RequireFromString("110.00"),
		DiscountPlatform: decimal.Zero,
		Withholdings:     decimal.RequireFromString("9.96"),
		NetAmount:        decimal.RequireFromString("51.77"),
		PayoutReference:  "PAY-2026-09-12-0043",
		DocumentRef:      "uber-payments-2026-09-08.csv",
	}
}

func pedidoDePlataformaCobrado(t *testing.T, ctx context.Context, st *store.Store, sufijo string) (int64, int64) {
	t.Helper()
	admin := makeUser(t, st, "admin_liq_"+sufijo, "admin")
	abrirCajaPrincipal(t, st, admin)
	return otroPedidoDePlataforma(t, ctx, st, admin, sufijo), admin
}

// otroPedidoDePlataforma NO abre la caja: solo hay una caja principal y abrirla dos veces choca con
// `one_open_session_per_register`. Un test que necesita dos pedidos usa esta.
func otroPedidoDePlataforma(t *testing.T, ctx context.Context, st *store.Store, admin int64, sufijo string) int64 {
	t.Helper()
	prod := makeProduct(t, st, "Soda liq "+sufijo, decimal.RequireFromString("110"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	ord, err := app.NewOrdersService(st, clock).Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &uber, OpenedBy: admin,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("2")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido: %v", err)
	}
	return ord.ID
}

func TestUnDocumentoCorregidoReemplazaLaLiquidacionYNoLaDuplica(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "dup")

	if _, err := svc.Upsert(ctx, pedido, liquidacionReal(), admin); err != nil {
		t.Fatalf("primera captura: %v", err)
	}
	corregida := liquidacionReal()
	corregida.CommissionAmount = decimal.RequireFromString("35.00")
	corregida.NetAmount = decimal.RequireFromString("49.77")
	v, err := svc.Upsert(ctx, pedido, corregida, admin)
	if err != nil {
		t.Fatalf("recaptura desde el documento corregido: %v", err)
	}
	if !v.CommissionAmount.Equal(decimal.RequireFromString("35.00")) {
		t.Fatalf("la recaptura dejó la comisión en %s: el documento corregido no ganó", v.CommissionAmount)
	}
	var n int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_settlements where order_id = $1`, pedido).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Fatalf("el pedido quedó con %d liquidaciones: la comisión se contaría dos veces en el "+
			"resumen del periodo", n)
	}
}

// "Todavía no llega el documento" y "el documento dice cero" NO son lo mismo. Es la mitad de la
// feature: sin la distinción, un periodo sin capturar se lee como un periodo sin comisiones.
func TestSinLiquidacionNoEsLoMismoQueUnaLiquidacionEnCeros(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "ceros")

	if _, err := svc.Get(ctx, pedido); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("un pedido sin liquidación devolvió %v y debía ser NO ENCONTRADO: devolver ceros "+
			"afirmaría que la plataforma no cobró nada", err)
	}

	ceros := domain.Settlement{}
	if _, err := svc.Upsert(ctx, pedido, ceros, admin); err != nil {
		t.Fatalf("una liquidación en ceros es un hecho legítimo y se rechazó: %v", err)
	}
	v, err := svc.Get(ctx, pedido)
	if err != nil {
		t.Fatalf("Get tras capturar ceros: %v", err)
	}
	if !v.CommissionAmount.IsZero() || !v.NetAmount.IsZero() {
		t.Fatalf("la liquidación en ceros quedó con %s de comisión y %s de neto", v.CommissionAmount, v.NetAmount)
	}
}

func TestElNetoNegativoSeAceptaPorqueEsLoQueDeVerdadPaso(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "neg")

	// Promoción que financió el restaurante por completo: el pedido le costó dinero al negocio.
	negativa := liquidacionReal()
	negativa.NetAmount = decimal.RequireFromString("-31.20")
	v, err := svc.Upsert(ctx, pedido, negativa, admin)
	if err != nil {
		t.Fatalf("el neto negativo se rechazó, y es lo que de verdad pasa con una promoción que "+
			"financió el restaurante: %v", err)
	}
	if !v.NetAmount.Equal(decimal.RequireFromString("-31.20")) {
		t.Fatalf("el neto quedó en %s", v.NetAmount)
	}
}

func TestLaLiquidacionRechazaLoQueUnDocumentoNoPuedeDecir(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "malos")

	casos := map[string]func(*domain.Settlement){
		"comisión negativa":                  func(s *domain.Settlement) { s.CommissionAmount = decimal.RequireFromString("-1") },
		"tasa fuera de rango":                func(s *domain.Settlement) { p := decimal.RequireFromString("180"); s.CommissionPct = &p },
		"descuento de plataforma > el total": func(s *domain.Settlement) { s.DiscountPlatform = decimal.RequireFromString("500") },
		"importe por encima del tope":        func(s *domain.Settlement) { s.ReportedGross = domain.MaxMoney.Add(decimal.NewFromInt(1)) },
		"exponente absurdo":                  func(s *domain.Settlement) { s.NetAmount = decimal.RequireFromString("1e100000000") },
	}
	for nombre, toca := range casos {
		t.Run(nombre, func(t *testing.T) {
			in := liquidacionReal()
			toca(&in)
			_, err := svc.Upsert(ctx, pedido, in, admin)
			if err == nil {
				t.Fatalf("se aceptó %q", nombre)
			}
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("%q se rechazó pero no como entrada inválida (sería un 500 en vez de un "+
					"400, y el spec pide lo contrario): %v", nombre, err)
			}
		})
	}
}

// FR-016: NINGUNA cifra de la liquidación entra a un total de venta, a un corte ni a un arqueo.
//
// Es el modo de falla más caro de la feature: la comisión es dinero que el negocio vendió y no
// recibió, pero restarla de una venta reescribiría lo que el POS cobró.
func TestRegistrarUnaLiquidacionNoMueveNingunaVenta(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	sales := app.NewSalesService(st, clock)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "fr016")

	f := filtroBase(fixedNow)
	antes, err := sales.Summary(ctx, f)
	if err != nil {
		t.Fatalf("resumen antes: %v", err)
	}
	fotoCaja := func() string {
		var v string
		if err := st.Pool.QueryRow(ctx, `
			select coalesce(sum(o.total),0)::text || '|' ||
			       coalesce((select sum(amount) from order_payments),0)::text || '|' ||
			       coalesce((select sum(declared) from register_session_totals),0)::text
			from orders o`).Scan(&v); err != nil {
			t.Fatalf("fotografiar la caja: %v", err)
		}
		return v
	}
	cajaAntes := fotoCaja()

	if _, err := svc.Upsert(ctx, pedido, liquidacionReal(), admin); err != nil {
		t.Fatalf("registrar la liquidación: %v", err)
	}

	despues, err := sales.Summary(ctx, f)
	if err != nil {
		t.Fatalf("resumen después: %v", err)
	}
	if !antes.Total.Equal(despues.Total) {
		t.Fatalf("el total de ventas pasó de %s a %s al registrar una liquidación: la comisión se "+
			"restó de una venta, y eso reescribe lo que el POS cobró", antes.Total, despues.Total)
	}
	if antes.Count != despues.Count {
		t.Fatalf("el conteo de ventas pasó de %d a %d", antes.Count, despues.Count)
	}
	if cajaAntes != fotoCaja() {
		t.Fatalf("el dinero de la caja se movió al registrar una liquidación: era %s y quedó en %s",
			cajaAntes, fotoCaja())
	}
}

func TestElResumenDePlataformasDelPeriodo(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewSettlementsService(st)
	pedido, admin := pedidoDePlataformaCobrado(t, ctx, st, "resumen")
	// Un segundo pedido de plataforma SIN liquidar: es lo que hace que los conjuntos difieran, que
	// es todo el punto de las tres cifras.
	otroPedidoDePlataforma(t, ctx, st, admin, "resumen2")

	if _, err := svc.Upsert(ctx, pedido, liquidacionReal(), admin); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	r, err := svc.Summary(ctx, filtroBase(fixedNow))
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if r.Vendido.Orders != 2 {
		t.Fatalf("lo vendido cubre %d pedidos de plataforma y debían ser 2", r.Vendido.Orders)
	}
	if r.SeQuedoLaPlataforma.Orders != 1 || r.LlegoAlBanco.Orders != 1 {
		t.Fatalf("las cifras del documento cubren %d pedidos y debía ser 1: solo uno está capturado",
			r.SeQuedoLaPlataforma.Orders)
	}
	if r.SinLiquidar.Orders != 1 {
		t.Fatalf("falta 1 por liquidar y el resumen dice %d", r.SinLiquidar.Orders)
	}
	// Comisión + retenciones del único documento capturado.
	if !r.SeQuedoLaPlataforma.Amount.Equal(decimal.RequireFromString("42.96")) {
		t.Fatalf("se quedó %s y debía ser 42.96 (33.00 de comisión + 9.96 de retenciones)",
			r.SeQuedoLaPlataforma.Amount)
	}
	if !r.LlegoAlBanco.Amount.Equal(decimal.RequireFromString("51.77")) {
		t.Fatalf("llegó al banco %s y el documento dice 51.77", r.LlegoAlBanco.Amount)
	}
}

// El cajero NO captura liquidaciones: es dinero que no pasó por la caja.
func TestLaLiquidacionExigeRolDeAdministracion(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Orders: app.NewOrdersService(st, clock),
		Settlements: app.NewSettlementsService(st), Sales: app.NewSalesService(st, clock),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	pedido, adminDelPedido := pedidoDePlataformaCobrado(t, ctx, st, "rol")
	cuerpo, _ := json.Marshal(map[string]string{
		"reportedGross": "220", "commissionAmount": "33", "netAmount": "51.77",
		"discountTotal": "0", "discountPlatform": "0", "withholdings": "0",
	})
	ruta := fmt.Sprintf("/api/v1/orders/%d/settlement", pedido)

	tok := func(username, role string) string {
		id := makeUser(t, st, username, role)
		s, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		return s
	}

	if got := do(t, r, http.MethodPut, ruta, tok("cajero_liq_rol", "cajero"), cuerpo, "application/json").Code; got != http.StatusForbidden {
		t.Fatalf("el cajero pudo capturar una liquidación (status %d): es dinero que no pasó por la caja", got)
	}
	if got := do(t, r, http.MethodGet, ruta, tok("cajero_liq_get", "cajero"), nil, "").Code; got != http.StatusForbidden {
		t.Fatalf("el cajero pudo LEER una liquidación (status %d)", got)
	}
	if got := do(t, r, http.MethodPut, ruta, tok("gerente_liq", "gerente"), cuerpo, "application/json").Code; got != http.StatusOK {
		t.Fatalf("el gerente no pudo capturar la liquidación (status %d)", got)
	}
	// Y sin liquidación, el GET es 404 y no ceros.
	otro := otroPedidoDePlataforma(t, ctx, st, adminDelPedido, "rol404")
	rutaOtro := fmt.Sprintf("/api/v1/orders/%d/settlement", otro)
	if got := do(t, r, http.MethodGet, rutaOtro, tok("admin_liq_404", "admin"), nil, "").Code; got != http.StatusNotFound {
		t.Fatalf("un pedido sin liquidación respondió %d y debía ser 404: devolver ceros afirmaría "+
			"que la plataforma no cobró nada", got)
	}
}
