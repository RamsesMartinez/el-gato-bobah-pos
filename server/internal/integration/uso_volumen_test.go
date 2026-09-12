//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// QUE QUEPA EN EL DISCO DEL NEGOCIO (US4, FR-010, FR-011).
//
// El techo declarado es **25 MB por empresa**, y este test es lo que lo vuelve verificable. Un
// techo que solo está escrito en un documento no es un techo: es una intención.

// El volumen de un año, con el PATRÓN DE ESCRITURA REAL.
//
// Lo que hace honesto a este test es el churn: en Postgres cada `update` deja muerta la versión
// vieja de la fila, y las filas del día son pocas y calientes. Un test que insertara el total ya
// sumado mediría un escenario que la operación NUNCA produce y pasaría en verde escondiendo
// justamente el costo que hay que vigilar.
//
// Se modela como es: los días pasados ya están fríos —su churn ocurrió cuando eran "hoy" y
// autovacuum hace rato que pasó— y solo el día en curso recibe incrementos de a poco.
func TestElUsoDeUnAnoCabeEnElTecho(t *testing.T) {
	if testing.Short() {
		t.Skip("siembra un año de uso; se omite en -short")
	}
	st := newTestStore(t)
	ctx := context.Background()

	// 400 días × ~135 combinaciones: el agregado de trece meses de un local ocupado (100
	// pedidos/día, 30× lo medido en producción el 2026-09-12).
	if _, err := st.Pool.Exec(ctx, `
		insert into usage_daily (day, screen, action, role, hits, company_id)
		select current_date - d,
		       p.screen,
		       case when a = 0 then null else 'accion-' || a end,
		       r.role,
		       10 + (d % 7),
		       $1
		  from generate_series(1, 400) d,
		       (values ('pos'),('caja'),('pedidos'),('reportes'),('ventas'),
		               ('gastos'),('catalogo'),('inventario'),('usuarios')) as p(screen),
		       generate_series(0, 4) a,
		       (values ('admin'::user_role),('gerente'::user_role),('cajero'::user_role)) as r(role)`,
		defaultCompanyID); err != nil {
		t.Fatalf("sembrar el agregado de un año: %v", err)
	}

	// 14 días de grano fino: 2,000 eventos/día.
	if _, err := st.Pool.Exec(ctx, `
		insert into usage_events (occurred_at, screen, action, role, company_id)
		select now() - (d * interval '1 day'),
		       'pos',
		       case when e % 3 = 0 then null else 'cobrar' end,
		       'cajero'::user_role,
		       $1
		  from generate_series(1, 14) d, generate_series(1, 2000) e`,
		defaultCompanyID); err != nil {
		t.Fatalf("sembrar el grano fino: %v", err)
	}

	// Y EL CHURN DEL DÍA EN CURSO: 2,000 incrementos repartidos sobre las filas de hoy, de a uno,
	// como los produce el endpoint cuando el lote trae una sola combinación.
	if _, err := st.Pool.Exec(ctx, `
		do $$
		declare i int;
		begin
		  for i in 1..2000 loop
		    insert into usage_daily (day, screen, action, role, hits, company_id)
		    values (current_date, 'pos', case when i % 5 = 0 then null else 'cobrar' end,
		            'cajero'::user_role, 1, `+itoa(defaultCompanyID)+`)
		    on conflict on constraint usage_daily_llave
		    do update set hits = usage_daily.hits + excluded.hits;
		  end loop;
		end $$;`); err != nil {
		t.Fatalf("simular el churn del día: %v", err)
	}

	var eventos, agregado int64
	if err := st.Pool.QueryRow(ctx, `
		select pg_total_relation_size('usage_events'), pg_total_relation_size('usage_daily')`,
	).Scan(&eventos, &agregado); err != nil {
		t.Fatalf("medir: %v", err)
	}
	const techo = 25 << 20 // 25 MB por empresa
	total := eventos + agregado
	t.Logf("[medición] usage_events %.1f MB · usage_daily %.1f MB · total %.1f MB (techo %d MB)",
		float64(eventos)/(1<<20), float64(agregado)/(1<<20), float64(total)/(1<<20), techo>>20)
	if total > techo {
		t.Fatalf("el uso de un año ocupa %.1f MB y el techo declarado son %d MB: o el agregado no está agregando —revisa el `nulls not distinct`— o el recorte no corrió",
			float64(total)/(1<<20), techo>>20)
	}
}

// EL RECORTE BORRA LO VIEJO Y NO TOCA LO DE ADENTRO.
func TestElRecorteDeUsoDejaLoQueEstaDentroDeLaRetencion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Dos eventos: uno de hace 20 días (fuera) y uno de ayer (dentro).
	// Dos filas del agregado: una de hace 14 meses (fuera) y una de hace un mes (dentro).
	for _, dias := range []int{20, 1} {
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_events (occurred_at, screen) values (now() - ($1::int * interval '1 day'), 'pos')`,
			dias); err != nil {
			t.Fatalf("sembrar evento de hace %d días: %v", dias, err)
		}
	}
	for _, dias := range []int{430, 30} {
		if _, err := st.Pool.Exec(ctx,
			`insert into usage_daily (day, screen, hits) values (current_date - $1::int, 'pos', 5)`,
			dias); err != nil {
			t.Fatalf("sembrar agregado de hace %d días: %v", dias, err)
		}
	}

	svc := app.NewUsageService(st)
	if err := svc.Recortar(ctx); err != nil {
		t.Fatalf("recortar: %v", err)
	}

	var eventos, agregado int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_events`).Scan(&eventos); err != nil {
		t.Fatalf("contar eventos: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_daily`).Scan(&agregado); err != nil {
		t.Fatalf("contar agregado: %v", err)
	}
	if eventos != 1 {
		t.Fatalf("quedaron %d eventos y debe quedar 1: el de hace 20 días se va, el de ayer se queda", eventos)
	}
	if agregado != 1 {
		t.Fatalf("quedaron %d filas del agregado y debe quedar 1", agregado)
	}
}

// LA PUERTA DE LAS COORDENADAS SIGUE ABIERTA (FR-013, SC-006).
//
// La promesa es que agregarlas después no exija migrar nada. La única forma de comprobarla antes de
// necesitarla es escribir unas y leerlas de vuelta, sin tocar el esquema.
func TestSePuedenGuardarCoordenadasSinMigrarNada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Pool.Exec(ctx, `insert into usage_events (screen, action) values ('pos', 'cobrar')`); err != nil {
		t.Fatalf("sembrar: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`update usage_events set detail = '{"x":120,"y":340}'::jsonb where screen = 'pos'`); err != nil {
		t.Fatalf("guardar coordenadas: %v", err)
	}

	var x, y int
	if err := st.Pool.QueryRow(ctx,
		`select (detail->>'x')::int, (detail->>'y')::int from usage_events where detail is not null`,
	).Scan(&x, &y); err != nil {
		t.Fatalf("leer coordenadas: %v", err)
	}
	if x != 120 || y != 340 {
		t.Fatalf("leyó (%d,%d) y guardó (120,340)", x, y)
	}
}

// EL CICLO DEL RECORTE TERMINA CUANDO SE APAGA LA API.
//
// No es celo: es una goroutine que sostiene una conexión de DUEÑO. Sin condición de término
// sobrevive al apagado, y el principio II no lo permite.
func TestElCicloDelRecorteTerminaAlApagar(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewUsageService(st)

	ctx, cancelar := context.WithCancel(context.Background())
	listo := make(chan struct{})
	go func() {
		// Un intervalo largo: lo que se prueba es que TERMINA por el contexto, no que dispare.
		svc.RecortarPeriodicamente(ctx, time.Hour)
		close(listo)
	}()

	cancelar()
	select {
	case <-listo:
	case <-time.After(5 * time.Second):
		t.Fatal("el ciclo del recorte no terminó al cancelar el contexto: queda una goroutine viva con una conexión de dueño")
	}
}
