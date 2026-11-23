# Implementation Plan: Alerta continua de pedido entrante y sección «Por aceptar»

**Branch**: `093-alerta-de-pedido-entrante` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/093-alerta-de-pedido-entrante/spec.md`

## Summary

Un módulo común de «pedidos entrantes» con contrato propio (`/orders/incoming`, evento
`incoming.changed`, `domain.IncomingOrder`) que hoy despacha a los pedidos de plataforma de la spec
021 y mañana al pedido web (#328). El servidor decide qué está pendiente —incluye ahora el plazo
vencido y la cancelación que manda la plataforma, que hoy se ignora—, y agrega rechazar con motivo.
En el front, una alarma continua montada en el shell (suena en cualquier pantalla), un indicador
global con el número de pendientes y la sección «Por aceptar» en Pedidos, que sustituyen la franja
y la hoja que hoy solo viven en Vender. Permiso nuevo `orders.accept_incoming` por herencia.
Detalle de cada decisión: [research.md](research.md).

## Technical Context

**Language/Version**: Go 1.27.4 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query, Zustand. Sin
dependencias nuevas (el sonido es Web Audio del navegador).

**Storage**: PostgreSQL. Una migración de solo filas (0118); sin tablas ni columnas nuevas.

**Testing**: `go test` (domain, table-driven), integración contra Postgres real bajo `appRoleStore`,
vitest + Testing Library, Playwright contra el `make start` del worktree.

**Target Platform**: equipo de mostrador táctil, 1280×854 mínimo; las maquetas se miden además a
1024×600 por pedido del dueño.

**Project Type**: web (monorepo `server/` + `web/`).

**Performance Goals**: sonar < 5 s desde que el servidor registra el pedido; apagar las demás
tabletas < 5 s.

**Constraints**: un sondeo de 15 s por tableta como red de abajo (journal y e2-micro); la alarma no
puede depender de temporizadores que el navegador frena; sin red la alarma sigue.

**Scale/Scope**: 1 a 3 tabletas por local; hora pico con 3 o más pedidos a la vez.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | `domain/incoming.go` puro (regla de pendiente, orden, motivos y su traducción). `app/incoming_orders.go` orquesta y despacha por origen; `app/pedidos_de_plataforma.go` gana `Rechazar` y la cancelación. `httpapi/handlers_incoming.go` fino. SQL en `queries/pedidos_de_plataforma.sql` + `make sqlc` |
| II. Errores | Sentinels existentes (`ErrPedidoYaDecidido`, `ErrConflict`, `ErrValidation`, `ErrNotFound`); mapeo solo en `httpapi.Error` |
| III. Dinero | El total se copia tal cual de la plataforma, como hoy; aceptar no cambia su aritmética |
| IV. Test-first | Bordes enumerados abajo, cada uno con su test antes del código; integración en los tres casos de RLS; migración con test sobre base restaurada con dos empresas |
| IV. Migración tolera la versión anterior | Solo agrega filas. Rutas y eventos viejos se conservan esta subida |
| V. Seguridad | `RequirePermission` en las rutas nuevas y viejas; motivo fuera de lista = 400; `PLATFORM_ORDERS_SIMULATED` rechazado por `config.Validate` fuera de desarrollo, con su test |
| VI. YAGNI | Despacho por `switch`, sin interfaz de una sola implementación; sin job de vencimiento; sin tono configurable |
| VII. Comentarios / idioma | Identificadores nuevos en inglés; texto de pantalla en español |
| VIII. Puertas | Ninguna se cierra. «Saber de quién es un pedido»: aceptar sigue guardando `opened_by` = quien aceptó y `decided_by` en la fila |
| Restricciones de pantalla | Sin `<select>`; motivos en hoja con filas de 44 px+; Aceptar y Rechazar separados; sin pendientes no se ocupa alto |
| Roles: solo permisos | `can('orders.accept_incoming')` y `RequirePermission`; ningún nombre de rol |

Sin violaciones; no hay tabla de complejidad.

### Bordes, pensados antes del código

| Borde | Qué falla si no se cubre | Test |
|---|---|---|
| Plataforma cancela el pedido | Suena para siempre por un pedido muerto | Integración: webhook `orders.cancel` → fuera de la lista + evento |
| Plazo vencido sin aviso | Ídem | Domain `IsPending`; integración: aceptar vencido = 409 sin llamar a la plataforma |
| Dos tabletas aceptan a la vez | Doble comanda, doble cobro | Integración: dos `Accept` concurrentes → uno 200, otro `ErrPedidoYaDecidido`, un solo `orders` |
| Aceptar mientras otra rechaza | Pedido rechazado en la plataforma y creado aquí | Integración: el segundo `execrows` = 0 deshace la transacción |
| Plataforma no confirma el rechazo | Fila `rechazado` y la plataforma sigue esperando | Integración con decisor que falla: la fila sigue `pendiente` |
| Motivo vacío o inventado | Explicación falsa al cliente | Domain + handler 400 |
| `lines: null` | Pantalla caída con API 200 | Test sobre el JSON crudo |
| Rol propio existente | Sus usuarios dejan de oír pedidos al subir | Test de la migración sobre base con roles propios |
| Usuario sin permiso | Ve u oye lo que no puede decidir | `RequirePermission` 403 + vitest: sin permiso no monta la alarma |
| Audio bloqueado | Tableta muda sin que nadie lo note | vitest con `AudioContext` falso en `suspended` → aviso visible |
| Recarga | Pierde la alerta | vitest: montar con pendientes en la respuesta inicial → suena |
| Red caída | Se apaga por error de red | vitest: respuesta con error tras una con datos → sigue sonando |
| Reconexión / subida azul-verde | Aviso perdido | vitest: `alReconectar` refetch; e2e offline→online |
| Tableta suspendida | Aviso perdido | vitest: `visibilitychange` → refetch y `resume()` |
| Simulación encendida en producción | Acepta aquí lo que la plataforma cancela | `config_test`: `APP_ENV=production` + flag → no arranca |
| Hoja abierta encima | El indicador queda tapado | vitest: capa del indicador > capa de hojas (`FRANJA` en `capas.ts`) |
| Dos avisos del mismo pedido | Franja vieja + indicador nuevo | Se borra `AvisoDePlataforma` de `POSPage`; test de que Vender no lo monta |

## Pantallas y maquetas

Medidas con el front real a **1024×600** (pedido del dueño) y a **1280×854** (mínimo de la
constitución).

1. **Indicador global de pedidos por aceptar** (opción A): una fila de 52 px en la columna del área
   de trabajo de `AppShell`, antes de la franja de agotados, con `position: relative` y la capa
   `FRANJA` (2100, `features/entrantes/capas.ts`) para quedar encima de hojas, toaster y bloqueo por PIN. Dice cuántos hay y el más
   urgente (origen, cliente, minutos), y lleva «Ver pedidos» a `/pedidos`. Con el sonido bloqueado,
   agrega el botón «Toca para activar el sonido». En Pedidos no se pinta: la sección ya lo muestra.
   Reemplaza la franja de Vender.
2. **Columna «Por aceptar» en Pedidos** (opción B2), solo con pendientes: 300 px a la izquierda de
   «En preparación» y «A entregar», con el mismo encabezado (título + `Badge` contador) y las tres
   columnas sobre panel `bg.muted` para que se separen. Cada pedido es una tarjeta gemela de la del
   tablero (`bg.panel`, borde neutro, nombre del cliente en 800, total a la derecha), con un cuadro
   de 36 px con el ícono del origen junto al nombre y el origen y folio en el renglón gris. Debajo
   del total, «quedan N min» en naranja si el origen tiene plazo, o «hace N min» en gris si no.
   Teléfono, dirección y cómo paga cuando el origen los da. **El más urgente va abierto** con sus
   renglones (como los del tablero, con borde `md`); los demás, plegados en un botón de 44 px
   «N productos» (avisa si alguno no está en el menú) que los abre. Botones: «Aceptar» verde sólido
   a la izquierda con `flex=1` y «Rechazar» en contorno rojo aparte a la derecha. La lista de la
   columna hace scroll propio (`maxH` en dvh). Rechazar abre `ConfirmSheet` destructivo con los 5
   motivos como filas de 52 px; «Rechazar pedido» se habilita al elegir uno.
3. **Pedidos: ticket y pestaña «Cerrados»** (pedidos por el dueño al ver la maqueta): pestañas
   «En curso» y «Cerrados» (`Tabs` de Chakra, como en Caja) en el renglón del título, sin quitarle
   alto al tablero. «Cerrados» muestra la lista de entregadas de hoy completa (deja de apilarse bajo
   el tablero y deja de cortarse en 5), con su ticket y devolver. Cada tarjeta de «En curso» gana una
   impresora de 44 px junto al total que abre el ticket (`ReprintTicket`) en 1 toque; «Ver ticket»
   sale del ⋮.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Indicador global | [maquetas](https://claude.ai/artifact/W4WTeEHJuRHLsaFo58Kf22) | A · franja en fila | No tapa controles de ninguna pantalla y queda encima de las hojas; cuesta 52 px solo mientras hay pendientes |
| Sección «Por aceptar» | [maquetas](https://claude.ai/artifact/Th3k5wVZyNRh1NrxBRtC9M) | B2 · columna izquierda, el más urgente abierto, origen como ícono | No le quita tarjetas al tablero; con 3 esperando deja 2 decidibles a 1024×600; el origen se reconoce junto al nombre |
| Pedidos: ticket y cerrados | [maquetas](https://claude.ai/artifact/Th3k5wVZyNRh1NrxBRtC9M) (v4) | Ticket junto al total + pestaña «Cerrados» | Ticket a 1 toque sin cortar botones a 1024×600; los entregados dejan de apilarse bajo el tablero |

## Revisión de arquitectura (2026-11-23) y lo que se corrigió

`db-architect` y `tablet-ui-reviewer` sobre este plan. Ninguna puerta del principio VIII se cierra.
Lo que cambió en el diseño por sus hallazgos:

**Pantalla**

- **Capas, en una sola constante** (`features/entrantes/capas.ts`, con su test): hojas de Chakra
  1400 < toaster 1700 < bloqueo por PIN 2000 < franja de pedidos **2100**. La franja queda encima del
  bloqueo por inactividad, que es justo cuando suele llegar un pedido; **bloqueada, la franja solo
  dice cuántos hay** (sin cliente ni productos) y el botón de activar sonido si hace falta; «Ver
  pedidos» aparece al desbloquear. Reemplaza a `CAPA_DEL_AVISO`, cuyo test se mueve, no se borra.
- **La franja se quita en Pedidos solo mientras la columna «Por aceptar» está a la vista.** En la
  pestaña «Cerrados» la franja sí se pinta, y la pestaña «En curso» lleva el número de pendientes.
- **Con la franja de agotados a la vez**, la de pedidos va primero. Chrome medido con las dos en el
  recorrido de prueba, no por separado.
- **La tarjeta abierta muestra hasta 3 renglones** y «+N más»: así sus botones quedan dentro de los
  primeros ~300 px de la columna a 1024×600. La columna hace scroll propio. Cifra corregida: a
  1024×600 se decide el primero sin desplazar y el segundo queda a la vista con sus botones; el
  tercero pide desplazar la columna.
- **La impresora va en el renglón del título** como `IconButton` de 44×44, sin margen negativo, y un
  test con `getBoundingClientRect` comprueba que no se encima con el total.
- **Texto**: «Ver pedido» con uno, «Ver pedidos» con varios. Si después de tocar el sonido sigue
  bloqueado, el botón sigue visible.
- Los renglones de «Por aceptar» no son tocables (solo lectura); un test mide ≥ 44 px en todo lo
  tocable de la columna a 1024×600.

**Base de datos**

- **0118 pone `company_id` a mano** en `role_permission_overrides` (el default sale de la sesión y en
  la migración no hay): `select r.company_id, r.id, 39, true from roles r where r.system_role_id is
  null on conflict do nothing`. Sin `set_config`: el owner salta RLS (0108 no usa `force`).
- **Down**: se niega solo si una empresa le **quitó** el permiso a un rol (`not granted`); si no,
  borra overrides, `system_role_permissions` y `permissions` de 39, en ese orden. Renglón
  `117` en `guardedDownCleanup`. El test de la migración siembra un rol propio en cada una de las dos
  empresas (con cero, el insert no hace nada y el test pasa vacío).
- **Orden de la lista**: `order by decide_before nulls last, placed_at, id`. Sin índice nuevo:
  `platform_incoming_orders_pendientes (company_id, decide_before) where state='pendiente'` ya lo
  sirve.
- **Cancelación** (`CancelIncomingOrder`, `:execrows`): `set state='cancelado', settled_at=now()
  where connection_id=$1 and external_order_id=$2 and state='pendiente'`, por el único
  `(company_id, connection_id, external_order_id)`. Cero filas (ya aceptado o ya cancelado) =
  aviso confirmado sin cambio: sin esto, la carrera aceptar/cancelar truena el check `quien_decidio`
  y el webhook responde 500.
- `deny_reason` es `text` sin check: los códigos nuevos caben y **no se agrega check** en esta subida
  (endurecería lo que escribe la API vieja).
- La API vieja ignora permisos desconocidos al calcular los efectivos (`KnownPermission`); su pantalla
  de roles puede listar el 39 unos segundos durante el traslape. Se verifica en la prueba de la
  migración que no falle.
- **Pendientes vencidos se acumulan como `pendiente`** sin listarse: aceptado, son decenas de filas y
  el índice es parcial; un webhook de cancelación tardío sí los pasa a `cancelado`.

## Project Structure

### Documentation (this feature)

```text
specs/093-alerta-de-pedido-entrante/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/incoming-orders.md
└── tasks.md
```

### Source Code

```text
server/
├── migrations/0118_incoming_orders_permission.sql   # + su test de integración
├── queries/pedidos_de_plataforma.sql                # lista con plazo, cancelar por folio externo
├── internal/domain/incoming.go (+ _test)            # IncomingOrder, IsPending, orden, motivos
├── internal/domain/permission.go                    # PermOrdersAcceptIncoming
├── internal/config/config.go (+ _test)              # PLATFORM_ORDERS_SIMULATED, solo desarrollo
├── internal/app/incoming_orders.go                  # servicio común, despacho por origen
├── internal/app/pedidos_de_plataforma.go            # Rechazar, cancelación del webhook, plazo al decidir, decisor simulado
├── internal/httpapi/handlers_incoming.go            # GET lista, POST accept/reject
├── internal/httpapi/router.go                       # rutas nuevas + permiso en las viejas
├── internal/httpapi/handlers_webhook_plataforma.go  # publica incoming.changed también al cancelar
└── internal/integration/incoming_orders_test.go     # RLS ×3, carreras, cancelación, migración

web/src/
├── api/pedidosEntrantes.ts
├── app/permissions.ts                               # 'orders.accept_incoming'
├── app/AppShell.tsx                                 # monta la alerta y el indicador
├── shared/entrantes/   (shared: la usan el shell y Pedidos; una pantalla no importa de otra)
│   ├── alarma.ts (+ test)                           # AudioContext, búfer en bucle, desbloqueo
│   ├── useEntrantes.ts (+ test)                     # consulta, evento, reconexión, visibilidad, sondeo
│   ├── AlertaDeEntrantes.tsx (+ test)               # indicador + aviso de sonido
│   ├── PorAceptar.tsx (+ test)                      # sección de Pedidos
│   └── RechazarSheet.tsx (+ test)                   # motivos
├── features/orders/OrdersBoardPage.tsx              # monta PorAceptar arriba
├── features/pos/POSPage.tsx                         # quita AvisoDePlataforma
└── e2e/entrantes.spec.ts
scripts/entrantes-de-prueba.sh                       # siembra pendientes en la base LOCAL (se niega fuera del worktree)
```

Se borran `AvisoDePlataforma`, `AvisoDePedidoEntrante`, `PedidosEntrantesSheet`,
`usePedidosDePlataforma` y `api/pedidosDePlataforma.ts` con sus tests, reemplazados por
`features/entrantes`. `urgencia.ts` se mueve a `features/entrantes`.

**Structure Decision**: monorepo existente; la feature front vive en su propia carpeta porque la
consumen el shell, Vender y Pedidos.

## Lo que queda para después

- Quitar `/orders/platform/pending`, `/orders/platform/{id}/accept` y los eventos `platform.order.*`
  en una subida posterior, cuando ningún front en caché los use.
- Marcar `expirado` en la fila si un reporte necesita contar vencidos.

> **Renumerada (2026-11-23):** la migración de esta feature es la **0118**. La 0117 la tomó la de
> fotos del catálogo (#325), que llegó antes a `develop`. El id de permiso 39 seguía libre.
