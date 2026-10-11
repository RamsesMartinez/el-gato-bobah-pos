-- +goose Up
-- Asistencia del personal y bitácora de auditoría (spec 033, decisiones 18 y 19 del dueño).
-- Todas las tablas son por empresa con RLS, y todas las FK entre tablas de empresa son compuestas:
-- los chequeos de integridad referencial saltan RLS. Las FK hacia `users` son NO ACTION (se revisan
-- al final del statement) y no RESTRICT: al borrar una empresa, el cascade puede llegar a `users`
-- antes que a estas tablas, y una revisión inmediata lo bloquearía.

-- 1. AJUSTES. La asistencia nace apagada y, con ella, la medición de retardos y extra.
alter table business_settings
  add column attendance_enabled          boolean  not null default false,
  add column attendance_measure_schedule boolean  not null default false,
  add column attendance_grace_in_min     smallint not null default 0 check (attendance_grace_in_min between 0 and 60),
  add column attendance_grace_out_min    smallint not null default 0 check (attendance_grace_out_min between 0 and 60);

-- A quién no se le mide (p. ej. el dueño): no aparece para checar ni en el reporte.
alter table users add column tracks_attendance boolean not null default true;

-- 2. ENTRADAS. Una fila por turno trabajado. `business_date` se guarda al insertar: calcularlo al
--    vuelo con la zona horaria vigente movería de día turnos ya cerrados si la zona cambia.
create table attendance_entries (
  id             bigint generated always as identity primary key,
  company_id     bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                 references companies(id) on delete cascade,
  user_id        bigint not null,
  -- Solo si la empresa tiene una sucursal; con varias queda nula y no se adivina.
  branch_id      bigint,
  business_date  date not null,
  clock_in_at    timestamptz not null,
  clock_out_at   timestamptz,
  status         text not null default 'open' check (status in ('open', 'closed', 'missing_out')),
  source         text not null default 'clock' check (source in ('clock', 'manual')),
  corrected      boolean not null default false,
  in_ip          inet,
  in_device      jsonb,
  in_device_key  text check (in_device_key ~ '^[0-9a-f]{32}$'),
  out_ip         inet,
  out_device     jsonb,
  out_device_key text check (out_device_key ~ '^[0-9a-f]{32}$'),
  created_by     bigint,
  created_at     timestamptz not null default now(),
  constraint attendance_entries_tenant_key unique (company_id, id),
  constraint attendance_entries_user_fkey
    foreign key (company_id, user_id) references users (company_id, id),
  constraint attendance_entries_created_by_fkey
    foreign key (company_id, created_by) references users (company_id, id),
  constraint attendance_entries_branch_fkey
    foreign key (company_id, branch_id) references branches (company_id, id),
  constraint attendance_entries_out_after_in check (clock_out_at is null or clock_out_at > clock_in_at),
  -- Solo una entrada cerrada lleva salida; `open` y `missing_out` no.
  constraint attendance_entries_closed_has_out check ((status = 'closed') = (clock_out_at is not null))
);
-- Una sola entrada abierta por persona: dos tabletas que checan a la vez no la duplican.
create unique index attendance_entries_one_open on attendance_entries (company_id, user_id) where status = 'open';
create index attendance_entries_by_date on attendance_entries (company_id, business_date);
create index attendance_entries_by_user on attendance_entries (company_id, user_id, business_date);

alter table attendance_entries enable row level security;
create policy tenant_isolation on attendance_entries
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Update solo de lo que cambia al registrar la salida o al corregir: quién checó, cómo se
-- registró y quién lo capturó no se reescriben nunca.
grant select, insert on attendance_entries to gatobobah_app;
grant update (clock_in_at, clock_out_at, status, corrected, business_date, out_ip, out_device, out_device_key)
  on attendance_entries to gatobobah_app;

-- Una hora ya registrada solo cambia con su corrección: registrar la salida (de nada a un valor) es
-- lo normal; mover una hora que ya estaba exige que en la MISMA transacción se haya guardado la
-- fila de attendance_corrections con el valor original (corrected_at = now() es el instante de la
-- transacción). Marcar `corrected` no basta: con eso solo, cualquier camino reescribía la hora sin
-- conservar la anterior. Y un turno cerrado o sin salida no se reabre: reabrirlo revive un turno
-- viejo sin rastro.
-- +goose StatementBegin
create function attendance_entries_guard_correction() returns trigger language plpgsql as $$
begin
  if (new.clock_in_at is distinct from old.clock_in_at
      or (old.clock_out_at is not null and new.clock_out_at is distinct from old.clock_out_at))
     and not exists (select 1 from attendance_corrections c
                      where c.company_id = new.company_id and c.entry_id = new.id and c.corrected_at = now()) then
    raise exception 'una hora ya registrada solo cambia con su corrección'
      using errcode = 'check_violation';
  end if;
  if old.status <> 'open' and new.status = 'open' then
    raise exception 'un turno cerrado no se reabre' using errcode = 'check_violation';
  end if;
  if old.status = 'closed' and new.status = 'missing_out' then
    raise exception 'un turno cerrado no queda sin salida' using errcode = 'check_violation';
  end if;
  return new;
end $$;
-- +goose StatementEnd
create trigger attendance_entries_guard_correction before update on attendance_entries
  for each row execute function attendance_entries_guard_correction();

-- 3. CORRECCIONES. Conservan el valor original (FR-010). Sin update en el grant: la base impone
--    que la corrección misma tampoco se reescribe.
create table attendance_corrections (
  id           bigint generated always as identity primary key,
  company_id   bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
               references companies(id) on delete cascade,
  entry_id     bigint not null,
  field        text not null check (field in ('clock_in_at', 'clock_out_at', 'created')),
  before       timestamptz,
  after        timestamptz not null,
  reason       text not null check (reason = btrim(reason) and char_length(reason) between 3 and 200),
  corrected_by bigint not null,
  corrected_at timestamptz not null default now(),
  constraint attendance_corrections_tenant_key unique (company_id, id),
  constraint attendance_corrections_entry_fkey
    foreign key (company_id, entry_id) references attendance_entries (company_id, id),
  constraint attendance_corrections_by_fkey
    foreign key (company_id, corrected_by) references users (company_id, id)
);
create index attendance_corrections_by_entry on attendance_corrections (company_id, entry_id);

alter table attendance_corrections enable row level security;
create policy tenant_isolation on attendance_corrections
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on attendance_corrections to gatobobah_app;

-- 4. HORARIOS POR VERSIÓN COMPLETA. Cambiar el horario es insertar una versión nueva con todos sus
--    días; el horario de un día es el de la versión vigente, y un día sin fila en ella es libre.
--    Guardarlo día por día con vigencia propia «resucitaba» el sábado que se dejó de trabajar.
create table work_schedule_versions (
  id             bigint generated always as identity primary key,
  company_id     bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                 references companies(id) on delete cascade,
  user_id        bigint not null,
  effective_from date not null,
  created_by     bigint not null,
  created_at     timestamptz not null default now(),
  constraint work_schedule_versions_tenant_key unique (company_id, id),
  constraint work_schedule_versions_one_per_day unique (company_id, user_id, effective_from),
  constraint work_schedule_versions_user_fkey
    foreign key (company_id, user_id) references users (company_id, id),
  constraint work_schedule_versions_by_fkey
    foreign key (company_id, created_by) references users (company_id, id)
);

alter table work_schedule_versions enable row level security;
create policy tenant_isolation on work_schedule_versions
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on work_schedule_versions to gatobobah_app;

-- `end_time < start_time` es un turno que termina al día siguiente. `shift_no` deja abierto el
-- turno partido (dos tramos el mismo día).
create table work_schedule_days (
  id         bigint generated always as identity primary key,
  company_id bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
             references companies(id) on delete cascade,
  version_id bigint not null,
  weekday    smallint not null check (weekday between 0 and 6),
  shift_no   smallint not null default 1 check (shift_no >= 1),
  start_time time not null,
  end_time   time not null,
  constraint work_schedule_days_tenant_key unique (company_id, id),
  constraint work_schedule_days_one_shift unique (company_id, version_id, weekday, shift_no),
  constraint work_schedule_days_version_fkey
    foreign key (company_id, version_id) references work_schedule_versions (company_id, id),
  constraint work_schedule_days_not_empty check (end_time <> start_time)
);

alter table work_schedule_days enable row level security;
create policy tenant_isolation on work_schedule_days
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on work_schedule_days to gatobobah_app;

-- 5. BITÁCORA DE AUDITORÍA. Se escribe en la misma transacción que la acción. `order_id` va sin FK a
--    propósito: un evento sobrevive a cualquier reorganización de pedidos; RLS filtra su lectura.
--    `action` sin check: la lista blanca vive en `domain` y crecer no cuesta una migración.
create table audit_events (
  id              bigint generated always as identity primary key,
  company_id      bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                  references companies(id) on delete cascade,
  branch_id       bigint,
  occurred_at     timestamptz not null default now(),
  actor_kind      text not null check (actor_kind in ('user', 'platform', 'system')),
  actor_user_id   bigint,
  actor_label     text,
  action          text not null,
  entity_type     text not null,
  entity_id       bigint not null,
  order_id        bigint,
  subject_user_id bigint,
  ip              inet,
  device          jsonb,
  device_key      text check (device_key ~ '^[0-9a-f]{32}$'),
  before          jsonb,
  after           jsonb,
  constraint audit_events_tenant_key unique (company_id, id),
  constraint audit_events_actor_user check ((actor_kind = 'user') = (actor_user_id is not null)),
  constraint audit_events_actor_fkey
    foreign key (company_id, actor_user_id) references users (company_id, id),
  constraint audit_events_subject_fkey
    foreign key (company_id, subject_user_id) references users (company_id, id),
  constraint audit_events_branch_fkey
    foreign key (company_id, branch_id) references branches (company_id, id)
);
create index audit_events_by_time on audit_events (company_id, occurred_at desc, id desc);
create index audit_events_by_order on audit_events (company_id, order_id, occurred_at) where order_id is not null;
create index audit_events_by_actor on audit_events (company_id, actor_user_id, occurred_at desc);
create index audit_events_by_device on audit_events (company_id, device_key, occurred_at desc) where device_key is not null;

alter table audit_events enable row level security;
create policy tenant_isolation on audit_events
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
grant select, insert on audit_events to gatobobah_app;

-- Solo agregar, también para el owner: un evento que se puede editar no prueba nada. La única
-- excepción es el cascade de una empresa que ya se borró (dar de baja un cliente).
-- +goose StatementBegin
create function audit_events_append_only() returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' and not exists (select 1 from companies where id = old.company_id) then
    return old;
  end if;
  raise exception 'la bitácora de auditoría es de solo agregar';
end $$;
-- +goose StatementEnd
create trigger audit_events_append_only before update or delete on audit_events
  for each row execute function audit_events_append_only();
create trigger audit_events_no_truncate before truncate on audit_events
  for each statement execute function audit_events_append_only();

-- +goose Down
-- Se niega si perdería algo: la bitácora, las entradas y los horarios son la única copia de esos
-- hechos. Tras desplegar, el rollback es restaurar el respaldo.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from audit_events) then
    raise exception 'hay eventos en audit_events: es la única copia de quién hizo qué';
  end if;
  if exists (select 1 from attendance_entries) or exists (select 1 from attendance_corrections) then
    raise exception 'hay registros de asistencia: es la única copia de las horas trabajadas';
  end if;
  if exists (select 1 from work_schedule_versions) then
    raise exception 'hay horarios capturados';
  end if;
end $$;
-- +goose StatementEnd

drop trigger audit_events_no_truncate on audit_events;
drop trigger audit_events_append_only on audit_events;
drop function audit_events_append_only();
drop trigger attendance_entries_guard_correction on attendance_entries;
drop function attendance_entries_guard_correction();
drop table audit_events;
drop table work_schedule_days;
drop table work_schedule_versions;
drop table attendance_corrections;
drop table attendance_entries;
alter table users drop column tracks_attendance;
alter table business_settings
  drop column attendance_grace_out_min,
  drop column attendance_grace_in_min,
  drop column attendance_measure_schedule,
  drop column attendance_enabled;
