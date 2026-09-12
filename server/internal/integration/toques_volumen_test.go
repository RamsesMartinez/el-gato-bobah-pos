//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// QUE QUEPA (US4 de la 019).
//
// Esta es la mitad que la 017 pagó cara: allá se propuso un renglón por evento y la auditoría la
// tumbó por dos caminos —el instante por toque deshacía el anonimato, y el volumen al tope del
// limitador eran 4 GB en dos semanas—. Un mapa por coordenada es ese caso con más fuerza: son miles
// de toques por turno, no decenas de aperturas.
//
// El techo declarado son **20 MB por empresa**. Un techo que solo está escrito en un documento no
// es un techo: es una intención.
func TestLosToquesDeUnTrimestreCabenEnElTecho(t *testing.T) {
	if testing.Short() {
		t.Skip("siembra un trimestre de toques; se omite en -short")
	}
	st := newTestStore(t)
	ctx := context.Background()

	// EL TRIMESTRE COMPLETO AL TOPE DE LO POSIBLE: 84 celdas × 2 orientaciones × 5 cortes de rol
	// (los cuatro roles y el «sin corte») × 92 días = 77,280 filas. Es el máximo por construcción y
	// no una estimación: la rejilla acota las combinaciones, así que ninguna cantidad de dedos las
	// aumenta.
	//
	// **LAS DOS ORIENTACIONES.** Se me olvidó una vez al calcular el techo y el número salió a la
	// mitad; es el multiplicador que más fácil se pierde porque en el día a día solo se usa una.
	if _, err := st.Pool.Exec(ctx, `
		insert into usage_touches_daily (day, screen, orientation, cell, role, hits, company_id)
		select current_date - d, 'pos', o.orientation, c.cell, r.role, 20 + (d % 11), $1
		  from generate_series(1, 92) d,
		       (values ('horizontal'),('vertical')) as o(orientation),
		       generate_series(0, 83) c(cell),
		       (values ('admin'::user_role),('gerente'::user_role),('cajero'::user_role),
		               ('mesero'::user_role),(null::user_role)) as r(role)`,
		defaultCompanyID); err != nil {
		t.Fatalf("sembrar el trimestre: %v", err)
	}

	// Y EL CHURN DEL DÍA EN CURSO, con el PATRÓN DE ESCRITURA REAL.
	//
	// Es lo que hace honesto al test: en Postgres cada `update` deja muerta la versión vieja de la
	// fila, y estas filas son pocas y calientes. Un test que insertara el total ya sumado mediría
	// un escenario que la operación nunca produce y pasaría en verde escondiendo justo el costo que
	// hay que vigilar. Los días pasados sí se siembran de un golpe, y eso es fiel: su churn ocurrió
	// cuando eran «hoy» y autovacuum hace rato que pasó.
	//
	// 20,000 incrementos repartidos entre las celdas calientes = varias horas al tope del
	// limitador (30 lotes/min × 50 toques). A partir de ahí el costo es lineal y lo acota
	// autovacuum.
	if _, err := st.Pool.Exec(ctx, `
		do $$
		declare i int;
		begin
		  for i in 1..20000 loop
		    insert into usage_touches_daily (day, screen, orientation, cell, role, hits, company_id)
		    values (current_date, 'pos',
		            case when i % 7 = 0 then 'vertical' else 'horizontal' end,
		            i % 84, 'cajero'::user_role, 1, `+itoa(defaultCompanyID)+`)
		    on conflict on constraint usage_touches_llave
		    do update set hits = usage_touches_daily.hits + excluded.hits;
		  end loop;
		end $$;`); err != nil {
		t.Fatalf("simular el churn del día al tope del limitador: %v", err)
	}

	var filas, total, indices int64
	if err := st.Pool.QueryRow(ctx, `
		select count(*),
		       pg_total_relation_size('usage_touches_daily'),
		       pg_indexes_size('usage_touches_daily')
		  from usage_touches_daily`).Scan(&filas, &total, &indices); err != nil {
		t.Fatalf("medir: %v", err)
	}
	// El techo es POR PANTALLA INSTRUMENTADA y por empresa, y así está declarado: hoy la lista
	// tiene una (`pos`) y ocupa casi todo. Agregar otra cuesta una línea de código **y otro tanto
	// de disco** — ver el renglón de AGENTS.md sobre dónde se agrega una.
	const techo = 20 << 20 // 20 MB por pantalla instrumentada, por empresa
	t.Logf("[medición] usage_touches_daily %.1f MB (%.1f de índices) en %d filas — %d bytes/fila; techo %d MB",
		float64(total)/(1<<20), float64(indices)/(1<<20), filas, total/filas, techo>>20)
	if total > techo {
		t.Fatalf("un trimestre de toques ocupa %.1f MB y el techo declarado son %d MB por pantalla: o el conteo dejó de agregar —revisa el `nulls not distinct` y el `on conflict`—, o el recorte no corrió, o se instrumentó otra pantalla sin volver a medir",
			float64(total)/(1<<20), techo>>20)
	}
}

// EL RECORTE DE TOQUES USA SU PROPIA RETENCIÓN, más corta que la del agregado.
//
// Son 92 días contra 396, y la diferencia es deliberada: el conteo por pantalla se mira año contra
// año, mientras que una rejilla de hace un año describe un layout que ya no existe. Si los dos
// compartieran constante, los toques vivirían trece meses y la mitad de ese tiempo estarían
// describiendo una pantalla que ya se rediseñó.
func TestElRecorteDeToquesUsaSuPropiaRetencion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Tres filas: una fuera de la retención de los toques, una dentro, y —la que importa— una que
	// estaría dentro si se usara la retención del agregado.
	for _, dias := range []int{200, 100, 30} {
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_touches_daily (day, screen, orientation, cell, hits)
			 values (current_date - $1::int, 'pos', 'horizontal', $1 % 84, 5)`, dias); err != nil {
			t.Fatalf("sembrar toques de hace %d días: %v", dias, err)
		}
	}
	// Y una del agregado a los 200 días, que NO se debe ir: comparten servicio y no retención.
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_daily (day, screen, hits) values (current_date - 200, 'pos', 5)`); err != nil {
		t.Fatalf("sembrar agregado: %v", err)
	}

	if err := app.NewUsageService(st).Recortar(ctx); err != nil {
		t.Fatalf("recortar: %v", err)
	}

	var quedanToques, quedaAgregado int
	if err := st.Pool.QueryRow(ctx,
		`select (select count(*) from usage_touches_daily), (select count(*) from usage_daily)`).
		Scan(&quedanToques, &quedaAgregado); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if quedanToques != 1 {
		t.Fatalf("quedaron %d filas de toques y debe quedar 1: la de hace 30 días", quedanToques)
	}
	if quedaAgregado != 1 {
		t.Fatal("el recorte de toques se llevó el agregado de hace 200 días: las retenciones son distintas a propósito")
	}
}
