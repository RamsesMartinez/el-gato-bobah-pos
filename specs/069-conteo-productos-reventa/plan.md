# Implementation Plan: Conteo con productos de existencia propia, y quitarles el contador

**Branch**: `069-conteo-productos-reventa` | **Date**: 2026-11-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/069-conteo-productos-reventa/spec.md` (issue #252)

## Summary

1. **Conteo**: los productos con existencia propia entran al conteo de su sucursal en una tabla
   propia de cifras (`stock_count_product_lines`, migración 0110), con la misma mecánica que los
   artículos: a ciegas, cifra guardada al capturar con lo que el sistema tenía, ajuste contra eso al
   cerrar, ligado al conteo. Detalle en [research.md](research.md) R1–R5.
2. **Quitar el contador**: modo nuevo `options` («Lo restan sus opciones») en la hoja de composición
   del producto; sale de la existencia propia por el camino de «No se cuenta» (`TargetNothing`) y el
   servidor exige que alguna opción reste. El modo leído gana `options`, deducido. R6.

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query

**Storage**: PostgreSQL con RLS; una tabla nueva

**Testing**: `go test` (domain, table-driven), integración contra Postgres real bajo `appRoleStore`
con `inTheThreeCases`; vitest en web; Playwright a 1024×600 para el recorrido

**Target Platform**: API Linux en VM; tabletas 1024×600

**Project Type**: web (server/ + web/)

**Performance Goals**: el conteo lista ~100 productos más los artículos sin paginar, igual que hoy

**Constraints**: tolerante a la API anterior corriendo (IV); sin `<select>` nativo, controles ≥ 44 px

**Scale/Scope**: 93 productos con existencia propia en la empresa real; 14 con grupos de opciones

## Constitution Check

| Principio | Cómo cumple |
| --- | --- |
| I. Capas | Cálculo del ajuste y del modo en `domain` (`PlanCountClose` generalizado, `ModeOf` con `OptionsDeduct`, `ValidCountPieces`); orquestación y tx en `app` (`stock_counts.go`, `composition.go`); SQL solo en `queries/stock_counts.sql` y `composition.sql` vía sqlc; handlers finos en `handlers_stock_counts.go` |
| II. Errores | Sentinels de `domain` (`ErrValidation`, `ErrConflict`, `ErrNotFound`, `ErrCountNeedsConfirm`); mapeo solo en `httpapi.Error` |
| III. Dinero | `unit_cost` al cerrar es copia de `current_cost`, sin aritmética nueva de dinero; cantidades con `Round4` |
| IV. Test-first | Tests de dominio y de integración vistos en rojo primero, incluidos los dos del criterio de aceptación; aislamiento de la tabla nueva y de las consultas nuevas bajo `appRoleStore` con `inTheThreeCases`; `TestEveryCompanyTableIsIsolated` la recorre sola; migración con su test |
| IV. Migración tolerante | Solo **agrega** una tabla; la API anterior no la lee. Riesgo aceptado y escrito: si la API anterior cierra un conteo durante el traslape, no ajusta productos (se recaptura en el siguiente) |
| V. Seguridad | Mismo permiso `inventory.count` y `menu.configure` de hoy; producto de otra empresa → 404 por RLS + FK compuesta; cantidades validadas en la frontera (entero ≥ 0, máximo de piezas) |
| VI. YAGNI | Sin lugar de conteo para productos, sin reporte de faltante de productos, sin columna de modo |
| VII. Idioma | Identificadores en inglés (`stock_count_product_lines`, `ModeOptions`, `OptionsDeduct`); textos en español |
| VIII. Puertas | Costo y último movimiento se guardan al cerrar (el faltante de productos se agrega después sin hecho perdido). Más de un almacén por sucursal: la cifra cuelga del conteo, que es por sucursal; agregar almacén al conteo sigue costando una columna. Ninguna puerta se cierra |
| Tableta | Las dos pantallas pasan por `/maquetas` y por `tablet-ui-reviewer` |

Sin violaciones; Complexity Tracking vacío.

## Pantallas y maquetas

Dos flujos con disposición cambiada, en pantallas existentes (opciones elegidas en «Maquetas»):

1. **Conteo** ([StockCountTab.tsx](../../web/src/features/backoffice/StockCountTab.tsx)), opción C:
   - **Lista**: los productos con existencia propia van **al final**, después de los lugares, en un
     grupo por su **categoría del menú** («Bebidas · del menú»). En la columna del lugar dicen
     «Producto del menú»; no tienen lugar ni empaques ni pesada. El aviso de cuántos productos hay
     al final **no es un renglón nuevo** (gastaría ~52 px, una fila de la lista): va como botón de
     44 px dentro del renglón de avance que ya existe («3 de 35 contados · 12 productos del menú al
     final ›») y al tocarlo la lista se desplaza al primer grupo de productos. «Producto del menú» se
     pinta apagado, como «Sin contar», para que no parezca el selector de lugar. El buscador y «Uno por uno» los incluyen (van después de los
     artículos).
   - **Captura**: la misma hoja con teclado, en piezas enteras (sin punto decimal).
   - **Vista previa**: primero las diferencias grandes, después el resto de lo contado (productos en la
     misma lista, marcados «· producto»); al final «Sin contar (N): se quedan como están» con los
     nombres detrás de «Ver cuáles» (con 43 nombres el recuadro empujaba las diferencias fuera de la
     pantalla); los que dejaron de contar su existencia, aparte. Los botones quedan fijos al pie.
   - **Resumen al cerrar**: cuenta artículos y productos corregidos y sin contar (el número de «sin
     contar» coincide con el de la vista previa), con plural correcto.
2. **Cambio a «Lo restan sus opciones»** ([CompositionSheet.tsx](../../web/src/shared/CompositionSheet.tsx)),
   opción B: en un producto cuyas opciones restan, la cuarta tarjeta «No se cuenta» se reemplaza por
   «Lo restan sus opciones» (Ej. cada sabor resta su lata). Al elegirla, un recuadro lista cada grupo
   con lo que resta cada opción y avisa si ningún grupo que resta es obligatorio. La nota de
   existencia la dice en palabras de quien opera («Vendiste 4 más de las que tenías anotadas»; con
   positivo, «Tenías 12 anotadas») y que esa cuenta queda en 0 al guardar, y pide contar las piezas de cada
   opción en el próximo conteo; el diálogo de confirmar lo repite y su botón dice «Sí, que lo resten
   sus opciones». Recetas muestra «Lo restan sus opciones».

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Conteo con productos (listar, capturar, vista previa, cierre) | [maqueta](https://claude.ai/artifact/E7abcMXKjRxbGYUawnPaCo) | C — por categoría del menú, al final de la lista (elegida por el dueño el 2026-11-08) | Se reconocen por su categoría y la diferencia del producto se ve en la vista previa sin desplazar. Con los ajustes de la evaluación sin contexto: aviso arriba de la lista de cuántos productos hay al final, y el resumen del cierre cuenta artículos y productos sin contar |
| Cambio a «Lo restan sus opciones» | [maqueta](https://claude.ai/artifact/8jYWyXjTePzDMwmgPLZrkv) | B — «Lo restan sus opciones» ocupa el lugar de «No se cuenta» (elegida por el dueño el 2026-11-08) | Cuatro tarjetas como hoy y desaparece la que ahí sería falsa. Con los ajustes de la evaluación sin contexto: la hoja dice la cifra del sistema y que queda en 0 al guardar, el diálogo de confirmar repite «cuenta cada sabor en el próximo conteo», y el botón dice «Sí, que lo resten sus opciones» |

## Project Structure

### Documentation (this feature)

```text
specs/069-conteo-productos-reventa/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/api.md
└── tasks.md            # /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/0110_count_product_lines.sql          # tabla nueva, RLS, grants
├── queries/stock_counts.sql                         # consultas de cifras de producto
├── queries/composition.sql                          # opciones que restan
├── internal/domain/stock_count.go                   # PlanCountClose genérico, ValidCountPieces
├── internal/domain/consumption_mode.go              # ModeOptions, OptionsDeduct
├── internal/app/stock_counts.go                     # listar/capturar/quitar/preview/cerrar productos
├── internal/app/composition.go                      # mode "options"
├── internal/app/recipes.go, inventory_health.go     # mostrar el modo nuevo
├── internal/httpapi/handlers_stock_counts.go, router.go
└── internal/integration/count_products_test.go, options_mode_test.go
web/src/
├── api/backoffice.ts, api/admin.ts                  # tipos
├── features/backoffice/StockCountTab.tsx            # productos en el conteo
├── shared/CompositionSheet.tsx                      # tarjeta «Lo restan sus opciones»
└── features/admin/RecipesPage.tsx                   # resumen del modo
```

**Structure Decision**: monorepo existente `server/` + `web/`; sin paquetes nuevos.

## Riesgos y notas de subida

- **Traslape de versiones**: la API anterior ignora la tabla nueva. Una tableta con la pantalla nueva
  hablando con la API anterior recibe 404/405 en `product-lines` durante segundos; reintenta.
- **Concurrencia**: capturar asegura la fila de existencia del producto antes de tomarla; el cierre
  toma las existencias de artículos y luego de productos con `nowait` y lee el modo de cada producto
  DESPUÉS de bloquear; si una venta o un cambio de modo las tiene, 409 «vuelve a intentar». Tests de
  integración: producto sin fila + venta concurrente, y cambio de modo a media cerrada (research R8).
- **Down de 0110**: se niega con datos y lleva su renglón en `guardedDownCleanup`.
- **Cierre por la API anterior**: deja renglones con `unit_cost` nulo, que significa «no se ajustó»
  (documentado en `almacen.md`).
- **Docs**: `docs/mecanica/almacen.md` gana una nota de lo que no se ve (tabla aparte, productos
  ignorados al cerrar si cambiaron de modo, modo `options` deducido).

## Complexity Tracking

Sin violaciones.

## Revisión de arquitectura (2026-11-08)

`db-architect`: 0 CRITICAL, 2 HIGH, 4 MEDIUM, 3 LOW. `tablet-ui-reviewer`: sin bloqueantes, 2 MEDIUM.
Todo lo HIGH y MEDIUM quedó incorporado (research R8, «Pantallas y maquetas» y «Riesgos»). Ninguna
puerta del principio VIII se cierra. Veredicto tras incorporarlos: aprobado.

## Prueba con usuario nuevo (2026-11-08)

Agente sin contexto, pantalla real a 1024×600 táctil contra el `make start` del worktree con la base
restaurada del 2026-10-22, más un «Refresco de lata (prueba)» con un grupo obligatorio de sabores
creado solo en esa copia (ningún producto real tiene sabores obligatorios que resten).

- **Conteo**: lo logró en ~16 toques. Verificado en la base: el refresco pasó de −2 a 15 con un
  `ajuste` de +17 ligado al conteo; el Kokomen de −5 a 8 (+13).
- **«Lo restan sus opciones»**: lo logró en ~6 toques. Verificado: el contador del refresco se cerró
  con −15 de «cambio de modo» y el producto ya no cuenta su existencia. No vendió ni abrió pedidos.
- **Corregido por lo que vio**: «sobraban 17» no se entendía → la vista previa dice también lo que
  tenía el sistema («el sistema marcaba 2 piezas de menos»); «esa cuenta queda en 0» se leía como
  perder lo recién contado → «Tus 15 … siguen donde están; el sistema deja de contarlas como producto».
- **Fuera de esta feature** (ya existían): la ficha del producto no avisa al guardar la receta y
  sigue diciendo «Pendiente» hasta reabrirla; «personalizado» y «elige 1–1 · def. 0–1» en los grupos
  no se entienden; el orden de lugares que anuncia el conteo no coincide con el de la lista en los
  datos restaurados.
