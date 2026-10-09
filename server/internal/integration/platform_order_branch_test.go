//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN PEDIDO DE LA TIENDA DE UNA SUCURSAL NO CAE EN EL TURNO DE OTRA (0076).
//
// La tienda de Uber es de la sucursal NORTE y el único turno abierto es el de la matriz. Si el
// pedido tomara «el» turno de la empresa, entraría al corte de la matriz y el de NORTE quedaría con
// un faltante por un dinero que nunca pasó por su caja.
func TestAPlatformOrderStaysInTheBranchOfItsStore(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	const llave = "llave-de-la-sucursal-norte-ok"
	empresa := makeCompany(t, st, "empresa-dos-sucursales")
	usuario := makeUserIn(t, st, empresa, "cajera-norte", "cajero")
	// La tienda antes que la segunda sucursal: con dos, ya no se asigna sola (EGB01).
	conexion := tiendaConLlave(t, st, empresa, "tienda-norte", llave)
	norte, _ := addBranch(t, st, empresa, "NORTE")
	if _, err := st.Pool.Exec(ctx, `update platform_connections set branch_id = $1 where id = $2`, norte, conexion); err != nil {
		t.Fatalf("mover la tienda a NORTE: %v", err)
	}

	var cajaMatriz int64
	if err := st.Pool.QueryRow(ctx,
		`insert into cash_registers (company_id, branch_id, name, is_primary) values ($1, $2, 'Caja matriz', true) returning id`,
		empresa, headquartersOf(t, st, empresa)).Scan(&cajaMatriz); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into register_sessions (company_id, business_date, opening_cash, opened_by, register_id) values ($1, $2, 0, $3, $4)`,
		empresa, fixedNow, usuario, cajaMatriz); err != nil {
		t.Fatalf("abrir el turno de la matriz: %v", err)
	}

	ctxT, soltar, err := st.AcquireTenant(ctx, empresa)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	svc := app.NewPedidosDePlataformaService(st,
		fixedClients{deciders: map[string]app.DecisorDePedidos{"Uber Eats": &decisorFalso{detalle: []byte(detalleDeUnPedido)}}},
		signingKeyCipher, "sandbox", clock)
	cuerpo := avisoDePedido("evt-norte", "tienda-norte")
	if _, err := svc.RecibirAviso(ctx, "Uber Eats", app.AvisoEntrante{
		Crudo: cuerpo, Firma: domain.FirmarParaPrueba(cuerpo, llave), Ambiente: "sandbox",
	}); err != nil {
		t.Fatalf("recibir: %v", err)
	}
	pend, err := svc.Pendientes(ctxT)
	if err != nil || len(pend) != 1 {
		t.Fatalf("pendientes: %v (%d)", err, len(pend))
	}
	vista, err := svc.Aceptar(ctxT, pend[0].ID, usuario)
	if err != nil {
		t.Fatalf("aceptar: %v", err)
	}

	var sucursal int64
	var sesion *int64
	if err := st.Pool.QueryRow(ctx,
		`select branch_id, register_session_id from orders where id = $1`, vista.ID).Scan(&sucursal, &sesion); err != nil {
		t.Fatal(err)
	}
	if sucursal != norte {
		t.Fatalf("el pedido de la tienda de NORTE quedó en la sucursal %d", sucursal)
	}
	if sesion != nil {
		t.Fatalf("el pedido de NORTE entró al turno %d de la matriz: su corte lo contaría como venta propia", *sesion)
	}
}
