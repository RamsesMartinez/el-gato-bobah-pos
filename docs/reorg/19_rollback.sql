-- Rollback de 19_papas_corte_recto.sql: las deja como estaban, apagadas y con el precio de FUDO.

begin;

update products set price = v.precio, is_active = false, updated_at = now()
from (values
  ('PAP_CH', 35.00),
  ('PAP_M',  45.00),
  ('PAP_G',  55.00),
  ('PAP_J',  70.00)
) as v(sku, precio)
where products.company_id = 2 and products.sku = v.sku;

do $$
declare
  n int;
begin
  select count(*) into n from products
   where company_id = 2 and sku in ('PAP_CH','PAP_M','PAP_G','PAP_J') and not is_active;
  if n <> 4 then
    raise exception 'se esperaban 4 papas apagadas y hay %; se aborta', n;
  end if;
end
$$;

commit;
