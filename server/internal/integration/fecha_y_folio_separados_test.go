//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"uuid"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL DEFECTO QUE SE REPORTÓ: "hice ventas hoy pero no se ven en la pantalla de ventas".
//
// La venta heredaba la fecha del turno abierto, sin techo. Medido el 2026-09-04 en el ambiente de
// pruebas: el turno abrió el 31 de agosto y nadie lo cerró, así que 158 pedidos y $6,664 quedaron
// archivados como 31 de agosto —incluidos los de ese mismo día— y `sales?preset=hoy` devolvía cero
// filas mientras el negocio vendía.
//
// El turno de este test tiene CUATRO días de antigüedad a propósito: con uno solo, un cálculo que
// se equivoque por una hora todavía pasa.
func TestLaVentaSeArchivaEnElDiaEnQueOcurrioYNoEnElDelTurno(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_fecha_reloj", "cajero")
	prod := makeProduct(t, st, "Café de hoy", decimal.RequireFromString("42"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")

	// Turno abierto hace cuatro días, como el que se encontró en producción.
	haceCuatroDias := fixedNow.AddDate(0, 0, -4)
	sess := abrirCajaEn(t, st, cajero, haceCuatroDias)

	svc := app.NewOrdersService(st, clock)
	o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("42")}},
	})
	if err != nil {
		t.Fatalf("crear la venta: %v", err)
	}

	var archivada time.Time
	var turno int64
	if err := st.Pool.QueryRow(ctx,
		`select business_date, register_session_id from orders where id = $1`, o.ID).Scan(&archivada, &turno); err != nil {
		t.Fatalf("leer la venta: %v", err)
	}

	quiere := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	if !archivada.Equal(quiere) {
		t.Errorf("la venta ocurrió el %s y quedó archivada el %s: la fecha se sigue heredando del "+
			"turno, que abrió el %s",
			quiere.Format("2006-01-02"), archivada.Format("2006-01-02"), haceCuatroDias.Format("2006-01-02"))
	}
	// Y la otra mitad: la venta SIGUE perteneciendo a su turno. Es lo que hace que el arqueo cuadre.
	if turno != sess {
		t.Errorf("la venta quedó en el turno %d y debía quedar en el %d: cambiar la fecha no puede "+
			"cambiar de qué corte responde el dinero", turno, sess)
	}
}

// El contador del folio vive por TURNO, y un turno que ya venía con folios repartidos continúa
// desde donde iba.
//
// Sin la semilla de la migración, el turno de 158 pedidos del ambiente de pruebas habría pedido el
// número 1 en su siguiente venta. Antes eso pasaba callado; ahora el índice único lo convierte en
// un 23505 y la venta no se puede cobrar, que es peor. Por eso la semilla no es opcional.
func TestUnTurnoConFoliosRepartidosContinuaLaNumeracion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_semilla", "cajero")
	prod := makeProduct(t, st, "Café semilla", decimal.RequireFromString("20"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	sess := abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	venta := func() *app.OrderView {
		t.Helper()
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("20")}},
		})
		if err != nil {
			t.Fatalf("crear: %v", err)
		}
		return o
	}

	venta()
	ultima := venta()

	// Se borra el contador para dejar al turno como lo dejaría la migración SIN semilla: con folios
	// ya repartidos y sin fila que lo recuerde.
	if _, err := st.Pool.Exec(ctx, `delete from folio_counters where register_session_id = $1`, sess); err != nil {
		t.Fatalf("borrar el contador: %v", err)
	}
	// La misma sentencia que siembra 0061. Si allá se escribe distinto —un `min` en vez de un `max`,
	// un group by incompleto— este test deja de reflejarla y hay que moverlos juntos.
	if _, err := st.Pool.Exec(ctx, `
		insert into folio_counters (company_id, register_session_id, last_number)
		select company_id, register_session_id, max(daily_number)
		from orders where register_session_id is not null
		group by company_id, register_session_id`); err != nil {
		t.Fatalf("sembrar: %v", err)
	}

	siguiente := venta()
	if siguiente.Number != ultima.Number+1 {
		t.Errorf("el turno traía el folio %d y la siguiente venta recibió el %d: la semilla no "+
			"continuó la numeración", ultima.Number, siguiente.Number)
	}
}

// Dos cobros simultáneos del mismo turno nunca reciben el mismo número.
//
// Lo que da esa garantía no es la consulta sino el CANDADO DE FILA del `on conflict do update`, que
// bloquea el contador hasta el commit. Al mover el contador de `order_counters` a `folio_counters`
// se puede perder sin que nada más se note: la numeración solo se rompe cuando dos personas cobran
// a la vez, o sea el día ocupado y no el día de la prueba manual.
func TestDosCobrosSimultaneosNoCompartenFolio(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_concurrente", "cajero")
	prod := makeProduct(t, st, "Café concurrente", decimal.RequireFromString("10"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	const cobros = 8
	numeros := make([]int, cobros)
	errs := make([]error, cobros)
	var wg sync.WaitGroup
	arranque := make(chan struct{})
	for i := range cobros {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := app.NewOrdersService(st, clock)
			<-arranque // que salgan todos juntos, no en fila
			o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
				ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
				Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
				Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("10")}},
			})
			if err != nil {
				errs[i] = err
				return
			}
			numeros[i] = o.Number
		}()
	}
	close(arranque)
	wg.Wait()

	vistos := map[int]int{}
	for i, n := range numeros {
		if errs[i] != nil {
			t.Fatalf("cobro %d: %v", i, errs[i])
		}
		vistos[n]++
	}
	for n, veces := range vistos {
		if veces > 1 {
			t.Errorf("el folio %d se repartió %d veces: el contador dejó de serializar", n, veces)
		}
	}
	if len(vistos) != cobros {
		t.Errorf("%d cobros repartieron %d folios distintos", cobros, len(vistos))
	}
}

// La tabla nueva funciona bajo el ROL DE LA APLICACIÓN, no solo como owner.
//
// El grant de 0024 fue puntual y no hay default privileges: una tabla creada después sin su grant
// pasa todos los tests, pasa `make start` —dev conecta como owner— y devuelve 42501 en el primer
// pedido de producción. Este test es el único lugar donde eso se ve antes de desplegarlo.
func TestElFolioSeReparteBajoElRolDeLaAplicacion(t *testing.T) {
	owner := newTestStore(t)
	app_ := appRoleStore(t)
	ctx := context.Background()

	cajero := makeUser(t, owner, "cajero_rol_app", "cajero")
	prod := makeProduct(t, owner, "Café rol", decimal.RequireFromString("15"), false)
	efectivo := paymentMethodID(t, owner, "Efectivo")
	abrirCajaPrincipal(t, owner, cajero)

	svc := app.NewOrdersService(app_, clock)
	if _, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("15")}},
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42501" {
			t.Fatalf("falta el grant de folio_counters al rol de la aplicación: %v", err)
		}
		t.Fatalf("cobrar con el rol de la app: %v", err)
	}
}

// El esquema impide que el contador de una empresa cuelgue del turno de otra.
//
// Los chequeos de integridad referencial de Postgres SALTAN RLS, así que una llave simple dejaría
// pasar el cruce a cualquier escritura que corra como owner — un data-fix, o el propio backfill de
// la migración. Es el mismo hueco que cerró 0041.
func TestElEsquemaRechazaUnContadorDeFolioQueCruzaEmpresas(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, owner, "otra-folio-counter")
	cajero := makeUser(t, owner, "cajero_cruce_folio", "cajero")
	sess := abrirCajaPrincipal(t, owner, cajero) // turno de la empresa por default

	_, err := owner.Pool.Exec(ctx,
		`insert into folio_counters (company_id, register_session_id, last_number) values ($1, $2, 1)`,
		otra, sess)
	if err == nil {
		t.Fatal("se pudo colgar el contador de la empresa nueva del turno de otra: la llave compuesta no está")
	}
	if !esViolacionDeLlave(err) {
		t.Fatalf("el rechazo tiene que venir de la llave foránea (23503) y vino de otra cosa: %v", err)
	}
}

// Cerrar y reabrir la caja el mismo día renumera desde 1, y eso es SEGURO por una razón concreta:
// cerrar exige que no queden pedidos vivos.
//
// Es la premisa sobre la que descansa toda la decisión de numerar por turno. Si alguien afloja la
// regla del cierre, el reinicio del folio deja de ser inofensivo y pasa a ser una colisión entre
// pedidos que están a la vez en la barra. Por eso se prueba aquí y no solo donde vive la regla.
func TestReabrirLaCajaElMismoDiaRenumeraSinColisionar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_reabre", "cajero")
	prod := makeProduct(t, st, "Café reabre", decimal.RequireFromString("30"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	back := app.NewBackofficeService(st, clock)
	svc := app.NewOrdersService(st, clock)
	principal := registerID(t, st, "Caja principal")

	if _, err := back.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("abrir el primer turno: %v", err)
	}
	primera, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("30")}},
	})
	if err != nil {
		t.Fatalf("primera venta: %v", err)
	}

	// LA PREMISA, probada aquí mismo: con el pedido vivo, la caja NO cierra. Si esto dejara de
	// fallar, el reinicio de folio de abajo pasaría de inofensivo a colisión.
	declarado := map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("30")}
	if _, err := back.CloseSession(ctx, principal, cajero, declarado, ""); !errors.Is(err, domain.ErrOpenOrders) {
		t.Fatalf("la caja cerró con un pedido vivo (o falló por otra cosa): %v", err)
	}

	entregarPendientes(t, st)
	if _, err := back.CloseSession(ctx, principal, cajero, declarado, ""); err != nil {
		t.Fatalf("cerrar el turno ya sin pendientes: %v", err)
	}
	if _, err := back.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("reabrir el mismo día: %v", err)
	}

	segunda, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("30")}},
	})
	if err != nil {
		t.Fatalf("venta del turno nuevo: %v", err)
	}

	if segunda.Number != 1 {
		t.Errorf("el turno nuevo arrancó en el folio %d y debía arrancar en 1: el contador no es "+
			"por turno", segunda.Number)
	}
	// Y los dos #1 del mismo día conviven en la base, que es lo que el índice viejo prohibía.
	if primera.Number != segunda.Number {
		t.Logf("folios %d y %d: el primer turno alcanzó a repartir más de uno", primera.Number, segunda.Number)
	}
	var vivos int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from orders where daily_number = $1 and status in ('abierta','lista')`,
		segunda.Number).Scan(&vivos); err != nil {
		t.Fatalf("contar vivos: %v", err)
	}
	if vivos > 1 {
		t.Errorf("hay %d pedidos VIVOS con el folio %d: el reinicio sí colisiona y la premisa del "+
			"cierre sin pendientes no se está cumpliendo", vivos, segunda.Number)
	}
}

// abrirCajaEn abre la caja principal con una fecha de apertura dada, para poder construir el turno
// viejo que el defecto necesitaba.
func abrirCajaEn(t *testing.T, st *store.Store, por int64, cuando time.Time) int64 {
	t.Helper()
	var regID int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from cash_registers where is_primary and is_active limit 1`).Scan(&regID); err != nil {
		t.Fatalf("caja principal: %v", err)
	}
	var sessID int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id, opened_at)
		 values ($3::date, 0, $1, $2, $4::timestamptz) returning id`, por, regID, cuando, cuando).Scan(&sessID); err != nil {
		t.Fatalf("abrir la caja en %s: %v", cuando.Format("2006-01-02"), err)
	}
	return sessID
}

// El contador de una empresa no se ve desde otra.
//
// RLS no aplica al owner, así que una fuga entre empresas no se ve hasta que hay un segundo
// cliente. Se prueba con el rol de la aplicación, que es el único que la sufre.
func TestElContadorDeFolioNoSeVeDesdeOtraEmpresa(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	cajero := makeUser(t, owner, "cajero_rls_folio", "cajero")
	prod := makeProduct(t, owner, "Café rls", decimal.RequireFromString("10"), false)
	efectivo := paymentMethodID(t, owner, "Efectivo")
	abrirCajaPrincipal(t, owner, cajero)
	svc := app.NewOrdersService(owner, clock)
	if _, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("10")}},
	}); err != nil {
		t.Fatalf("sembrar la venta de la empresa 1: %v", err)
	}

	// Un contador de OTRA empresa, colgado de un turno suyo, metido como owner (que salta RLS).
	otra := makeCompany(t, owner, "otra-rls-folio")
	otroCajero := makeUserIn(t, owner, otra, "cajero_otra_rls", "cajero")
	// La empresa nueva nace sin cajas: se le crea la suya antes del turno.
	var otraCaja int64
	if err := owner.Pool.QueryRow(ctx, `
		insert into cash_registers (company_id, name, is_primary, is_active)
		values ($1, 'Caja principal', true, true) returning id`, otra).Scan(&otraCaja); err != nil {
		t.Fatalf("caja de la otra empresa: %v", err)
	}
	var otroTurno int64
	if err := owner.Pool.QueryRow(ctx, `
		insert into register_sessions (company_id, business_date, opening_cash, opened_by, register_id)
		values ($1, $2::date, 0, $3, $4) returning id`,
		otra, fixedNow, otroCajero, otraCaja).Scan(&otroTurno); err != nil {
		t.Fatalf("turno de la otra empresa: %v", err)
	}
	if _, err := owner.Pool.Exec(ctx,
		`insert into folio_counters (company_id, register_session_id, last_number) values ($1, $2, 99)`,
		otra, otroTurno); err != nil {
		t.Fatalf("contador de la otra empresa: %v", err)
	}

	var ajenos int
	if err := appSt.Pool.QueryRow(ctx,
		`select count(*) from folio_counters where company_id <> $1`, defaultCompanyID).Scan(&ajenos); err != nil {
		t.Fatalf("leer contadores con el rol de la app: %v", err)
	}
	if ajenos != 0 {
		t.Errorf("el rol de la aplicación ve %d contadores de otras empresas: la política de RLS no "+
			"está aislando folio_counters", ajenos)
	}
}

// La migración 0061 se puede revertir y volver a aplicar.
//
// Un `Down` que nadie corrió es una promesa. Este lo corre: tras revertir, la tabla no está y las
// restricciones viejas vuelven; tras reaplicar, la tabla vuelve sembrada desde los pedidos.
func TestLaMigracionDelFolioSeRevierteYSeReaplica(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_reversible", "cajero")
	prod := makeProduct(t, st, "Café reversible", decimal.RequireFromString("10"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	sess := abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)
	var ultimo int
	for range 3 {
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("10")}},
		})
		if err != nil {
			t.Fatalf("sembrar: %v", err)
		}
		ultimo = o.Number
	}

	if _, err := st.Pool.Exec(ctx, downDeLaMigracion(t, "0061_folio_por_turno.sql")); err != nil {
		t.Fatalf("revertir 0061: %v", err)
	}
	var existe bool
	if err := st.Pool.QueryRow(ctx,
		`select exists(select 1 from pg_tables where tablename = 'folio_counters')`).Scan(&existe); err != nil {
		t.Fatalf("consultar la tabla: %v", err)
	}
	if existe {
		t.Error("tras revertir, folio_counters sigue ahí: el Down no la borra")
	}

	if _, err := st.Pool.Exec(ctx, sqlDeLaMigracion(t, "0061_folio_por_turno.sql")); err != nil {
		t.Fatalf("reaplicar 0061: %v", err)
	}
	var sembrado int
	if err := st.Pool.QueryRow(ctx,
		`select last_number from folio_counters where register_session_id = $1`, sess).Scan(&sembrado); err != nil {
		t.Fatalf("leer el contador reaplicado: %v", err)
	}
	if sembrado != ultimo {
		t.Errorf("tras reaplicar, el contador quedó en %d y el último folio repartido fue %d: la "+
			"semilla no reconstruye el estado", sembrado, ultimo)
	}
}

// El NOMBRE que se canta también se reparte por turno, no por fecha.
//
// Si el número se mueve al turno y el nombre se queda colgado de la fecha, los dos caminos quedan a
// medio separar: qué nombre sale dependería de cuál de las dos cosas cambió primero. Lo que tiene
// que sostenerse es que dos pedidos VIVOS del mismo turno nunca comparten nombre.
func TestDosPedidosVivosDelMismoTurnoNoCompartenNombre(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_nombres", "cajero")
	prod := makeProduct(t, st, "Café nombres", decimal.RequireFromString("10"), false)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	vistos := map[string]bool{}
	for i := range 6 {
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		})
		if err != nil {
			t.Fatalf("crear el pedido %d: %v", i, err)
		}
		if o.FolioName == "" {
			t.Fatalf("el pedido %d salió sin nombre: no hay con qué cantarlo", i)
		}
		if vistos[o.FolioName] {
			t.Errorf("el nombre %q se repartió dos veces entre pedidos vivos del mismo turno: el "+
				"nombre dejó de seguir al turno", o.FolioName)
		}
		vistos[o.FolioName] = true
	}
}

// Una zona guardada que dejó de ser válida no archiva la venta en UTC ni impide cobrar.
//
// Caer a UTC corre la fecha seis horas y se ve plausible: es el peor modo de fallo posible, porque
// nadie lo audita. Y fallar tampoco es opción — esta función está en el camino de un cobro.
func TestConZonaInvalidaLaVentaCaeAlDefaultDelProductoYSeCobra(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.Pool.Exec(ctx,
		`update business_settings set timezone = 'Marte/Olympus_Mons' where company_id = $1`,
		defaultCompanyID); err != nil {
		t.Fatalf("dejar una zona inválida: %v", err)
	}

	cajero := makeUser(t, st, "cajero_zona_rota", "cajero")
	prod := makeProduct(t, st, "Café zona rota", decimal.RequireFromString("10"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	svc := app.NewOrdersService(st, clock)
	o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("10")}},
	})
	if err != nil {
		t.Fatalf("con la zona rota la venta no se pudo cobrar, y cobrar es lo que no puede fallar: %v", err)
	}

	var archivada time.Time
	if err := st.Pool.QueryRow(ctx,
		`select business_date from orders where id = $1`, o.ID).Scan(&archivada); err != nil {
		t.Fatalf("leer la fecha: %v", err)
	}
	quiere := domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone))
	if !archivada.Equal(quiere) {
		t.Errorf("con la zona inválida la venta quedó el %s y debía quedar el %s (el default del "+
			"producto, nunca UTC)", archivada.Format("2006-01-02"), quiere.Format("2006-01-02"))
	}
}

// UN TURNO CON MÁS PEDIDOS QUE NOMBRES TIENE LA LISTA SIGUE VENDIENDO.
//
// Es el borde que el ambiente de pruebas ya tiene encima: su turno abierto lleva 158 pedidos y la
// lista por defecto tiene 88 nombres. Antes el alcance de la unicidad era el DÍA y el índice único
// también; ahora los dos son el TURNO, así que la frontera se cruza en el mismo punto — pero el
// índice nuevo la vigila, y un choque que el servicio no resuelva deja de ser un nombre repetido
// para convertirse en una venta que no se puede cobrar.
//
// Lo que se prueba es justo eso: que agotar la bolsa NO tumbe la venta.
func TestUnTurnoLargoAgotaLaBolsaYAunAsiSeSigueCobrando(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_bolsa_agotada", "cajero")
	prod := makeProduct(t, st, "Café de turno largo", decimal.RequireFromString("10"), false)
	abrirCajaPrincipal(t, st, cajero)
	svc := app.NewOrdersService(st, clock)

	// Más que los 88 de la lista por defecto, para pasarse de la vuelta completa.
	const pedidos = 95
	nombres := map[string]int{}
	for i := range pedidos {
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		})
		if err != nil {
			t.Fatalf("el pedido %d del turno no se pudo crear: agotar la bolsa de nombres dejó al "+
				"sistema sin poder vender — %v", i+1, err)
		}
		if o.FolioName == "" {
			t.Fatalf("el pedido %d salió sin nombre: no hay con qué cantarlo", i+1)
		}
		nombres[o.FolioName]++
	}

	// Y ninguno se repitió dentro del turno: es lo que el índice único garantiza, y lo que el
	// servicio tiene que respetar dándoles la vuelta.
	for n, veces := range nombres {
		if veces > 1 {
			t.Errorf("el nombre %q se repartió %d veces en el mismo turno", n, veces)
		}
	}
	if len(nombres) != pedidos {
		t.Errorf("%d pedidos repartieron %d nombres distintos", pedidos, len(nombres))
	}
}
