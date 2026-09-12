//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// NUNCA UNA CAPTURA, NUNCA LA PERSONA (US2 de la 019), contra la base de verdad.
//
// Va antes que la rejilla por lo mismo que en la 017: lo que se escriba con la identidad de alguien
// ya se escribió, y una rejilla se puede pintar mañana.

// Tras registrar toques, nada en la fila apunta a quién tocó ni a cuándo.
func TestLosToquesNoGuardanAQuienToco(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)

	// Dos del mismo rol para que el corte SÍ se conserve: así lo que este test mira es la ausencia
	// de la persona y no el efecto de la supresión, que tiene su propio test abajo.
	makeUser(t, st, "cajero_toque_uno", "cajero")
	makeUser(t, st, "cajero_toque_dos", "cajero")

	// `RegistrarToques` recibe un ROL y una CELDA. No hay parámetro por donde entre una persona ni
	// un punto: la identidad y la coordenada fina no se filtran después, no existen.
	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 0, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 83, Orientacion: domain.OrientacionHorizontal},
	}}); err != nil {
		t.Fatalf("registrar toques: %v", err)
	}

	var filas, suma int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_touches_daily`).Scan(&filas, &suma); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if suma != 2 {
		t.Fatalf("se contaron %d de 2 toques: con la tabla vacía este test pasaría en verde sin medir nada", suma)
	}
	if filas != 2 {
		t.Fatalf("quedaron %d filas y son dos celdas distintas", filas)
	}
}

// EL CASO QUE IMPORTA: un rol con una sola persona no se puede cortar (FR-011).
//
// Con un solo mesero en la empresa, «el mesero tocó aquí 300 veces» es su nombre. Y no basta con
// dejar el rol en blanco: si ES EL ÚNICO rol suprimido, el balde `sin corte` también es él. Por eso
// en ese caso no se escribe nada.
func TestElRolDeUnaSolaPersonaNoSeGuardaEnLosToques(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)

	makeUser(t, st, "mesero_toque_solito", "mesero")
	if _, err := svc.Registrar(ctx, domain.RoleMesero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 12, Orientacion: domain.OrientacionHorizontal},
	}}); err != nil {
		t.Fatalf("registrar el toque del mesero: %v", err)
	}

	var filas int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_touches_daily`).Scan(&filas); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if filas != 0 {
		t.Fatal("se escribió la rejilla de la única persona activa: con un solo rol bajo el umbral, `sin corte` no tapa a nadie — es esa persona")
	}

	// Y CON EL BALDE LLENO SÍ SE MIDE, sin rol. Si no, un local chico saldría con la rejilla vacía
	// y eso se lee como «aquí nadie toca» en vez de como «aquí no se puede cortar».
	makeUser(t, st, "admin_toque_solito", "admin")
	if _, err := svc.Registrar(ctx, domain.RoleMesero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 12, Orientacion: domain.OrientacionHorizontal},
	}}); err != nil {
		t.Fatalf("registrar con el balde lleno: %v", err)
	}

	var conRol, sinRol int
	if err := st.Pool.QueryRow(ctx, `
		select count(*) filter (where role is not null),
		       coalesce(sum(hits) filter (where role is null), 0)
		  from usage_touches_daily`).Scan(&conRol, &sinRol); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if conRol != 0 {
		t.Fatal("el rol quedó escrito con una sola persona en él: el corte identifica por eliminación")
	}
	if sinRol != 1 {
		t.Fatalf("el toque sin corte se contó %d veces y debe contarse una: con dos personas en el balde, suprimir el rol no es tirar el dato", sinRol)
	}
}

// MIL TOQUES EN LA MISMA ZONA NO SON MIL RENGLONES (FR-012).
//
// Es la promesa de la que depende que esta feature quepa, y la única forma de romperla en silencio
// es que alguien cambie el `upsert` por un `insert`: seguiría midiendo bien y la tabla crecería con
// los dedos. Un turno real son miles de toques por tableta.
func TestUnaRafagaDeToquesNoCreaFilasNuevas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_rafaga_uno", "cajero")
	makeUser(t, st, "cajero_rafaga_dos", "cajero")

	// Veinte lotes al tope, que es como llegarían de verdad: la tableta manda cada diez segundos.
	const lotes, porLote = 20, domain.MaxToquesPorLote
	for i := 0; i < lotes; i++ {
		lote := make([]domain.Toque, 0, porLote)
		for j := 0; j < porLote; j++ {
			lote = append(lote, domain.Toque{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal})
		}
		if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: lote}); err != nil {
			t.Fatalf("registrar la ráfaga %d: %v", i, err)
		}
	}

	var filas, suma int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_touches_daily`).Scan(&filas, &suma); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if filas != 1 {
		t.Fatalf("quedaron %d filas para UNA celda: el conteo dejó de ser conteo y la tabla crece con los dedos", filas)
	}
	if suma != lotes*porLote {
		t.Fatalf("se contaron %d de %d toques", suma, lotes*porLote)
	}
}

// LAS DOS ORIENTACIONES NO SE MEZCLAN (FR-016), ya desde la escritura.
//
// La celda 37 es un lugar distinto en cada forma de pantalla. Si compartieran fila, la rejilla
// pintaría un mapa que nadie tocó nunca — y no habría forma de separarlas después.
func TestLaMismaCeldaEnDosOrientacionesSonDosFilas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_orient_uno", "cajero")
	makeUser(t, st, "cajero_orient_dos", "cajero")

	if _, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: domain.OrientacionVertical},
	}}); err != nil {
		t.Fatalf("registrar toques: %v", err)
	}

	var filas int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from usage_touches_daily where cell = 37`).Scan(&filas); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if filas != 2 {
		t.Fatalf("la celda 37 quedó en %d fila(s): horizontal y vertical son lugares distintos", filas)
	}
}

// Lo que no está en la lista instrumentada no llega a la base, y el servicio dice cuántos descartó.
//
// Ese número es el único testigo de que una versión del front quedó midiendo una pantalla que el
// servidor ya no acepta: sin él, la rejilla solo muestra menos.
func TestElToqueFueraDeLaListaSeDescartaYSeCuenta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewUsageService(st)
	makeUser(t, st, "cajero_descarte_uno", "cajero")
	makeUser(t, st, "cajero_descarte_dos", "cajero")

	descartados, err := svc.Registrar(ctx, domain.RoleCajero, app.LoteDeMedicion{Toques: []domain.Toque{
		{Pantalla: "pos", Celda: 5, Orientacion: domain.OrientacionHorizontal},
		// Medible en la 017, pero NO instrumentada para toques.
		{Pantalla: "caja", Celda: 5, Orientacion: domain.OrientacionHorizontal},
		// Fuera de la rejilla.
		{Pantalla: "pos", Celda: 84, Orientacion: domain.OrientacionHorizontal},
		// La orientación inventada: la que crearía un balde invisible si entrara.
		{Pantalla: "pos", Celda: 5, Orientacion: "landscape"},
	}})
	if err != nil {
		t.Fatalf("registrar toques: %v", err)
	}
	if descartados.Toques != 3 {
		t.Fatalf("descartó %d de 3", descartados.Toques)
	}

	var filas int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_touches_daily`).Scan(&filas); err != nil {
		t.Fatalf("leer los toques: %v", err)
	}
	if filas != 1 {
		t.Fatalf("entraron %d filas y solo una era válida", filas)
	}
}
