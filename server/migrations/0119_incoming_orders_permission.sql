-- +goose Up
-- ---------------------------------------------------------------------------------------------
-- Pedidos entrantes (spec 093): ver, aceptar y rechazar lo que llega de fuera (hoy, los pedidos de
-- plataforma; después, el pedido web) pasa a tener permiso propio.
--
-- Hasta hoy esas rutas no tenían gate: cualquier sesión veía y aceptaba. Por eso el permiso llega
-- por herencia a los cuatro sugeridos, y a los roles PROPIOS que ya existen se les da una vez: un
-- rol propio no hereda permisos nuevos (constitución), y sin esto sus usuarios dejarían de oír los
-- pedidos al subir. Los roles propios que se creen después no lo traen.
--
-- `company_id` va explícito: su default sale de la sesión (`app.company_id`), y en una migración no
-- hay sesión. El owner salta RLS (0108 no usa `force`), así que no hace falta fijarla.
--
-- Solo agrega filas: la API anterior, durante el traslape, ignora un permiso que no conoce.
-- ---------------------------------------------------------------------------------------------
insert into permissions (id, code, area) values (39, 'orders.accept_incoming', 'orders');
insert into system_role_permissions (system_role_id, permission_id) values (1, 39), (2, 39), (3, 39), (4, 39);
insert into role_permission_overrides (company_id, role_id, permission_id, granted)
select r.company_id, r.id, 39, true from roles r where r.system_role_id is null
on conflict do nothing;

-- +goose Down
-- Se niega si una empresa le QUITÓ el permiso a un rol: bajar borraría esa decisión, y volver a
-- subir no la repone. Los `granted = true` de roles propios sí los repone el Up.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from role_permission_overrides where permission_id = 39 and not granted) then
    raise exception 'una empresa le quitó orders.accept_incoming a un rol: bajar borraría esa decisión';
  end if;
end $$;
-- +goose StatementEnd
delete from role_permission_overrides where permission_id = 39;
delete from system_role_permissions where permission_id = 39;
delete from permissions where id = 39;
