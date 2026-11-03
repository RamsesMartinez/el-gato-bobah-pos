-- +goose Up
-- Fondo de caja fijo (spec 035). NÚMERO PROVISIONAL: se renumera al siguiente libre de develop al
-- fusionar (la 034 también trae una 0089; ese choque lo resuelve quien fusione), junto con la
-- constante fixedFloatVersion de su test.
--
-- Solo agrega: columnas nulables, un tipo de movimiento y un índice. Se sube con cajas abiertas y la
-- API anterior sigue funcionando sobre este esquema (constitución, «se sube con el negocio operando»).

-- 1. Lo que se queda en el cajón al cerrar, por caja. Nulo = sin configurar (el cierre pide teclearlo
--    como antes); cero es un fondo válido: el cajón se vacía.
alter table cash_registers add column fixed_float numeric(10,2)
  constraint cash_registers_fixed_float_check check (fixed_float is null or fixed_float >= 0);

-- 2. Snapshot del fijo contra el que se cerró: cambiarlo mañana no reescribe si hoy se dejó otra
--    cantidad. Nulo en los cierres de antes y en cajas sin fijo.
alter table register_sessions add column fixed_float_at_close numeric(10,2)
  constraint register_sessions_fixed_float_at_close_check check (fixed_float_at_close is null or fixed_float_at_close >= 0);

-- 3. El retiro al cierre: un tipo propio y no una salida con concepto. Las consultas de salidas
--    filtran kind = 'salida' y el neto del cajón lo excluye: se registra después de contar.
--    NOT VALID + VALIDATE: la tabla la escribe la caja viva y así el candado exclusivo dura lo que
--    tarda en cambiar el catálogo, no lo que tarda en revisar las filas.
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida', 'propina', 'reverso', 'retiro')) not valid;
alter table register_cash_movements validate constraint register_cash_movements_kind_check;

-- Las tres cláusulas de 0087: el retiro, como la propina, dice a quién se le dio y copia su nombre.
alter table register_cash_movements drop constraint register_cash_movements_tip_shape;
alter table register_cash_movements add constraint register_cash_movements_tip_shape check (
  (kind in ('propina', 'retiro')) = (recipient_user_id is not null)
  and (recipient_user_id is null) = (recipient_name is null)
  and (recipient_name is null or length(btrim(recipient_name)) between 1 and 120)
) not valid;
alter table register_cash_movements validate constraint register_cash_movements_tip_shape;

-- Un retiro por turno. Sin company_id a propósito: session_id ya es de una sola empresa (el mismo caso
-- que register_cash_movements_one_reverse, 0088). La garantía contra el doble cierre es el candado de
-- LockSessionForClose; esto es la segunda barrera.
create unique index register_cash_movements_one_withdrawal on register_cash_movements (session_id)
  where kind = 'retiro';

-- +goose Down
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from register_cash_movements where kind = 'retiro')
     or exists (select 1 from cash_registers where fixed_float is not null)
     or exists (select 1 from register_sessions where fixed_float_at_close is not null) then
    raise exception 'hay retiros o fondos fijos guardados: bajar el fondo fijo los perdería';
  end if;
end $$;
-- +goose StatementEnd
drop index register_cash_movements_one_withdrawal;
alter table register_cash_movements drop constraint register_cash_movements_tip_shape;
alter table register_cash_movements add constraint register_cash_movements_tip_shape check (
  (kind = 'propina') = (recipient_user_id is not null)
  and (recipient_user_id is null) = (recipient_name is null)
  and (recipient_name is null or length(btrim(recipient_name)) between 1 and 120)
);
alter table register_cash_movements drop constraint register_cash_movements_kind_check;
alter table register_cash_movements add constraint register_cash_movements_kind_check
  check (kind in ('entrada', 'salida', 'propina', 'reverso'));
alter table register_sessions drop column fixed_float_at_close;
alter table cash_registers drop column fixed_float;
