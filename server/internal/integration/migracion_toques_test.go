//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// LA MIGRACIÓN DE LOS TOQUES POR ZONA (spec 019).
//
// Lo que estos casos vigilan es lo que hace segura a esta feature: que **no exista** dónde guardar
// un instante ni una persona, y que los cuatro `check` estén puestos. El de la orientación es el
// que menos se nota y el que más duele: sin él, una tableta que mande 'landscape' crea un balde
// invisible —la fila entra, pasa el rango de celda porque 0..83 vale en las dos formas, y la
// consola nunca la muestra—.

func TestLosToquesNoTienenDondeGuardarNiPersonaNiHora(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var existe bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.tables where table_name = 'usage_touches_daily')`,
	).Scan(&existe); err != nil {
		t.Fatalf("consultar la tabla: %v", err)
	}
	if !existe {
		t.Fatal("no existe usage_touches_daily")
	}

	filas, err := st.Pool.Query(ctx, `
		select column_name, data_type
		  from information_schema.columns
		 where table_name = 'usage_touches_daily'`)
	if err != nil {
		t.Fatalf("consultar columnas: %v", err)
	}
	defer filas.Close()
	var culpables []string
	for filas.Next() {
		var col, tipo string
		if err := filas.Scan(&col, &tipo); err != nil {
			t.Fatalf("scan: %v", err)
		}
		// Ninguna columna de persona…
		switch col {
		case "user_id", "created_by", "username", "device_id", "station_id":
			culpables = append(culpables, col+" (identifica a alguien)")
		}
		// …y ninguna de tiempo más fina que el día. Un `updated_at` en una fila que se toca con
		// cada lote diría a qué hora estuvo activa esa zona, que es medio camino de vuelta.
		if strings.Contains(tipo, "timestamp") {
			culpables = append(culpables, col+" ("+tipo+": más fino que el día)")
		}
	}
	if len(culpables) > 0 {
		t.Fatalf("la tabla tiene columnas que no debe: %s", strings.Join(culpables, ", "))
	}
}

// Los cuatro `check`, cada uno con lo que rechaza.
func TestLosCheckDeLosToques(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	casos := []struct {
		nombre string
		sql    string
		args   []any
	}{
		{"celda fuera de la rejilla", `insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, 'pos', 'horizontal', 84, 1)`, nil},
		{"orientación inventada", `insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, 'pos', 'landscape', 0, 1)`, nil},
		{"conteo negativo", `insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, 'pos', 'horizontal', 0, -1)`, nil},
		{"pantalla absurda", `insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, $1, 'horizontal', 0, 1)`, []any{strings.Repeat("a", 5000)}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, c.sql, c.args...); err == nil {
				t.Fatalf("el esquema lo aceptó: %s", c.nombre)
			}
		})
	}

	// Y lo normal entra, o los casos de arriba pasarían con la tabla rota.
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, 'pos', 'horizontal', 37, 1)`); err != nil {
		t.Fatalf("un toque normal no entró: %v", err)
	}
}

// La llave suma en vez de crear filas, también con el rol nulo (que es el caso de la supresión).
func TestLosToquesSumanEnLaMismaFila(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const upsert = `
		insert into usage_touches_daily (day, screen, orientation, cell, role, hits)
		values (current_date, 'pos', 'horizontal', 37, null, $1)
		on conflict on constraint usage_touches_llave
		do update set hits = usage_touches_daily.hits + excluded.hits`
	for _, n := range []int{5, 995} {
		if _, err := st.Pool.Exec(ctx, upsert, n); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	var filas, total int
	if err := st.Pool.QueryRow(ctx,
		`select count(*), coalesce(sum(hits),0) from usage_touches_daily where cell = 37`).Scan(&filas, &total); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if filas != 1 {
		t.Fatalf("quedaron %d filas y debe quedar 1: mil toques en la misma zona suben un contador, no crean nada", filas)
	}
	if total != 1000 {
		t.Fatalf("hits = %d, quiere 1000", total)
	}
}

// LA CONSOLA VE LOS TOQUES DE TODAS LAS EMPRESAS; EL NEGOCIO, SOLO LOS SUYOS.
func TestLaConsolaVeLosToquesDeTodasLasEmpresas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	prepararRolDePlataforma(t, st)

	otra := makeCompany(t, st, "otra-empresa-toques")
	for _, empresa := range []int64{defaultCompanyID, otra} {
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_touches_daily (day, screen, orientation, cell, hits, company_id)
			 values (current_date, 'pos', 'horizontal', 12, 4, $1)`, empresa); err != nil {
			t.Fatalf("sembrar toques de %d: %v", empresa, err)
		}
	}

	plataforma := platformRoleStore(t)
	var vistas int
	if err := plataforma.Pool.QueryRow(ctx, `select count(*) from usage_touches_daily`).Scan(&vistas); err != nil {
		t.Fatalf("la consola no pudo leer: %v", err)
	}
	if vistas != 2 {
		t.Fatalf("la consola ve %d filas de 2: sin la política la rejilla sale vacía y nada falla", vistas)
	}

	app := appRoleStore(t)
	var suyas int
	if err := app.Pool.QueryRow(ctx, `select count(*) from usage_touches_daily`).Scan(&suyas); err != nil {
		t.Fatalf("el negocio no pudo leer lo suyo: %v", err)
	}
	if suyas != 1 {
		t.Fatalf("el negocio ve %d filas y debe ver 1", suyas)
	}
}

// Revertir y volver a aplicar.
func TestRevertirLosToquesYVolverAAplicarlos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	const antesDeLosToques = 69
	migrarAbajoHasta(t, st.Pool, antesDeLosToques)

	var sigue bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.tables where table_name = 'usage_touches_daily')`,
	).Scan(&sigue); err != nil {
		t.Fatalf("consultar tras revertir: %v", err)
	}
	if sigue {
		t.Fatal("la tabla sobrevivió al Down")
	}

	migrarArriba(t, st.Pool)
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_touches_daily (day, screen, orientation, cell, hits) values (current_date, 'pos', 'horizontal', 0, 1)`); err != nil {
		t.Fatalf("tras reaplicar no acepta un toque: %v", err)
	}
}
