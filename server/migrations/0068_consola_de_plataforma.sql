-- +goose Up

-- La consola de plataforma (spec 016): una superficie para quien VENDE y mantiene el producto,
-- sin ningún punto de contacto con la del negocio.
--
-- La decisión que ordena todo: la separación no se impone con un `if`, se impone tres veces —firma
-- distinta del token, rol de base propio y grants explícitos—, y cada barrera detiene sola lo que
-- las otras dejen pasar. Esta migración pone la segunda y la tercera.

set local lock_timeout = '3s';

-- ---------------------------------------------------------------------------------------------
-- 1. El operador de plataforma, en su propia tabla.
-- ---------------------------------------------------------------------------------------------

-- SIN company_id, y eso es el punto. El login del negocio consulta `users`; si el operador no está
-- ahí, no puede encontrarlo NI EXISTIENDO EL MISMO NOMBRE. La separación sale por construcción y no
-- de un `where` que alguien pueda olvidar al escribir la siguiente consulta.
--
-- Sin `pin_hash`: el PIN existe para relevos en el mostrador, y aquí no hay mostrador.
--
-- Sin RLS: RLS particiona filas POR EMPRESA y esta tabla no tiene ese eje. Lo que la protege es que
-- el rol de la aplicación no tiene ningún permiso sobre ella — ver los grants de abajo.
create table platform_operators (
  id            bigserial primary key,
  username      citext      not null unique,
  name          text        not null,
  password_hash text        not null,
  is_active     boolean     not null default true,
  created_at    timestamptz not null default now(),
  -- `updated_at` porque las tres columnas de arriba CAMBIAN —la bandera del binario rota la
  -- contraseña y reactiva al operador— y sin esto no queda rastro de cuándo. Es la misma pareja
  -- que lleva `users`; la asimetría la encontró una revisión de esquema, y agregarla ahora cuesta
  -- una línea porque esta migración todavía no se ha desplegado.
  updated_at    timestamptz not null default now()
);

-- Nace VACÍA a propósito. Una credencial sembrada por una migración es una credencial pública: está
-- en el repositorio, con su hash, para siempre. El primer operador se crea con la bandera del
-- binario, que lee variables de entorno.
--
-- Y la simetría, que hay que cuidar HACIA ADELANTE: el rol de la aplicación no recibe ningún
-- permiso sobre esta tabla, y hoy no hace falta escribirlo porque su `grant` fue puntual (0024) y
-- ocurrió antes de que la tabla existiera. Lo que sí puede romperlo es una migración futura que
-- haga `grant ... on all tables in schema public to gatobobah_app` por comodidad: eso le abriría
-- las credenciales de la plataforma al negocio de un plumazo. Lo vigila
-- TestElRolDeLaAppNoPuedeLeerLosOperadores.

-- ---------------------------------------------------------------------------------------------
-- 2. El rol de base, y lo único que puede leer.
-- ---------------------------------------------------------------------------------------------

-- Sin password aquí, igual que el rol de la aplicación: el bootstrap se lo fija desde
-- PLATFORM_DB_PASSWORD al arrancar. Un LOGIN sin password no se puede conectar hasta entonces.
-- +goose StatementBegin
do $$ begin
  if not exists (select 1 from pg_roles where rolname = 'gatobobah_platform') then
    create role gatobobah_platform;
  end if;
end $$;
-- +goose StatementEnd

-- El `login` va APARTE y sin condición, y eso no es redundancia: el `Down` se lo quita, así que
-- crear el rol "solo si no existe" dejaría la segunda vuelta de `up → down → up` con un rol que
-- existe y no se puede conectar. La API fallaría al arrancar con un error que no menciona ninguna
-- migración. Reaplicar tiene que dejar las cosas como la primera vez.
alter role gatobobah_platform with login;

-- El que se olvida y no se nota: sin `usage` sobre el esquema, los `select` de abajo no sirven de
-- nada. Es el mismo olvido que la 0024 documenta.
grant usage on schema public to gatobobah_platform;

-- LOS PERMISOS SE ESCRIBEN COMO LISTA DE LO PERMITIDO, NUNCA COMO LISTA DE PROHIBICIONES.
--
-- Una lista de prohibiciones se queda corta el día que nace una tabla nueva; una de permisos, no.
-- Es la diferencia entre olvidarse de prohibir —que falla ABIERTO— y olvidarse de permitir, que
-- falla cerrado y se nota en el primer request.
--
-- NUNCA `grant ... on all tables in schema public` para este rol, ni por atajo al agregar las
-- tablas de la spec 017. Escribirlo una sola vez expondría `orders`, `users` y `order_payments` de
-- golpe, que es exactamente lo que las tres barreras existen para impedir.
grant select on companies to gatobobah_platform;
grant select on platform_operators to gatobobah_platform;

-- SOLO LECTURA. No hay un solo `insert`, `update` ni `delete` para este rol, y no es un descuido:
-- la primera versión de la consola no modifica nada de ningún cliente. Construir las acciones de
-- soporte (spec 018) exigirá un cambio DELIBERADO de permisos aquí — que es justo la puerta que uno
-- quiere que cueste abrir.

-- ---------------------------------------------------------------------------------------------
-- 3. Y una política de RLS, porque el grant NO basta.
-- ---------------------------------------------------------------------------------------------

-- `companies` lleva la política `company_self`, que limita cada fila a la empresa de la conexión.
-- RLS TAMBIÉN LE APLICA al rol de plataforma —solo la esquivan los superusuarios—, así que con el
-- `select` de arriba la consola vería UNA empresa de dos, o ninguna. Medido contra una copia real
-- antes de escribir esto.
--
-- Las políticas se suman por comando, así que abrir ésta no toca la del negocio: el rol de la
-- aplicación sigue viendo solo la suya.
--
-- LO QUE NO SE HACE: darle `bypassrls` al rol. Resolvería esto y abriría todo lo demás el día que
-- alguien agregue un grant por comodidad. Por eso el arranque del binario lo rechaza.
create policy plataforma_lee_todas_las_empresas on companies
  for select to gatobobah_platform using (true);

-- +goose Down

-- EL DOWN NO BORRA EL ROL, y esto no es pereza.
--
-- En Postgres los roles son objetos del SERVIDOR, no de cada base. Si el rol tiene permisos en otra
-- base del mismo servidor, `drop role` falla con `dependent_objects_still_exist`, y `drop owned by`
-- no alcanza esos grants porque están fuera de su alcance por diseño. Contado el 2026-09-11: CI y
-- producción tienen 1 base y el borrado funcionaría; la VM de pruebas tiene 2 y la máquina de
-- desarrollo 5, donde truena.
--
-- La 0024 arrastra ese defecto. Aquí no se repite: un `Down` correcto no puede depender de cuántas
-- bases tenga el servidor donde se ejecute.
--
-- Lo que sí hace, y es lo único que importa al revertir: quitarle el acceso. `nologin` corta
-- conexiones NUEVAS de inmediato. Ojo — no corta las ya abiertas; si se revierte por una emergencia
-- de seguridad hay que cerrar también las sesiones vivas.
--
-- Borrar el rol del servidor queda como paso manual, cuando se confirme que ninguna otra base lo
-- usa.
drop policy if exists plataforma_lee_todas_las_empresas on companies;
drop table if exists platform_operators;
-- +goose StatementBegin
do $$ begin
  if exists (select 1 from pg_roles where rolname = 'gatobobah_platform') then
    execute 'revoke all on companies from gatobobah_platform';
    execute 'revoke usage on schema public from gatobobah_platform';
    execute 'alter role gatobobah_platform with nologin';
  end if;
end $$;
-- +goose StatementEnd
