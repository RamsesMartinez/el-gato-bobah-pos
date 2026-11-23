//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Pedidos entrantes (spec 093): rechazar, la cancelación de la plataforma, el plazo y las carreras
// entre tabletas. Lo que decide si la alerta continua sirve es que «pendiente» signifique lo mismo
// en el servidor y en la plataforma: un pedido muerto que sigue pendiente es una tableta que suena
// para siempre.

// pendienteDirecto inserta un pedido de plataforma pendiente sin pasar por el aviso: las carreras
// necesitan muchos pedidos distintos, y el aviso siempre trae el mismo folio.
func pendienteDirecto(t *testing.T, st *store.Store, empresa, conexion int64, folio string, plazo time.Time) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into platform_incoming_orders (connection_id, external_order_id, display_id, placed_at,
		       decide_before, total, service_type, customer_name, company_id)
		values ($1, $2, $2, $3, $4, 120, 'domicilio', 'Ana', $5) returning id`,
		conexion, folio, plazo.Add(-time.Minute), plazo, empresa).Scan(&id); err != nil {
		t.Fatalf("sembrar el pendiente %s: %v", folio, err)
	}
	return id
}

type escenario struct {
	st       *store.Store
	empresa  int64
	usuario  int64
	conexion int64
	decisor  *decisorFalso
	svc      *app.PedidosDePlataformaService
}

func nuevoEscenario(t *testing.T, nombre string, ahora time.Time) escenario {
	t.Helper()
	st := newTestStore(t)
	empresa := makeCompany(t, st, "entrantes-"+nombre)
	usuario := makeUserIn(t, st, empresa, "cajera-"+nombre, "cajero")
	conexion := tiendaConLlave(t, st, empresa, "tienda-"+nombre, "llave-"+nombre+"-suficientemente-larga")
	dec := &decisorFalso{detalle: []byte(detalleDeUnPedido)}
	svc := app.NewPedidosDePlataformaService(st,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": dec}}, signingKeyCipher,
		"sandbox", func() time.Time { return ahora })
	return escenario{st, empresa, usuario, conexion, dec, svc}
}

func (e escenario) tenant(t *testing.T) context.Context {
	t.Helper()
	ctx, soltar, err := e.st.AcquireTenant(context.Background(), e.empresa)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(soltar)
	return ctx
}

func estadoDe(t *testing.T, st *store.Store, id int64) (estado string, motivo *string, quien *int64) {
	t.Helper()
	if err := st.Pool.QueryRow(context.Background(),
		`select state::text, deny_reason, decided_by from platform_incoming_orders where id = $1`, id).
		Scan(&estado, &motivo, &quien); err != nil {
		t.Fatal(err)
	}
	return
}

func TestRechazarConMotivoDejaElRastroYApagaElPendiente(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "rechaza", fixedNow)
	id := pendienteDirecto(t, e.st, e.empresa, e.conexion, "R-1", fixedNow.Add(10*time.Minute))
	ctx := e.tenant(t)

	if err := e.svc.Rechazar(ctx, id, e.usuario, "out_of_stock"); err != nil {
		t.Fatalf("rechazar: %v", err)
	}
	estado, motivo, quien := estadoDe(t, e.st, id)
	if estado != "rechazado" || motivo == nil || *motivo != "out_of_stock" || quien == nil || *quien != e.usuario {
		t.Fatalf("quedó %q, motivo %v, por %v", estado, motivo, quien)
	}
	if p, _ := e.svc.Pendientes(ctx); len(p) != 0 {
		t.Fatalf("sigue en la lista tras rechazarlo: %d", len(p))
	}
	// Rechazar lo ya rechazado, o aceptarlo, se dice: otra tableta ganó.
	if err := e.svc.Rechazar(ctx, id, e.usuario, "other"); !errors.Is(err, domain.ErrPedidoYaDecidido) {
		t.Fatalf("rechazar dos veces dio %v", err)
	}
	if _, err := e.svc.Aceptar(ctx, id, e.usuario); !errors.Is(err, domain.ErrPedidoYaDecidido) {
		t.Fatalf("aceptar un rechazado dio %v", err)
	}
}

func TestUnRechazoQueLaPlataformaNoConfirmaSigueSonando(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "niega", fixedNow)
	e.decisor.niega = errors.New("la plataforma no respondió")
	id := pendienteDirecto(t, e.st, e.empresa, e.conexion, "N-1", fixedNow.Add(10*time.Minute))
	ctx := e.tenant(t)

	if err := e.svc.Rechazar(ctx, id, e.usuario, "too_busy"); err == nil {
		t.Fatal("el rechazo se dio por bueno sin que la plataforma lo confirmara")
	}
	if estado, _, _ := estadoDe(t, e.st, id); estado != "pendiente" {
		t.Fatalf("quedó %q: la plataforma sigue esperando y aquí ya no suena", estado)
	}
	if p, _ := e.svc.Pendientes(ctx); len(p) != 1 {
		t.Fatalf("pendientes = %d, debía seguir 1", len(p))
	}
}

func TestUnMotivoFueraDeLaListaNoLlegaALaPlataforma(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "motivo", fixedNow)
	id := pendienteDirecto(t, e.st, e.empresa, e.conexion, "M-1", fixedNow.Add(10*time.Minute))
	if err := e.svc.Rechazar(e.tenant(t), id, e.usuario, "STORE_CLOSED"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("un motivo inventado dio %v", err)
	}
	if n := e.decisor.decisiones.Load(); n != 0 {
		t.Fatalf("la plataforma recibió %d decisiones con un motivo inválido", n)
	}
}

// UN PEDIDO VENCIDO YA NO SE DECIDE NI SE LISTA. La plataforma lo cancela sola al vencer; si su aviso
// no llega, sin esto la tableta sonaría para siempre por un pedido que ya no existe.
func TestUnPedidoVencidoNiSuenaNiSeDecide(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "vencido", fixedNow)
	vencido := pendienteDirecto(t, e.st, e.empresa, e.conexion, "V-1", fixedNow.Add(-time.Second))
	vigente := pendienteDirecto(t, e.st, e.empresa, e.conexion, "V-2", fixedNow.Add(5*time.Minute))
	ctx := e.tenant(t)

	p, err := e.svc.Pendientes(ctx)
	if err != nil || len(p) != 1 || p[0].ID != vigente {
		t.Fatalf("pendientes = %+v (%v): el vencido no debía listarse", p, err)
	}
	if _, err := e.svc.Aceptar(ctx, vencido, e.usuario); !errors.Is(err, domain.ErrPedidoYaDecidido) {
		t.Fatalf("aceptar un vencido dio %v", err)
	}
	if err := e.svc.Rechazar(ctx, vencido, e.usuario, "closed"); !errors.Is(err, domain.ErrPedidoYaDecidido) {
		t.Fatalf("rechazar un vencido dio %v", err)
	}
	if n := e.decisor.decisiones.Load(); n != 0 {
		t.Fatalf("un pedido vencido llegó %d veces a la plataforma", n)
	}
}

func avisoDeCancelacion(eventID, tienda, folio string) []byte {
	cuerpo, _ := json.Marshal(map[string]any{
		"event_type": "orders.cancel",
		"event_id":   eventID,
		"meta":       map[string]any{"user_id": tienda, "resource_id": folio},
	})
	return cuerpo
}

func TestLaCancelacionDeLaPlataformaApagaElPendiente(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "cancela", fixedNow)
	const llave = "llave-cancela-suficientemente-larga"
	pend := pendienteDirecto(t, e.st, e.empresa, e.conexion, "C-1", fixedNow.Add(5*time.Minute))
	acep := pendienteDirecto(t, e.st, e.empresa, e.conexion, "C-2", fixedNow.Add(5*time.Minute))
	ctx := e.tenant(t)
	if _, err := e.svc.Aceptar(ctx, acep, e.usuario); err != nil {
		t.Fatal(err)
	}
	recibir := func(evento, folio string) app.AvisoRecibido {
		t.Helper()
		cuerpo := avisoDeCancelacion(evento, "tienda-cancela", folio)
		r, err := e.svc.RecibirAviso(context.Background(), "Uber Eats", app.AvisoEntrante{
			Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
		})
		if err != nil {
			t.Fatalf("la cancelación de %s respondió error (la plataforma la reintentaría): %v", folio, err)
		}
		return r
	}

	if r := recibir("evt-c1", "C-1"); !r.Cambio || r.Empresa != e.empresa {
		t.Fatalf("la cancelación no avisó a las tabletas: %+v", r)
	}
	if estado, _, _ := estadoDe(t, e.st, pend); estado != "cancelado" {
		t.Fatalf("el pendiente quedó %q tras cancelarlo la plataforma", estado)
	}
	// LLEGA TARDE: la tableta ya lo había aceptado. No se pisa, y no es un error.
	if r := recibir("evt-c2", "C-2"); r.Cambio {
		t.Fatal("una cancelación tardía de un aceptado se contó como cambio")
	}
	if estado, _, _ := estadoDe(t, e.st, acep); estado != "aceptado" {
		t.Fatalf("una cancelación tardía pisó un aceptado: quedó %q", estado)
	}
}

// DOS TABLETAS ACEPTAN A LA VEZ, VEINTE VECES (SC-004): cada pedido entra una sola vez a cocina y al
// corte, y la que pierde se entera.
func TestDosTabletasAceptandoALaVezEntranUnaSolaVez(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "carrera", fixedNow)
	for i := range 20 {
		folio := fmt.Sprintf("D-%02d", i)
		id := pendienteDirecto(t, e.st, e.empresa, e.conexion, folio, fixedNow.Add(5*time.Minute))
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for k := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, soltar, err := e.st.AcquireTenant(context.Background(), e.empresa)
				if err != nil {
					errs[k] = err
					return
				}
				defer soltar()
				_, errs[k] = e.svc.Aceptar(ctx, id, e.usuario)
			}()
		}
		wg.Wait()
		ganaron := 0
		for _, err := range errs {
			switch {
			case err == nil:
				ganaron++
			case !errors.Is(err, domain.ErrPedidoYaDecidido):
				t.Fatalf("%s: la tableta que perdió recibió %v, no «ya lo atendió otra»", folio, err)
			}
		}
		var pedidos, pagos int
		if err := e.st.Pool.QueryRow(context.Background(), `
			select count(*), coalesce(sum((select count(*) from order_payments p where p.order_id = o.id)), 0)
			  from orders o where o.platform_order_ref = $1`, folio).Scan(&pedidos, &pagos); err != nil {
			t.Fatal(err)
		}
		if ganaron != 1 || pedidos != 1 || pagos != 1 {
			t.Fatalf("%s: ganaron %d, pedidos %d, cobros %d", folio, ganaron, pedidos, pagos)
		}
	}
}

func TestAceptarMientrasOtraRechazaSoloGanaUna(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "cruce", fixedNow)
	for i := range 10 {
		folio := fmt.Sprintf("X-%02d", i)
		id := pendienteDirecto(t, e.st, e.empresa, e.conexion, folio, fixedNow.Add(5*time.Minute))
		var wg sync.WaitGroup
		var errAcepta, errRechaza error
		wg.Add(2)
		go func() {
			defer wg.Done()
			ctx, soltar, _ := e.st.AcquireTenant(context.Background(), e.empresa)
			defer soltar()
			_, errAcepta = e.svc.Aceptar(ctx, id, e.usuario)
		}()
		go func() {
			defer wg.Done()
			ctx, soltar, _ := e.st.AcquireTenant(context.Background(), e.empresa)
			defer soltar()
			errRechaza = e.svc.Rechazar(ctx, id, e.usuario, "too_busy")
		}()
		wg.Wait()
		if (errAcepta == nil) == (errRechaza == nil) {
			t.Fatalf("%s: aceptar %v, rechazar %v; debía ganar exactamente una", folio, errAcepta, errRechaza)
		}
		estado, _, _ := estadoDe(t, e.st, id)
		var pedidos int
		_ = e.st.Pool.QueryRow(context.Background(), `select count(*) from orders where platform_order_ref = $1`, folio).Scan(&pedidos)
		if errAcepta == nil && (estado != "aceptado" || pedidos != 1) || errRechaza == nil && (estado != "rechazado" || pedidos != 0) {
			t.Fatalf("%s: quedó %q con %d pedidos (aceptar %v, rechazar %v)", folio, estado, pedidos, errAcepta, errRechaza)
		}
	}
}

// AISLAMIENTO EN LOS TRES CASOS: ni otra empresa, ni una conexión reciclada, ni una sin empresa ven,
// aceptan o rechazan los pedidos entrantes de la dueña.
func TestLosPedidosEntrantesNoCruzanEmpresas(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "aislado", fixedNow)
	id := pendienteDirecto(t, e.st, e.empresa, e.conexion, "A-1", fixedNow.Add(5*time.Minute))
	otra := makeCompany(t, e.st, "entrantes-ajena")
	otroUsuario := makeUserIn(t, e.st, otra, "cajera-ajena", "cajero")

	inTheThreeCases(t, e.empresa, otra, func(t *testing.T, st *store.Store, ctx context.Context) {
		plataforma := app.NewPedidosDePlataformaService(st,
			fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": e.decisor}}, signingKeyCipher,
			"sandbox", clock)
		svc := app.NewIncomingOrdersService(plataforma)
		if p, err := svc.Pending(ctx); err == nil && len(p) != 0 {
			t.Fatalf("vio %d pedidos de la otra empresa", len(p))
		}
		if _, err := svc.Accept(ctx, domain.IncomingSourcePlatform, id, otroUsuario); err == nil {
			t.Fatal("aceptó un pedido de la otra empresa")
		}
		if err := svc.Reject(ctx, domain.IncomingSourcePlatform, id, otroUsuario, "closed"); err == nil {
			t.Fatal("rechazó un pedido de la otra empresa")
		}
	})
	if estado, _, _ := estadoDe(t, e.st, id); estado != "pendiente" {
		t.Fatalf("el pedido de la dueña quedó %q", estado)
	}
}

// LA FORMA COMÚN: el servicio de pedidos entrantes entrega lo de la plataforma con su origen, sus
// renglones como arreglo y ordenado por urgencia.
func TestElServicioComunEntregaLaFormaComun(t *testing.T) {
	t.Parallel()
	e := nuevoEscenario(t, "comun", fixedNow)
	tarde := pendienteDirecto(t, e.st, e.empresa, e.conexion, "F-1", fixedNow.Add(9*time.Minute))
	pronto := pendienteDirecto(t, e.st, e.empresa, e.conexion, "F-2", fixedNow.Add(2*time.Minute))
	svc := app.NewIncomingOrdersService(e.svc)
	ctx := e.tenant(t)
	p, err := svc.Pending(ctx)
	if err != nil || len(p) != 2 {
		t.Fatalf("pendientes %d: %v", len(p), err)
	}
	if p[0].ID != pronto || p[1].ID != tarde {
		t.Fatalf("orden %d, %d: el que vence antes va primero", p[0].ID, p[1].ID)
	}
	if p[0].Source != domain.IncomingSourcePlatform || p[0].SourceName != "Uber Eats" || p[0].Lines == nil {
		t.Fatalf("forma común incompleta: %+v", p[0])
	}
	if _, err := svc.Accept(ctx, domain.IncomingSourcePlatform, pronto, e.usuario); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reject(ctx, domain.IncomingSourcePlatform, tarde, e.usuario, "nope"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("motivo inválido dio %v", err)
	}
}
