-- Consultas de la CONSOLA DE PLATAFORMA (spec 016).
--
-- Todas corren sobre la conexión del rol `gatobobah_platform`, que solo tiene `select` sobre
-- `companies` y `platform_operators`. Si alguien agrega aquí una consulta sobre una tabla de
-- operación, no va a compilar mal ni a devolver datos de más: va a fallar con 42501 en el primer
-- request, que es exactamente lo que se quiere.

-- name: GetPlatformOperatorByUsername :one
-- El login de la consola. Trae también `is_active` en vez de filtrarlo en el WHERE: el servicio
-- necesita distinguir "no existe" de "está apagado" para gastar el mismo tiempo en los dos casos,
-- y responder lo mismo. Filtrarlo aquí haría que la rama del apagado saliera en microsegundos.
select id, username, name, password_hash, is_active
from platform_operators
where username = $1;

-- name: GetPlatformOperatorByID :one
-- La usa el middleware en CADA request: retirarle el acceso a un operador tiene que morder en el
-- siguiente, no cuando caduque su token. Es una lectura por PK sobre una tabla de una o dos filas.
select id, username, name, is_active
from platform_operators
where id = $1;

-- name: ListCompaniesForPlatform :many
-- El catálogo de clientes. Ni una cifra de dinero (FR-008) y ni un dato de los empleados de un
-- cliente (FR-016): lo que no se selecciona no se puede filtrar mal después.
--
-- Ve TODAS las empresas gracias a la política `plataforma_lee_todas_las_empresas`; el `select` a
-- secas devolvería una sola, porque `companies` lleva RLS y también le aplica a este rol.
select id, slug, name, is_active, created_at
from companies
order by name;

-- name: UpsertPlatformOperator :one
-- La escribe el DUEÑO de la base desde la bandera del binario, nunca el rol de la consola: éste no
-- tiene `insert` ni `update` sobre su propia tabla, y esa omisión es deliberada.
--
-- Reactiva al operador si estaba apagado, porque es una herramienta de recuperación: quien corre
-- esto tiene acceso al servidor y a las variables de entorno, y lo que quiere es volver a entrar.
insert into platform_operators (username, name, password_hash)
values ($1, $2, $3)
on conflict (username) do update
  set name = excluded.name,
      password_hash = excluded.password_hash,
      is_active = true,
      updated_at = now()
returning id;
