# Implementation Plan: Emparejar la tienda de plataforma conectada

**Branch**: `026-emparejar-plataformas` | **Date**: 2026-10-03 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/026-emparejar-plataformas/spec.md`

## Summary

Se rehace la pantalla de emparejar con el diseño B: la lista a la izquierda, filtrada por grupo con
sus conteos y un buscador, y el panel a la derecha. «Por revisar» tiene dos modos, uno por uno y en
lote.

En la base:
- Una pareja puede apuntar a una **opción** del POS (hoy guarda el id de la opción como si fuera
  producto).
- Se marca cuál pareja da el **precio de captura** cuando varias van al mismo producto.
- Se guardan las decisiones «solo existe en Uber» y «no se vende en Uber».
- Cada precio por plataforma sabe **quién lo puso**: a mano o la lectura del menú.

Tras cada lectura buena, la lectura sincroniza los precios de lo emparejado y deja escrito qué
cambió. La captura a mano de un precio de plataforma conectada se rechaza en el servidor y se ve
bloqueada en el POS. Una migración (0077) y nada se escribe en Uber.

## Technical Context

**Language/Version**: Go 1.27 (server); TypeScript + React 19 + Chakra v3 (web)

**Primary Dependencies**: pgx + sqlc, goose; TanStack Query. Sin dependencias nuevas.

**Storage**: PostgreSQL con RLS por empresa

**Testing**: `go test ./...` (unitarios en `domain`, integración bajo `appRoleStore` con
`inTheThreeCases`); vitest para las pantallas; e2e contra el ambiente de pruebas a 1024×600

**Target Platform**: VM Linux; tableta de 7 a 10 pulgadas (1024×600)

**Project Type**: web-service + web app

**Performance Goals**: la pantalla carga 90 renglones (65 platillos y 25 opciones) sin paginar;
confirmar en lote 12 parejas es una sola petición

**Constraints**: nada escribe en Uber (`solo_lectura.go`); controles de 44 px; sin `<select>` nativo;
nadie decide nada al operar (constitución 1.15.0)

**Scale/Scope**: 1 migración, 3 tablas nuevas pequeñas, 2 columnas nuevas en tablas existentes,
1 pantalla rehecha, 1 diálogo del POS bloqueado

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Capas | Esquema en 0077; consultas en `queries/menus_plataforma.sql` y `platform_prices.sql`. Reglas puras en `domain/menu_de_plataforma.go`: parecido para ordenar candidatos, qué grupo le toca a cada renglón, qué precios cambiaron y cuál es el precio de captura. Orquestación y transacciones en `app/menus_de_plataforma.go`. Handlers finos |
| II. Errores | Sentinels nuevos en `domain`: `ErrPlatformPriceManaged` (precio que pone la plataforma), `ErrCapturePriceRequired`, `ErrLinkKindMismatch`. Se mapean en `httpapi.Error` |
| III. Dinero | El precio de Uber llega en centavos y se convierte con `PesosDeCentavos` + `Round2` antes de escribir `numeric`. La sincronización escribe **la copia** que usa la captura a mano; los pedidos por webhook ya traen su precio |
| IV. Pruebas | Primero se escriben los casos de borde (lista abajo), cada uno con su test. Aislamiento de las tablas nuevas en los tres casos. Migración probada sobre un respaldo real con dos empresas |
| V. Seguridad | Rutas de admin y gerente, como hoy. La FK compuesta impide ligar el catálogo de otra empresa, también para opciones. El bloqueo del precio vive en el servidor, no solo en el front |
| VI. YAGNI | Sin escribir en Uber, sin propuestas «difusas» (siguen exactas), sin precio por sucursal |
| VII. Idioma | Tablas, columnas y símbolos nuevos en inglés |
| VIII. Puertas | Ver abajo |

**Puertas (VIII)**:
- **Lista de productos por plataforma**: esta feature la acerca, no la cierra. El platillo de Uber
  con su nombre y precio vive en `platform_menu_items`, y la pareja es la relación uno a varios.
- **Precio por sucursal**: `product_platform_prices` sigue siendo por (producto, plataforma). Con dos
  tiendas de Uber en dos sucursales, la sincronización escribiría el precio de la última que se leyó.
  **Se rechaza la sincronización** si la plataforma tiene más de una tienda conectada en la empresa,
  con un mensaje claro, en vez de mezclar. Partir la llave por sucursal es el cambio que se hará
  cuando haya una segunda tienda. Se puede agregar después al mismo costo: es una columna en una
  tabla de copias que se rellena con la siguiente lectura.

### Casos de borde, antes del código

1. Una opción de Uber ligada a un **producto**, o un platillo ligado a una **opción**: se rechaza
   (`ErrLinkKindMismatch`).
2. La opción del POS de **otra empresa**: la FK compuesta lo rechaza (23503), también como owner.
3. Varios platillos al mismo producto (o varias opciones a la misma opción) **sin** precio de
   captura elegido: la segunda pareja exige elegirlo. Al quitar la pareja que lo tenía, pasa sola a otra (la más reciente), sin preguntar.
4. **Lectura fallida o vacía**: no toca un solo precio.
5. Un platillo emparejado que **desaparece** de Uber: la pareja se conserva y su precio no se borra.
6. **Precio de Uber en cero** (platillo gratis o agotado): no se sincroniza, porque `product_platform_prices`
   exige `price > 0`. Se reporta.
7. **Dos personas** confirman la misma propuesta a la vez: el lote es idempotente (`on conflict`) y el
   segundo ve el estado nuevo.
8. **Capturar a mano** el precio de un producto que lo pone la plataforma: 409 del servidor aunque el
   front esté desactualizado.
9. **Pedido aceptado** con una opción ligada: hoy `pedidos_de_plataforma.go:419` trata toda pareja
   como producto. Debe filtrar `local_kind = producto`.
10. **Decisión «solo existe en Uber»** sobre un platillo que después se empareja: la pareja gana y la
    decisión se borra en la misma transacción.
11. **Pareja de opción guardada como producto** por el defecto actual de `GuardarPareja`: la
    migración aborta si existe alguna (ver data-model).
12. **Un script de `docs/reorg/`** que escriba directo en las tablas de precios se salta el bloqueo:
    revisarlos antes de aplicar.

## Diseño (Phase 0)

1. **Pareja a opción**: `platform_item_links.modifier_option_id`, con FK compuesta a
   `modifier_options`. `product_id` pasa a nulable, y un `check` exige exactamente uno de los dos,
   según `local_kind`.
2. **Precio de captura**: `platform_item_links.is_capture_price`, con un único parcial por
   (tienda, producto). Lo pone la pantalla al crear la primera pareja de un producto, y la pantalla
   obliga a elegirlo al crear la segunda.
3. **Decisiones**: `platform_item_exclusions` (platillo u opción de la tienda: «solo existe en Uber»)
   y `local_item_exclusions` (producto u opción del POS: «no se vende en Uber»), por tienda.
4. **Quién puso el precio**: `product_platform_prices.source` (`manual`|`platform`) con `synced_at`,
   y lo mismo en `modifier_option_platform_prices`. `UpsertPlatformPrice` a mano rechaza la fila con
   `source = platform`.
5. **Sincronización**: al terminar una lectura buena, en la misma goroutine y en una transacción, se
   calculan los cambios con una función pura (`domain.PriceSync`) y se escriben. Los cambios quedan
   en `platform_price_changes` (lectura, renglón, precio anterior, precio nuevo) para el aviso «qué
   cambió».
6. **Candidatos ordenados por parecido**: `domain.RankCandidates`, por tokens en común sobre el
   nombre normalizado. No propone nada; solo ordena el buscador.
7. **Confirmar en lote**: `POST /connections/{id}/links/batch`, una transacción, idempotente.
8. **Pantalla**: `EmparejarPage` se rehace en tres piezas. `PairingList` (la lista, los grupos, el
   buscador y el selector de modo), `ReviewPanel` (uno por uno) y `BatchPanel` (lote).
   `CorrectPanel` corrige una pareja confirmada. En vertical, el panel se abre como hoja inferior.
   La conexión y las llaves siguen en `PlataformasPage`; el trabajo diario va en su ruta.
9. **Formato de precio**: `moneyExact` en `utils/format.ts`, con 2 decimales fijos, para las
   pantallas de plataforma. No se cambia `money()` porque lo usan pantallas donde «$45» es lo
   correcto.
10. **La sincronización corre dentro del mismo `WithTenant` de `correrLectura`**: la goroutine nace
    con `context.Background()`, que no trae empresa; fuera de esa transacción RLS no deja escribir.
11. **Un solo cálculo de grupos**: el conteo de cada grupo y el «N de M emparejados» del encabezado
    salen de la misma función de `domain` que asigna el grupo a cada renglón (principio III: lista y
    resumen del mismo predicado).
12. **Pantalla a 1024×600** (revisión de tableta): todos los toggles (grupo, platillos/opciones,
    modo) a 44 px desde una sola constante (los tableros B1 los dibujaron a 42); la lista lleva su
    propio scroll con el patrón `Page fill` + `flex="1" minH={0}` y un test de contenedor como el de
    `MenuDePlataformaPage.test.tsx`; el aviso «Los precios los pone Uber» vive en el encabezado y se
    ve en todos los grupos y modos. El paso de elegir el precio de captura está dibujado en el
    tablero B5 del lienzo.
13. **Deshacer un lote**: no hay deshacer del lote completo; una pareja mal confirmada se quita
    desde «Listos». Aceptado porque el lote se revisa antes con los dos nombres a la vista.
14. **Producto genérico** (historia 6): `products.system_kind = 'platform_unpaired'`, uno por
    empresa, inactivo y con `needs_prep`. `copiarRenglones` lo usa cuando el renglón no tiene pareja
    y escribe las opciones en la nota. Verificar con el ticket (`web/src/features/pos/Ticket.tsx`)
    que un producto inactivo sí se imprime en la comanda; si el ticket filtra por activo, se ajusta.

## Project Structure

### Documentation

```text
specs/026-emparejar-plataformas/
├── plan.md
├── data-model.md
├── contracts/api.md
├── quickstart.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0077_platform_pairing.sql
├── queries/menus_plataforma.sql, platform_prices.sql
├── internal/domain/menu_de_plataforma.go (+ _test)    # RankCandidates, PriceSync, grupos, captura
├── internal/app/menus_de_plataforma.go                 # sincronización, lote, decisiones
├── internal/app/pedidos_de_plataforma.go               # solo parejas de producto
├── internal/app/platform_prices.go                     # rechazo del precio que pone la plataforma
├── internal/httpapi/handlers_menus_plataforma.go, router.go, respond.go
└── internal/integration/platform_pairing_test.go, platform_pairing_migration_test.go
web/src/
├── features/admin/EmparejarPage.tsx (+ PairingList, ReviewPanel, BatchPanel, CorrectPanel, tests)
├── features/pos/PlatformPriceDialog.tsx, ModifierSheet.tsx    # bloqueado con «Lo pone Uber»
├── api/plataformas.ts, utils/format.ts
```
