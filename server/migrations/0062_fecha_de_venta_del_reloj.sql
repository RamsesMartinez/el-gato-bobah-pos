-- +goose Up
-- LA FECHA DE UNA VENTA SIGNIFICA LO MISMO EN TODO EL HISTÓRICO.
--
-- 0061 y el cambio de servicio hacen que de aquí en adelante `orders.business_date` sea el día en
-- que la venta ocurrió, en la zona del local. Las ventas anteriores traen el día del TURNO que las
-- cobró, que casi siempre es el mismo pero no siempre. Sin esta corrección, la columna significa una
-- cosa antes de cierto día y otra después, y nadie que lea un reporte sabe cuál de las dos.
--
-- Medido contra los datos reales ANTES de escribirla:
--   * El Gato Bobah (el negocio en operación): 0 de 31 filas cambian.
--   * Bobah Pruebas (misma base): 2 de 61.
--   * Ambiente de pruebas: 158, todas del turno que quedó abierto del 31-ago al 4-sep.
--
-- NO MUEVE UN PESO DE UN ARQUEO A OTRO, y eso no es una esperanza: el arqueo agrupa por
-- `register_session_id` y no lee esta columna (ver el comentario de ExpectedByMethodForSession en
-- queries/cash.sql). Tampoco toca `daily_number`, `folio_name` ni el turno.

-- El respaldo es lo que hace REVERSIBLE una corrección de datos. Sin él, el Down sería una promesa
-- que nadie puede cumplir — y este repo ya tuvo una migración que declaraba "sin rollback" y
-- resultó ser un bloqueador al descubrirse tarde.
--
-- Guarda también la fecha nueva para que el update de abajo NO tenga que repetir la expresión: dos
-- copias de la misma regla se desincronizan, y aquí la que se quedara vieja decidiría de qué día es
-- una venta.
--
-- Sin RLS y sin grants: no la lee la aplicación, solo las migraciones. Un grant que nadie usa es
-- superficie que alguien acaba usando.
create table orders_business_date_fix (
  order_id       bigint primary key references orders(id) on delete cascade,
  previous_date  date not null,
  corrected_date date not null
);

insert into orders_business_date_fix (order_id, previous_date, corrected_date)
select o.id, o.business_date, dia.d
from orders o
cross join lateral (
  -- La zona del local, cayendo al default del PRODUCTO y no a UTC. Caer a UTC correría la fecha
  -- seis horas en silencio, que es exactamente el modo de fallo que esta feature viene a cerrar.
  select (o.opened_at at time zone coalesce(nullif(
            (select bs.timezone from business_settings bs where bs.company_id = o.company_id), ''),
            'America/Mexico_City'))::date as d
) dia
where o.business_date <> dia.d;

update orders o set business_date = f.corrected_date
from orders_business_date_fix f
where f.order_id = o.id;

-- +goose Down
update orders o set business_date = f.previous_date
from orders_business_date_fix f
where f.order_id = o.id;

drop table if exists orders_business_date_fix;
