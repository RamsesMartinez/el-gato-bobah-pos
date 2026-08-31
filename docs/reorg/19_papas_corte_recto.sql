-- 19 — Papas fritas de corte recto: activarlas y ponerles el precio nuevo.
--
-- POR QUÉ. El equipo reportó que "no estaban en el sistema". Sí estaban: los cuatro productos se
-- migraron completos desde FUDO —SKU, receta, categoría Snacks y su grupo "Sazonador alitas"— pero
-- llegaron con is_active en false y nunca se encendieron. Se verificó que la activación es
-- IDÉNTICA entre la empresa 1 (bobah-pruebas) y la 2 (gatobobah): no se perdió nada al partir las
-- empresas, venían así desde el import.
--
-- Se quedan como CUATRO productos y no como un producto con el tamaño de modificador, que fue lo
-- primero que se planteó. El sistema no tiene el concepto de variante (documentado como pendiente
-- en 16_NOTAS_porciones_recetas.md), los ~40 productos con tamaño del catálogo son productos
-- separados, y colapsarlos en uno perdería el costo y el margen POR TAMAÑO — justo el reporte con
-- el que se detectó esto.
--
-- Precios nuevos contra el costo que ya calcula cada receta:
--   PAP_CH (170g)  $40  costo 23.40  margen 41.5 %
--   PAP_M  (270g)  $52  costo 32.83  margen 36.9 %
--   PAP_G  (350g)  $60  costo 39.09  margen 34.9 %
--   PAP_J  (500g)  $80  costo 52.77  margen 34.0 %
--
-- Los cuatro quedan por encima del margen que tenían en FUDO (36.23 / 30.71 / 32.51 / 28.62 %).
--
-- Aplicar dentro de una transacción; el bloque final aborta si no tocó exactamente 4 filas.

begin;

-- company_id explícito: esto corre como owner, que SALTA RLS. Sin el filtro tocaría también los
-- cuatro gemelos de bobah-pruebas, que deben seguir apagados por ser el histórico de pruebas.
update products set price = v.precio, is_active = true, updated_at = now()
from (values
  ('PAP_CH', 40.00),
  ('PAP_M',  52.00),
  ('PAP_G',  60.00),
  ('PAP_J',  80.00)
) as v(sku, precio)
where products.company_id = 2 and products.sku = v.sku;

do $$
declare
  n int;
begin
  select count(*) into n from products
   where company_id = 2 and sku in ('PAP_CH','PAP_M','PAP_G','PAP_J')
     and is_active and price in (40.00, 52.00, 60.00, 80.00);
  if n <> 4 then
    raise exception 'se esperaban 4 papas activas con precio nuevo y hay %; se aborta sin tocar nada', n;
  end if;
end
$$;

commit;
