//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// POR ROL, NUNCA POR PERSONA (US2), probado contra la base de verdad.
//
// Es la única decisión de esta feature que no se puede corregir después: lo que se escriba con la
// identidad de alguien, ya se escribió. Por eso estos tests van antes que el mapa.

// Tras ingerir eventos, NADA en la base apunta a quien los hizo.
func TestElUsoNoGuardaAQuienLoUso(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)

	// Dos usuarios del mismo rol: así el corte por rol SÍ se conserva y el test mira lo que
	// importa —que no esté la persona— y no el efecto de la supresión.
	uno := makeUser(t, st, "cajero_uso_uno", "cajero")
	makeUser(t, st, "cajero_uso_dos", "cajero")

	if _, err := svc.Registrar(ctx, domain.RoleCajero, []domain.EventoDeUso{
		{Pantalla: "pos"},
		{Pantalla: "pos", Accion: "cobrar"},
	}); err != nil {
		t.Fatalf("registrar uso: %v", err)
	}

	// `Registrar` recibe un ROL, no un usuario: la identidad no puede llegar ni por error, porque
	// no hay parámetro por donde. Lo que sí se comprueba aquí es la otra mitad, la que sí podría
	// romperse en runtime: que `detail` —la puerta de las coordenadas del futuro— quede VACÍA.
	//
	// Que ninguna de las dos tablas tenga columna de persona lo prueba
	// TestElEventoDeUsoNoPuedeGuardarAQuienLoHizo, contra el esquema.
	_ = uno
	var conDetalle int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from usage_events where detail is not null`).Scan(&conDetalle); err != nil {
		t.Fatalf("revisar detail: %v", err)
	}
	if conDetalle != 0 {
		t.Fatalf("%d eventos traen `detail`: esa columna existe para las coordenadas del futuro, y llena desde el cuerpo es el hueco por donde entra lo que FR-003 prohíbe", conDetalle)
	}

	var eventos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_events`).Scan(&eventos); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	if eventos != 2 {
		t.Fatalf("se guardaron %d eventos de 2: el test de arriba pasaría en verde con la tabla vacía", eventos)
	}
}

// EL CASO QUE DE VERDAD IMPORTA: un rol con una sola persona no se puede cortar (FR-009).
//
// Decir «el gerente hizo estas 40 acciones» en una empresa con un gerente es decir su nombre. La
// supresión ocurre al ESCRIBIR: lo que no se escribió no se puede consultar, ni con acceso a la
// base, ni dentro de un año cuando ya nadie recuerde esta regla.
func TestElRolDeUnaSolaPersonaSeGuardaSinCorte(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)

	// Un solo mesero en la empresa.
	makeUser(t, st, "mesero_solito", "mesero")
	if _, err := svc.Registrar(ctx, domain.RoleMesero, []domain.EventoDeUso{{Pantalla: "pos"}}); err != nil {
		t.Fatalf("registrar uso del mesero: %v", err)
	}

	var conRol int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from usage_daily where role is not null`).Scan(&conRol); err != nil {
		t.Fatalf("leer el agregado: %v", err)
	}
	if conRol != 0 {
		t.Fatal("el rol quedó escrito con una sola persona en él: el corte identifica por eliminación, y eso es decir su nombre")
	}

	// Y con dos, el corte sí se conserva: si no, la feature no mide nada por rol nunca.
	makeUser(t, st, "gerente_uso_uno", "gerente")
	makeUser(t, st, "gerente_uso_dos", "gerente")
	if _, err := svc.Registrar(ctx, domain.RoleGerente, []domain.EventoDeUso{{Pantalla: "caja"}}); err != nil {
		t.Fatalf("registrar uso del gerente: %v", err)
	}
	var conRolGerente int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from usage_daily where role = 'gerente'`).Scan(&conRolGerente); err != nil {
		t.Fatalf("leer el agregado del gerente: %v", err)
	}
	if conRolGerente != 1 {
		t.Fatalf("con dos gerentes el corte debe conservarse, y hay %d filas con rol", conRolGerente)
	}
}

// El lote pre-agregado NO pierde eventos: el grano fino guarda uno por toque.
//
// Si el pre-agregado se colara al grano fino, el día que se midan coordenadas habría un punto por
// combinación en vez de uno por dedo, y la puerta de FR-013 quedaría cerrada sin que nadie lo note.
func TestElGranoFinoGuardaUnoPorEvento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_grano_uno", "cajero")
	makeUser(t, st, "cajero_grano_dos", "cajero")

	lote := []domain.EventoDeUso{{Pantalla: "pos"}, {Pantalla: "pos"}, {Pantalla: "pos"}}
	if _, err := svc.Registrar(ctx, domain.RoleCajero, lote); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	var eventos, hits int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_events`).Scan(&eventos); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `select coalesce(sum(hits),0) from usage_daily`).Scan(&hits); err != nil {
		t.Fatalf("sumar el agregado: %v", err)
	}
	if eventos != 3 {
		t.Fatalf("el grano fino guardó %d filas de 3 toques: el pre-agregado se coló donde no debe", eventos)
	}
	if hits != 3 {
		t.Fatalf("el agregado suma %d de 3", hits)
	}
}

// Lo que no está en la lista blanca no llega a la base, y el servicio dice cuántos descartó.
func TestLoDesconocidoSeDescartaYSeCuenta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_desconocido_uno", "cajero")
	makeUser(t, st, "cajero_desconocido_dos", "cajero")

	descartados, err := svc.Registrar(ctx, domain.RoleCajero, []domain.EventoDeUso{
		{Pantalla: "pos"},
		{Pantalla: "pantalla-que-no-existe"},
		{Pantalla: "pos", Accion: "hacer-magia"},
	})
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if descartados != 2 {
		t.Fatalf("descartó %d de 2: sin ese número, una versión del front que manda nombres viejos deja de medir y nadie se entera", descartados)
	}
	var eventos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_events`).Scan(&eventos); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if eventos != 1 {
		t.Fatalf("entraron %d eventos y solo uno era válido", eventos)
	}
}
