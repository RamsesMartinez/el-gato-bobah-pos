-- +goose Up
-- Correcciones de un corte cerrado (#290, spec 082 fase 3). Lo firmado al cerrar —el conteo, los
-- totales y el fondo— no se reescribe: una corrección se AGREGA con el valor de antes, el de después,
-- quién, cuándo y por qué, y se deshace con otra que la revierte. Es el precedente del reverso de una
-- salida (0088). Hoy estas correcciones se hacían con SQL a mano y sin rastro (#289).
--
-- Aditiva: la API anterior no la lee ni la escribe, así que convive con la nueva durante la subida.
create table cash_session_adjustments (
  id            bigint generated always as identity primary key,
  company_id    bigint not null default nullif(current_setting('app.company_id', true), '')::bigint
                references companies(id) on delete cascade,
  session_id    bigint not null,
  kind          text not null
                constraint cash_session_adjustments_kind check (kind in ('count_correction', 'float_correction', 'note', 'reversal')),
  -- Lo que decía antes (lo firmado o la corrección vigente) y lo que debió decir. Nulo antes solo en
  -- el fondo de un corte anterior a la 0088, que no lo guardó.
  before_amount numeric(10,2) constraint cash_session_adjustments_before_nonneg check (before_amount >= 0),
  after_amount  numeric(10,2) constraint cash_session_adjustments_after_nonneg check (after_amount >= 0),
  reason        text not null
                constraint cash_session_adjustments_reason check (reason in ('miscount', 'unrecorded_movement', 'wrong_register', 'float_error', 'other')),
  note          text not null
                constraint cash_session_adjustments_note check (note = btrim(note) and char_length(note) between 1 and 200),
  reverses_id   bigint,
  created_by    bigint not null,
  created_at    timestamptz not null default now(),

  -- `no action` y no `restrict`: no se borra un corte ni una persona con correcciones, pero el
  -- borrado en cascada de una empresa no depende del orden en que Postgres resuelva las llaves.
  constraint cash_session_adjustments_session_fkey
    foreign key (company_id, session_id) references register_sessions (company_id, id),
  constraint cash_session_adjustments_created_by_fkey
    foreign key (company_id, created_by) references users (company_id, id),
  constraint cash_session_adjustments_reverse_shape check ((kind = 'reversal') = (reverses_id is not null)),
  constraint cash_session_adjustments_amount_shape check (
    case kind
      when 'count_correction' then before_amount is not null and after_amount is not null
      when 'float_correction' then after_amount is not null
      else before_amount is null and after_amount is null
    end)
);
-- La llave que permite que un reverso apunte a una corrección DEL MISMO CORTE y de la misma empresa.
create unique index cash_session_adjustments_tenant_key on cash_session_adjustments (company_id, session_id, id);
alter table cash_session_adjustments add constraint cash_session_adjustments_reverses_fkey
  foreign key (company_id, session_id, reverses_id) references cash_session_adjustments (company_id, session_id, id);
-- Un reverso por corrección. Sin company_id a propósito, como 0088: reverses_id es un id de una tabla
-- con identidad propia y no choca entre empresas; dos reversos de la misma corrección es el defecto
-- que se cierra.
create unique index cash_session_adjustments_one_reverse on cash_session_adjustments (reverses_id)
  where reverses_id is not null;

alter table cash_session_adjustments enable row level security;
create policy tenant_isolation on cash_session_adjustments
  using (company_id = nullif(current_setting('app.company_id', true), '')::bigint)
  with check (company_id = nullif(current_setting('app.company_id', true), '')::bigint);
-- Solo se agregan: ni update ni delete para la API.
grant select, insert on cash_session_adjustments to gatobobah_app;

-- +goose Down
-- Se niega con correcciones: bajar borraría el rastro de quién corrigió qué corte y por qué, y la
-- API anterior volvería a mostrar la diferencia sin corregir.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from cash_session_adjustments) then
    raise exception 'hay correcciones de cortes: bajar perdería su rastro';
  end if;
end $$;
-- +goose StatementEnd
drop table cash_session_adjustments;
