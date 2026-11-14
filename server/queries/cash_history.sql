-- Histórico y detalle de cortes (spec 082, fase 2: partido de cash.sql). ListSessions y CountSessions
-- comparten `where` y se editan juntos.

-- name: ListSessions :many
select s.id, s.business_date, s.status, s.opening_cash, s.currency, s.opened_at, s.closed_at, s.notes,
       r.name as register_name,
       ob.name as opened_by_name, cb.name as closed_by_name,
       -- Las PIEZAS de la diferencia, no la suma: la suma la hace domain.CutDifference, la misma
       -- función que el detalle y el cierre (#290). Sumaba aquí todas las diferencias por método más
       -- la del conteo, y el diálogo de cierre sumaba otra cosa: el mismo corte salía con dos cifras.
       --
       -- Subconsultas correlacionadas y no un join a los renglones: un turno tiene N renglones de
       -- método, y unir eso con otra 1:N multiplicaría las sumas. El conteo de cierre sí va como
       -- left join porque es uno solo por turno (session_cash_counts_un_momento).
       coalesce((select sum(t.declared - t.expected) from register_session_totals t
                 where t.session_id = s.id and not t.affects_cash_drawer), 0)::numeric(10,2) as off_drawer_difference,
       coalesce((select sum(t.declared - t.expected) from register_session_totals t
                 where t.session_id = s.id and t.affects_cash_drawer), 0)::numeric(10,2) as on_drawer_difference,
       cc.expected as drawer_expected, cc.total as drawer_counted,
       s.opening_reason, s.opening_reason_note,
       -- El turno siguiente de la MISMA caja y su motivo de apertura: es lo que explica la diferencia
       -- de este corte (#290). Lateral y no una ventana `lead`: la ventana corre después del filtro y
       -- del `limit`, y la última fila de cada página se quedaría sin su turno siguiente.
       -- ponytail: sin índice (register_id, id); son 1 a 3 turnos por caja y día, el mismo costo que
       -- ya paga is_latest_of_register. Si el Histórico se vuelve lento, ese índice es el siguiente paso.
       -- 0 = sin turno siguiente. coalesce y no NULL: sqlc tipa la columna del lateral como no nula
       -- y un NULL tumbaría la consulta entera (AGENTS.md, «un agregado que se puede vaciar»).
       coalesce(nx.id, 0)::bigint as next_id, nx.opening_reason as next_opening_reason, nx.opening_reason_note as next_opening_reason_note,
       -- El último turno de su caja: con arqueo ciego, su arqueo no se le muestra al cajero hasta
       -- que la caja se vuelve a abrir (spec 035, R8).
       not exists (select 1 from register_sessions o
                   where o.register_id = s.register_id and o.id > s.id) as is_latest_of_register
from register_sessions s
join cash_registers r on r.id = s.register_id
join users ob on ob.id = s.opened_by
left join users cb on cb.id = s.closed_by
left join session_cash_counts cc on cc.session_id = s.id and cc.moment = 'cierre'
left join lateral (select o.id, o.opening_reason, o.opening_reason_note from register_sessions o
                   where o.register_id = s.register_id and o.id > s.id order by o.id limit 1) nx on true
-- El mismo `where` que CountSessions, y se editan juntos: si divergen, «Página 3 de 2».
where (sqlc.narg('desde')::date is null or s.business_date >= sqlc.narg('desde')::date)
  and (sqlc.narg('hasta')::date is null or s.business_date <= sqlc.narg('hasta')::date)
-- `id` desempata: con dos turnos abiertos en el mismo instante, una página podría repetir uno y
-- saltarse otro.
order by s.opened_at desc, s.id desc
limit sqlc.arg('lim') offset sqlc.arg('off');

-- name: GetSessionNeighbors :one
-- Los vecinos de un turno en SU caja, para el detalle (#290): el corte cerrado contra el que comparó
-- su apertura y el turno siguiente con su motivo. Mismo criterio que ListSessions.
-- 0 = no hay; coalesce por la misma razón que en ListSessions.
select coalesce((select p.id from register_sessions p
                 where p.register_id = s.register_id and p.id < s.id and p.status = 'cerrada'
                 order by p.id desc limit 1), 0)::bigint as previous_id,
       coalesce(nx.id, 0)::bigint as next_id, nx.opening_reason as next_opening_reason, nx.opening_reason_note as next_opening_reason_note
from register_sessions s
left join lateral (select o.id, o.opening_reason, o.opening_reason_note from register_sessions o
                   where o.register_id = s.register_id and o.id > s.id order by o.id limit 1) nx on true
where s.id = $1;

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

-- name: ListExpensesBySession :many
-- Gastos atribuidos a un corte (efectivo y no-efectivo): sección "Gastos" del resumen del corte.
--
-- La atribución al corte pasó del encabezado del gasto a CADA PAGO (0029): un gasto con pago
-- partido puede tocar dos cortes distintos, así que lo que se lista es el PAGO, y el importe
-- que se muestra es el del pago, no el del gasto completo.
select ep.id, e.id as expense_id, ec.name as category, s.name as supplier,
       pm.name as payment_method, ep.amount, e.currency, e.status, ep.drawer_extra
from expense_payments ep
join expenses e on e.id = ep.expense_id
join expense_categories ec on ec.id = e.category_id
join payment_methods pm on pm.id = ep.payment_method_id
left join suppliers s on s.id = e.supplier_id
where ep.register_session_id = $1
order by ep.id;

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
