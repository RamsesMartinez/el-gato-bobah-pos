//go:build integration

package integration

import (
	"context"
	"testing"
)

// LA BITÁCORA DE AUDITORÍA ES DE SOLO AGREGAR. El rol de la app no tiene `update` ni `delete`, y un
// trigger cubre al owner: un evento que se puede editar no prueba nada. La única salida es borrar la
// empresa entera (dar de baja un cliente, limpiar pruebas), y esa no puede quedar bloqueada.
func TestTheAuditLogCannotBeEditedOrDeleted(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "audit-solo-agregar")
	ana := makeUserIn(t, st, co, "ana-audit", "gerente")
	var id int64
	if err := st.Pool.QueryRow(ctx, `insert into audit_events (company_id, actor_kind, actor_user_id, action, entity_type, entity_id)
		values ($1, 'user', $2, 'price.set', 'product', 7) returning id`, co, ana).Scan(&id); err != nil {
		t.Fatalf("evento: %v", err)
	}

	conn := conexionDeEmpresa(t, appRoleStore(t), co)
	_, err := conn.Exec(ctx, `update audit_events set action = 'order.created' where id = $1`, id)
	requirePgCode(t, err, "42501", "editar un evento como gatobobah_app")
	_, err = conn.Exec(ctx, `delete from audit_events where id = $1`, id)
	requirePgCode(t, err, "42501", "borrar un evento como gatobobah_app")

	if _, err := st.Pool.Exec(ctx, `update audit_events set action = 'order.created' where id = $1`, id); err == nil {
		t.Error("el owner editó un evento")
	}
	if _, err := st.Pool.Exec(ctx, `delete from audit_events where id = $1`, id); err == nil {
		t.Error("el owner borró un evento suelto")
	}
	if _, err := st.Pool.Exec(ctx, `truncate audit_events`); err == nil {
		t.Error("el owner vació la bitácora con truncate")
	}
}

// BORRAR UNA EMPRESA SE LLEVA SU ASISTENCIA Y SU BITÁCORA. Dos cosas podían bloquearlo: el trigger
// de solo agregar, y las llaves hacia `users`, que si revisaran al instante tropezarían con el orden
// en que el cascade borra.
func TestDeletingACompanyTakesItsAttendanceAndAuditLog(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "audit-baja")
	otherCompany := makeCompany(t, st, "audit-queda")
	ana := makeUserIn(t, st, co, "ana-baja", "gerente")
	luis := makeUserIn(t, st, otherCompany, "luis-queda", "gerente")

	entry := openEntry(t, st, co, ana)
	for _, stmt := range []string{
		`update attendance_entries set clock_out_at = '2026-10-10 17:00-06', status = 'closed' where id = $2 and company_id = $1`,
		`insert into attendance_corrections (company_id, entry_id, field, after, reason, corrected_by)
		 values ($1, $2, 'clock_out_at', '2026-10-10 17:00-06', 'Olvidó checar', (select user_id from attendance_entries where id = $2))`,
		`insert into audit_events (company_id, actor_kind, actor_user_id, subject_user_id, action, entity_type, entity_id)
		 values ($1, 'user', (select user_id from attendance_entries where id = $2), (select user_id from attendance_entries where id = $2), 'attendance.corrected', 'attendance_entry', $2)`,
	} {
		if _, err := st.Pool.Exec(ctx, stmt, co, entry); err != nil {
			t.Fatalf("sembrar (%s): %v", stmt, err)
		}
	}
	var v int64
	if err := st.Pool.QueryRow(ctx, `insert into work_schedule_versions (company_id, user_id, effective_from, created_by)
		values ($1, $2, '2026-10-13', $2) returning id`, co, ana).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into work_schedule_days (company_id, version_id, weekday, start_time, end_time) values ($1, $2, 1, '09:00', '17:00')`, co, v); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into audit_events (company_id, actor_kind, actor_user_id, action, entity_type, entity_id)
		values ($1, 'user', $2, 'price.set', 'product', 1)`, otherCompany, luis); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Pool.Exec(ctx, `delete from companies where id = $1`, co); err != nil {
		t.Fatalf("borrar una empresa con asistencia y bitácora: %v", err)
	}
	for _, table := range tables0089 {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from `+table+` where company_id = $1`, co).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s conserva %d filas de la empresa borrada", table, n)
		}
	}
	var ofOther int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where company_id = $1`, otherCompany).Scan(&ofOther); err != nil {
		t.Fatal(err)
	}
	if ofOther != 1 {
		t.Errorf("borrar una empresa tocó la bitácora de otra: quedan %d eventos, se esperaba 1", ofOther)
	}
}
