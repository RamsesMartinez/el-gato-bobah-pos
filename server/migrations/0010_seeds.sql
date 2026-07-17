-- +goose Up
-- Unidades base (to_base = factor a la base del kind: g / ml / pieza)
insert into units (code, name, kind, to_base) values
  ('g',     'Gramo',       'masa',    1),
  ('kg',    'Kilogramo',   'masa',    1000),
  ('ml',    'Mililitro',   'volumen', 1),
  ('l',     'Litro',       'volumen', 1000),
  ('floz',  'Onza líquida','volumen', 29.5735),
  ('cda',   'Cucharada',   'volumen', 15),
  ('cdta',  'Cucharadita', 'volumen', 5),
  ('pieza', 'Pieza',       'pieza',   1);

insert into channels (code, name) values
  ('pos',    'Punto de venta'),
  ('qr',     'Menú QR'),
  ('online', 'Menú online');

insert into payment_methods (name, kind, affects_cash_drawer, sort_key) values
  ('Efectivo',           'efectivo',      true,  100),
  ('Tarjeta',            'tarjeta',       false, 200),
  ('Transferencia SPEI', 'transferencia', false, 300),
  ('Didi',               'plataforma',    false, 400),
  ('Uber Eats',          'plataforma',    false, 500),
  ('Rappi',              'plataforma',    false, 600);

insert into delivery_platforms (name) values
  ('Didi'), ('Uber Eats'), ('Rappi'), ('Propio');

insert into expense_categories (name, financial_group) values
  ('Pago colaboradores', 'administrativo'),
  ('Propinas',           'operacional'),
  ('Insumos',            'operacional'),
  ('Servicios',          'operacional');

-- +goose Down
delete from expense_categories;
delete from delivery_platforms;
delete from payment_methods;
delete from channels;
delete from units;
