-- +goose Up
-- Completa el backfill de 0045, que se dejó fuera los pedidos reembolsados.
--
-- Un reembolso solo se hace sobre un pedido YA ENTREGADO: la comida salió de la cocina y el
-- cliente se la llevó, y por eso reembolsar no repone stock —el costo ya consumido ES la pérdida—.
-- Sus renglones tienen que decir que se entregaron. Con 0045 quedaron en cero, o sea afirmando lo
-- contrario de lo que pasó.
--
-- No rompe nada hoy: un pedido reembolsado es terminal y ninguna pantalla le mira los renglones.
-- Se arregla porque un dato que miente en silencio es el que alguien lee como verdadero cuando por
-- fin construya el reporte que lo consulta, y para entonces nadie recordará que este hueco existió.
--
-- Solo toca lo histórico. De aquí en adelante llegan bien: un pedido pasa por 'entregada' —con sus
-- renglones ya marcados— antes de poder reembolsarse.
update order_lines l
   set delivered_qty = l.quantity
  from orders o
 where o.id = l.order_id
   and o.status = 'reembolsada'
   and l.cancelled_at is null
   and l.delivered_qty < l.quantity;

-- +goose Down
-- Sin vuelta atrás: no se puede distinguir lo que puso esta migración de lo que marcó el operador
-- entregando renglón a renglón. Poner en cero los reembolsados borraría entregas reales.
select 1;
