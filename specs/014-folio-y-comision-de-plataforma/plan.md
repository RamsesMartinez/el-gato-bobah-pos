# Implementation Plan: el folio con el que llegó el pedido, y lo que la plataforma se quedó

**Branch**: `014-folio-y-comision-de-plataforma` | **Date**: 2026-09-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/014-folio-y-comision-de-plataforma/spec.md`

## Summary

Un pedido de plataforma gana **el identificador con el que la plataforma lo nombra** —una columna de
texto en `orders`, capturada en el mismo acto en que se levanta el pedido, mientras el operador la
tiene enfrente en la tablet de la app— y, cuando llega el documento de pago días después, **una
liquidación propia** que copia lo que ese documento dice: bruto reportado, comisión, quién financió
el descuento, retenciones, neto y la referencia del depósito.

Las dos mitades no son simétricas y por eso el orden importa: **el folio es irrecuperable** (el
reporte de pagos de Rappi expone 3 meses y el de Uber 31 días; un folio que no se capture ahora no
existe después), mientras que la liquidación se puede capturar el mes que viene si el pedido tiene
folio.

El andamiaje ya está: `orders.delivery_platform_id` existe con FK compuesta por empresa, el selector
de lista activa es un bloque de pantalla que ya se muestra solo en pedidos de plataforma, y la
pantalla de Ventas ya tiene el patrón de "lista y resumen del mismo `where`". El trabajo real son
**una migración**, **dos validaciones de dominio**, **un filtro que toca cinco consultas a la vez** y
**44 px de pantalla que no sobran**.

La revisión de arquitectura (hook `after_plan`) corrió sobre este plan y sus hallazgos ya están
aplicados; la sección *Hallazgos de la revisión, aplicados* dice cuáles y qué cambió.

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript / React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose (migraciones embebidas), shopspring/decimal · Vite, Chakra UI v3, TanStack Query, Zustand

**Storage**: PostgreSQL multi-tenant con RLS por `company_id`; el rol de app es `gatobobah_app`

**Testing**: `go test` (unitario + `-tags=integration` contra Postgres real, con **dos empresas**) · vitest · Playwright a 1024×600 contra el ambiente desplegado

**Target Platform**: tablets de 7–10", presupuesto real **1024×600**; navegador, sin instalar nada

**Project Type**: web (backend `server/` + frontend `web/`)

**Performance Goals**: el filtro y la columna nuevos no vuelven perceptiblemente más lenta ninguna consulta de Ventas (SC-008); el filtro de pendientes va por índice parcial, no por scan

**Constraints**: producción con datos reales de un negocio en operación. Ninguna cifra de venta, corte o arqueo ya cerrado cambia (SC-005). El alto de pantalla es requisito funcional, no preferencia

**Scale/Scope**: 1 empresa activa, 3 plataformas con margen, ~70 pedidos en la base de prueba; el histórico de plataforma de dos años y medio de FUDO **queda fuera** por decisión del spec

## Constitution Check

*GATE: pasa antes de Fase 0 y se vuelve a evaluar tras el diseño (abajo).*

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering** | `domain`: `NormalizePlatformRef`, `Settlement.Validate`, `ValidSignedMoney`, la clasificación del resumen — **todo puro, sin I/O**. `app`: `OrdersService` (folio) y `SettlementsService` nuevo (liquidación), con `store.WithTx` donde hay más de una escritura. `httpapi`: handlers finos que decodifican y mapean. Las consultas nuevas van por **sqlc**, nunca SQL a mano |
| **II. Errores** | Sentinel nuevo `ErrPlatformRefTaken` envolviendo `ErrConflict` con `%w`; el mapeo a HTTP vive **solo** en `httpapi.Error`. `context.Context` primero y propagado hasta la query |
| **III. Dinero** | Cada importe de la liquidación pasa por `Round2` **antes** de tocar `numeric(10,2)` y por `ValidMoney`/`ValidSignedMoney` en la frontera. **Ninguna cifra de la liquidación entra a un total de ventas, a un corte ni a un arqueo** (FR-016), y el resumen que las junta declara por cifra qué incluye y qué excluye, con su test de regresión que falla **nombrando el concepto duplicado** |
| **IV. Test-first** | Los casos de borde ya están enumerados en el spec, antes del código. Cada tarea de implementación va después de su test. La lógica pura se prueba sin base de datos; los grants, el aislamiento, el índice único y la migración, **contra Postgres real bajo `appRoleStore` y con dos empresas** |
| **V. Seguridad** | Autorización en el servidor (`RequireRole`), front como espejo. Escritura del folio alcanzable por cajero → `rateLimitUser`, igual que los precios de plataforma. El filtro nuevo se **rechaza** si es desconocido, nunca cae a un default. El mensaje del duplicado nombra un pedido que la búsqueda encontró **bajo RLS**, así que no puede ser de otra empresa. Pasa por `security-auditor` antes de mergear |
| **VI. YAGNI** | Sin entidad de depósito ni de documento de pago; sin cliente HTTP; sin estimación de comisión; sin producto por plataforma; sin columna de estado "provisional" (la presencia de la fila **es** el estado); sin valor `capturado` en el filtro, que nadie pidió |
| **VII. Comentarios** | Cada decisión no obvia lleva su porqué: por qué el índice es parcial, por qué la tasa es nullable y el monto no, por qué el neto no tiene check de signo, por qué el descuento del restaurante no se almacena |
| **VIII. No cerrar puertas** | Sección propia abajo, con la pregunta contestada por escrito |

**Gate**: pasa. Sin violaciones que justificar en Complexity Tracking.

### La pregunta del principio VIII, contestada por escrito

> ¿Esto se puede agregar después al mismo costo?

| Decisión | Respuesta | Qué se hace hoy |
|---|---|---|
| Guardar el folio | **No.** El hecho expira: Rappi conserva 3 meses de historia, Uber 31 días | Se construye |
| Guardarlo **completo** y como texto | **No.** Del código corto de 5 caracteres no se reconstruye el UUID, y un entero no cubre a las tres plataformas | Texto, sin normalizar |
| La referencia del depósito | **No.** Expira con el mismo reloj | Texto dentro de la liquidación |
| Comisión como snapshot | **No.** Recalcular el pasado con la tasa de hoy reescribe la historia | Monto y tasa por pedido |
| Quién financió el descuento | **No.** Sin la columna, cada promoción se registra como pérdida propia del restaurante | `discount_platform` |
| Quién escribió el folio y cuándo | **No** el hecho; **sí** la columna | Se guarda (D-12) |
| Pantalla de conciliación depósito ↔ pedidos | **Sí** | No se construye |
| Base de cálculo de la comisión de Uber | **Sí**, y hoy nadie la puede llenar bien | No se construye |
| Entidad de depósito / de documento | **Sí** | No se construye |
| Valor `capturado` en el filtro | **Sí** | No se construye |

**Puertas que esta feature CRUZA** (dejan de ser puertas en la tabla de la constitución y pasan a ser
features): *conciliar el depósito contra los pedidos que lo formaron* y *cuánto deja cada
plataforma*. La tabla del principio VIII se actualiza en el commit de la feature.

**Puertas que quedan abiertas y mejor alimentadas**: promociones de plataforma (el descuento se
guarda por origen de financiamiento), precio sugerido por plataforma (con tasas **medidas** en vez de
configuradas), conectarse por API (los campos que llenaría un conector son los mismos).

**La que no se cierra pero tampoco se resuelve**: un pedido de plataforma sigue exigiendo turno de
caja abierto. Este plan no lo empeora. El día que un conector meta pedidos solo, resolverlo será una
migración sobre datos vivos — y por eso está dicho aquí y en el spec.

## Project Structure

### Documentation (this feature)

```text
specs/014-folio-y-comision-de-plataforma/
├── spec.md              # el requerimiento
├── plan.md              # este archivo
├── research.md          # las 13 decisiones de diseño, con lo que se descartó
├── data-model.md        # esquema de 0065, para el db-architect
├── quickstart.md        # cómo validar que funciona, y qué se mide
├── contracts/
│   └── api.md           # endpoints nuevos y los que cambian
└── checklists/
```

### Source Code (repository root)

```text
Makefile                        # + el camino al respaldo anonimizado de producción (D-17)
scripts/
└── respaldo-anonimo.sh         # nuevo: baja el dump, borra PII, restaura en la base de pruebas

server/
├── migrations/
│   └── 0065_folio_y_liquidacion_de_plataforma.sql   # 3 columnas + 3 checks + 4 índices + tabla + RLS + GRANTS
├── queries/
│   ├── orders.sql              # + folio en CreateOrder, SetPlatformRef, FindOrderByPlatformRef
│   ├── sales.sql               # cinco PARES (D-8) + la búsqueda exacta por folio (D-16)
│   └── settlements.sql         # nuevo: upsert, get, resumen del periodo
└── internal/
    ├── domain/
    │   ├── platform_ref.go     # NormalizePlatformRef + MaxPlatformRefLen  (+ _test.go)
    │   ├── settlement.go       # Settlement.Validate, DiscountRestaurant, PlatformMoneySummary  (+ _test.go)
    │   ├── limits.go           # + ValidSignedMoney  (+ _test.go)
    │   ├── sales.go            # + el filtro en SalesFilter.Validate (whitelist)  (+ _test.go)
    │   └── errors.go           # + ErrPlatformRefTaken
    ├── app/
    │   ├── orders.go           # el folio entra en CreateOrderCmd; SetPlatformRef
    │   ├── settlements.go      # nuevo: Upsert, Get, Summary
    │   └── sales.go            # el filtro viaja a las cinco consultas
    ├── httpapi/
    │   ├── handlers_orders.go       # + PATCH /orders/{id}/platform-ref
    │   ├── handlers_settlements.go  # nuevo
    │   ├── handlers_sales.go        # + folioPlataforma
    │   └── router.go                # rutas + RequireRole + rateLimitUser
    └── integration/
        ├── folio_de_plataforma_test.go      # unicidad, RLS, grants, checks, dos empresas
        ├── corregir_folio_no_mueve_dinero_test.go   # SC-005
        └── liquidacion_de_plataforma_test.go        # upsert, 404 vs ceros, resumen

web/src/
├── features/pos/
│   ├── PlatformPicker.tsx      # el campo de folio EN EL MISMO RENGLÓN; botones 40px → 44px
│   └── POSPage.tsx             # la hoja que pide el dato al mandar: UNA salida, footer fijo en dvh
├── features/sales/
│   ├── SalesPage.tsx           # buscador por folio + toggle de pendientes + folio TRUNCADO en la celda
│   ├── SalesSummaryTiles.tsx   # los tiles de plataforma, cada uno con su conteo de pedidos
│   ├── SaleDetailDialog.tsx    # ver/corregir folio; registrar liquidación
│   └── LiquidacionSheet.tsx    # nuevo: 7 campos de dinero en DOS columnas + footer fijo
├── stores/ticket.ts            # platformOrderRef en el ticket activo
└── api/{pos,sales,settlements}.ts

web/e2e/                        # el alto medido a 1024×600 y el flujo de captura
docs/
├── presupuesto-de-pantalla-1024x600.md   # se ACTUALIZA con la medición nueva
├── matriz-de-pantallas.md                # renglones nuevos, con su test
└── matriz-de-cobro.md                    # "registrar liquidación no mueve el cobro"
```

**Structure Decision**: monorepo existente. Backend en `server/` con el layering de la constitución;
frontend en `web/`. Ningún directorio nuevo de primer nivel.

## Orden de construcción

Sigue las prioridades del spec, y cada etapa entrega valor sola:

| Etapa | Qué entrega | Por qué en ese orden |
|---|---|---|
| **1 — P1: el folio** | Migración 0065 + validación + captura + unicidad | Es lo único irrecuperable. Aunque nada más se construya, la conciliación manual se vuelve posible |
| **2 — P2: la visibilidad** | Filtro de pendientes en las cinco consultas + corrección después del hecho | Sin esto, la salida de escape convierte el folio en un campo opcional que nadie llena |
| **3 — P3: la liquidación** | Tabla, endpoints, formulario y resumen del periodo | Es recuperable mientras el pedido tenga folio |

La migración es **una sola** y entra en la etapa 1 con la tabla de liquidación incluida: las dos
piezas apuntan a la misma fila de `orders`, y partirla dejaría una ventana en la que la segunda mitad
no tiene con qué probarse. Lo que se difiere es el **código**, no el esquema.

## Los riesgos que este plan ya vio

**Lo que puede salir mal, dicho antes de que salga.**

1. **El campo de folio se come un renglón del mosaico.** Medido: a 1024×600 con plataforma activa y
   sin el aviso de caja hay 3 renglones y **25 px** de sobra, y el piso táctil es 44 px. Por eso el
   campo va **en el mismo renglón que los botones de plataforma** (+4 px, 3 renglones intactos) y no
   apilado debajo (+48 px, 3 → 2). Si con más plataformas configuradas el `flexWrap` lo baja, cuesta
   el renglón que SC-007 permite — y se declara. **Se mide con Playwright, no se supone.**
2. **El índice parcial existe y el planner no lo usa.** Medido: con el patrón `narg is null or (…)`
   y plan genérico —al que pgx cae solo— Postgres no puede probar el predicado del índice parcial y
   filtra a mano. Por eso el filtro va con el predicado **literal** en dos variantes de cada
   consulta, y por eso SC-008 se verifica con `EXPLAIN` sobre la ejecución vía pgx y no con psql.
3. **La liquidación queda apuntando al pedido de otra empresa.** Los chequeos de integridad
   referencial saltan RLS por diseño: una FK simple lo aceptaría y el error saldría en el resumen de
   dinero, no en el `insert`. La FK es compuesta con `company_id`, como en 0041.
4. **La lista y el resumen divergen.** El filtro nuevo toca **cinco** consultas que viven en el mismo
   archivo a propósito. Olvidar una deja la pantalla diciendo dos cosas y a quien la lee sin forma de
   saber cuál. Ya costó un turno con $4,500 de faltante inexplicable.
5. **La comisión se resta de una venta.** Es el modo de falla que el principio III nombra. La defensa
   es que las cifras viven en endpoints distintos, cada una declara qué incluye y qué excluye, y hay
   un test que falla **nombrando el concepto que se duplicó**.
6. **El `grant` que falta.** Una tabla nueva sin su `grant` pasa la migración, pasa los tests y pasa
   `make start` —dev sirve como owner— y en producción devuelve `42501` en el primer request.
7. **La cadena vacía.** `""` no es "sin folio". Se cierra en tres capas (dominio, check, índice
   parcial) porque cada una falla por su lado.
8. **`ValidMoney` no sirve para el neto**: exige no negativo, y el neto negativo es real. Reusarlo
   rechazaría el caso que el spec manda aceptar.
9. **Corregir un folio mueve una cifra.** No se razona: se fotografía el corte, el arqueo y el
   resumen antes y después, contra Postgres real.
10. **El teclado tapa la salida explícita.** La hoja del folio llega con el campo enfocado; si el
    botón de escape vive dentro del scroll, SC-003 pasa de un toque a dos. Footer fijo en `dvh`.
11. **La pantalla de Ventas nunca se midió.** El presupuesto documentado es solo del POS, y esta
    feature le agrega un toggle, una fila de tiles y texto en una celda. Se mide antes de aprobarla,
    con el método del POS.

## Hallazgos de la revisión de arquitectura, aplicados

`db-architect` y `tablet-ui-reviewer` corrieron sobre este plan antes de que existiera código. Lo
que cambió:

| # | Hallazgo | Qué cambió en el diseño |
|---|---|---|
| 1 | El índice parcial de pendientes no lo usa el planner con el patrón `narg … is null or (…)` y plan genérico — medido con 150k pedidos | Cinco **pares** de consulta con el predicado literal en vez de cinco con el patrón OR (D-8) |
| 2 | FK simple a `orders` en una tabla de dinero: los chequeos de integridad referencial saltan RLS | `orders_id_company_key unique (id, company_id)` + FK compuesta en `platform_settlements` |
| 3 | Nueve campos de liquidación apilados no caben en 600 px (~630 px estimados, antes del teclado) | Siete campos de dinero en dos columnas, dos de texto al final, footer fijo en `dvh` (D-14) |
| 4 | El folio de hasta 64 caracteres envuelve la celda "Tipo" en casi todos los renglones de la vista filtrada | Truncado con elipsis en la lista; completo en el detalle (D-13) |
| 5 | El teclado tapa la salida explícita de la hoja del folio, y SC-003 pasa de uno a dos toques | Footer fijo en `dvh`, con su comprobación en el quickstart (D-10) |
| 6 | El trío del folio puede quedar a medias y romper el rastro de D-12 | Check todo-o-nada en el esquema |
| 7 | Tres tiles hermanos sobre conjuntos distintos (96 vs 74 pedidos) se restan a ojo | Cada tile lleva su conteo de pedidos; `incluye`/`excluye` detrás de un icono de ayuda (D-15) |
| 8 | El filtro booleano como `Picker` cuesta cuatro toques el ciclo | Chip/toggle: dos (D-13) |
| 9 | Los botones del `PlatformPicker` miden 40 px desde que se escribieron | 44 px, en el mismo archivo que ya se toca (D-13) |
| 10 | `payout_reference` y `document_ref` sin tope de longitud | Checks de 128 y 256, y sin cadena vacía |
| 11 | La pantalla de **Ventas** nunca se midió; el presupuesto documentado es solo del POS | Se mide antes de aprobar la fila de tiles; declarado como pendiente en D-15 y en el quickstart |

**Lo que la revisión confirmó y no cambió**: la aritmética de renglones del campo de folio, la
decisión D-4, la reversibilidad del `Down`, y que **ninguna decisión de este plan cierra una puerta
del principio VIII**. La base de cálculo de la comisión resultó además reconstruible mientras la tasa
no sea nula.

**Decisión tomada al aplicar, que la revisión no pidió**: el endpoint de corrección **no borra** el
folio. Ver D-12.

## Desviaciones del spec: ninguna abierta

Al planear apareció **una**, y ya se resolvió enmendando el spec en vez de implementando la excepción
en silencio, que es lo que manda Governance:

| FR | Qué pasó |
|---|---|
| **FR-012** | Pedía guardar el descuento financiado por el restaurante. Cualquiera de las dos partes determina la tercera, y guardar las dos partes vuelve **imposible por construcción** el rechazo del escenario 6 de la US-3. **Enmendado el 2026-09-07**: el spec ahora pide guardar el total y la parte de la plataforma, e *informar* la del restaurante. Esquema y spec vuelven a decir lo mismo |

## Hallazgos de `/speckit-analyze`, resueltos

El gate corrió sobre spec, plan y tasks y encontró dos CRITICAL. Los dos venían de este plan:

| Hallazgo | Qué cambió |
|---|---|
| **No había forma de buscar un pedido por su folio.** SC-002 pide localizar cualquier renglón del documento "en un solo paso" y la prueba independiente de la US-1 dice "se busca por el folio" — el plan agregaba el filtro y la columna, y ninguna búsqueda | Búsqueda exacta por folio en la US-2, con su propio índice parcial (D-16) |
| **Los tests corrían contra una base sembrada limpia.** La constitución exige respaldo real con dos empresas, y SC-005 exige una copia de los datos de producción | Camino nuevo de respaldo anonimizado apoyado en `make prod-db-tunnel` (D-17) |

Y cuatro de severidad alta o media: faltaba el test del limitador de intentos del endpoint de
corrección, faltaba probar que no alcanza el pedido de otra empresa, SC-004 (30 segundos) no tenía
quién lo midiera, y **"folio" ya significa dos cosas en la misma pantalla** (D-18). Los cuatro tienen
tarea ahora.

## Complexity Tracking

Sin violaciones de la constitución que justificar.
