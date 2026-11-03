-- Asistencia del personal (spec 033). RLS acota a la empresa; company_id lo pone el default desde
-- la sesión.

-- name: GetAttendanceSettings :one
select attendance_enabled, attendance_measure_schedule, attendance_grace_in_min, attendance_grace_out_min, timezone
  from business_settings;

-- name: ListAttendanceRoster :many
-- Quiénes pueden checar: activos y con «Lleva asistencia». Primero los que están dentro.
-- `left_at` es la última salida del día del negocio: sin ella, quien ya salió se ve «sin entrada».
select u.id, u.name, (u.pin_hash is not null)::boolean as has_pin, e.clock_in_at as open_since,
       l.clock_out_at as left_at
  from users u
  left join attendance_entries e on e.user_id = u.id and e.status = 'open'
  left join lateral (
    select x.clock_out_at from attendance_entries x
     where x.user_id = u.id and x.business_date = @business_date::date and x.clock_out_at is not null
     order by x.clock_out_at desc limit 1
  ) l on true
 where u.is_active and u.tracks_attendance
 order by (e.id is null), lower(u.name);

-- name: LockUserForAttendance :one
-- Serializa todo lo que toca la asistencia de UNA persona (checar, corregir, capturar a mano): un
-- `for update` sobre sus entradas no bloquea nada cuando aún no tiene ninguna, y dos capturas a la
-- vez del mismo turno quedarían duplicadas.
select id, name, is_active, tracks_attendance, pin_hash from users where id = $1 for update;

-- name: LockUserAttendanceForCash :one
-- Abrir y cerrar caja leen la asistencia de una persona con su fila tomada (spec 046). `for share`
-- choca con el `for update` de LockUserForAttendance, así que una entrada o salida en otra tableta
-- espera a que la caja termine (o al revés) y la regla se decide con lo que de verdad quedó. No
-- `for update`: ése choca también con el `for key share` de cada insert que la referencia
-- (`orders.opened_by`) y frenaría las ventas de esa persona.
select id, name, is_active, tracks_attendance from users where id = $1 for share;

-- name: GetOpenAttendance :one
select id, clock_in_at from attendance_entries where user_id = $1 and status = 'open';

-- name: ListActiveBranchIDs :many
-- La sucursal de un registro solo se pone si hay una (lo decide el servicio). Se devuelve la lista y
-- no un `case … end::bigint`: el cast hace que sqlc lo tipe no nulable y el NULL de «varias»
-- tumbaría la consulta (AGENTS.md, agregados que se vacían).
select id from branches where is_active order by id limit 2;

-- name: InsertAttendanceEntry :one
insert into attendance_entries (user_id, branch_id, business_date, clock_in_at, clock_out_at, status, source,
                                in_ip, in_device, in_device_key, created_by)
values (sqlc.arg('user_id'), sqlc.narg('branch_id'), sqlc.arg('business_date'), sqlc.arg('clock_in_at'),
        sqlc.narg('clock_out_at'), sqlc.arg('status'), sqlc.arg('source'), sqlc.narg('in_ip'),
        sqlc.narg('in_device'), sqlc.narg('in_device_key'), sqlc.narg('created_by'))
returning id;

-- name: CloseAttendanceEntry :execrows
update attendance_entries
   set clock_out_at = sqlc.arg('clock_out_at'), status = 'closed', out_ip = sqlc.narg('out_ip'),
       out_device = sqlc.narg('out_device'), out_device_key = sqlc.narg('out_device_key')
 where id = sqlc.arg('id') and status = 'open';

-- name: MarkAttendanceMissingOut :execrows
update attendance_entries set status = 'missing_out' where id = $1 and status = 'open';

-- name: SetAttendanceSettings :execrows
update business_settings
   set attendance_enabled = sqlc.arg('enabled'), attendance_measure_schedule = sqlc.arg('measure'),
       attendance_grace_in_min = sqlc.arg('grace_in'), attendance_grace_out_min = sqlc.arg('grace_out'),
       updated_at = now();

-- name: GetUserTracksAttendance :one
select tracks_attendance from users where id = $1;

-- name: SetUserTracksAttendance :execrows
update users set tracks_attendance = $2 where id = $1;

-- name: InsertScheduleVersion :one
insert into work_schedule_versions (user_id, effective_from, created_by) values ($1, $2, $3) returning id;

-- name: InsertScheduleDay :exec
insert into work_schedule_days (version_id, weekday, shift_no, start_time, end_time) values ($1, $2, $3, $4, $5);

-- name: ListScheduleDays :many
-- Todas las versiones con sus días. Una versión sin días (todo libre) sale con weekday nulo.
select v.id as version_id, v.user_id, v.effective_from, d.weekday, d.shift_no, d.start_time, d.end_time
  from work_schedule_versions v
  left join work_schedule_days d on d.version_id = v.id
 where (sqlc.narg('user_id')::bigint is null or v.user_id = sqlc.narg('user_id'))
 order by v.user_id, v.effective_from, d.weekday, d.shift_no;

-- name: ListAttendanceEntriesInRange :many
select e.id, e.user_id, u.name, e.business_date, e.clock_in_at, e.clock_out_at, e.status, e.source, e.corrected,
       coalesce(host(e.in_ip), '')::text as in_ip, coalesce(host(e.out_ip), '')::text as out_ip,
       e.in_device, e.out_device, e.in_device_key, e.out_device_key
  from attendance_entries e
  join users u on u.id = e.user_id
 where e.business_date between sqlc.arg('from_date') and sqlc.arg('to_date')
   and (sqlc.narg('user_id')::bigint is null or e.user_id = sqlc.narg('user_id'))
 order by lower(u.name), e.business_date, e.clock_in_at;

-- name: GetAttendanceEntryForUpdate :one
select id, user_id, clock_in_at, clock_out_at, status from attendance_entries where id = $1 for update;

-- name: ListUserAttendanceNear :many
-- Los registros de una persona cerca de un intervalo, para revisar solapes. Se lee bajo el candado
-- de la persona (LockUserForAttendance).
select id, clock_in_at, clock_out_at, status from attendance_entries
 where user_id = sqlc.arg('user_id')
   and clock_in_at < sqlc.arg('until_at')
   and coalesce(clock_out_at, 'infinity'::timestamptz) > sqlc.arg('since_at');

-- name: CorrectAttendanceEntry :exec
update attendance_entries
   set clock_in_at = sqlc.arg('clock_in_at'), clock_out_at = sqlc.narg('clock_out_at'), status = sqlc.arg('status'),
       business_date = sqlc.arg('business_date'), corrected = true
 where id = sqlc.arg('id');

-- name: InsertAttendanceCorrection :exec
insert into attendance_corrections (entry_id, field, before, after, reason, corrected_by)
values (sqlc.arg('entry_id'), sqlc.arg('field'), sqlc.narg('before'), sqlc.arg('after'), sqlc.arg('reason'), sqlc.arg('corrected_by'));
