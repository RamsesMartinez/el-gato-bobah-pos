-- +goose Up
-- Cada devolución sabe en qué TURNO y qué DÍA ocurrió, y cuánta propina regresó; cada pago, qué día
-- entró (spec 031).
--
-- Hasta aquí una devolución por tarjeta no estaba en ningún turno: el corte esperaba el cobro
-- completo y, con «declarar automático», firmaba como recibido dinero que se le había regresado al
-- cliente. Y lo que Ventas reportaba por método salía del día del PEDIDO, así que un cobro días
-- después o una devolución el mes siguiente reescribían un periodo ya cerrado. El dueño decidió que
-- cada peso se clasifica por la hora y el turno de su propio movimiento: eso es un hecho que, si no
-- se registra cuando ocurre, no se puede reconstruir (una ventana de tiempo se equivoca en cuanto
-- haya dos cajas).
--
-- lock_timeout como 0060: los ALTER toman candados sobre tablas que se escriben con cada cobro, y es
-- preferible que la migración falle limpio en 3 segundos a que el POS se quede esperando detrás.
set local lock_timeout = '3s';

alter table order_refunds
  add column register_session_id bigint,
  add column business_date date,
  -- Propina devuelta, APARTE del monto: la propina es del personal y no ingreso del negocio, así
  -- que no entra a `orders.refund_amount`. Solo la devuelve cancelar con devolución.
  add column tip_amount numeric(10,2) not null default 0;

-- Una fila puede devolver solo propina (el monto de ese medio ya se había devuelto antes), pero
-- nunca nada.
alter table order_refunds drop constraint order_refunds_amount_check;
alter table order_refunds add constraint order_refunds_amount_check
  check (amount >= 0 and tip_amount >= 0 and amount + tip_amount > 0);

-- LLAVE COMPUESTA, como 0061: los chequeos de integridad referencial saltan RLS, y con una llave
-- simple una devolución de una empresa podría colgar del turno de otra. `no action` y no `restrict`:
-- borrar una empresa arrastra en cascada sus turnos y sus devoluciones en la misma sentencia, y
-- `no action` se verifica al final de ella. Una devolución es un hecho contable: un turno suelto no se
-- borra.
alter table order_refunds add constraint order_refunds_session_fkey
  foreign key (company_id, register_session_id) references register_sessions (company_id, id);

-- Arranca por company_id: RLS agrega ese predicado a toda consulta del rol de la app. Es lo que lee
-- el corte (las devoluciones de un turno).
create index order_refunds_company_session on order_refunds (company_id, register_session_id);

-- Las de efectivo ya sabían su turno: el de su salida de caja.
update order_refunds r
   set register_session_id = m.session_id
  from register_cash_movements m
 where m.id = r.cash_movement_id
   and m.company_id = r.company_id;

-- Las que no tocaron el cajón, hechas durante el turno principal que SIGUE ABIERTO al migrar, van a
-- ese turno: sin esto ese corte cerraría sin restarlas. Aquí la ventana de tiempo sí es exacta —hay
-- un solo turno abierto por caja principal y sucursal—. Las de turnos ya cerrados se quedan sin
-- turno a propósito: su corte ya guardó la cifra que se firmó.
update order_refunds r
   set register_session_id = s.id
  from orders o, register_sessions s
  join cash_registers c on c.id = s.register_id
 where o.id = r.order_id
   and r.register_session_id is null
   and r.cash_movement_id is null
   and s.status = 'abierta'
   and c.is_primary
   and c.branch_id = o.branch_id
   and s.company_id = r.company_id
   and r.created_at >= s.opened_at;

-- El día en la zona del local, con la misma expresión que 0062 y el mismo default de producto: caer a
-- UTC correría la fecha seis horas en silencio.
update order_refunds r
   set business_date = (r.created_at at time zone coalesce(nullif(
         (select bs.timezone from business_settings bs where bs.company_id = r.company_id), ''),
         'America/Mexico_City'))::date;

-- El día de cada pago. Nullable: un binario anterior en rollback tiene que poder seguir cobrando.
-- Lo histórico lo rellena 0081.
alter table order_payments add column business_date date;
create index order_payments_company_day on order_payments (company_id, business_date);

-- +goose Down
-- Se niega si perdería dinero: una devolución de solo propina no cabe en el check anterior, y borrar
-- `tip_amount` borraría el rastro de un dinero que salió del cajón.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from order_refunds where amount = 0 or tip_amount > 0) then
    raise exception 'hay devoluciones con propina devuelta: el Down de 0080 las perdería';
  end if;
end $$;
-- +goose StatementEnd
drop index if exists order_payments_company_day;
alter table order_payments drop column business_date;
drop index if exists order_refunds_company_session;
alter table order_refunds drop constraint order_refunds_session_fkey;
alter table order_refunds drop constraint order_refunds_amount_check;
alter table order_refunds add constraint order_refunds_amount_check check (amount > 0);
alter table order_refunds drop column tip_amount, drop column business_date, drop column register_session_id;
