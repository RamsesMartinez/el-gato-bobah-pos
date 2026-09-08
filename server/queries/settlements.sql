-- La liquidación de un pedido de plataforma: lo que dice el documento de pago que llega días
-- después de la venta.
--
-- NINGUNA de estas consultas toca un total de venta, un corte ni un arqueo, y no es casualidad: la
-- comisión es dinero que el negocio vendió y no recibió, pero restarla de una venta reescribiría lo
-- que el POS cobró. Son cifras hermanas y separadas, y así se piden.
--
-- Sin filtro de company_id: RLS lo agrega para el rol de la app.

-- name: UpsertSettlement :one
-- Registrar o RE-registrar la liquidación desde un documento corregido.
--
-- `on conflict (order_id) do update` y no un borra-e-inserta: la PK garantiza "a lo más una por
-- pedido" y el upsert lo resuelve en una sola instrucción, sin la ventana en la que el pedido se
-- queda sin liquidación. `captured_at` se REESCRIBE a propósito: lo que vale es la última lectura
-- del documento vigente, no la primera.
insert into platform_settlements (
  order_id, reported_gross, commission_amount, commission_pct,
  discount_total, discount_platform, withholdings, net_amount,
  payout_reference, document_ref, captured_by
) values (
  @order_id, @reported_gross, @commission_amount, sqlc.narg('commission_pct'),
  @discount_total, @discount_platform, @withholdings, @net_amount,
  sqlc.narg('payout_reference'), sqlc.narg('document_ref'), @captured_by
)
on conflict (order_id) do update set
  reported_gross    = excluded.reported_gross,
  commission_amount = excluded.commission_amount,
  commission_pct    = excluded.commission_pct,
  discount_total    = excluded.discount_total,
  discount_platform = excluded.discount_platform,
  withholdings      = excluded.withholdings,
  net_amount        = excluded.net_amount,
  payout_reference  = excluded.payout_reference,
  document_ref      = excluded.document_ref,
  captured_by       = excluded.captured_by,
  captured_at       = now()
returning *;

-- name: GetSettlement :one
-- Sin fila = "todavía no llega el documento". Es distinto de una liquidación en ceros, que es "la
-- plataforma no cobró nada", y por eso el que llama traduce el ErrNoRows a un 404 en vez de
-- devolver ceros: los dos casos se ven igual si se colapsan.
select ps.*, u.name as captured_by_name
from platform_settlements ps
left join users u on u.id = ps.captured_by
where ps.order_id = $1;

-- name: PlatformSettlementSummary :one
-- Las TRES cifras de un periodo, en una consulta.
--
-- El `left join` es 1:1 (la PK de la liquidación es el order_id), así que aquí NO aplica la regla de
-- no unir dos 1:N: no hay filas que multiplicar. Los pedidos sin liquidación entran igual y su
-- ausencia se cuenta, que es lo que hace que las tres cifras declaren conjuntos distintos.
--
-- Canceladas y reembolsadas quedan fuera de lo VENDIDO por la misma regla que el resumen de Ventas:
-- son ingreso que no ocurrió. Su liquidación, si existiera, tampoco cuenta — no se puede haber
-- cobrado comisión de una venta que no pasó.
select
  count(*)::int                                                        as pedidos,
  coalesce(sum(o.total), 0)::numeric(12,2)                             as vendido,
  count(ps.order_id)::int                                              as liquidados,
  coalesce(sum(ps.commission_amount), 0)::numeric(12,2)                as comision,
  coalesce(sum(ps.withholdings), 0)::numeric(12,2)                     as retenciones,
  coalesce(sum(ps.net_amount), 0)::numeric(12,2)                       as neto,
  count(*) filter (where ps.order_id is null)::int                     as sin_liquidar,
  count(*) filter (where o.platform_order_ref is null)::int            as sin_folio
from orders o
left join platform_settlements ps on ps.order_id = o.id
where o.delivery_platform_id is not null
  and o.business_date between @desde and @hasta
  and o.status not in ('cancelada', 'reembolsada');
