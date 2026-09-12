//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// LA MIGRACIÓN DEL USO DEL SISTEMA (spec 017).
//
// Tres cosas que solo se ven contra Postgres de verdad: que la tabla del grano fino NO tenga por
// dónde guardar a una persona, que la llave del agregado SUME en vez de crear filas nuevas cuando
// hay nulos, y que las columnas de texto estén acotadas.

// LA TABLA DEL GRANO FINO NO TIENE DÓNDE GUARDAR A NADIE (FR-002, US2).
//
// No es que la aplicación no lo escriba: es que no hay columna. Lo que no existe no se llena por
// descuido, no se llena en un data-fix y no aparece en un `select *` dentro de seis meses.
func TestElEventoDeUsoNoPuedeGuardarAQuienLoHizo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	for _, tabla := range []string{"usage_events", "usage_daily"} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from information_schema.tables where table_name = $1)`, tabla,
		).Scan(&existe); err != nil {
			t.Fatalf("consultar %s: %v", tabla, err)
		}
		if !existe {
			t.Fatalf("no existe %s: la feature no tiene dónde escribir", tabla)
		}
	}

	// Cualquier columna que huela a persona. La lista es de nombres porque lo que se busca es que
	// nadie la agregue "de paso" al implementar algo más.
	filas, err := st.Pool.Query(ctx,
		`select table_name, column_name from information_schema.columns
		  where table_name in ('usage_events','usage_daily')
		    and column_name in ('user_id','usuario_id','created_by','opened_by','username','user_name','device_id','station_id')`)
	if err != nil {
		t.Fatalf("consultar columnas: %v", err)
	}
	defer filas.Close()
	var culpables []string
	for filas.Next() {
		var tabla, col string
		if err := filas.Scan(&tabla, &col); err != nil {
			t.Fatalf("scan: %v", err)
		}
		culpables = append(culpables, tabla+"."+col)
	}
	if len(culpables) > 0 {
		t.Fatalf("hay columnas que identifican a una persona: %s — lo que se escribe con nombre, se escribió", strings.Join(culpables, ", "))
	}

	// Y ninguna FK hacia `users`, que sería la misma fuga con otro disfraz.
	var fks int
	if err := st.Pool.QueryRow(ctx, `
		select count(*)
		  from information_schema.table_constraints tc
		  join information_schema.constraint_column_usage ccu on ccu.constraint_name = tc.constraint_name
		 where tc.table_name in ('usage_events','usage_daily')
		   and tc.constraint_type = 'FOREIGN KEY'
		   and ccu.table_name = 'users'`).Scan(&fks); err != nil {
		t.Fatalf("consultar FKs: %v", err)
	}
	if fks > 0 {
		t.Fatalf("hay %d llave(s) foránea(s) hacia users: la identidad entra por ahí igual", fks)
	}
}

// EL AGREGADO SUMA AUNQUE HAYA NULOS, QUE ES EL CASO NORMAL Y NO EL RARO.
//
// `action` es nula en TODA apertura de pantalla y `role` es nulo en todo evento suprimido por
// k-anonimato. En SQL `null <> null`, así que con una llave única común cada uno de esos eventos
// crearía una fila nueva: el agregado dejaría de agregar en silencio y la tabla crecería como la de
// eventos. `nulls not distinct` (Postgres 15+) es lo que lo evita.
func TestElAgregadoDeUsoSumaConNulos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const upsert = `
		insert into usage_daily (day, screen, action, role, hits)
		values (current_date, 'pos', null, null, $1)
		on conflict on constraint usage_daily_llave
		do update set hits = usage_daily.hits + excluded.hits`

	for _, n := range []int{3, 5} {
		if _, err := st.Pool.Exec(ctx, upsert, n); err != nil {
			t.Fatalf("upsert con nulos: %v", err)
		}
	}

	var filas, total int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_daily where screen = 'pos'`,
	).Scan(&filas, &total); err != nil {
		t.Fatalf("leer el agregado: %v", err)
	}
	if filas != 1 {
		t.Fatalf("quedaron %d filas y debe quedar 1: la llave no está sumando con nulos, y el agregado no agrega", filas)
	}
	if total != 8 {
		t.Fatalf("hits = %d, quiere 8: el upsert no está sumando excluded.hits", total)
	}
}

// UN NOMBRE ABSURDO NO ENTRA A LA BASE.
//
// La lista blanca de Go es la barrera buena, pero estas columnas reciben lo que venga en el cuerpo
// del POST. Un front roto —o una ruta futura que se salte la validación— puede escribir kilobytes
// por evento e inflar justo el volumen que la feature promete acotar. Un control que solo vive en
// Go se rodea por otra ruta; uno en la columna, no.
func TestUnNombreDePantallaAbsurdoNoEntra(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	enorme := strings.Repeat("a", 5000)
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_events (screen) values ($1)`, enorme); err == nil {
		t.Fatal("el esquema aceptó una pantalla de 5 KB: el check de longitud no está")
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_events (screen, action) values ('pos', $1)`, enorme); err == nil {
		t.Fatal("el esquema aceptó una acción de 5 KB")
	}
	// Y lo normal sí entra, o el test de arriba pasaría con la tabla rota.
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_events (screen, action) values ('pos', 'cobrar')`); err != nil {
		t.Fatalf("un evento normal no entró: %v", err)
	}
}

// LA CONSOLA VE EL USO DE TODAS LAS EMPRESAS, Y NO ALCANZA EL GRANO FINO.
//
// Las dos mitades son la feature: sin la política, la conexión de plataforma no fija
// `app.company_id` y `tenant_isolation` la deja viendo CERO filas — el mapa saldría vacío sin que
// nada fallara, que es la peor forma de fallar. Y sin el grant ausente sobre `usage_events`, la
// consola estaría leyendo hechos en vez de conteos.
func TestLaConsolaVeElAgregadoDeTodasYNoElGranoFino(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	prepararRolDePlataforma(t, st)

	otra := makeCompany(t, st, "otra-empresa-uso")
	// Una fila de cada empresa. El owner las escribe con company_id explícito: salta RLS.
	for _, empresa := range []int64{defaultCompanyID, otra} {
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_daily (day, screen, hits, company_id) values (current_date, 'pos', 7, $1)`,
			empresa); err != nil {
			t.Fatalf("sembrar uso de la empresa %d: %v", empresa, err)
		}
	}

	plataforma := platformRoleStore(t)
	var vistasPorLaConsola int
	if err := plataforma.Pool.QueryRow(ctx, `select count(*) from usage_daily`).Scan(&vistasPorLaConsola); err != nil {
		t.Fatalf("la consola no pudo leer el agregado: %v", err)
	}
	if vistasPorLaConsola != 2 {
		t.Fatalf("la consola ve %d filas de 2: sin la política ve cero y el mapa sale vacío sin que nada falle", vistasPorLaConsola)
	}

	// Y el grano fino le está negado.
	var n int
	err := plataforma.Pool.QueryRow(ctx, `select count(*) from usage_events`).Scan(&n)
	if err == nil {
		t.Fatalf("la consola leyó usage_events (%d filas): mira conteos, no hechos — y mañana esos hechos llevan coordenadas", n)
	}
	if !esPermisoDenegado(err) {
		t.Fatalf("usage_events no falló por permiso denegado: %v", err)
	}

	// El negocio sigue encerrado en lo suyo: abrir la política de plataforma no puede aflojar eso.
	app := appRoleStore(t)
	var vistasPorElNegocio int
	if err := app.Pool.QueryRow(ctx, `select count(*) from usage_daily`).Scan(&vistasPorElNegocio); err != nil {
		t.Fatalf("el rol del negocio no pudo leer su agregado: %v", err)
	}
	if vistasPorElNegocio != 1 {
		t.Fatalf("el rol del negocio ve %d filas y debe ver 1: el aislamiento por empresa se aflojó", vistasPorElNegocio)
	}
}

// REVERTIR DEJA EL ESQUEMA COMO ESTABA, Y VOLVER A APLICAR LO DEJA USABLE.
func TestRevertirElUsoYVolverAAplicarlo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const antesDelUso = 68
	migrarAbajoHasta(t, st.Pool, antesDelUso)

	for _, tabla := range []string{"usage_events", "usage_daily"} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from information_schema.tables where table_name = $1)`, tabla,
		).Scan(&existe); err != nil {
			t.Fatalf("consultar %s tras revertir: %v", tabla, err)
		}
		if existe {
			t.Fatalf("%s sobrevivió al Down: revertir dejó el esquema a medias", tabla)
		}
	}

	migrarArriba(t, st.Pool)
	if _, err := st.Pool.Exec(ctx, `insert into usage_events (screen) values ('pos')`); err != nil {
		t.Fatalf("tras reaplicar, la tabla no acepta un evento: %v", err)
	}
}
