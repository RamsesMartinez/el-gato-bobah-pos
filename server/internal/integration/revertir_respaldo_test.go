//go:build integration

package integration

import (
	"context"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramthedev/el-gato-bobah-pos/server/migrations"
)

// UN RESPALDO REAL SE PUEDE BAJAR HASTA ANTES DE LA PRIMERA MIGRACIÓN QUE SE PRUEBA CONTRA ÉL (#116).
//
// Las pruebas de migración bajan el respaldo hasta la versión anterior a la suya y lo vuelven a
// subir. Con datos de producción eso cruza los Down que se niegan a perder un hecho de dinero o de
// caja: el 2026-10-20 la 0088 se negó por los cobros con terminal, y ocho pruebas de la 0064 a la
// 0082 fallaban sin haber llegado a lo que prueban. El Down tiene razón en producción; en la copia
// de prueba esos hechos se limpian antes (ver clearWhatTheDownGuards).
func TestARealBackupRevertsPastEveryGuardedDown(t *testing.T) {
	st := restoredStore(t)
	start := versionDeEsquema(t, st)

	migrarAbajoHasta(t, st.Pool, 63)
	if v := versionDeEsquema(t, st); v != 63 {
		t.Fatalf("el respaldo quedó en la %d y debía bajar a la 63", v)
	}
	migrarArriba(t, st.Pool)
	if v := versionDeEsquema(t, st); v != start {
		t.Fatalf("tras subir de nuevo quedó en la %d y empezó en la %d", v, start)
	}
}

// UNA PRUEBA NO DEJA EL RESPALDO DISTINTO PARA LA SIGUIENTE. Cada una trabaja sobre su propia
// copia: lo que una baja, limpia o deja a medias al fallar no lo hereda otra, ni la corrida de
// mañana. Las dos subpruebas hacen de dos pruebas: ninguna comparte la copia con la raíz, que no
// pidió ninguna.
func TestEachTestGetsItsOwnCopyOfTheBackup(t *testing.T) {
	var source, firstCopy string
	t.Run("una prueba revierte su copia", func(t *testing.T) {
		st := restoredStore(t)
		firstCopy = currentDatabase(t, st.Pool)
		source = restoredSourceName(t)
		if firstCopy == source {
			t.Fatalf("la prueba trabaja sobre %s, la base restaurada misma, y no sobre una copia", source)
		}
		migrarAbajoHasta(t, st.Pool, 63)
	})
	t.Run("la siguiente recibe la suya, sin lo que bajó la anterior", func(t *testing.T) {
		st := restoredStore(t)
		if name := currentDatabase(t, st.Pool); name == firstCopy || name == source {
			t.Fatalf("la segunda prueba cayó en %s: no es una copia nueva", name)
		}
		if v := versionDeEsquema(t, st); v <= 63 {
			t.Fatalf("la copia nueva llegó en la %d: heredó lo que bajó la prueba anterior", v)
		}
	})
}

// DENTRO DE UNA PRUEBA, TODO APUNTA A LA MISMA COPIA: el owner, el rol de la app y las subpruebas.
// Lo que siembra el owner tiene que verlo la app, y una subprueba que termina no puede soltar la
// copia que la raíz sigue usando.
func TestOneTestSharesItsCopy(t *testing.T) {
	t.Parallel()
	owner := restoredStore(t)
	name := currentDatabase(t, owner.Pool)
	if app := restoredAppRoleStore(t); currentDatabase(t, app.Pool) != name {
		t.Fatalf("el rol de la app cayó en otra copia que el owner (%s)", name)
	}
	t.Run("una subprueba", func(t *testing.T) {
		if sub := restoredStore(t); currentDatabase(t, sub.Pool) != name {
			t.Fatalf("la subprueba cayó en otra copia que su raíz (%s)", name)
		}
	})
	if got := currentDatabase(t, owner.Pool); got != name {
		t.Fatalf("tras la subprueba la raíz quedó en %s y no en %s", got, name)
	}
}

func currentDatabase(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var name string
	if err := pool.QueryRow(context.Background(), `select current_database()`).Scan(&name); err != nil {
		t.Fatalf("la copia ya no responde: %v", err)
	}
	return name
}

// TODO DOWN QUE SE NIEGA CON DATOS TIENE SU LIMPIEZA DE PRUEBA, o una razón escrita para no tenerla.
// Sin esto, la siguiente migración que agregue una guarda vuelve a dejar sin correr, con el primer
// respaldo que traiga ese dato, todas las pruebas que bajan por debajo de ella.
//
// Lo que NO ve: que la limpieza cubra cada cláusula de la guarda (eso solo lo dice un respaldo que
// traiga el dato), ni un Down que no se niega pero pierde algo que su Up no vuelve a aceptar, como
// la 0072. Ese lo encuentra TestARealBackupRevertsPastEveryGuardedDown al volver a subir.
func TestEveryGuardedDownHasItsTestCleanup(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	version := regexp.MustCompile(`^(\d+)_`)
	var missing []string
	for _, f := range files {
		src, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		_, down, ok := strings.Cut(string(src), "+goose Down")
		if !ok || !strings.Contains(down, "raise exception") {
			continue
		}
		m := version.FindStringSubmatch(f)
		if m == nil {
			t.Fatalf("%s no empieza con su número de versión", f)
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		_, cleaned := guardedDownCleanup[v]
		_, excused := guardedDownWithoutCleanup[v]
		if !cleaned && !excused {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("estos Down se niegan con datos y no tienen limpieza en guardedDownCleanup ni razón en "+
			"guardedDownWithoutCleanup; con un respaldo que traiga ese dato, toda prueba que baje por debajo "+
			"de ellos falla antes de probar nada: %s", strings.Join(missing, ", "))
	}
}

// UNA PRUEBA DE «EL DOWN SE NIEGA» SE NIEGA EN SU MIGRACIÓN, NO EN UNA DE ARRIBA. Con un respaldo
// real, bajar de golpe desde la punta se topaba primero con la guarda de otra migración (la 0099),
// y la prueba de la guarda de la 0096 pasaba en verde sin haber llegado a ella.
func TestTheRefusalTestsReachTheirOwnDown(t *testing.T) {
	st := restoredStore(t)
	start := versionDeEsquema(t, st)
	if start <= 96 {
		t.Skipf("el respaldo está en la %d: no hay migraciones arriba de la 0096 que se interpongan", start)
	}
	if _, err := st.Pool.Exec(context.Background(), `
		insert into ingredient_cost_corrections (company_id, ingredient_id, previous_cost, new_cost, user_id)
		select i.company_id, i.id, 0, 1, (select min(u.id) from users u where u.company_id = i.company_id)
		  from ingredients i order by i.id limit 1`); err != nil {
		t.Fatalf("sembrar una corrección de costo: %v", err)
	}
	err := migrarAbajoHastaErr(t, st.Pool, 95)
	if err == nil {
		t.Fatal("el Down de la 0096 corrió con una corrección de costo registrada")
	}
	if !strings.Contains(err.Error(), "0096_") {
		t.Fatalf("el Down se negó, pero no en la 0096: %v", err)
	}
	if v := versionDeEsquema(t, st); v != 96 {
		t.Fatalf("la base quedó en la %d; debía bajar hasta la 96 y negarse ahí", v)
	}
}
