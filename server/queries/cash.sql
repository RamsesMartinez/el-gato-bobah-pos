-- Medios de pago (lookup)

-- name: ListPaymentMethods :many
select id, name, kind, affects_cash_drawer, auto_declare from payment_methods where is_active order by sort_key, name;

-- name: GetPaymentMethod :one
select id, name, kind, affects_cash_drawer, auto_declare from payment_methods where id = $1;

-- name: UpdatePaymentMethodAutoDeclare :one
update payment_methods set auto_declare = $2 where id = $1
returning id, name, kind, affects_cash_drawer, auto_declare;

-- name: InsertExpenseCashMovement :exec
-- Salida de efectivo del cajón al pagar un gasto en efectivo (liga el gasto al corte).
insert into register_cash_movements (session_id, kind, amount, concept, expense_id, user_id)
values ($1, 'salida', $2, $3, $4, $5);

-- Cajas (registros físicos con nombre; la primaria recibe las ventas del POS)

-- name: ListCashRegisters :many
-- Cajas activas + el id de su sesión abierta (null si está cerrada). Para pickers y la vista de caja.
-- LEFT JOIN (no subquery escalar) para que sqlc infiera open_session_id como nullable; el índice
-- único one_open_session_per_register garantiza ≤1 sesión abierta por caja (sin duplicar filas).
select r.id, r.name, r.is_primary, r.is_active, s.id as open_session_id
from cash_registers r
left join register_sessions s on s.register_id = r.id and s.status = 'abierta'
where r.is_active
order by r.is_primary desc, r.name;

-- name: ListAllCashRegisters :many
select id, name, is_primary, is_active from cash_registers order by is_primary desc, name;

-- name: GetCashRegister :one
select id, name, is_primary, is_active from cash_registers where id = $1;

-- name: CreateCashRegister :one
-- is_primary/is_active toman su default (secundaria, activa): la primaria la fija la migración.
insert into cash_registers (name) values ($1)
returning id, name, is_primary, is_active;

-- name: UpdateCashRegister :one
update cash_registers set name = $2, is_active = $3 where id = $1
returning id, name, is_primary, is_active;

-- Cortes de caja (sesiones de una caja)

-- name: GetOpenSessionByRegister :one
select * from register_sessions where register_id = $1 and status = 'abierta' limit 1;

-- name: ListOpenSessions :many
-- Todas las cajas abiertas (para el POS / aviso y validaciones de traspaso).
select s.id, s.register_id, r.name as register_name, r.is_primary, s.opening_cash, s.currency, s.opened_at
from register_sessions s
join cash_registers r on r.id = s.register_id
where s.status = 'abierta'
order by r.is_primary desc, r.name;

-- name: AnyOpenSession :one
select exists(select 1 from register_sessions where status = 'abierta');

-- name: OpenSession :one
insert into register_sessions (business_date, opening_cash, opened_by, register_id)
values ($1, $2, $3, $4)
returning *;

-- name: CloseSession :exec
update register_sessions set status = 'cerrada', closed_by = $2, closed_at = now(), notes = $3
where id = $1;

-- name: SaveSessionTotal :exec
insert into register_session_totals (session_id, payment_method_id, expected, declared, tips)
values ($1, $2, $3, $4, $5);

-- name: ListSessions :many
select s.id, s.business_date, s.status, s.opening_cash, s.currency, s.opened_at, s.closed_at, s.notes,
       r.name as register_name,
       ob.name as opened_by_name, cb.name as closed_by_name,
       coalesce((select sum(difference) from register_session_totals t where t.session_id = s.id), 0)::numeric(10,2) as total_difference
from register_sessions s
join cash_registers r on r.id = s.register_id
join users ob on ob.id = s.opened_by
left join users cb on cb.id = s.closed_by
order by s.opened_at desc limit $1;

-- name: GetSession :one
select s.*, r.name as register_name, ob.name as opened_by_name, cb.name as closed_by_name
from register_sessions s
join cash_registers r on r.id = s.register_id
join users ob on ob.id = s.opened_by
left join users cb on cb.id = s.closed_by
where s.id = $1;

-- name: ListSessionTotals :many
select t.payment_method_id, pm.name, pm.kind, pm.affects_cash_drawer, t.expected, t.declared, t.tips,
       (t.declared - t.expected)::numeric(10,2) as difference
from register_session_totals t
join payment_methods pm on pm.id = t.payment_method_id
where t.session_id = $1
order by pm.sort_key;

-- Movimientos de efectivo (entradas/salidas del cajón durante la sesión).
-- name: InsertCashMovement :one
insert into register_cash_movements (session_id, kind, amount, concept, user_id)
values ($1, $2, $3, $4, $5)
returning *;

-- name: ListCashMovements :many
-- expense_id: no-null si el movimiento es la salida de un gasto → el front lo excluye de la tabla
-- de efectivo (los gastos van en su propia sección) para no contarlos dos veces.
select m.id, m.kind, m.amount, m.concept, m.created_at, u.name as user_name, m.transfer_id, m.expense_id
from register_cash_movements m
join users u on u.id = m.user_id
where m.session_id = $1
order by m.created_at;

-- name: ListExpensesBySession :many
-- Gastos atribuidos a un corte (efectivo y no-efectivo): sección "Gastos" del resumen del corte.
--
-- La atribución al corte pasó del encabezado del gasto a CADA PAGO (0029): un gasto con pago
-- partido puede tocar dos cortes distintos, así que lo que se lista es el PAGO, y el importe
-- que se muestra es el del pago, no el del gasto completo.
select ep.id, e.id as expense_id, ec.name as category, s.name as supplier,
       pm.name as payment_method, ep.amount, e.currency, e.status
from expense_payments ep
join expenses e on e.id = ep.expense_id
join expense_categories ec on ec.id = e.category_id
join payment_methods pm on pm.id = ep.payment_method_id
left join suppliers s on s.id = e.supplier_id
where ep.register_session_id = $1
order by ep.id;

-- Neto de efectivo movido en la sesión (entradas − salidas); suma al efectivo esperado al cerrar.
-- name: NetCashMovements :one
select coalesce(sum(case when kind = 'entrada' then amount else -amount end), 0)::numeric(10,2) as net
from register_cash_movements where session_id = $1;

-- Traspasos entre cajas: la fila de traspaso + cada pierna como movimiento ligado.
-- name: CreateCashTransfer :one
insert into cash_transfers (from_session_id, to_session_id, amount, note, created_by)
values ($1, $2, $3, $4, $5)
returning id;

-- name: InsertTransferMovement :exec
insert into register_cash_movements (session_id, kind, amount, concept, user_id, transfer_id)
values ($1, $2, $3, $4, $5, $6);

-- Totales esperados por método, del TURNO indicado.
-- name: ExpectedByMethodForSession :many
-- expected = ventas (amount); tips = propinas (tip_amount) por método desde la apertura. Ambas son
-- dinero recibido: entran al esperado del corte, pero se muestran como líneas separadas (Ventas / Propinas).
-- kind viaja además de affects_cash_drawer porque distinguen cosas distintas: el segundo dice si
-- ese dinero se cuenta en el arqueo (lo cumplen el efectivo del mostrador Y el de las plataformas),
-- y el primero identifica al ÚNICO al que pertenecen el fondo de apertura y los movimientos de
-- caja. Sumar el fondo a todo lo que toca el cajón lo contaba una vez por método.
select pm.id as payment_method_id, pm.name, pm.kind, pm.affects_cash_drawer, pm.auto_declare,
       coalesce(sum(op.amount), 0)::numeric(10,2) as expected,
       coalesce(sum(op.tip_amount), 0)::numeric(10,2) as tips
from payment_methods pm
-- Por register_session_id y no por `created_at >= apertura`. La ventana de tiempo daba el
-- resultado correcto por COINCIDENCIA: solo la caja principal vende y no puede haber dos turnos
-- suyos abiertos, así que la ventana y el turno coincidían. El día que exista una segunda caja
-- que cobre —una barra, otro mostrador—, dos turnos traslapados sumarían el mismo dinero y los
-- dos parecerían cuadrar. El vínculo explícito lo hace correcto por construcción.
left join order_payments op on op.payment_method_id = pm.id and op.register_session_id = $1
where pm.is_active
group by pm.id, pm.name, pm.kind, pm.affects_cash_drawer, pm.auto_declare
order by pm.sort_key;

-- name: GetOpenPrimarySession :one
-- La sesión que habilita cobrar. Es SIEMPRE la de la caja principal: las secundarias (caja fuerte,
-- caja externa) existen para traspasos y gastos, y si una de ellas bastara para vender el efectivo
-- del mostrador caería en un arqueo que no es el suyo.
select s.id, s.register_id, s.business_date
from register_sessions s
join cash_registers r on r.id = s.register_id
where s.status = 'abierta' and r.is_primary and r.is_active
limit 1;

-- name: SeedBasePaymentMethods :exec
-- Métodos de pago base para una empresa recién creada. Desde 0037 la tabla es per-tenant, así que
-- una empresa nueva nace SIN NINGUNO y no podría cobrar: /payment-methods devolvería vacío y el
-- checkout se quedaría sin botones. Antes los heredaba por ser una tabla global.
--
-- Los de PLATAFORMA quedan fuera a propósito: vender por Uber/DiDi/Rappi exige que ese negocio haya
-- hecho su propia vinculación con la plataforma, y darle tres formas de cobro que no tiene
-- contratadas es peor que no darle ninguna.
insert into payment_methods (company_id, name, kind, affects_cash_drawer, is_active, sort_key, auto_declare)
values
  ($1, 'Efectivo',           'efectivo',      true,  true, 100, false),
  ($1, 'Tarjeta débito',     'tarjeta',       false, true, 200, true),
  ($1, 'Tarjeta crédito',    'tarjeta',       false, true, 250, true),
  ($1, 'Transferencia SPEI', 'transferencia', false, true, 300, true)
on conflict (company_id, name) do nothing;

-- name: GetBusinessTimezone :one
-- Zona horaria del local, para calcular la FECHA de negocio. La base guarda instantes en UTC; la
-- fecha es una decisión de calendario y depende de dónde está el negocio.
select timezone from business_settings limit 1;
