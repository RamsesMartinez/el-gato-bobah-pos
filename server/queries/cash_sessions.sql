-- Turnos de caja: abrir, cerrar, el turno vivo y lo que el cierre necesita leer (spec 082, fase 2:
-- partido de cash.sql sin cambiar ninguna consulta).

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
-- opened_at: la hora movida hacia atrás (spec 048); ausente = ahora.
insert into register_sessions (business_date, opening_cash, opened_by, register_id, opened_at)
values ($1, $2, $3, $4, coalesce(sqlc.narg('opened_at')::timestamptz, now()))
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

-- name: LastCloseOfRegister :one
-- La hora del último cierre de una caja. Ninguna fila = nunca cerró.
select closed_at from register_sessions
where register_id = $1 and status = 'cerrada' and closed_at is not null
order by closed_at desc limit 1;

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
-- fixed_float_at_close: el fijo que la hoja de cierre mostró (spec 035), para saber después si ese
-- día se dejó otra cantidad aunque el fijo haya cambiado.
update register_sessions set float_left = sqlc.arg(float_left), fixed_float_at_close = sqlc.narg(fixed_float_at_close)
 where id = sqlc.arg(id);

-- name: SetOpeningExtras :one
-- El motivo de una apertura que no coincide con el cierre anterior, y el modo de arqueo de tarjeta
-- de la sucursal copiado al abrir: cambiarlo con la caja abierta no cambia lo que ya se le pide a
-- quien cuenta.
update register_sessions s set opening_reason = sqlc.narg('reason'), opening_reason_note = sqlc.narg('note'),
       card_count_mode = coalesce((select b.card_count_mode from cash_registers r join branches b on b.id = r.branch_id
                                    where r.id = s.register_id), 'auto')
 where s.id = sqlc.arg(id)
returning s.card_count_mode;

-- name: IsLatestSessionOfRegister :one
-- ¿Es el último turno de su caja? Mientras lo sea y esté cerrado, lo contado al cerrarlo es el
-- esperado de la siguiente apertura (spec 035, R8).
select not exists (select 1 from register_sessions o
                   where o.register_id = sqlc.arg(register_id) and o.id > sqlc.arg(session_id)::bigint) as latest;

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
-- Cuándo cerró por última vez un turno que vendió (uno con pedidos). Sin filas si nunca.
--
-- No lee `is_primary`: esa columna dice qué caja vende HOY, y la venta se puede pasar a otra caja
-- (spec 049). Preguntarle por un turno pasado haría que pasar la venta moviera este instante y
-- volviera a mostrar los entregados de días cerrados.
select s.closed_at from register_sessions s
where s.closed_at is not null
  and exists (select 1 from orders o where o.register_session_id = s.id)
order by s.closed_at desc limit 1;

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
