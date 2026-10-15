//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La migración 0089 (asistencia y auditoría), probada ANTES de escribirla.
//
// POR QUÉ ESTA NO EXIGE UN RESPALDO RESTAURADO. La constitución pide probar una migración contra un
// respaldo real con al menos dos empresas porque una migración que RELLENA datos existentes pasa
// verde con una sola empresa y rompe en producción. La 0089 no rellena nada: crea cinco tablas
// vacías y agrega columnas con default constante. Es el mismo criterio de la 0066
// (migracion_conteo_de_efectivo_test.go). Si algún día esta migración gana un `update` sobre filas
// existentes, la excepción deja de valer y el test pasa a correr contra un respaldo.
//
// Lo que SÍ necesita dos empresas es el aislamiento: se crean aquí con `makeCompany`.

var tables0089 = []string{"attendance_entries", "attendance_corrections", "work_schedule_versions", "work_schedule_days", "audit_events"}

// openEntry inserta, como owner, una entrada abierta de `user` en `company` y devuelve su id.
func openEntry(t *testing.T, st *store.Store, company, user int64) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into attendance_entries (company_id, user_id, business_date, clock_in_at)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06') returning id`, company, user).Scan(&id); err != nil {
		t.Fatalf("insertar entrada abierta: %v", err)
	}
	return id
}

func TestMigration0089CreatesTablesWithRLSAndGrants(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	for _, table := range tables0089 {
		var rls, sel, ins, upd, del bool
		if err := st.Pool.QueryRow(ctx, `
			select c.relrowsecurity,
			       has_table_privilege('gatobobah_app', c.oid, 'select'),
			       has_table_privilege('gatobobah_app', c.oid, 'insert'),
			       has_table_privilege('gatobobah_app', c.oid, 'update'),
			       has_table_privilege('gatobobah_app', c.oid, 'delete')
			  from pg_class c where c.relname = $1 and c.relnamespace = 'public'::regnamespace`, table).
			Scan(&rls, &sel, &ins, &upd, &del); err != nil {
			t.Fatalf("%s no existe: %v", table, err)
		}
		if !rls || !sel || !ins {
			t.Errorf("%s: rls=%v select=%v insert=%v", table, rls, sel, ins)
		}
		if del {
			t.Errorf("%s: gatobobah_app puede borrar; nada de asistencia ni de auditoría se borra", table)
		}
		// Solo las entradas se actualizan, y por columna (siguiente test). Una corrección, un horario
		// o un evento no se editan: el valor original es la evidencia.
		if upd && table != "attendance_entries" {
			t.Errorf("%s: gatobobah_app puede actualizar; debería ser solo insertar", table)
		}
	}
}

func TestTheAppRoleCannotRewriteWhoOrHowAnEntryWasMade(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "asis-cols")
	ana := makeUserIn(t, st, co, "ana-cols", "cajero")
	beto := makeUserIn(t, st, co, "beto-cols", "cajero")
	id := openEntry(t, st, co, ana)

	conn := conexionDeEmpresa(t, appRoleStore(t), co)
	for _, stmt := range []string{
		"update attendance_entries set user_id = $2 where id = $1",
		"update attendance_entries set source = 'manual' where id = $1 and $2::bigint is not null",
		"update attendance_entries set created_by = $2 where id = $1",
	} {
		_, err := conn.Exec(context.Background(), stmt, id, beto)
		requirePgCode(t, err, "42501", stmt)
	}
}

func TestAnEntryCannotStoreAnImpossibleState(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "asis-estado")
	ana := makeUserIn(t, st, co, "ana-estado", "cajero")

	_, err := st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at, clock_out_at, status)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06', '2026-10-10 17:00-06', 'missing_out')`, co, ana)
	exigeViolacionDeRestriccion(t, err, "una entrada «sin salida» que sí tiene salida")

	_, err = st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at, clock_out_at, status)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06', '2026-10-10 09:00-06', 'closed')`, co, ana)
	exigeViolacionDeRestriccion(t, err, "una salida que no es posterior a la entrada")

	_, err = st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at, status)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06', 'closed')`, co, ana)
	exigeViolacionDeRestriccion(t, err, "una entrada cerrada sin salida")

	_, err = st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at, in_device_key)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06', 'NO-ES-HEX')`, co, ana)
	exigeViolacionDeRestriccion(t, err, "una clave de aparato malformada")

	openEntry(t, st, co, ana)
	_, err = st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at)
		values ($1, $2, '2026-10-10', '2026-10-10 09:05-06')`, co, ana)
	exigeViolacionDeRestriccion(t, err, "dos entradas abiertas de la misma persona")
}

// LA HORA NO SE REESCRIBE SIN SU CORRECCIÓN. Registrar la salida (de nada a un valor) es normal;
// mover una hora que ya estaba exige que en la MISMA transacción se haya guardado la fila de
// corrección con el valor original. Marcar `corrected = true` no basta: con eso solo, cualquier
// camino reescribía la hora sin conservar la anterior.
func TestARecordedTimeOnlyChangesAsACorrection(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "asis-hora")
	ana := makeUserIn(t, st, co, "ana-hora", "cajero")
	id := openEntry(t, st, co, ana)

	if _, err := st.Pool.Exec(ctx, `update attendance_entries set clock_out_at = '2026-10-10 17:00-06', status = 'closed' where id = $1`, id); err != nil {
		t.Fatalf("registrar la salida debe poderse: %v", err)
	}
	_, err := st.Pool.Exec(ctx, `update attendance_entries set clock_in_at = '2026-10-10 08:00-06', corrected = true where id = $1`, id)
	exigeViolacionDeRestriccion(t, err, "mover la entrada sin guardar su corrección")

	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `insert into attendance_corrections (company_id, entry_id, field, before, after, reason, corrected_by)
		values ($1, $2, 'clock_out_at', '2026-10-10 17:00-06', '2026-10-10 18:00-06', 'Olvidó checar', $3)`, co, id, ana); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update attendance_entries set clock_out_at = '2026-10-10 18:00-06', corrected = true where id = $1`, id); err != nil {
		t.Fatalf("una corrección con su fila debe poderse: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// Un turno cerrado no se reabre ni se vuelve «sin salida»: reabrirlo choca con el único abierto o
// revive un turno viejo sin rastro.
func TestAClosedEntryIsNeverReopened(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "asis-reabrir")
	ana := makeUserIn(t, st, co, "ana-reabrir", "cajero")
	id := openEntry(t, st, co, ana)
	if _, err := st.Pool.Exec(ctx, `update attendance_entries set clock_out_at = '2026-10-10 17:00-06', status = 'closed' where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	_, err := st.Pool.Exec(ctx, `update attendance_entries set clock_out_at = null, status = 'open' where id = $1`, id)
	exigeViolacionDeRestriccion(t, err, "reabrir un turno cerrado")
	otherUser := openEntry(t, st, co, makeUserIn(t, st, co, "beto-reabrir", "cajero"))
	if _, err := st.Pool.Exec(ctx, `update attendance_entries set status = 'missing_out' where id = $1`, otherUser); err != nil {
		t.Fatalf("dejar una abierta sin salida debe poderse: %v", err)
	}
	_, err = st.Pool.Exec(ctx, `update attendance_entries set status = 'open' where id = $1`, otherUser)
	exigeViolacionDeRestriccion(t, err, "reabrir un turno que quedó sin salida")
}

func TestAScheduleCannotStoreImpossibleDays(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "asis-horario")
	ana := makeUserIn(t, st, co, "ana-horario", "cajero")
	var v int64
	if err := st.Pool.QueryRow(ctx, `insert into work_schedule_versions (company_id, user_id, effective_from, created_by)
		values ($1, $2, '2026-10-13', $2) returning id`, co, ana).Scan(&v); err != nil {
		t.Fatalf("versión: %v", err)
	}
	_, err := st.Pool.Exec(ctx, `insert into work_schedule_days (company_id, version_id, weekday, start_time, end_time) values ($1, $2, 7, '09:00', '17:00')`, co, v)
	exigeViolacionDeRestriccion(t, err, "un día de la semana 7")
	_, err = st.Pool.Exec(ctx, `insert into work_schedule_days (company_id, version_id, weekday, start_time, end_time) values ($1, $2, 1, '09:00', '09:00')`, co, v)
	exigeViolacionDeRestriccion(t, err, "un turno que empieza y termina a la misma hora")
	// Un turno que cruza la medianoche es válido: termina al día siguiente.
	if _, err := st.Pool.Exec(ctx, `insert into work_schedule_days (company_id, version_id, weekday, shift_no, start_time, end_time) values ($1, $2, 4, 2, '22:00', '06:00')`, co, v); err != nil {
		t.Fatalf("un turno nocturno debe guardarse: %v", err)
	}
	_, err = st.Pool.Exec(ctx, `insert into work_schedule_days (company_id, version_id, weekday, shift_no, start_time, end_time) values ($1, $2, 4, 2, '10:00', '11:00')`, co, v)
	exigeViolacionDeRestriccion(t, err, "dos turnos con el mismo número el mismo día")
}

// UNA FILA NO APUNTA A LA PERSONA DE OTRA EMPRESA. Los chequeos de FK saltan RLS: sin la llave
// compuesta, una entrada de A podría colgar de un usuario de B.
func TestAttendanceRowsDoNotCrossCompanies(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "asis-cruce-a")
	b := makeCompany(t, st, "asis-cruce-b")
	fromB := makeUserIn(t, st, b, "user-cruce-b", "cajero")
	_, err := st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06')`, a, fromB)
	exigeViolacionDeRestriccion(t, err, "una entrada de A colgada de un usuario de B")
	_, err = st.Pool.Exec(ctx, `insert into audit_events (company_id, actor_kind, actor_user_id, action, entity_type, entity_id)
		values ($1, 'user', $2, 'order.created', 'order', 1)`, a, fromB)
	exigeViolacionDeRestriccion(t, err, "un evento de A firmado por un usuario de B")
}

func TestMigration0089DownRefusesWithDataAndLeavesNothingWithout(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	co := makeCompany(t, st, "asis-down")
	ana := makeUserIn(t, st, co, "ana-down", "cajero")
	if _, err := st.Pool.Exec(ctx, `insert into audit_events (company_id, actor_kind, actor_user_id, action, entity_type, entity_id)
		values ($1, 'user', $2, 'order.created', 'order', 1)`, co, ana); err != nil {
		t.Fatalf("evento: %v", err)
	}
	db := prepararGoose(t, st.Pool)
	if err := downTo(db, 88); err == nil {
		t.Fatal("el Down borró la bitácora de auditoría, que es la única copia de esos hechos")
	}

	// Sin filas: la limpieza la hace el cascade de la empresa, que también prueba que el trigger de
	// solo agregar deja pasar el borrado de una empresa entera.
	if _, err := st.Pool.Exec(ctx, `delete from companies where id = $1`, co); err != nil {
		t.Fatalf("borrar la empresa con eventos: %v", err)
	}
	migrarAbajoHasta(t, st.Pool, 88)
	for _, table := range tables0089 {
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass($1) is not null`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("el Down dejó %s en pie", table)
		}
	}
	var fn bool
	if err := st.Pool.QueryRow(ctx, `select exists (select 1 from pg_proc where proname like 'audit_events_%' or proname like 'attendance_entries_%')`).Scan(&fn); err != nil {
		t.Fatal(err)
	}
	if fn {
		t.Error("el Down dejó funciones de trigger: un Up posterior chocaría con ellas")
	}
}

// requirePgCode falla si `err` no es el error de Postgres `code`.
func requirePgCode(t *testing.T, err error, code, what string) {
	t.Helper()
	if got := pgCode(err); got != code {
		t.Fatalf("%s: se esperaba el error %s y llegó %q (%v)", what, code, got, err)
	}
}

// downTo es migrarAbajoHasta sin el Fatal: para comprobar que un Down se NIEGA.
func downTo(db *sql.DB, version int64) error {
	return goose.DownToContext(context.Background(), db, ".", version)
}
