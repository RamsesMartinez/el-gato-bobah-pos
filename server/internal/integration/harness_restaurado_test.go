//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/migrations"
)

// La base RESTAURADA de un respaldo real de producción, anonimizado.
//
// POR QUÉ EXISTE, aparte de `newTestStore`. La constitución exige que una migración se pruebe
// "contra una base restaurada de un respaldo real y con al menos dos empresas", y `newTestStore`
// hace justo lo contrario: `drop schema public cascade` y migrar desde cero. Con una base sembrada
// limpia, todo camino "por cada otra empresa" es un no-op, y las formas que de verdad muerden —el
// pedido viejo sin nombre de folio, el que quedó sin sesión de caja, los 158 archivados con la
// fecha equivocada— sencillamente no existen.
//
// Es una URL SEPARADA y no la de siempre porque las dos no pueden convivir: el primer test que
// llamara a `newTestStore` sobre la base restaurada la borraría entera.
//
// Se prepara con `make respaldo-anonimo`. Sin la variable, los tests que la usan se omiten — igual
// que hace `testURL` con los de integración. Cada prueba trabaja sobre una copia (restoredURL).

// restoredSourceURL es la base restaurada misma. Nadie prueba contra ella: es la plantilla de la
// copia de cada prueba (ver restoredURL).
func restoredSourceURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_RESTORED_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_RESTORED_DATABASE_URL no definido; corre `make respaldo-anonimo` para prepararla")
	}
	return u
}

func restoredSourceName(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(restoredSourceURL(t))
	if err != nil {
		t.Fatalf("URL de la base restaurada ilegible: %v", err)
	}
	return strings.TrimPrefix(u.Path, "/")
}

var (
	restoredAdminOnce sync.Once
	restoredAdminPool *store.Store

	restoredCopiesMu sync.Mutex
	restoredCopies   = map[string]*restoredCopy{}
	restoredCopyN    atomic.Int64
)

// restoredCopy es la copia de una prueba raíz y cuántos de sus `t` (ella o sus subpruebas) la usan.
// Se suelta cuando el último termina: si la soltara el `t` que la pidió primero y ése fuera una
// subprueba, la raíz se quedaría usando una base borrada o recibiría otra sin lo que ya sembró.
type restoredCopy struct {
	url   string
	users int
}

// restoredURL devuelve la COPIA de la base restaurada que le toca a esta prueba, y la crea la
// primera vez que la prueba la pide.
//
// Por qué una copia por prueba. Bajar un respaldo real limpia de la copia lo que los Down protegen
// (clearWhatTheDownGuards), y una prueba que falla a media bajada deja el esquema en otra versión.
// Sobre la base restaurada misma, las dos cosas las heredaba la prueba siguiente y la corrida de
// mañana: una falla en la 0099 dejaba a todas las demás empezando en la 0099, y el respaldo dejaba
// de ser real para siempre. `create database … template` copia los archivos (~25 MB, menos de un
// segundo) y deja la restaurada intacta.
func restoredURL(t *testing.T) string {
	t.Helper()
	source := restoredSourceURL(t)
	root := pruebaRaiz(t.Name())
	restoredCopiesMu.Lock()
	defer restoredCopiesMu.Unlock()
	c, ok := restoredCopies[root]
	if !ok {
		c = &restoredCopy{url: newRestoredCopy(t, source)}
		restoredCopies[root] = c
	}
	c.users++
	// Se registra ANTES del store que la usa: los cleanups corren al revés, así que la cuenta baja
	// —y la copia se suelta— cuando ese store ya se cerró.
	t.Cleanup(func() {
		restoredCopiesMu.Lock()
		defer restoredCopiesMu.Unlock()
		c.users--
		if c.users > 0 {
			return
		}
		delete(restoredCopies, root)
		_, _ = restoredAdmin(t, source).Pool.Exec(context.Background(),
			"drop database if exists "+strings.TrimPrefix(mustPath(t, c.url), "/")+" with (force)")
	})
	return c.url
}

func mustPath(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("URL ilegible: %v", err)
	}
	return u.Path
}

// newRestoredCopy copia la base restaurada en una base nueva y devuelve su URL.
func newRestoredCopy(t *testing.T, source string) string {
	t.Helper()
	adm := restoredAdmin(t, source)
	ctx := context.Background()
	sourceName := restoredSourceName(t)
	name := restoredCopyPrefix + itoa(int(restoredCopyN.Add(1)))
	if _, err := adm.Pool.Exec(ctx, "drop database if exists "+name+" with (force)"); err != nil {
		t.Fatalf("soltar la copia previa: %v", err)
	}
	if _, err := adm.Pool.Exec(ctx, "create database "+name+" template "+sourceName); err != nil {
		t.Fatalf("copiar la base restaurada %s (¿queda alguien conectado a ella?): %v", sourceName, err)
	}
	// Ni el `grant connect` ni los ajustes de la base viajan con la copia. El ajuste que importa es
	// app.company_id: con él los inserts del owner se sellan con una empresa (respaldo-anonimo.sh).
	if _, err := adm.Pool.Exec(ctx, "grant connect on database "+name+" to gatobobah_app"); err != nil {
		t.Fatalf("dar connect sobre la copia: %v", err)
	}
	rows, err := adm.Pool.Query(ctx, `
		select format('alter database %I set %I = %L', $2::text, split_part(c, '=', 1), substr(c, strpos(c, '=') + 1))
		  from pg_db_role_setting s
		  join pg_database d on d.oid = s.setdatabase
		  cross join unnest(s.setconfig) c
		 where d.datname = $1 and s.setrole = 0`, sourceName, name)
	if err != nil {
		t.Fatalf("leer los ajustes de la base restaurada: %v", err)
	}
	var settings []string
	for rows.Next() {
		var stmt string
		if err := rows.Scan(&stmt); err != nil {
			t.Fatal(err)
		}
		settings = append(settings, stmt)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range settings {
		if _, err := adm.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("copiar el ajuste (%s): %v", stmt, err)
		}
	}
	return conBase(source, name)
}

// restoredCopyPrefix nombra las copias, para reconocer las que dejó una corrida que murió antes de
// soltarlas (restoredAdmin las suelta al empezar).
const restoredCopyPrefix = "egb_restaurada_"

// restoredAdmin habla con el servidor de la base restaurada por la base `postgres`: crear y soltar
// la copia exige estar conectado a otra base, y a la restaurada no, porque `template` se niega si
// queda una sola sesión abierta contra ella.
func restoredAdmin(t *testing.T, source string) *store.Store {
	t.Helper()
	restoredAdminOnce.Do(func() {
		ctx := context.Background()
		st, err := store.New(ctx, conBase(source, "postgres"))
		if err != nil {
			return
		}
		// Las copias que dejó una corrida que murió antes de su cleanup: nadie más las usa.
		var leftovers []string
		rows, err := st.Pool.Query(ctx, `select datname from pg_database where datname like $1`, restoredCopyPrefix+"%")
		if err == nil {
			for rows.Next() {
				var name string
				if rows.Scan(&name) == nil {
					leftovers = append(leftovers, name)
				}
			}
			rows.Close()
		}
		for _, name := range leftovers {
			_, _ = st.Pool.Exec(ctx, "drop database if exists "+pgx.Identifier{name}.Sanitize()+" with (force)")
		}
		restoredAdminPool = st
	})
	if restoredAdminPool == nil {
		t.Fatalf("no se pudo abrir la conexión de administración del servidor de %s", restoredSourceName(t))
	}
	return restoredAdminPool
}

// restoredStore conecta como OWNER y **no toca el esquema**: ni lo borra ni lo migra. Quien
// necesite migrar lo hace explícito con migrarArriba, porque en un test de migración el momento en
// que se aplica ES lo que se está probando.
func restoredStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(context.Background(), restoredURL(t))
	if err != nil {
		t.Fatalf("store.New sobre la base restaurada: %v", err)
	}
	t.Cleanup(st.Close)
	exigeDosEmpresas(t, st)
	return st
}

// restoredAppRoleStore conecta como gatobobah_app (no-superusuario) → RLS y los grants SÍ aplican.
// Es el único ángulo desde el que se ve un grant faltante: como owner, la consulta pasa igual de
// verde con el grant borrado.
func restoredAppRoleStore(t *testing.T) *store.Store {
	t.Helper()
	u, err := url.Parse(restoredURL(t))
	if err != nil {
		t.Fatalf("URL de la base restaurada ilegible: %v", err)
	}
	u.User = url.UserPassword("gatobobah_app", appRolePassword)
	st, err := store.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("store.New como rol de app sobre la base restaurada: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// exigeDosEmpresas falla ruidoso si el respaldo llegó con una sola. No es paranoia: con una
// empresa, un test de aislamiento entre empresas pasa en verde sin haber probado nada, que es
// exactamente el modo de falla que esta base viene a cerrar.
func exigeDosEmpresas(t *testing.T, st *store.Store) {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from companies`).Scan(&n); err != nil {
		t.Fatalf("contar empresas de la base restaurada: %v", err)
	}
	if n < 2 {
		t.Fatalf("la base restaurada tiene %d empresa(s) y hacen falta al menos 2: con una sola, "+
			"todo camino \"por cada otra empresa\" es un no-op y la migración pasa verde para romper "+
			"en producción. Vuelve a correr `make respaldo-anonimo`", n)
	}
}

// migrarArriba sube la base restaurada hasta la última migración; migrarAbajoHasta la baja.
//
// goose es global (SetBaseFS/SetDialect son estado de paquete), así que estas dos son el único
// lugar del test que lo configura; hacerlo en cada caso deja el orden de los tests decidiendo qué
// dialecto quedó puesto.
func migrarArriba(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	db := prepararGoose(t, pool)
	if err := goose.UpContext(context.Background(), db, "."); err != nil {
		t.Fatalf("goose up sobre la base restaurada: %v", err)
	}
}

// migrarAbajoHasta revierte TODO lo que esté por encima de `version`, en orden inverso.
//
// Existe porque un test de `Down` que llama a `migrarAbajo` está diciendo en realidad "revierte la
// última", y eso solo es cierto mientras su migración SEA la última. En cuanto llega la siguiente,
// el test o deshace la migración equivocada o truena por una dependencia que no existía cuando se
// escribió — las dos formas de que un test de reversibilidad deje de probar lo que dice probar.
//
// Con esto, el test de la migración N revierte hasta N-1 sin importar cuántas hayan llegado después,
// y de paso ejercita que las posteriores también se revierten.
//
// Baja UNA a la vez, y antes de cada Down que se niega con datos limpia lo que ese Down protege
// (clearWhatTheDownGuards). Bajar de golpe con DownTo dejaba la primera guarda decidiendo: con un
// respaldo real, la 0099 o la 0088 se negaban y la prueba moría sin llegar a su migración.
func migrarAbajoHasta(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()
	ctx := context.Background()
	db := prepararGoose(t, pool)
	for {
		current, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			t.Fatalf("leer la versión antes de bajar: %v", err)
		}
		if current <= version {
			return
		}
		clearWhatTheDownGuards(t, pool, current)
		if err := goose.DownContext(ctx, db, "."); err != nil {
			t.Fatalf("goose down de la %04d (bajando hasta la %d): %v", current, version, err)
		}
	}
}

// migrarAbajoHastaErr baja hasta `version` y devuelve el error del ÚLTIMO Down en vez de tronar: el
// Down que se niega es lo que se prueba. Todo lo de arriba se baja con su limpieza; si se bajara de
// golpe desde la punta, con un respaldo real la guarda de otra migración (la 0099) se negaba primero
// y la prueba pasaba en verde sin haber llegado a la suya (TestTheRefusalTestsReachTheirOwnDown).
func migrarAbajoHastaErr(t *testing.T, pool *pgxpool.Pool, version int64) error {
	t.Helper()
	migrarAbajoHasta(t, pool, version+1)
	return goose.DownToContext(context.Background(), prepararGoose(t, pool), ".", version)
}

// guardedDownCleanup dice, por migración, cómo dejar una base de PRUEBA sin lo que su Down se niega a
// perder. SOLO EN PRUEBAS, y solo justo antes de ese Down.
//
// En producción esas guardas son la única barrera entre un rollback y un hecho de dinero perdido, y
// tienen razón. En la copia de un respaldo, el hecho no es lo que se prueba: una prueba de la 0076
// que tiene que cruzar la 0088 de bajada se moría porque el negocio ya cobró con terminal. Cada
// renglón borra o pone en su valor neutro lo que nombra la guarda, más lo que cuelga de eso por
// llave foránea (los días de un horario, los renglones de un conteo), en el esquema que hay en ese
// momento. Lo demás llega intacto a la prueba.
//
// Una migración nueva cuyo Down se niegue con datos agrega aquí su limpieza, o su razón en
// guardedDownWithoutCleanup; TestEveryGuardedDownHasItsTestCleanup truena si falta.
var guardedDownCleanup = map[int64][]string{
	103: {`update stock_movements set effective_on = business_day_of(company_id, created_at)
	        where effective_on is not null and effective_on <> business_day_of(company_id, created_at)`},
	102: {`update expense_payments set drawer_extra = 0 where drawer_extra > 0`},
	101: {
		`update expense_items set excluded = false where excluded`,
		`update expenses set document_total = null where document_total is not null`,
	},
	100: {`update expense_payments set include_in_count = false where register_session_id is null and include_in_count`},
	99:  {`update expense_payments set register_id = null where register_id is not null and register_session_id is null`},
	96: {
		`delete from ingredient_cost_corrections`,
		`update stock_movements set replacement_id = null where replacement_id is not null`,
		`delete from ingredient_replacements`,
	},
	94: {
		`update stock_movements set waste_record_id = null where waste_record_id is not null`,
		`delete from waste_records`,
	},
	// La cláusula de las unidades de longitud no se limpia: pasar un insumo medido en metros a otra
	// unidad es inventar su cantidad. Con un respaldo que lo traiga, bajar de la 0092 falla nombrando
	// esa guarda, y debe.
	92: {
		`update stock_movements set expense_item_id = null, stock_count_id = null
		  where expense_item_id is not null or stock_count_id is not null`,
		`delete from stock_count_lines`,
		`delete from stock_counts`,
		`update expense_items set presentation_id = null, applied_qty_base = 0, not_coming_qty = 0, voided_at = null, is_gift = false
		  where presentation_id is not null or applied_qty_base <> 0 or not_coming_qty <> 0 or voided_at is not null or is_gift`,
	},
	90: {
		`delete from register_cash_movements where kind = 'retiro'`,
		`update cash_registers set fixed_float = null where fixed_float is not null`,
		`update register_sessions set fixed_float_at_close = null where fixed_float_at_close is not null`,
	},
	// La bitácora es de solo agregar ni siquiera para el owner: el trigger se apaga alrededor del
	// borrado. Desde la 0089 cualquier pedido deja eventos, también los que siembra una prueba.
	89: {
		`alter table audit_events disable trigger audit_events_append_only`,
		`delete from audit_events`,
		`alter table audit_events enable trigger audit_events_append_only`,
		`delete from attendance_corrections`,
		`delete from attendance_entries`,
		`delete from work_schedule_days`,
		`delete from work_schedule_versions`,
	},
	88: {
		`delete from session_terminal_counts`,
		`update order_payments set card_terminal_id = null, card_terminal_name = null where card_terminal_id is not null`,
		`update order_refunds set card_refund_folio = null, card_refund_captured_by = null where card_refund_folio is not null`,
		`delete from register_cash_movements where kind = 'reverso'`,
		`update register_cash_movements set concept_id = null where concept_id is not null`,
	},
	87: {
		`delete from tip_payout_sources`,
		`delete from tip_carryovers`,
		`delete from register_cash_movements where kind = 'propina'`,
		`update register_sessions set tips_carried_over = 0 where tips_carried_over > 0`,
	},
	82: {
		`delete from order_draft_adds`,
		`delete from order_draft_lines`,
		`delete from order_drafts`,
	},
	80: {
		`update order_refunds set tip_amount = 0 where tip_amount > 0`,
		`delete from order_refunds where amount = 0`,
	},
	79: {
		`delete from order_payment_voids`,
		`delete from order_payment_lines`,
		`delete from order_line_moves`,
		`delete from order_line_move_batches`,
		`update orders set merged_into_order_id = null where merged_into_order_id is not null`,
		`update order_payments set split_part = null, split_of = null, payment_number = null
		  where split_part is not null or payment_number is not null`,
	},
	77: {`delete from platform_item_links where modifier_option_id is not null`},
	73: {`update orders set service_type = 'domicilio' where delivery_platform_id is not null and service_type = 'para_llevar'`},
	// La 0072 no se niega, pero su Down tira quién descontó y su Up no acepta un descuento sin
	// autor: con un respaldo que ya tiene descuentos, bajar y volver a subir truena en la subida. El
	// total del pedido no se toca: sigue neto del descuento que se quitó, y ninguna prueba que baje
	// de la 0072 compara totales contra subtotal menos descuento.
	72: {`update orders set discount_total = 0, discount_set_by = null, discount_set_at = null where discount_total > 0`},
}

// guardedDownWithoutCleanup son los Down que se niegan y NO se limpian, con la razón. Una prueba que
// baje por debajo de ellos con un respaldo que traiga el dato falla nombrando la guarda, y debe.
var guardedDownWithoutCleanup = map[int64]string{
	76: "una empresa con dos sucursales no tiene equivalente de una sola: habría que decidir a cuál " +
		"van sus cajas, pedidos y existencias, y eso es inventar datos, no limpiarlos",
}

// clearWhatTheDownGuards aplica la limpieza de la migración `version`, si tiene, en una transacción.
//
// Las restricciones NOT VALID se apartan mientras tanto y vuelven igual, sin validar: producción
// tiene filas viejas que no las cumplen, y Postgres las vuelve a revisar en CUALQUIER update de la
// fila (el mismo motivo que en respaldo-anonimo.sh).
func clearWhatTheDownGuards(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()
	stmts, ok := guardedDownCleanup[version]
	if !ok {
		return
	}
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		select conrelid::regclass::text, conname, pg_get_constraintdef(oid)
		  from pg_constraint
		 where contype = 'c' and not convalidated and connamespace = 'public'::regnamespace
		   and conrelid <> 0 and conislocal`)
	if err != nil {
		t.Fatalf("leer las restricciones sin validar: %v", err)
	}
	type unvalidated struct{ table, name, def string }
	var aside []unvalidated
	for rows.Next() {
		var u unvalidated
		if err := rows.Scan(&u.table, &u.name, &u.def); err != nil {
			t.Fatal(err)
		}
		aside = append(aside, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, u := range aside {
		if _, err := tx.Exec(ctx, "alter table "+u.table+" drop constraint "+pgx.Identifier{u.name}.Sanitize()); err != nil {
			t.Fatalf("apartar %s.%s: %v", u.table, u.name, err)
		}
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("limpiar lo que protege el Down de la %04d (%s): %v", version, stmt, err)
		}
	}
	// pg_get_constraintdef termina en «NOT VALID»: vuelven sin validar, como estaban.
	for _, u := range aside {
		if _, err := tx.Exec(ctx, "alter table "+u.table+" add constraint "+pgx.Identifier{u.name}.Sanitize()+" "+u.def); err != nil {
			t.Fatalf("reponer %s.%s: %v", u.table, u.name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("confirmar la limpieza de la %04d: %v", version, err)
	}
}

func prepararGoose(t *testing.T, pool *pgxpool.Pool) *sql.DB {
	t.Helper()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose SetDialect: %v", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// conexionDeEmpresa devuelve una conexión con el GUC del tenant fijado EN ESA conexión.
//
// Hace falta porque `AcquireTenant` fija `app.company_id` en la conexión que guarda dentro del
// ctx, y esa conexión solo la usa `store.QC(ctx)`. Una consulta suelta por `st.Pool` toma OTRA
// conexión del pool, que trae el default de la base — así que un test de aislamiento escrito con
// `st.Pool.Query(tenantCtx, …)` no prueba el aislamiento: consulta como la empresa por defecto y
// pasa en verde con la policy borrada.
func conexionDeEmpresa(t *testing.T, st *store.Store, empresa int64) *pgxpool.Conn {
	t.Helper()
	conn, err := st.Pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("tomar conexión para la empresa %d: %v", empresa, err)
	}
	t.Cleanup(conn.Release)
	if _, err := conn.Exec(context.Background(),
		"select set_config('app.company_id', $1, false)", strconv.FormatInt(empresa, 10)); err != nil {
		t.Fatalf("fijar el tenant %d en la conexión: %v", empresa, err)
	}
	return conn
}

// exigeViolacionDeRestriccion falla si `err` es nil O si el error es "no existe esa columna/tabla".
//
// Sin la segunda mitad, un test de restricciones pasa en VERDE mientras la migración no existe: el
// UPDATE truena con 42703 y el test lo lee como "la restricción funcionó". Es la trampa de un test
// que nunca se vio en rojo por la razón correcta.
func exigeViolacionDeRestriccion(t *testing.T, err error, queDeberiaImpedir string) {
	t.Helper()
	if err == nil {
		t.Fatalf("el esquema lo aceptó: %s", queDeberiaImpedir)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42703", "42P01": // columna / tabla inexistente
			t.Fatalf("el esquema ni siquiera tiene la pieza que se está probando (%s): %v — esto NO "+
				"es la restricción funcionando", pgErr.Code, err)
		case "23514", "23505", "23503": // check, unique, foreign key
			return
		}
	}
	t.Fatalf("se rechazó, pero no por una restricción del esquema: %v", err)
}

// versionDeEsquema dice en qué migración quedó la base. Un test que la mueve la deja como la
// encontró, y este es el número con el que se comprueba.
func versionDeEsquema(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var v int64
	if err := st.Pool.QueryRow(context.Background(),
		`select max(version_id) from goose_db_version where is_applied`).Scan(&v); err != nil {
		t.Fatalf("leer la versión del esquema: %v", err)
	}
	return v
}
