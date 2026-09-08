//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El folio de la plataforma, visto desde el SERVICIO.
//
// Lo que el esquema garantiza —unicidad por empresa y plataforma, checks de forma— ya lo prueba
// `migracion_folio_y_liquidacion_test.go` contra el respaldo real. Aquí se prueba lo que el esquema
// NO puede: que el servicio normalice antes de guardar, y que al chocar diga CUÁL pedido tiene el
// folio en vez de devolver un conflicto genérico.

// pedidoDePlataformaConFolio arma la venta mínima de plataforma que el servicio acepta.
func pedidoDePlataformaConFolio(cajero, prod int64, plataforma int16, folio string) app.CreateOrderCmd {
	cmd := app.CreateOrderCmd{
		ClientUUID:         uuid.New(),
		ServiceType:        "domicilio",
		DeliveryPlatformID: &plataforma,
		OpenedBy:           cajero,
		Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	}
	if folio != "" {
		cmd.PlatformOrderRef = &folio
	}
	return cmd
}

func TestElFolioSeGuardaTalCualConLosExtremosRecortados(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_folio", "cajero")
	prod := makeProduct(t, st, "Crepa", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)

	// Con espacios de los dos lados, como queda al pegarlo del reporte de pago. Las mayúsculas y
	// los guiones se conservan: cualquier otra transformación destruye lo único que sirve para
	// encontrar el pedido en el documento.
	const folio = "4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44"
	ord, err := svc.Create(ctx, pedidoDePlataformaConFolio(cajero, prod, uber, "  "+folio+"  "))
	if err != nil {
		t.Fatalf("Create con folio: %v", err)
	}
	if ord.PlatformOrderRef == nil {
		t.Fatal("el pedido salió sin folio: el dato irrecuperable de esta feature no se guardó")
	}
	if *ord.PlatformOrderRef != folio {
		t.Fatalf("el folio quedó como %q y debía quedar %q: la única normalización es recortar los extremos",
			*ord.PlatformOrderRef, folio)
	}
}

func TestUnFolioRepetidoDiceCualPedidoLoTiene(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dup", "cajero")
	prod := makeProduct(t, st, "Waffle", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)

	const folio = "DIDI-9988776655"
	primero, err := svc.Create(ctx, pedidoDePlataformaConFolio(cajero, prod, uber, folio))
	if err != nil {
		t.Fatalf("el primer pedido: %v", err)
	}

	_, err = svc.Create(ctx, pedidoDePlataformaConFolio(cajero, prod, uber, folio))
	if err == nil {
		t.Fatal("el segundo pedido tomó el mismo folio: la conciliación deja de poder decir cuál " +
			"pedido formó el depósito")
	}
	if !errors.Is(err, domain.ErrPlatformRefTaken) {
		t.Fatalf("se rechazó, pero no como ErrPlatformRefTaken (%v): la pantalla no puede llevar el "+
			"foco al campo del folio si el error no es distinguible", err)
	}
	// El mensaje NOMBRA al dueño. Un aviso genérico manda al operador a buscar a ciegas entre las
	// ventas del día, con el repartidor esperando enfrente.
	msg := err.Error()
	if !strings.Contains(msg, primero.FolioName) && !strings.Contains(msg, "#") {
		t.Fatalf("el mensaje no dice cuál pedido ya tiene el folio: %q", msg)
	}
}

func TestElMismoFolioEnDosPlataformasEsUnaCapturaLegitima(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dos_plat", "cajero")
	prod := makeProduct(t, st, "Malteada", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	rappi := platformID(t, st, defaultCompanyID, "Rappi")
	abrirCajaPrincipal(t, st, cajero)

	const folio = "1234567890"
	if _, err := svc.Create(ctx, pedidoDePlataformaConFolio(cajero, prod, uber, folio)); err != nil {
		t.Fatalf("el pedido de Uber: %v", err)
	}
	// Rechazar esto tiraría una captura legítima, y eso es lo caro de quitar después: el mismo
	// número puede existir en dos plataformas distintas.
	if _, err := svc.Create(ctx, pedidoDePlataformaConFolio(cajero, prod, rappi, folio)); err != nil {
		t.Fatalf("el mismo número en OTRA plataforma se rechazó y es legítimo: %v", err)
	}
}

func TestElServicioRechazaLoQueNoEsUnFolio(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_malo", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("100"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, cajero)

	casos := []struct{ nombre, folio, queRompe string }{
		{"cadena vacía", " ", `"" no es "sin folio": la ausencia se representa como ausencia`},
		{"solo espacios", "     ", "es la cadena vacía por otro camino"},
		{"más largo que el tope", strings.Repeat("x", domain.MaxPlatformRefLen+1),
			"sin cota, un pegado accidental de media pantalla entra a la columna"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cmd := pedidoDePlataformaConFolio(cajero, prod, uber, "x")
			cmd.PlatformOrderRef = &c.folio
			_, err := svc.Create(ctx, cmd)
			if err == nil {
				t.Fatalf("se aceptó: %s", c.queRompe)
			}
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("se rechazó pero no como ErrValidation (sería un 500 en vez de un 400): %v", err)
			}
		})
	}
}

func TestUnPedidoDeMostradorNoAceptaFolioDePlataforma(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_mostrador", "cajero")
	prod := makeProduct(t, st, "Pan", decimal.RequireFromString("50"), false)
	abrirCajaPrincipal(t, st, cajero)

	folio := "UBER-123"
	_, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:       uuid.New(),
		ServiceType:      "mostrador",
		OpenedBy:         cajero,
		PlatformOrderRef: &folio,
		Lines:            []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err == nil {
		t.Fatal("un pedido de mostrador aceptó folio de plataforma: la pantalla no lo ofrece, pero " +
			"un cliente de API sí puede mandarlo y el servidor es la única barrera")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("se rechazó pero no como ErrValidation: %v", err)
	}
}

// Un pedido CANCELADO conserva su folio. La plataforma también lo canceló y su documento de pago lo
// trae, así que borrarlo destruye justo el rastro que sirve para explicar la cancelación.
func TestUnPedidoCanceladoConservaSuFolio(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	admin := makeUser(t, st, "admin_cancela", "admin")
	prod := makeProduct(t, st, "Jugo", decimal.RequireFromString("60"), false)
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	abrirCajaPrincipal(t, st, admin)

	const folio = "UBER-CANCELADO-1"
	ord, err := svc.Create(ctx, pedidoDePlataformaConFolio(admin, prod, uber, folio))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Cancel(ctx, ord.ID, admin, "el cliente ya no lo quiso"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	var guardado *string
	if err := st.Pool.QueryRow(ctx,
		`select platform_order_ref from orders where id = $1`, ord.ID).Scan(&guardado); err != nil {
		t.Fatalf("releer el pedido: %v", err)
	}
	if guardado == nil || *guardado != folio {
		t.Fatalf("el folio se perdió al cancelar (quedó %v): el documento de pago de la plataforma "+
			"trae ese pedido cancelado, y sin folio no hay cómo explicarlo", guardado)
	}
}
