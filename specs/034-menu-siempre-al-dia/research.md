# Research: El menú de la tableta siempre al día

## R-1 Versión automática: triggers de sentencia con tablas de transición
- **Decision**: una función `bump_catalog_version()` `security definer`, disparada `after insert`,
  `after update`, `after delete` `for each statement` con `referencing new table`/`old table` en las
  diez tablas. Suma 1 a cada `company_id` distinto de las filas afectadas (`insert … on conflict do
  update`), saltando empresas que ya no existen.
- **Rationale**: cubre pantalla, scripts y migraciones; deriva la empresa de las filas, así que no
  depende de `app.company_id`; una sentencia de mil filas cuesta un upsert por empresa.
- **Alternatives**: trigger por fila (mil upserts por sentencia); `app.company_id` en el trigger
  (falla con scripts que no lo ponen); invalidar desde Go (es lo que falló); `LISTEN/NOTIFY` (exige
  un proceso escuchando y no sirve a la llave del caché); hash del contenido en cada request (arma
  el menú siempre, que es lo que el caché evita).
- Una declaración con varios eventos no admite tablas de transición en Postgres: van tres triggers
  por tabla.

## R-2 Security definer
- **Decision**: la función corre como el dueño, con `set search_path = public, pg_temp`; el rol
  `gatobobah_app` solo recibe `select` sobre `catalog_versions`.
- **Rationale**: escribir el contador no debe requerir que la API tenga `insert/update` sobre él
  (la API vieja no los tendría igual) ni depender de la política RLS del que escribe. No amplía
  nada: solo cuenta filas que el que escribe ya tocó.

## R-3 ETag y llave
- **Decision**: `"<schema>-<company>-<version>-<YYYYMMDD>"`. La fecha es `current_date` de la misma
  base que filtra la disponibilidad. Misma cadena para la llave de Redis.
- **Alternatives**: hash del cuerpo (exige armar el menú para saber si cambió).

## R-4 Revalidación en la tableta
- **Decision**: `If-None-Match` explícito desde `posApi.menu` con el último ETag y menú en memoria
  del módulo; `refetchInterval: 60_000`, `refetchOnWindowFocus: 'always'` (TanStack escucha
  `visibilitychange`). Un error de refetch deja `data` intacto y la pantalla solo cae a «Error
  cargando el menú» si no hay menú alguno.
- **Alternatives**: caché HTTP del navegador (implícito, la URL es la misma para dos empresas en
  una tableta y no se puede probar en vitest).

## R-5 Código estable
- **Decision**: `MENU_CHANGED` (422) para `ErrProductNotSell`, `ErrOptionNotFound`,
  `ErrOptionOverMax`. La tableta ya respeta el máximo por renglón del menú que tiene, así que un
  exceso solo llega con un menú viejo. Ningún cliente actual lee `UNPROCESSABLE` ni
  `OPTION_OVER_MAX` (verificado con grep en `web/`), así que el cambio es compatible.

## R-6 Invalidaciones manuales
- **Decision**: se quitan (`MenuCache.Invalidate` y sus cinco llamadas). Se conserva la
  publicación `menu.updated`, que da el refresco instantáneo a las otras tabletas.
