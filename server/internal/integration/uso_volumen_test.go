//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
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

	// Y EL CHURN DEL DÍA EN CURSO, al TECHO DEL LIMITADOR y no al del uso honesto: 30 lotes por
	// minuto × 50 eventos × 60 minutos × 24 horas son 2.16 millones de eventos diarios por cuenta.
	// Ese es el borde contra el que hay que medir —lo dice la constitución: el test se escribe
	// contra el borde, no contra el caso feliz—, y es el que la auditoría usó para tumbar la tabla
	// de grano fino, donde habrían sido 298 MB en un día.
	//
	// Aquí no crecen las FILAS: la lista blanca acota las combinaciones posibles, así que dos
	// millones de eventos solo suben contadores. Se simulan 20,000 incrementos —el churn de varias
	// horas al tope— porque lo que se mide es el costo de las versiones muertas, y a partir de ahí
	// es lineal y acotado por autovacuum.
	if _, err := st.Pool.Exec(ctx, `
		do $$
		declare i int;
		begin
		  for i in 1..20000 loop
		    insert into usage_daily (day, screen, action, role, hits, company_id)
		    values (current_date, 'pos', case when i % 5 = 0 then null else 'cobrar' end,
		            'cajero'::user_role, 1, `+itoa(defaultCompanyID)+`)
		    on conflict on constraint usage_daily_llave
		    do update set hits = usage_daily.hits + excluded.hits;
		  end loop;
		end $$;`); err != nil {
		t.Fatalf("simular el churn del día al tope del limitador: %v", err)
	}

	var total int64
	if err := st.Pool.QueryRow(ctx, `select pg_total_relation_size('usage_daily')`).Scan(&total); err != nil {
		t.Fatalf("medir: %v", err)
	}
	const techo = 25 << 20 // 25 MB por empresa
	t.Logf("[medición] usage_daily %.1f MB (techo %d MB), con el churn al tope del limitador",
		float64(total)/(1<<20), techo>>20)
	if total > techo {
		t.Fatalf("el uso de un año ocupa %.1f MB y el techo declarado son %d MB: o el agregado no está agregando —revisa el `nulls not distinct`— o el recorte no corrió",
			float64(total)/(1<<20), techo>>20)
	}
}

// EL RECORTE BORRA LO VIEJO Y NO TOCA LO DE ADENTRO.
func TestElRecorteDeUsoDejaLoQueEstaDentroDeLaRetencion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Dos filas: una de hace 14 meses (fuera de la retención) y una de hace un mes (dentro).
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

	var quedan int
	if err := st.Pool.QueryRow(ctx, `select count(*) from usage_daily`).Scan(&quedan); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if quedan != 1 {
		t.Fatalf("quedaron %d filas y debe quedar 1: la de hace 14 meses se va, la del mes pasado se queda", quedan)
	}
}

// LA PUERTA DE LAS COORDENADAS SE CRUZÓ (FR-013, SC-006), y se cruzó por otro lado del que este
// test esperaba.
//
// El plan de la 017 la dejaba como una columna `jsonb` en una tabla de grano fino, y este test
// ensayaba justo esa migración: una tabla con `occurred_at`, `x` y `y`. La spec 019 la construyó y
// **no se parece a eso**: es `usage_touches_daily`, un conteo por zona, sin instante y sin punto,
// por las mismas dos razones que habían tumbado la tabla fina —el instante por fila se cruza con
// `register_sessions.closed_by` y el volumen no cabía—.
//
// El test se queda porque lo que fijaba sigue siendo cierto y ahora es un hecho verificable en vez
// de un ensayo: medir dónde cae el dedo **no exigió migrar una sola fila** de lo ya escrito.
func TestLaPuertaDeLasCoordenadasSeCruzoSinMigrarLoEscrito(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Pool.Exec(ctx,
		`insert into usage_daily (day, screen, hits) values (current_date, 'pos', 7)`); err != nil {
		t.Fatalf("sembrar uso: %v", err)
	}

	// La tabla de la 019 vive AL LADO, no dentro: ninguna columna del agregado se tocó.
	if _, err := st.Pool.Exec(ctx,
		`insert into usage_touches_daily (day, screen, orientation, cell, hits)
		 values (current_date, 'pos', 'horizontal', 37, 3)`); err != nil {
		t.Fatalf("la tabla de zonas no convive con el agregado: %v", err)
	}

	var hits int
	if err := st.Pool.QueryRow(ctx, `select hits from usage_daily where screen = 'pos'`).Scan(&hits); err != nil {
		t.Fatalf("releer el conteo: %v", err)
	}
	if hits != 7 {
		t.Fatalf("el conteo cambió a %d: cruzar la puerta tocó lo ya escrito", hits)
	}

	// Y la forma en que se cruzó es la que importa: sin instante y sin punto. Si alguien agregara
	// cualquiera de los dos, el anonimato de las DOS tablas se deshace —el instante las cruza—.
	var conTiempo int
	if err := st.Pool.QueryRow(ctx, `
		select count(*) from information_schema.columns
		 where table_name = 'usage_touches_daily'
		   and (data_type like 'timestamp%' or column_name in ('x', 'y'))`).Scan(&conTiempo); err != nil {
		t.Fatalf("leer el catálogo: %v", err)
	}
	if conTiempo != 0 {
		t.Fatal("la tabla de zonas ganó un instante o una coordenada: es exactamente el diseño que la auditoría de la 017 tumbó")
	}
}

// EL CICLO DEL RECORTE TERMINA CUANDO SE APAGA LA API.
//
// No es celo: es una goroutine que sostiene una conexión de DUEÑO. Sin condición de término
// sobrevive al apagado, y el principio II no lo permite.
func TestElCicloDelRecorteTerminaAlApagar(t *testing.T) {
	// Migra el esquema y deja el harness listo; el recorte abre su propia conexión.
	newTestStore(t)

	ctx, cancelar := context.WithCancel(context.Background())
	listo := make(chan struct{})
	go func() {
		// Un intervalo largo: lo que se prueba es que TERMINA por el contexto, no que dispare.
		// Una conexión NUEVA por pasada, como en producción: devolver la del test haría que el
		// `defer st.Close()` del recorte cerrara el store del harness.
		app.RecortarPeriodicamente(ctx, time.Hour, func(c context.Context) (*store.Store, error) {
			return store.New(c, testURL(t))
		})
		close(listo)
	}()

	cancelar()
	select {
	case <-listo:
	case <-time.After(5 * time.Second):
		t.Fatal("el ciclo del recorte no terminó al cancelar el contexto: queda una goroutine viva con una conexión de dueño")
	}
}
