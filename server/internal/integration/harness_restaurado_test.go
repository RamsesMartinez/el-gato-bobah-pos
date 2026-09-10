//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strconv"
	"testing"

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
// que hace `testURL` con los de integración.

func restoredURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_RESTORED_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_RESTORED_DATABASE_URL no definido; corre `make respaldo-anonimo` para prepararla")
	}
	return u
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

// migrarArriba y migrarAbajo mueven la base restaurada una migración a la vez.
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
func migrarAbajoHasta(t *testing.T, pool *pgxpool.Pool, version int64) {
	t.Helper()
	db := prepararGoose(t, pool)
	if err := goose.DownToContext(context.Background(), db, ".", version); err != nil {
		t.Fatalf("goose down-to %d: %v", version, err)
	}
}

func migrarAbajo(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	db := prepararGoose(t, pool)
	if err := goose.DownContext(context.Background(), db, "."); err != nil {
		t.Fatalf("goose down sobre la base restaurada: %v", err)
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
