# Implementation Plan: El menú de la tableta siempre al día

**Branch**: `034-menu-siempre-al-dia` | **Date**: 2026-10-10 | **Spec**: [spec.md](spec.md)

## Summary

Un contador por empresa (`catalog_versions`) que suben **triggers de sentencia** en las diez tablas
que lee el menú del POS, sin importar quién escribe. La llave del menú en Redis y su `ETag` llevan
ese contador, el día y la forma del documento, así que un menú viejo no se puede servir. La tableta
revalida con `If-None-Match` al volver al frente y cada 60 s (304 sin cuerpo). Si aun así manda un
producto o extra que ya no vale, el servidor responde `MENU_CHANGED` y la tableta recarga el menú,
avisa en neutro y reabre la hoja de extras sin tocar el resto de la cuenta.

## Technical Context

- **Lenguaje**: Go 1.27 (chi, pgx + sqlc, goose embebido) · React 19 + Chakra v3 + TanStack Query.
- **Almacenamiento**: PostgreSQL con RLS por `company_id`; Redis solo caché del menú.
- **Testing**: unitarios en `domain`/`cache`/`httpapi`; integración contra Postgres real bajo
  `appRoleStore` con `inTheThreeCases`; vitest en `web/`.
- **Plataforma**: tabletas 1024×600; el aviso es un toast informativo, sin pantalla nueva.
- **Restricciones**: se despliega con el negocio operando → migración aditiva, API vieja compatible.
- **Siguiente migración libre en develop**: `0089` (la 033 vive en otra rama: verificar al commitear).

## Tablas que alimentan el menú (leídas de `queries/menu.sql` y `platform_prices.sql`)

`categories`, `products`, `product_channels`, `channels`, `product_modifier_groups`,
`modifier_groups`, `modifier_options`, `delivery_platforms`, `product_platform_prices`,
`modifier_option_platform_prices`. Las diez tienen `company_id`. Combos (`combo_slots`) **no** entran
al documento del menú hoy: no se vigilan (se agregan el día que el menú los lea).

## Constitution Check

| Principio | Cómo se cumple | Estado |
| --- | --- | --- |
| I Layering | ETag/llave en `cache` (puro); lectura de versión vía sqlc en `app.MenuService.Version`; handler fino; código de error mapeado solo en `httpapi.Error` | OK |
| II Errores | Sin sentinels nuevos: `ErrProductNotSell`, `ErrOptionNotFound`, `ErrOptionOverMax` cambian de código a `MENU_CHANGED` en `respond.go` | OK |
| III Dinero | No toca dinero | N/A |
| IV Test-first | Bordes enumerados abajo, cada uno con test antes del código; migración con test a dos empresas y `inTheThreeCases` | OK |
| V Seguridad | Ruta existente con su auth; ETag incluye empresa (dos empresas en la misma tableta no comparten respuesta); la función del trigger es `security definer` con `search_path` fijo y solo suma contadores de empresas cuyas filas el que escribe ya pudo tocar | OK |
| VI YAGNI | Se **quitan** las invalidaciones manuales del menú (quedan muertas con la llave versionada); se conserva `menu.updated` en vivo. Sin configuración de la cadencia | OK |
| VII Idioma | Identificadores en inglés (`catalog_versions`, `bump_catalog_version`, `MENU_CHANGED`); texto en español | OK |
| VIII Puertas | Ver abajo | OK |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

Sí: un contador derivado, reconstruible en cualquier momento (empieza en 0 y solo importa que
cambie). **Más de una sucursal**: si el menú llega a ser por sucursal, la versión por empresa sigue
siendo correcta (más invalidaciones, nunca menos). No cierra ninguna puerta.

## Bordes (se piensan antes del código)

| # | Borde | Test |
| --- | --- | --- |
| E1 | Cambio por SQL directo sin `app.company_id` y tocando dos empresas → sube las dos | integración, como owner |
| E2 | Cambio como `gatobobah_app` bajo RLS → sube solo la suya | integración, `appRoleStore` |
| E3 | Sentencia sin filas → no sube | integración |
| E4 | Borrar una empresa (cascada a productos) no truena por la FK del contador | integración |
| E5 | La API vieja (sin leer el contador) sigue escribiendo catálogo | cubierto por E2: los inserts del rol app no requieren grant nuevo |
| E6 | Cambio de día → otro ETag | unitario de `cache.MenuETag` |
| E7 | Dos empresas misma versión → ETag distinto | unitario |
| E8 | `If-None-Match` igual → 304 sin cuerpo; distinto o ausente → 200 con ETag | unitario de handler |
| E9 | Redis caído → arma de la base, mismo ETag | unitario (cache nil) |
| E10 | Revalidación fallida (sin red) no borra el menú ni cambia la pantalla a «Error cargando el menú» | vitest |
| E11 | 304 devuelve el menú que ya tenía la tableta | vitest de `posApi.menu` |
| E12 | `MENU_CHANGED` al agregar → toast info, recarga, reabre hoja; otros renglones intactos | vitest |
| E13 | `MENU_CHANGED` de un producto que ya no está o no tiene extras → no abre hoja | vitest |
| E14 | Varios rechazos seguidos → un solo aviso (toast con id fijo) | vitest |
| E15 | Lectura del contador en `inTheThreeCases` (otra empresa, conexión reciclada, sin empresa) | integración |
| E16 | `RecomputeAll` (mil `update` en una tx) no bloquea cobros: los pedidos no escriben en las diez tablas | revisión de queries (hecha) + test E2 de que solo el catálogo sube |

## Capas

| Pieza | Capa |
| --- | --- |
| Tabla `catalog_versions`, función `bump_catalog_version()` y 30 triggers | `migrations/0089_catalog_versions.sql` |
| `CatalogVersion :one` | `queries/menu.sql` → sqlc |
| `MenuService.Version(ctx)` | `app/menu.go` |
| `MenuETag`, llave versionada; se quita `Invalidate` | `cache/menu.go` |
| 304 condicional en `PosMenu`; se quitan invalidaciones en admin/modifiers/platform_prices/handlers.go/AdminReload (este último conserva popular + recomendador) | `httpapi/` |
| `MENU_CHANGED` | `httpapi/respond.go` |
| `api.getConditional`, `posApi.menu` con ETag | `web/src/api/` |
| `useMenu`: 60 s + foco; `POSPage` no se cae por un refetch fallido | `web/src/hooks`, `features/pos` |
| `onMenuCambiado` en `useCuenta` (agregar y cambiar) y reapertura de la hoja en `POSPage` | `web/src/features/pos` |
| Regla en `AGENTS.md` | docs |

## Project Structure

```text
server/migrations/0089_catalog_versions.sql
server/queries/menu.sql
server/internal/{cache,app,httpapi}/…
server/internal/integration/catalog_versions_test.go
web/src/{api,hooks,features/pos}/…
AGENTS.md
```

## Riesgos

- **Contención**: el trigger toma el candado de una fila por empresa hasta el commit del que
  escribe catálogo. Solo escrituras de catálogo compiten entre sí; cobrar no escribe esas tablas ni
  lee el contador con candado (lectura MVCC).
- **Despliegue**: migrar primero; la API vieja ignora tabla y triggers. La API nueva con caché vieja
  usa otra llave: el primer request arma el menú, una vez.
- **Costo por escritura**: un `insert … on conflict` por sentencia de catálogo; despreciable.

## Complexity Tracking

Sin violaciones.

## Revisión after_plan (db-architect, 2026-10-10)

`tablet-ui-reviewer` no corrió: el plan no agrega pantalla ni cambia disposición (un toast y la
hoja de extras que ya existe). Hallazgos de `db-architect` y cómo quedan:

| # | Hallazgo | Resolución |
| --- | --- | --- |
| B1 | Leer la versión ANTES de armar el documento; usar esa misma para llave y ETag | `PosMenu` lee versión → arma → guarda bajo esa llave. Un commit a medias deja el menú nuevo bajo la llave vieja (servido de más, inofensivo), nunca al revés |
| B2 | `order by company_id` en el upsert (interbloqueo) y `exists` en companies (borrado de empresa) | En la migración; E4 borra una empresa con productos |
| 3 | El día del ETag de la misma fuente que el filtro | `current_date` en la misma consulta que la versión (R-3) |
| 4 | `RecomputeAll` sostiene el candado del contador hasta su commit | Aceptado: solo espera otra escritura de catálogo, nunca un cobro |
| 5 | El alta perezosa del producto genérico de plataforma (0077) escribe `categories`/`products` desde un pedido | Corrige E16: el arranque ya lo siembra, así que en operación es un `select`; si faltara, un pedido sube la versión una vez. Aceptado |
| 6 | `security definer`: `search_path = pg_catalog, public`, nombres calificados, `revoke … from public` | Hecho |
| 7 | Grant explícito y política con `nullif` | Hecho; lo vigila `TestEveryCompanyTableIsIsolated` |
| 9 | `lock_timeout` en la migración | `set local lock_timeout = '3s'` |
| 10 | Down explícito | Borra los 30 triggers, la función y la tabla |

Veredicto tras aplicar: sin bloqueantes abiertos.
