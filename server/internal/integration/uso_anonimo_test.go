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

	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Eventos: []domain.EventoDeUso{
		{Pantalla: "pos"},
		{Pantalla: "pos", Accion: "cobrar"},
	}}); err != nil {
		t.Fatalf("registrar uso: %v", err)
	}

	// `Registrar` recibe un ROL, no un usuario: la identidad no puede llegar ni por error, porque
	// no hay parámetro por donde. Y no queda NINGÚN instante por fila con el que cruzar: lo que se
	// guarda es un conteo por día, no un renglón por toque — ver el porqué en la migración 0069.
	_ = uno

	var filas, suma int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_daily`).Scan(&filas, &suma); err != nil {
		t.Fatalf("leer el agregado: %v", err)
	}
	if suma != 2 {
		t.Fatalf("se contaron %d de 2 eventos: el test de arriba pasaría en verde con la tabla vacía", suma)
	}
	if filas != 2 {
		t.Fatalf("quedaron %d combinaciones y son 2 (una apertura y un cobro)", filas)
	}
}

// EL CASO QUE DE VERDAD IMPORTA: un rol con una sola persona no se puede cortar (FR-009).
//
// Decir «el gerente hizo estas 40 acciones» en una empresa con un gerente es decir su nombre. La
// supresión ocurre al ESCRIBIR: lo que no se escribió no se puede consultar, ni con acceso a la
// base, ni dentro de un año cuando ya nadie recuerde esta regla.
//
// Y la supresión mira la plantilla ENTERA, no solo ese rol: si es el único por debajo del umbral,
// el balde «sin corte» también lo identifica. Lo prueba en tabla
// `TestElBaldeSinCorteNoPuedeSerUnaSolaPersona`, en el dominio.
func TestElRolDeUnaSolaPersonaSeGuardaSinCorte(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)

	// Un solo mesero en la empresa.
	makeUser(t, st, "mesero_solito", "mesero")
	if _, err := svc.Registrar(ctx, domain.RoleMesero, app.LoteDeMedicion{Eventos: []domain.EventoDeUso{{Pantalla: "pos"}}}); err != nil {
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
	if _, err := svc.Registrar(ctx, domain.RoleGerente, app.LoteDeMedicion{Eventos: []domain.EventoDeUso{{Pantalla: "caja"}}}); err != nil {
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

// El lote pre-agregado NO pierde eventos: tres toques son tres, aunque se escriban en un solo
// `update`.
func TestElLotePreAgregadoNoPierdeEventos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_grano_uno", "cajero")
	makeUser(t, st, "cajero_grano_dos", "cajero")

	lote := []domain.EventoDeUso{{Pantalla: "pos"}, {Pantalla: "pos"}, {Pantalla: "pos"}}
	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Eventos: lote}); err != nil {
		t.Fatalf("registrar: %v", err)
	}

	var filas, hits int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_daily`).Scan(&filas, &hits); err != nil {
		t.Fatalf("leer el agregado: %v", err)
	}
	if filas != 1 {
		t.Fatalf("quedaron %d filas y los tres toques son la misma combinación", filas)
	}
	if hits != 3 {
		t.Fatalf("el agregado suma %d de 3 toques: se perdieron por el camino", hits)
	}
}

// Lo que no está en la lista blanca no llega a la base, y el servicio dice cuántos descartó.
func TestLoDesconocidoSeDescartaYSeCuenta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_desconocido_uno", "cajero")
	makeUser(t, st, "cajero_desconocido_dos", "cajero")

	descartados, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Eventos: []domain.EventoDeUso{
		{Pantalla: "pos"},
		{Pantalla: "pantalla-que-no-existe"},
		{Pantalla: "pos", Accion: "hacer-magia"},
	}})
	if err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if descartados.Eventos != 2 {
		t.Fatalf("descartó %d de 2: sin ese número, una versión del front que manda nombres viejos deja de medir y nadie se entera", descartados.Eventos)
	}
	var hits int
	if err := st.Pool.QueryRow(ctx, `select coalesce(sum(hits),0) from usage_daily`).Scan(&hits); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if hits != 1 {
		t.Fatalf("se contaron %d eventos y solo uno era válido", hits)
	}
}
