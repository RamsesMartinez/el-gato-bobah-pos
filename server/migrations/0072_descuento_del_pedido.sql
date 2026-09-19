-- +goose Up
-- El descuento que se le hace a un pedido (spec 022).
--
-- La columna del DINERO ya existía: `orders.discount_total` nació en 0007 y ha valido cero desde
-- entonces —la constitución la lista como puerta abierta, "la columna está, la feature no"—. Lo que
-- falta y agrega esta migración es el RASTRO: quién descontó y cuándo.
--
-- El rastro no es adorno. Es el único control que esta feature deja contra el abuso: cualquiera que
-- capture un pedido puede descontar, sin rol ni PIN, porque un pedido de plataforma con promoción
-- llega a cualquier hora y exigir a alguien con rol bloquearía la captura. Lo que sostiene esa
-- decisión es poder saber después quién lo hizo.

-- lock_timeout como en 0040, 0041, 0042 y 0065, y por el mismo motivo: `alter table` toma ACCESS
-- EXCLUSIVE sobre `orders`, la tabla que toca cada venta. El riesgo no es el tamaño sino la COLA.
--
-- Por qué NO se usa aquí el patrón `not valid` + `validate constraint` aparte, que es lo correcto
-- para una tabla grande: goose corre el archivo en UNA sola transacción, así que el ACCESS
-- EXCLUSIVE del `add constraint` se sostiene hasta el commit y el `validate` no evitaría nada.
-- Partirlo en dos migraciones sí daría el beneficio, y no se hace porque `orders` tiene cientos de
-- filas: el escaneo es de milisegundos. El día que sean millones, el patrón correcto es ese y este
-- comentario es la nota de por qué hoy no.
set local lock_timeout = '3s';

alter table orders
  add column discount_set_by bigint references users(id),
  add column discount_set_at timestamptz;

-- Sin `on delete`, igual que `opened_by`, `cancelled_by`, `refunded_by` y `platform_ref_set_by`:
-- los usuarios se desactivan, nunca se borran, y ninguna consulta los borra.

alter table orders
  add constraint orders_descuento_no_negativo check (discount_total >= 0),
  -- El total en cero es legítimo (un pedido descontado al 100 %); negativo no: sería devolverle
  -- dinero a alguien sin que nadie lo haya autorizado, y ningún reporte lo diría.
  add constraint orders_total_no_negativo check (total >= 0),
  -- El dinero y su rastro van juntos: un descuento POSITIVO siempre tiene autor. Anclado en `> 0`
  -- y no en `is not null` porque la columna es `not null default 0`: un pedido sin descuento vale
  -- cero, y cero no tiene a quién responsabilizar.
  --
  -- Sin este check, el rastro sería una promesa de la aplicación: basta que la consulta que quita
  -- un descuento olvide limpiar estas dos columnas, o que una ruta nueva escriba el monto sin el
  -- actor, para que quede dinero descontado sin nadie detrás — y nada fallaría.
  add constraint orders_descuento_con_rastro
    check ((discount_total > 0) = (discount_set_by is not null)),
  add constraint orders_rastro_del_descuento_completo
    check ((discount_set_by is null) = (discount_set_at is null));

-- EL CHECK QUE NO VA, para que nadie lo agregue después creyendo que faltaba:
-- `check (discount_total <= subtotal)` parece obvio y rompería la cancelación de un renglón.
-- Cancelar baja el subtotal y NO baja el descuento (es lo que una persona decidió), así que un
-- pedido de $385 con $50 de descuento al que se le cancela casi todo queda legítimamente con
-- subtotal $20 y descuento $50 — total $0, que es lo que `RecalcOrderTotals` calcula con su
-- `greatest(..., 0)`. Con el check, esa cancelación explotaría en la cara del operador.

-- +goose Down
alter table orders drop constraint if exists orders_rastro_del_descuento_completo;
alter table orders drop constraint if exists orders_descuento_con_rastro;
alter table orders drop constraint if exists orders_total_no_negativo;
alter table orders drop constraint if exists orders_descuento_no_negativo;
alter table orders drop column if exists discount_set_at;
alter table orders drop column if exists discount_set_by;
-- `discount_total` NO se toca: existe desde 0007 y no es de esta migración. Los montos ya
-- capturados se quedan donde están; bajar esta migración apaga la feature, no borra lo cobrado.
