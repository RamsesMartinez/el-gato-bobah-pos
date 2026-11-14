//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const sessionAdjustmentsVersion = 113

// LA 0113 (correcciones de un corte cerrado, #290 fase 3) CONTRA UN RESPALDO REAL CON DOS EMPRESAS.
// Es una tabla nueva: lo que se prueba es que la API anterior sigue cerrando y leyendo cortes con la
// tabla puesta, que las reglas de forma viven en la base (un reverso solo revierte una corrección del
// MISMO corte, y una sola vez), y que el Down se niega en cuanto hay una corrección.
func TestMigration0113OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, sessionAdjustmentsVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	// Dos cortes cerrados de empresas distintas.
	type corte struct{ company, session, user int64 }
	var cortes []corte
	rows, err := st.Pool.Query(ctx, `
		select distinct on (s.company_id) s.company_id, s.id, s.opened_by
		from register_sessions s where s.status = 'cerrada' order by s.company_id, s.id desc`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c corte
		if err := rows.Scan(&c.company, &c.session, &c.user); err != nil {
			t.Fatal(err)
		}
		cortes = append(cortes, c)
	}
	rows.Close()
	if len(cortes) < 2 {
		t.Fatalf("el respaldo trae cortes cerrados de %d empresas; la prueba necesita dos", len(cortes))
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", sessionAdjustmentsVersion); err != nil {
		t.Fatal(err)
	}

	// LA API ANTERIOR NO CONOCE LA TABLA: sus lecturas del histórico y del corte siguen igual.
	t.Run("la API anterior sigue leyendo el histórico", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from register_sessions where status = 'cerrada'`).Scan(&n); err != nil || n == 0 {
			t.Fatalf("leer cortes con la 0113 puesta: %d %v", n, err)
		}
	})

	insertar := func(c corte, kind string, before, after any, reverses any) (int64, error) {
		var id int64
		err := st.Pool.QueryRow(ctx, `
			insert into cash_session_adjustments (company_id, session_id, kind, before_amount, after_amount, reason, note, reverses_id, created_by)
			values ($1, $2, $3, $4, $5, 'miscount', 'prueba 0113', $6, $7) returning id`,
			c.company, c.session, kind, before, after, reverses, c.user).Scan(&id)
		return id, err
	}

	a, b := cortes[0], cortes[1]
	var corrA int64
	t.Run("una corrección de conteo con antes y después", func(t *testing.T) {
		var err error
		if corrA, err = insertar(a, "count_correction", "1000", "1120", nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("las reglas de forma viven en la base", func(t *testing.T) {
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "rewrite", nil, "1", nil); return e }(), "un tipo que no existe")
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "count_correction", "1000", nil, nil); return e }(), "un conteo sin monto")
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "note", nil, "5", nil); return e }(), "una nota con monto")
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "count_correction", "1000", "-1", nil); return e }(), "un monto negativo")
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "reversal", nil, nil, nil); return e }(), "un reverso sin lo que revierte")
	})
	t.Run("un reverso solo revierte una corrección de SU corte, y una vez", func(t *testing.T) {
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(b, "reversal", nil, nil, corrA); return e }(),
			"revertir desde otro corte la corrección de un corte ajeno")
		if _, err := insertar(a, "reversal", nil, nil, corrA); err != nil {
			t.Fatalf("revertir en su corte: %v", err)
		}
		exigeViolacionDeRestriccion(t, func() error { _, e := insertar(a, "reversal", nil, nil, corrA); return e }(), "revertir dos veces")
	})

	t.Run("el Down se niega con correcciones y su limpieza lo deja bajar", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con correcciones: perdería el rastro de quién corrigió qué")
		}
		migrarAbajoHasta(t, st.Pool, sessionAdjustmentsVersion-1)
		var existe bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass('cash_session_adjustments') is not null`).Scan(&existe); err != nil || existe {
			t.Fatalf("la tabla sigue tras el Down: %v %v", existe, err)
		}
	})
}
