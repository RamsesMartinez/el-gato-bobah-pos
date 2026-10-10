-- Medios de pago (lookup)

-- name: ListPaymentMethods :many
-- delivery_platform_id: a qué plataforma pertenece el método, o NULL si no es de plataforma. Es
-- lo que deja al POS ofrecer solo los dos de la plataforma activa sin comparar nombres.
select id, name, kind, affects_cash_drawer, auto_declare, delivery_platform_id from payment_methods where is_active order by sort_key, name;

-- name: ListAllPaymentMethods :many
-- Todos, incluidos los apagados. Es la lista de la pantalla de AJUSTES, y por eso no puede ser la
-- misma que ofrece el POS para cobrar.
--
-- Existe porque apagar un método era una puerta de un solo sentido: la tabla de interruptores se
-- pintaba con la lista filtrada, así que el renglón desaparecía junto con su propio interruptor y
-- no quedaba camino en la aplicación para volver a encenderlo. Un dedo que falla por milímetros
-- sobre «Efectivo» dejaba al mostrador sin cobrar en efectivo hasta que alguien entrara a la base.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods order by sort_key, name;

-- name: GetPaymentMethod :one
-- Trae `is_active` para que quien MUEVE DINERO con este método pueda rechazarlo si el negocio lo
-- apagó. No se filtra en el where: configurar un método desactivado —cambiarle el auto-declare, por
-- ejemplo— tiene que seguir siendo posible, y ahí el estado no estorba.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods where id = $1;

-- name: LockPaymentMethod :one
-- El mismo renglón que GetPaymentMethod, tomado para actualizar.
--
-- Existe porque configurar un método es leer-validar-escribir, y sin lock dos PATCH simultáneos
-- —uno que enciende «automático» y otro que enciende «va al cajón»— pasan cada uno su validación
-- contra el estado viejo y dejan escrita la combinación que el código considera imposible. Que
-- hoy no se pierda dinero por eso es una coincidencia del orden en que `CloseSession` resuelve los
-- métodos del cajón, no una garantía.
select id, name, kind, is_cash, affects_cash_drawer, auto_declare, delivery_platform_id, is_active
from payment_methods where id = $1 for update;

-- name: UpdatePaymentMethodFlags :one
-- Los tres interruptores del método, en una actualización PARCIAL: lo que no viene, no cambia.
--
-- `sqlc.narg` y no tres parámetros obligatorios, porque un `bool` sin puntero no distingue "ausente"
-- de "falso": un PATCH que solo quería desactivar el método le habría apagado de paso el interruptor
-- del cajón, sacando su dinero del arqueo. Plata movida por el tipo de dato y no por el negocio.
update payment_methods set
  auto_declare        = coalesce(sqlc.narg('auto_declare'), auto_declare),
  is_active           = coalesce(sqlc.narg('is_active'), is_active),
  affects_cash_drawer = coalesce(sqlc.narg('affects_cash_drawer'), affects_cash_drawer)
where id = $1
returning id, name, kind, affects_cash_drawer, auto_declare, is_active;

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
-- `affects_cash_drawer` se GUARDA aquí y no se vuelve a leer del catálogo: desde la spec 015 ese
-- interruptor se puede cambiar, y un corte que se reagrupa según el flag de hoy es un arqueo que se
-- lee distinto según cuándo lo abras. Es el mismo snapshot que `order_lines.unit_price`.
insert into register_session_totals (session_id, payment_method_id, expected, declared, tips, affects_cash_drawer)
values ($1, $2, $3, $4, $5, $6);

-- name: ListSessions :many
select s.id, s.business_date, s.status, s.opening_cash, s.currency, s.opened_at, s.closed_at, s.notes,
       r.name as register_name,
       ob.name as opened_by_name, cb.name as closed_by_name,
       -- La diferencia total del corte son DOS sumandos desde la spec 015: lo que difieren los
       -- métodos que no tocan el cajón, más la diferencia única del cajón. Los métodos de cajón
       -- guardan `declared = expected`, así que su parte de la primera suma es cero: sin el segundo
       -- sumando, esta pantalla —donde alguien audita— muestra cuadrados los cortes que no cuadran.
       --
       -- Va como SUBCONSULTA CORRELACIONADA y no como join: un turno tiene hasta dos conteos
       -- (apertura y cierre) y N renglones de método, y unir las dos 1:N multiplicaría cada
       -- diferencia por el número de conteos. Es el patrón que ya documentamos para
       -- order_payments/order_lines.
       (coalesce((select sum(difference) from register_session_totals t where t.session_id = s.id), 0)
        + coalesce((select c.difference from session_cash_counts c
                    where c.session_id = s.id and c.moment = 'cierre'), 0))::numeric(10,2) as total_difference
from register_sessions s
join cash_registers r on r.id = s.register_id
join users ob on ob.id = s.opened_by
left join users cb on cb.id = s.closed_by
-- El mismo `where` que CountSessions, y se editan juntos: si divergen, «Página 3 de 2».
where (sqlc.narg('desde')::date is null or s.business_date >= sqlc.narg('desde')::date)
  and (sqlc.narg('hasta')::date is null or s.business_date <= sqlc.narg('hasta')::date)
-- `id` desempata: con dos turnos abiertos en el mismo instante, una página podría repetir uno y
-- saltarse otro.
order by s.opened_at desc, s.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountSessions :one
-- Gemela de ListSessions con su mismo `where`. Sin índice propio a propósito: son uno a tres
-- turnos por día y empresa, y `register_sessions_company` ya deja fuera a las demás empresas.
select count(*) from register_sessions s
where (sqlc.narg('desde')::date is null or s.business_date >= sqlc.narg('desde')::date)
  and (sqlc.narg('hasta')::date is null or s.business_date <= sqlc.narg('hasta')::date);

-- name: GetSession :one
select s.*, r.name as register_name, ob.name as opened_by_name, cb.name as closed_by_name
from register_sessions s
join cash_registers r on r.id = s.register_id
join users ob on ob.id = s.opened_by
left join users cb on cb.id = s.closed_by
where s.id = $1;

-- name: ListSessionTotals :many
-- platform_name viaja igual que en ExpectedByMethodForSession, y por el mismo motivo: es lo que
-- permite subtotalizar por plataforma. Faltaba aquí, así que el subtotal existía en el turno vivo y
-- desaparecía en el histórico — justo cuando llega el depósito de la plataforma y sirve para
-- conciliar.
--
-- `t.affects_cash_drawer` y NO `pm.`: el flag se lee del renglón guardado, no del catálogo de hoy.
-- Desde la spec 015 ese interruptor se puede cambiar, y leerlo en vivo haría que un corte cerrado
-- se reagrupara según la configuración del día en que alguien lo abra — las cifras no cambiarían,
-- pero la forma del reporte sí, y un arqueo que se lee distinto cada vez no se puede auditar.
select t.payment_method_id, pm.name, pm.kind, t.affects_cash_drawer, t.expected, t.declared, t.tips,
       coalesce(dp.name, '') as platform_name,
       (t.declared - t.expected)::numeric(10,2) as difference
from register_session_totals t
join payment_methods pm on pm.id = t.payment_method_id
left join delivery_platforms dp on dp.id = pm.delivery_platform_id
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
--
-- is_refund: la salida de caja de una devolución (spec 029). El corte la presenta en
-- «Devoluciones» de su medio y no en «Salidas de efectivo»; el esperado no cambia, porque el neto
-- de movimientos la sigue restando.
select m.id, m.kind, m.amount, m.concept, m.created_at, u.name as user_name, m.transfer_id, m.expense_id, m.reverses_id,
       exists (select 1 from register_cash_movements x where x.reverses_id = m.id) as reversed,
       exists (select 1 from order_refunds r where r.cash_movement_id = m.id) as is_refund
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
-- Un reverso solo corrige salidas (domain.CanReverse): devuelve al cajón lo que la salida restó.
select coalesce(sum(case when kind in ('entrada', 'reverso') then amount else -amount end), 0)::numeric(10,2) as net
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
-- El nombre de la plataforma viaja para poder subtotalizar por ella sin comparar nombres de
-- método: "Uber Eats en línea" y "Uber Eats efectivo" son la misma plataforma, y deducirlo del
-- texto se rompe el día que alguien renombre un método.
--
-- Dos cifras más desde la spec 031, porque el dinero se clasifica por el turno de CADA movimiento:
--   * `earlier`: lo que este turno cobró de pedidos abiertos en OTRO turno. Ya está en `expected`;
--     viaja aparte para que el corte no lo llame venta suya.
--   * `refunded`: lo que este turno le devolvió al cliente por ese medio SIN sacarlo del cajón
--     (tarjeta, plataformas). Se resta del esperado. Lo que salió del cajón NO va aquí: ya baja por
--     su salida de caja, y restarlo otra vez sería contarlo dos veces.
--
-- Pagos y devoluciones se agregan cada uno por método antes de unirse: son dos 1:N y unirlos
-- multiplicaría las sumas.
with pagos as (
  -- Por register_session_id y no por `created_at >= apertura`. La ventana de tiempo daba el
  -- resultado correcto por COINCIDENCIA: solo la caja principal vende y no puede haber dos turnos
  -- suyos abiertos. El día que exista una segunda caja que cobre, dos turnos traslapados sumarían el
  -- mismo dinero y los dos parecerían cuadrar. El vínculo explícito lo hace correcto por construcción.
  select op.payment_method_id,
         sum(op.amount) as expected,
         sum(op.tip_amount) as tips,
         sum(op.amount) filter (where o.register_session_id is distinct from sqlc.arg(session_id)::bigint) as earlier
    from order_payments op
    join orders o on o.id = op.order_id
   where op.register_session_id = sqlc.arg(session_id)::bigint
   group by op.payment_method_id
), devueltos as (
  -- Partidas por si salieron del cajón (spec 029). Solo `refunded` se resta del esperado; las del
  -- cajón ya bajan por su salida de caja. Las otras tres viajan para PRESENTAR: el corte pone toda
  -- devolución en «Devoluciones» de su medio, venta y propina por separado, igual que Ventas.
  select r.payment_method_id,
         sum(r.amount + r.tip_amount) filter (where r.cash_movement_id is null) as refunded,
         sum(r.tip_amount) filter (where r.cash_movement_id is null) as refunded_tips,
         sum(r.amount + r.tip_amount) filter (where r.cash_movement_id is not null) as drawer_refunded,
         sum(r.tip_amount) filter (where r.cash_movement_id is not null) as drawer_refunded_tips
    from order_refunds r
   where r.register_session_id = sqlc.arg(session_id)::bigint
   group by r.payment_method_id
)
select pm.id as payment_method_id, pm.name, pm.kind, pm.affects_cash_drawer, pm.auto_declare,
       coalesce(dp.name, '') as platform_name,
       coalesce(p.expected, 0)::numeric(10,2) as expected,
       coalesce(p.tips, 0)::numeric(10,2) as tips,
       coalesce(p.earlier, 0)::numeric(10,2) as earlier,
       coalesce(d.refunded, 0)::numeric(10,2) as refunded,
       coalesce(d.refunded_tips, 0)::numeric(10,2) as refunded_tips,
       coalesce(d.drawer_refunded, 0)::numeric(10,2) as drawer_refunded,
       coalesce(d.drawer_refunded_tips, 0)::numeric(10,2) as drawer_refunded_tips
from payment_methods pm
left join delivery_platforms dp on dp.id = pm.delivery_platform_id
left join pagos p on p.payment_method_id = pm.id
left join devueltos d on d.payment_method_id = pm.id
-- Un método que se apaga a media jornada tiene que seguir en el arqueo si ya cobró —o devolvió—
-- en este turno: filtrar solo por activo hacía DESAPARECER del esperado el dinero que ya se movió.
--
-- `or pm.kind = 'efectivo'`: el renglón del efectivo del mostrador NUNCA se cae, aunque esté
-- apagado y no haya cobrado nada. Es el único dueño del fondo de apertura y de los movimientos de
-- caja —quien los suma es el bucle de Go que mira este `kind`—, así que sin su renglón el fondo no
-- tiene dónde vivir: medido, apagar «Efectivo» con $500 de fondo dejaba al cajón esperando $0 con
-- los billetes adentro, y el corte cerraba con $500 de sobrante fantasma.
where pm.is_active or p.payment_method_id is not null or d.payment_method_id is not null or pm.kind = 'efectivo'
order by pm.sort_key;

-- name: CollectedByMethodInOpenSessions :one
-- Cuánto lleva cobrado un método en los turnos que siguen ABIERTOS.
--
-- Es lo que decide si se puede mover su interruptor «va al cajón» (spec 015, FR-017): cambiarlo con
-- dinero ya adentro mueve el esperado del cajón en billetes que están físicamente ahí. Suma sobre
-- todos los turnos abiertos y no solo el de la caja principal: el interruptor es del método, no de
-- una caja, y un turno abierto de otra caja cuenta igual.
--
-- Propinas incluidas: también son dinero que entró por ese método y que el esperado ya cuenta.
select coalesce(sum(op.amount + op.tip_amount), 0)::numeric(10,2) as cobrado
from order_payments op
join register_sessions s on s.id = op.register_session_id
where op.payment_method_id = $1 and s.status = 'abierta';

-- name: GetOpenPrimarySession :one
-- La sesión que habilita cobrar. Es SIEMPRE la de la caja principal: las secundarias (caja fuerte,
-- caja externa) existen para traspasos y gastos, y si una de ellas bastara para vender el efectivo
-- del mostrador caería en un arqueo que no es el suyo.
--
-- Trae también opened_at: es lo que deja avisar que el turno abierto ya no es de hoy. La fecha de
-- negocio del turno NO sirve para eso — dice con qué día se abrió, no cuándo.
select s.id, s.register_id, s.business_date, s.opened_at
from register_sessions s
join cash_registers r on r.id = s.register_id
where s.status = 'abierta' and r.is_primary and r.is_active
  -- La caja principal es una por sucursal (0076). Sin selector, `current_branch_id()` es la única
  -- activa y truena si hay dos: nunca cobra en el turno de otra sucursal.
  and r.branch_id = current_branch_id()
limit 1;

-- name: GetOpenPrimarySessionOfBranch :one
-- La de una sucursal dada. La usa un pedido de plataforma: su sucursal es la de su tienda, no la
-- de la sesión de quien lo acepta.
select s.id, s.register_id, s.business_date, s.opened_at
from register_sessions s
join cash_registers r on r.id = s.register_id
where s.status = 'abierta' and r.is_primary and r.is_active and r.branch_id = $1
limit 1;

-- name: CurrentBranch :one
-- La sucursal de la sesión (0076). Error EGB01 si la empresa tiene cero o más de una activa.
select current_branch_id()::bigint;

-- name: LockOpenPrimarySession :one
-- La misma sesión, pero BLOQUEADA hasta que la transacción del cobro termine.
--
-- Sin el lock, entre leer la sesión y escribir el pago cabe un cierre de caja completo: el corte
-- calcula su esperado, lo persiste en register_session_totals —que es una foto, no se recalcula— y
-- el pago aterriza con el id de esa sesión ya cerrada. Ese dinero no lo espera NINGÚN arqueo: el
-- cerrado ya está escrito, y el siguiente filtra los pagos por su propia sesión. El efectivo está
-- físicamente en el cajón y no figura en los libros.
--
-- `for share` y no `for update`: el cobro no modifica la sesión, solo necesita que nadie la cierre
-- mientras escribe. El cierre sí la actualiza, así que queda esperando en vez de adelantarse.
--
-- El `of s` es obligatorio: sin él Postgres intentaría bloquear también cash_registers, que este
-- cobro no tiene por qué tocar.
select s.id, s.register_id, s.business_date
from register_sessions s
join cash_registers r on r.id = s.register_id
where s.status = 'abierta' and r.is_primary and r.is_active
  -- El MISMO predicado que GetOpenPrimarySession (0076): si divergen, una consulta decide que se
  -- puede cobrar en un turno y la otra graba el pago en el de otra sucursal.
  and r.branch_id = current_branch_id()
limit 1
for share of s;

-- name: SeedBasePaymentMethods :exec
-- Métodos de pago base para una empresa recién creada. Desde 0037 la tabla es per-tenant, así que
-- una empresa nueva nace SIN NINGUNO y no podría cobrar: /payment-methods devolvería vacío y el
-- checkout se quedaría sin botones. Antes los heredaba por ser una tabla global.
--
-- Los de PLATAFORMA quedan fuera a propósito: vender por Uber/DiDi/Rappi exige que ese negocio haya
-- hecho su propia vinculación con la plataforma, y darle tres formas de cobro que no tiene
-- contratadas es peor que no darle ninguna.
-- `is_cash` se escribe explícito: la columna nace en `false` y la 0067 le puso un `check` que
-- exige que solo lo que se cobra en billetes entre al cajón. Sin esto, sembrar «Efectivo» con
-- `affects_cash_drawer` viola la restricción y **una empresa nueva no se puede crear** — lo
-- encontró el propio `check`, en el sembrado de la segunda empresa de los tests de aislamiento.
insert into payment_methods (company_id, name, kind, is_cash, affects_cash_drawer, is_active, sort_key, auto_declare)
values
  ($1, 'Efectivo',           'efectivo',      true,  true,  true, 100, false),
  ($1, 'Tarjeta débito',     'tarjeta',       false, false, true, 200, true),
  ($1, 'Tarjeta crédito',    'tarjeta',       false, false, true, 250, true),
  ($1, 'Transferencia SPEI', 'transferencia', false, false, true, 300, true)
on conflict (company_id, name) do nothing;

-- name: GetBusinessTimezone :one
-- Zona horaria del local, para calcular la FECHA de negocio. La base guarda instantes en UTC; la
-- fecha es una decisión de calendario y depende de dónde está el negocio.
select timezone from business_settings limit 1;

-- name: SeedDeliveryPlatforms :exec
-- Plataformas de reparto de una empresa nueva. Son etiquetas: sin ellas la venta no puede registrar
-- por dónde entró, y ni siquiera existe "Propio" para el reparto del propio negocio.
--
-- El margen nace en 0, NO en 35%. El margen es la decisión de negocio que viene con la vinculación:
-- ese local todavía no tiene contrato con Uber, y estrenarlo cobrando 35% más sería una sorpresa
-- cara. El dueño del sistema lo configura cuando ese negocio lo pide, junto con sus métodos de pago
-- de plataforma — que por la misma razón tampoco se siembran.
--
-- company_id NO se lista: lo pone el DEFAULT desde el GUC del tenant. Tampoco se podría — 0023
-- agregó esa columna con SQL dinámico y para sqlc no existe. Corre dentro de WithTenant.
insert into delivery_platforms (name, price_markup_pct)
values ('Didi', 0), ('Uber Eats', 0), ('Rappi', 0), ('Propio', 0)
on conflict do nothing;

-- name: OpenOrdersInSession :many
-- Pedidos del turno que todavía no terminaron. Bloquean el cierre: un pedido abierto o listo es
-- comida que va a salir y dinero que no se decidió, y si el turno cierra con esos pendientes su
-- venta cae en un arqueo ya firmado y el corte deja de poder cuadrar.
--
-- Solo abierta y lista: cancelada y reembolsada son terminales y no hay nada que entregar; exigir
-- "terminarlas" dejaría al operador sin salida más que dejar la caja abierta.
--
-- El id viaja para que el cierre ofrezca «Abrir» esa cuenta en el POS (spec 030), y el total para
-- que diga de cuánto es cada uno sin tener que abrirlo.
select o.id, o.daily_number, o.folio_name, o.total
from orders o
where o.register_session_id = $1
  and o.status in ('abierta', 'lista')
order by o.daily_number;

-- name: OwingDeliveredOrders :many
-- Pedidos de mostrador ENTREGADOS que todavía deben, de cualquier día y cualquier turno. Bloquean el
-- cierre de la caja principal (no hay fiados: decisión del dueño, 2026-10-09). La pantalla del cierre
-- los lista desde esta MISMA consulta, para que lo que se ve y lo que bloquea no puedan divergir.
--
-- Los de plataforma no entran: los paga la plataforma, no el cliente en la caja. Lo cobrado se
-- pre-agrega en un lateral (orders tiene dos hijas 1:N); el dominio decide con PorCobrar si debe.
select o.id, o.daily_number, o.folio_name, o.total, coalesce(p.pagado, 0)::numeric(12,2) as paid,
       o.written_off_amount
from orders o
left join lateral (
  select sum(op.amount) as pagado from order_payments op where op.order_id = o.id
) p on true
where o.status = 'entregada'
  and o.delivery_platform_id is null
  and o.merged_into_order_id is null
  and o.total - o.written_off_amount > coalesce(p.pagado, 0)
order by o.business_date, o.daily_number;

-- name: UncollectedInSession :one
-- La venta del turno que NINGÚN pago cubre, y en cuántos pedidos está.
--
-- Es la hermana de OpenOrdersInSession: aquélla dice qué comida no ha salido, ésta qué dinero no
-- entró. No bloquea el cierre —entregar sin cobrar es una decisión legítima del negocio— pero el
-- arqueo tiene que nombrarla: sin ella un turno cierra con los diez métodos en diferencia $0.00,
-- porque el arqueo solo compara pagos contra declarado y una venta sin pago no aparece por ningún
-- lado. Medido el 8 de septiembre de 2026: $554.00 en cinco pedidos, invisibles.
--
-- El mismo register_session_id que SessionSales y que el esperado por método: las tres cifras de la
-- pantalla salen del turno, no de una ventana de tiempo.
--
-- Los pagos se pre-agregan en un lateral en vez de unir order_payments directo: orders tiene DOS
-- hijas 1:N (líneas y pagos) y unir cualquiera de ellas a un agregado multiplica las filas.
--
-- Cancelada y reembolsada quedan fuera: su venta no ocurrió, así que no hay dinero que reclamar.
-- Los pagos que cuentan son los de ESTE turno (o sin turno, anteriores al vínculo): un cobro hecho
-- en otro turno es dinero de ese otro corte, que lo explica como «cobro de otros turnos». Contarlo
-- aquí movía lo «sin cobrar» de un corte ya firmado cada vez que otro turno cobraba (spec 031, D12).
-- Lo dado por perdido («cancelar lo que falta», 2026-10-09) NO es «sin cobrar»: tiene su propia
-- cifra en el corte (SessionWrittenOff). Cada peso en un solo renglón.
select coalesce(sum(o.total - o.written_off_amount - coalesce(p.pagado, 0)), 0)::numeric(12,2) as monto,
       count(*)::int as pedidos
from orders o
left join lateral (
  select sum(op.amount) as pagado from order_payments op
   where op.order_id = o.id
     and (op.register_session_id is null or op.register_session_id = o.register_session_id)
) p on true
where o.register_session_id = $1
  and o.status not in ('cancelada', 'reembolsada')
  and o.total - o.written_off_amount > coalesce(p.pagado, 0);

-- name: SessionWrittenOff :one
-- Lo dado por perdido de los pedidos del turno («cancelar lo que falta», 2026-10-09). Ni cobro ni
-- sin cobrar: con esto el corte cierra la resta vendido = cobrado + sin cobrar + perdido.
select coalesce(sum(o.written_off_amount), 0)::numeric(12,2) as monto
from orders o
where o.register_session_id = $1
  and o.status not in ('cancelada', 'reembolsada');

-- name: SessionCashByCashier :many
-- Cuánto cobró cada persona en el turno, separando efectivo de lo demás.
--
-- Existe porque dos estaciones cobran contra el MISMO cajón: partir la caja en dos no serviría —
-- dos arqueos contando el mismo dinero son dos cifras inventadas—, así que la responsabilidad se
-- rastrea por quien cobró, no por el mueble. El dato ya estaba en received_by desde el principio y
-- solo lo usaba el reparto de propinas.
--
-- El efectivo va aparte de lo demás porque es lo único que está físicamente en el cajón: una
-- diferencia de arqueo solo puede venir de esa columna.
--
-- Cancelada y reembolsada quedan fuera: su dinero no entró al cajón. Las propinas también, que son
-- del personal y no ingreso del negocio (ver el principio de dinero de la constitución).
select coalesce(u.name, 'Sin asignar') as cashier,
       coalesce(sum(op.amount) filter (where pm.kind = 'efectivo'), 0)::numeric(12,2) as cash,
       coalesce(sum(op.amount) filter (where pm.kind <> 'efectivo'), 0)::numeric(12,2) as other,
       count(*)::int as payments
from order_payments op
join orders o on o.id = op.order_id
join payment_methods pm on pm.id = op.payment_method_id
left join users u on u.id = op.received_by
where op.register_session_id = $1
  and o.status not in ('cancelada', 'reembolsada')
group by u.name
order by cash desc, other desc;

-- name: AbrioElTurnoPrincipal :one
-- Cuándo abrió el turno vigente de la caja principal. Sin filas si no hay ninguno abierto.
--
-- Dos consultas y no una: escritas juntas, la ausencia de turno viaja como NULL dentro de una fila
-- que siempre existe, y sqlc infiere la nulabilidad copiando la de la COLUMNA —`opened_at` es not
-- null— generando un tipo que no acepta NULL. El escaneo tronaba con la caja cerrada: todas las
-- noches, y en cualquier negocio recién instalado. Así, "no hay turno" es cero filas, que el
-- llamador ya sabe leer.
--
-- El costo de separarlas: entre las dos cabe un cierre de caja, y las respuestas describirían
-- momentos distintos del mismo turno. Se acepta porque esto decide UNA VISTA —hasta cuándo se ve un
-- pedido entregado— y no una transacción; el peor caso es que la lista se recorte un refresco antes
-- o después.
select s.opened_at from register_sessions s
join cash_registers r on r.id = s.register_id
where r.is_primary and s.status = 'abierta'
order by s.opened_at desc limit 1;

-- name: CerroLaCajaPrincipal :one
-- Cuándo se cerró por última vez la caja principal. Sin filas si nunca se ha cerrado.
select s.closed_at from register_sessions s
join cash_registers r on r.id = s.register_id
where r.is_primary and s.closed_at is not null
order by s.closed_at desc limit 1;

-- name: SessionSales :many
-- Las ventas que ESTE corte cobró, para poder verlas desde el corte.
--
-- Por register_session_id y no por ventana de tiempo, igual que ExpectedByMethodForSession y por la
-- misma razón: la ventana deja fuera lo que se cobró tarde y mete lo que no era del turno.
--
-- Existe porque el día de una venta y el turno que la cobró dejaron de coincidir: la fecha la da el
-- reloj y el turno puede cruzar la medianoche. Sin esto no habría dónde ver qué ventas responden por
-- el dinero de un arqueo.
--
-- Trae las canceladas y reembolsadas, con su estado: son parte de lo que pasó en el turno. Lo que NO
-- las incluye es el total, y de eso se encarga la gemela de abajo.
select o.id, o.daily_number, o.folio_name, o.opened_at, o.status, o.service_type,
       o.total, o.refund_amount
from orders o
-- El pedido juntado con otro (spec 027) no es venta ni cancelación del turno: sus productos están
-- en el pedido con el que se juntó. La gemela lleva la misma línea.
where o.register_session_id = $1
  and o.merged_into_order_id is null
order by o.opened_at desc, o.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: CountSessionSales :one
-- La gemela de SessionSales, con el MISMO where. Dos cosas que la pantalla necesita y que la lista
-- recortada no puede dar:
--
--   * cuántas ventas hay EN TOTAL, para que un recorte no se lea como "esto es todo";
--   * cuánto suman las que dejaron ingreso.
--
-- El total excluye canceladas y reembolsadas —su dinero no entró— y no toca las propinas, que son
-- del personal y no ingreso del negocio. La pantalla declara las dos exclusiones: una cifra
-- agregada que no dice qué incluye invita a sumarla con otra.
select count(*)::int as total,
       coalesce(sum(o.total) filter (where o.status not in ('cancelada', 'reembolsada')), 0)::numeric(12,2) as ingreso
from orders o
where o.register_session_id = $1
  and o.merged_into_order_id is null;

-- name: ListDenominations :many
-- Qué piezas se pueden contar en una moneda. Solo las activas: una denominación retirada de
-- circulación no vuelve a ofrecerse, pero sigue existiendo para los arqueos que la usaron.
--
-- De mayor a menor por sort_key, que es como se cuenta un cajón: primero los billetes grandes.
select id, currency, value, is_coin
from cash_denominations
where currency = $1 and is_active
order by sort_key;

-- name: SaveCashCount :one
-- El conteo de un momento del turno. El total viene YA calculado por el dominio desde las piezas:
-- esta consulta no suma nada, y por eso `total` es un parámetro y no un `sum()`.
--
-- Un segundo conteo del mismo momento choca con `session_cash_counts_un_momento` y sube como 23505.
-- El servicio lo traduce a conflicto: con dos tabletas compartiendo cuenta, dos personas pueden
-- llegar al cierre a la vez, y eso tiene que decir qué pasó y no "el servidor se rompió".
--
-- `expected` es el esperado del CAJÓN en el momento del cierre, y se guarda en vez de recalcularse:
-- sale de `order_payments`, y una venta cancelada o reembolsada después movería la cifra contra la
-- que el operador firmó. Va nulo en la apertura, donde no hay nada que esperar.
insert into session_cash_counts (session_id, moment, total, expected, manual_reason, created_by)
values ($1, $2, $3, $4, $5, $6)
returning id, session_id, moment, total, manual_reason, created_by, created_at;

-- name: SaveCashCountLine :exec
-- Un renglón del conteo. Solo se llama con piezas > 0: el cero no genera fila (FR-009), y el
-- `check (pieces > 0)` del esquema está para que eso no dependa de que el servicio se acuerde.
insert into session_cash_count_lines (count_id, denomination_id, pieces)
values ($1, $2, $3);

-- name: GetCashCount :one
-- El conteo de un momento, si lo hay. Un turno sin conteo es lo normal en los cortes anteriores a
-- esta funcionalidad, así que "no hay filas" es una respuesta legítima y no un error.
select id, session_id, moment, total, expected, difference, manual_reason, created_by, created_at
from session_cash_counts
where session_id = $1 and moment = $2;

-- name: ListCashCountLines :many
-- Las piezas de un conteo, con el valor de cada denominación.
--
-- `subtotal` viaja calculado desde la base y no se deja para la pantalla: lo lee un humano
-- comparando contra su cajón, y dos multiplicaciones del mismo dato son dos formas de que difieran.
-- El valor sale del catálogo por join y no de una copia en el renglón: una denominación no cambia
-- de valor —un billete de $500 vale $500—, y lo que sí puede cambiar es que se retire, que es
-- justo lo que `on delete restrict` impide que borre este join.
select l.denomination_id, d.value, d.is_coin, l.pieces,
       (d.value * l.pieces)::numeric(12,2) as subtotal
from session_cash_count_lines l
join cash_denominations d on d.id = l.denomination_id
where l.count_id = $1
order by d.sort_key;

-- name: ListSessionPaymentVoids :many
-- Los pagos devueltos en un turno, para la lista aparte del corte (spec 027). No cambian el
-- esperado por método: el pago devuelto ya no está en order_payments.
select pm.name as method_name, v.amount, v.tip_amount, o.daily_number, coalesce(o.folio_name, '')::text as folio_name,
       coalesce(u.name, '')::text as voided_by, v.voided_at, v.reason
from order_payment_voids v
join payment_methods pm on pm.id = v.payment_method_id
join orders o on o.id = v.order_id
left join users u on u.id = v.voided_by
where v.register_session_id = $1
order by v.voided_at, v.id;

-- name: LockSessionForClose :one
-- El turno que se cierra, BLOQUEADO para todo lo que lo usa (spec 031, D8).
--
-- El cierre calculaba el esperado fuera de su transacción: un cobro, un pago devuelto o un
-- movimiento que confirmaba entre esa lectura y el cierre quedaba en el turno cerrado sin entrar al
-- esperado firmado. Con `for update` lo que ya tenía el turno con `for share` termina antes de que
-- el cierre lea, y lo que llega después espera y lo ve cerrado.
select id from register_sessions where id = $1 and status = 'abierta' for update;

-- name: LockOpenSessionForShare :one
-- Un turno abierto, con candado compartido, para escribir en él un movimiento de caja, un traspaso
-- o el pago de un gasto. Cero filas = ya se cerró (o se está cerrando y terminó primero).
select id from register_sessions where id = $1 and status = 'abierta' for share;

-- name: ListSessionRefunds :many
-- Las devoluciones de un turno, para la lista del corte (spec 031). Una devolución es dinero que se
-- le regresó al cliente; un pago devuelto (ListSessionPaymentVoids) es un cobro que no ocurrió. Son
-- dos hechos y el esperado los trata distinto, por eso son dos listas.
select pm.name as method_name, r.amount, r.tip_amount, o.daily_number, coalesce(o.folio_name, '')::text as folio_name,
       (r.cash_movement_id is not null)::boolean as from_drawer,
       coalesce(u.name, '')::text as refunded_by, r.created_at, r.reason
from order_refunds r
join payment_methods pm on pm.id = r.payment_method_id
join orders o on o.id = r.order_id
left join users u on u.id = r.refunded_by
where r.register_session_id = $1
order by r.created_at, r.id;

-- name: ClaimOrphanRefunds :exec
-- Las devoluciones que se hicieron SIN turno abierto entran al turno que se abre (spec 031, D7).
--
-- Solo las que no tocaron el cajón (las de efectivo sin turno se rechazan), solo las de la sucursal
-- del turno, y solo las hechas DESPUÉS del último cierre de esa caja: las anteriores son de antes de
-- que existiera este vínculo, y su corte ya firmó sin ellas.
--
-- ponytail: con una sola caja que vende por sucursal, «la que abre» es la correcta. Con dos cajas
-- que vendan, la reclama la primera que abra; para entonces la devolución tendría que guardar de
-- qué caja es.
update order_refunds r
   set register_session_id = s.id
  from register_sessions s
  join cash_registers c on c.id = s.register_id,
       orders o
 where s.id = sqlc.arg(session_id)
   and c.is_primary
   and o.id = r.order_id
   and o.branch_id = c.branch_id
   and r.register_session_id is null
   and r.cash_movement_id is null
   and r.created_at >= coalesce((select max(p.closed_at) from register_sessions p
                                  where p.register_id = s.register_id and p.id <> s.id), '-infinity'::timestamptz);

-- name: LastClosingCountOfRegister :one
-- El fondo que dejó el último cierre de esta caja (spec 032, punto 6; decisión del 2026-10-10): lo
-- que se dejó, o en cierres de antes de esa decisión, todo lo contado. Solo lo lee el servidor para
-- decidir si la apertura pide motivo: nunca viaja a la pantalla, o el conteo dejaría de ser a ciegas.
select coalesce(s.float_left, c.total)::numeric(10,2) as total
  from register_sessions s
  join session_cash_counts c on c.session_id = s.id and c.moment = 'cierre'
 where s.register_id = $1 and s.status = 'cerrada'
 order by s.closed_at desc, s.id desc
 limit 1;

-- name: SetFloatLeft :exec
update register_sessions set float_left = $2 where id = $1;

-- name: SetOpeningExtras :one
-- El motivo de una apertura que no coincide con el cierre anterior, y el modo de arqueo de tarjeta
-- de la sucursal copiado al abrir: cambiarlo con la caja abierta no cambia lo que ya se le pide a
-- quien cuenta.
update register_sessions s set opening_reason = sqlc.narg('reason'), opening_reason_note = sqlc.narg('note'),
       card_count_mode = coalesce((select b.card_count_mode from cash_registers r join branches b on b.id = r.branch_id
                                    where r.id = s.register_id), 'auto')
 where s.id = sqlc.arg(id)
returning s.card_count_mode;
