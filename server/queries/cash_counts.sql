-- Arqueos: denominaciones y conteos del cajón (spec 082, fase 2: partido de cash.sql).

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
