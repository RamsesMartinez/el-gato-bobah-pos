# Implementation Plan: El descuento que se le hace a un pedido

**Branch**: `022-descuento-en-el-pedido` | **Date**: 2026-09-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/022-descuento-en-el-pedido/spec.md`

## Summary

El descuento se resta **dentro de `orders.total`**, que es la cifra que ya suman el corte de caja,
las ventas, los reportes y la liquidación de plataforma. Eso hace que la feature sea barata donde
podría haber sido cara: ningún agregado cambia de fórmula. Lo que sí se toca es cada pantalla que
muestra el **desglose**, la aritmética del dominio y el recálculo del pedido cuando se le agregan o
se le cancelan renglones.

Tres piezas cargan el peso:

1. **`domain`** decide el monto (puro, sin I/O): resolver el porcentaje a pesos, rechazar lo
   absurdo, aplicar el descuento antes del envío y nunca dejar el total negativo.
2. **`RecalcOrderTotals`** —una query— mantiene esa aritmética viva cuando el pedido cambia después.
3. **Una migración** agrega el rastro (`discount_set_by`, `discount_set_at`); la columna del dinero
   ya existe desde [0007](../../server/migrations/0007_orders.sql).

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 + React 19 (web)

**Primary Dependencies**: chi · pgx + sqlc · goose · shopspring/decimal | Vite · Chakra UI v3 ·
TanStack Query · Zustand

**Storage**: PostgreSQL. `orders.discount_total numeric(10,2) not null default 0` **ya existe**;
esta feature agrega dos columnas de rastro y **ninguna tabla**.

**Testing**: `go test ./...` (unitarios en `domain`, integración contra Postgres real bajo
`appRoleStore`) · vitest (web) · Playwright contra el ambiente de pruebas.

**Target Platform**: tabletas de 7–10", presupuesto **1024×600**.

**Project Type**: monorepo web (server/ + web/).

**Constraints**: el descuento se captura con el cliente enfrente; el panel del ticket ya está al
límite de alto en 600 px, así que el control **no puede ocupar alto fijo** en el 95 % de los pedidos
que no llevan descuento.

**Scale/Scope**: un local, ~200 pedidos/día. Nada de esto es sensible al volumen.

## Constitution Check

*GATE: pasa antes de Phase 0 y se vuelve a evaluar después del diseño.*

| Principio | Cómo lo cumple este plan |
|---|---|
| **I. Layering** | `domain/descuento.go` (puro: resolver, validar, aplicar) → `app/orders.go` (orquesta y persiste en la misma tx) → `httpapi/handlers_orders.go` (handler fino) → `queries/orders.sql` + `make sqlc`. Ninguna regla de dinero en el handler ni en la pantalla. |
| **II. Errores** | Sentinel nuevo `domain.ErrDescuentoMayorQueLaVenta` envuelto con `%w` y mapeado a 422 **solo** en `httpapi.Error`. El resto cae en `domain.ErrValidation` (400). |
| **III. Dinero** | `Round2` en cada frontera, `ValidMoney` antes de tocar `numeric`. El descuento se clasifica **una vez**: es una reducción del ingreso dentro de `total`, **no** un renglón hermano del subtotal ni un gasto. El envío queda fuera del descuento por la misma razón. |
| **IV. Test-first** | Los bordes ya están enumerados en el spec; la tabla de *Bordes → test* de abajo los ata a un test que se escribe **antes**. La migración va con su test de integración (lo exige también el hook `migracion-con-test.sh`). |
| **V. Seguridad** | El monto lo recalcula el servidor; el `%` se resuelve contra **su** subtotal, no contra el del cliente. Entrada malformada = 400, nunca un default silencioso. El endpoint nuevo va bajo `RequireAuth` con los mismos roles que capturan pedidos. |
| **VI. YAGNI** | Sin catálogo de promociones, sin descuento por renglón, sin tope configurable, sin tabla nueva. Se reusa la columna que ya existe. |
| **VII. Comentarios** | Cada decisión no obvia (por qué el monto y no el porcentaje; por qué el descuento no se recalcula al agregar líneas; por qué `greatest(...,0)`) va comentada con su **porqué**. |
| **VIII. Puerta abierta** | **Pregunta obligada — ¿esto se puede agregar después al mismo costo?** Para *quién financió el descuento*: **no**, y el dueño lo eximió por escrito (ver Assumptions del spec). Para todo lo demás de esta feature —tope, autorización, descuento por renglón, catálogo de promos— **sí**: son pantallas y validaciones sobre un dato que ya quedará registrado. El plan no cierra ninguna de esas puertas. |

**Resultado del gate: pasa**, con la exención del principio VIII declarada y firmada por el dueño.

## Project Structure

### Documentation (this feature)

```text
specs/022-descuento-en-el-pedido/
├── plan.md              # este archivo
├── spec.md
├── research.md          # decisiones y alternativas descartadas
├── data-model.md        # columnas, checks y aritmética
├── quickstart.md        # cómo verificarlo de punta a punta
├── contracts/
│   └── api.md           # forma del request y de la respuesta
└── checklists/
    └── requirements.md
```

### Source Code

```text
server/
├── migrations/
│   └── 0072_descuento_del_pedido.sql      # NUEVO: rastro + checks (la columna del dinero ya existe)
├── queries/
│   └── orders.sql                          # RecalcOrderTotals, CreateOrder, SetOrderDiscount, lecturas
├── internal/
│   ├── domain/
│   │   ├── descuento.go                    # NUEVO: ResolverDescuento, AplicarDescuento, sentinel
│   │   ├── descuento_test.go               # NUEVO: table-driven sobre los bordes
│   │   ├── order.go                        # ApplyDeliveryFee suma sobre el total YA descontado
│   │   └── errors.go                       # ErrDescuentoMayorQueLaVenta
│   ├── app/
│   │   └── orders.go                       # Create (descuento), SetDiscount (nuevo)
│   ├── httpapi/
│   │   ├── handlers_orders.go              # body del create + PUT /orders/{id}/discount
│   │   ├── router.go                       # la ruta nueva
│   │   └── respond.go                      # 422 del sentinel
│   └── integration/
│       └── descuento_test.go               # NUEVO: migración, RLS, aritmética sobre Postgres real
└── ...

web/src/
├── domain/
│   ├── descuento.ts                        # NUEVO: gemelo de envio.ts (una sola fuente del monto)
│   └── descuento.test.ts                   # NUEVO
├── types/pos.ts                            # TicketTab.descuento/descuentoModo; CreateOrderBody
├── stores/ticket.ts                        # el descuento vive en la CUENTA (sobrevive F5)
├── features/pos/
│   ├── Ticket.tsx                          # el control de captura
│   ├── POSPage.tsx                          # los TRES lugares que pintan total
│   └── preCuenta.ts                        # el descuento viaja al papel
├── utils/printReceipt.ts                   # renglón de descuento cuando existe
├── shared/CobrarSheet.tsx                  # sale gratis: ya lee total/outstanding del servidor
└── features/sales/SaleDetailDialog.tsx     # el descuento en el detalle de la venta
```

**Structure Decision**: monorepo existente, sin directorios nuevos. La única pieza estructural nueva
es `domain/descuento.go` + su gemelo `web/src/domain/descuento.ts`, y existen por la misma razón que
`envio.ts`: **el monto del descuento se calcula en un solo lugar de cada lado**. La cifra que la
pantalla pinta y la que el servidor cobra ya divergieron una vez en este repo (la pantalla ofrecía
cobrar $115 de un pedido de $95); duplicar la fórmula es repetir ese defecto.

## Decisiones de diseño

### 1. Qué viaja del cliente al servidor

El cliente manda **o** un monto **o** un porcentaje, nunca los dos, y nunca el total resultante:

```jsonc
{ "discountAmount": 50.00 }   // o
{ "discountPercent": 20 }     // o ninguno de los dos = sin descuento
```

El servidor resuelve el porcentaje **contra su propio subtotal** y guarda el monto. Mandar el monto
ya calculado desde la pantalla sería creerle al cliente una cifra de dinero, que es exactamente lo
que `BuildOrder` no hace con los precios.

### 2. Dónde entra en la aritmética

```text
BuildOrder          → Subtotal, Total = Subtotal
AplicarDescuento    → Discount, Total = max(Subtotal − Discount, 0)
ApplyDeliveryFee    → Total = Total + envío
```

`ApplyDeliveryFee` hoy escribe `Total = Round2(Subtotal + fee)` y **borraría el descuento**: cambia a
sumar sobre el total ya descontado. Es una línea, y su comentario de "Total ya = Subtotal" deja de
ser cierto — se corrige con ella.

### 3. Qué pasa cuando el pedido cambia después

`RecalcOrderTotals` recalcula el subtotal desde los renglones vivos. Se le agrega el descuento ya
guardado y el piso en cero:

```sql
total = greatest(nuevo_subtotal - o.discount_total, 0) + o.delivery_fee
```

**El descuento NO se recalcula** aunque se haya capturado como porcentaje: se guardó como pesos, y
esa es la razón de guardarlo así. Un descuento que se recalculara solo cambiaría de monto porque
alguien agregó un café, sin que nadie lo decidiera.

### 4. El rastro

Dos columnas espejo de las que ya existen para el folio de plataforma
(`platform_ref_set_by` / `platform_ref_set_at`): `discount_set_by` y `discount_set_at`. Se llenan
cuando hay descuento y quedan nulas cuando no — **no** hay check todo-o-nada con el monto, porque un
descuento de cero es la ausencia y no debe exigir autor.

### 5. El control en la pantalla de 600 px

**Corregido tras la revisión de arquitectura**, que midió el diseño anterior: un botón
`+ Descuento` en su propia fila costaba 44 px + 8 px de margen **a todos los pedidos**, y bajaba los
renglones de producto visibles en un pedido a domicilio de ≈3.5 a ≈2.9. Le cobraba alto al 95 % que
no descuenta, que es justo lo que D6 decía evitar.

Lo que queda:

- **El acceso vive en la fila que ya existe**: el renglón `Total` de la zona de totales
  ([Ticket.tsx:160](../../web/src/features/pos/Ticket.tsx)) gana un botón chico a su derecha. Esa
  fila mide 40 px y el mínimo tappable son 44: el costo real del acceso es **4 px**, no 52.
- **Al tocarlo se despliega el renglón de captura**: campo numérico + selector `$` / `%` de dos
  botones tipo segmento. Con descuento aplicado el renglón se queda visible con su monto — es
  dinero, tiene que verse sin abrir nada.
- **Al enfocar el campo se hace `scrollIntoView`** y la zona de totales lleva su `maxH` en **dvh**.
  Sin eso, el teclado numérico de Android (≈40–45 % del alto, unos 240–270 px de 600) puede empujar
  el botón COBRAR fuera de la pantalla, y el único rescate sería el scroll de la página entera
  anidado con el de la lista de renglones. Es la familia de defecto que ya costó la lista de
  diferencias de la 020.

**El toque extra que este diseño acepta, y por qué.** A 1024×600 el panel del ticket **arranca
colapsado** (`panelHidden` nace en `true` bajo `max-height: 720px`,
[POSPage.tsx:276](../../web/src/features/pos/POSPage.tsx)): la vista por default es el catálogo con
la píldora flotante, y desde ahí se cobra sin abrir el panel. Aplicar un descuento cuesta entonces
**un toque más** — abrir el panel — que SC-001 no contaba.

Se acepta en vez de meter el descuento en la píldora o en la barra angosta porque **ahí no hay
ancho**: la barra ya se desborda en 1024×600 antes de agregarle nada. El caso raro paga el toque; el
caso de todos los días no paga ni ancho ni alto.

### 6. Qué pinta cada superficie

| Superficie | Con descuento | Por qué |
|---|---|---|
| Panel del ticket | Renglón de descuento + total rebajado | Es donde se captura; ahí sí hay dónde leerlo |
| Píldora flotante | **Solo el total ya rebajado** | No hay ancho para un desglose, y FR-010 no lo pide aquí |
| Barra angosta | **Solo el total ya rebajado** | Igual |
| Ticket / precuenta / hoja de cobro / detalle de venta | Desglose completo con nombre | FR-010 |

Las tres primeras calculan hoy `total + envio.monto` cada una por su cuenta
([POSPage.tsx:528, :555](../../web/src/features/pos/POSPage.tsx) y el panel). **Las tres pasan a
leer el mismo cálculo de `web/src/domain/descuento.ts`**: que cada superficie sume lo suyo es
exactamente cómo la pantalla llegó a ofrecer cobrar $115 de un pedido de $95.

## Bordes → dónde queda su test

Cada borde del spec, con el test que lo atrapa. Los de `domain` se escriben **antes** del código.

| Borde | Test | Dónde |
|---|---|---|
| Campo vacío ≠ 0 | `descuentoDeLaCuenta` con `''` → sin descuento, `paraElServidor: undefined` | `web/src/domain/descuento.test.ts` |
| Descuento > subtotal | `ResolverDescuento` → `ErrDescuentoMayorQueLaVenta` (422, no 500) | `domain/descuento_test.go` |
| Porcentaje con residuo | 33 % de $100.05 → $33.02 (Round2), y el total cierra | `domain/descuento_test.go` |
| Porcentaje absurdo (120, −5, NaN, texto) | `ErrValidation` | `domain/descuento_test.go` |
| Monto y porcentaje a la vez | `ErrValidation` (ambigüedad, no "gana uno") | `domain/descuento_test.go` |
| Descuento + envío | $385 − $50 + $35 = $370 | `domain/descuento_test.go` |
| Se cancela una línea y el subtotal cae bajo el descuento | total = 0, nunca negativo | `integration/descuento_test.go` |
| Se agregan líneas después | el descuento sigue igual; el total sube por el renglón nuevo | `integration/descuento_test.go` |
| Pedido ya cobrado | `SetDiscount` → conflicto, el total no se mueve | `integration/descuento_test.go` |
| La migración | corre sobre respaldo real, con **dos empresas**, y los pedidos viejos quedan en cero | `integration/descuento_test.go` |
| RLS/grants de las columnas nuevas | el endpoint funciona bajo `gatobobah_app`, no solo como owner | `integration/descuento_test.go` (`appRoleStore`) |
| El ticket cierra | subtotal − descuento + envío = total, sobre el **HTML crudo** del papel | `web/src/utils/printReceipt.test.ts` |
| Sin descuento no hay renglón | el papel no trae "Descuento $0.00" | `web/src/utils/printReceipt.test.ts` |
| Los tres lugares que pintan total | panel, píldora y barra angosta muestran la misma cifra | `web/src/features/pos/POSPage.test.tsx` |
| La zona de totales no crece sin límite | tiene `maxH` acotado, no un `overflowY` que empuja hacia abajo | `web/src/features/pos/Ticket.test.tsx` |
| Una cuenta guardada ANTES del deploy | hidrata sin `descuento`/`descuentoModo` y no revienta: se trata como sin descuento | `web/src/stores/ticket.test.ts` |

## Riesgos conocidos

- **El estado persistido del carrito (`egb:ticket:v2`) gana campos.** Una cuenta abierta antes del
  deploy hidrata sin `descuento` ni `descuentoModo`. El código los trata como ausentes (que es "sin
  descuento"), y eso deja test. Sin esa guarda, la primera cuenta vieja que se abra después del
  deploy revienta al parsear `undefined`.
- **El alto de la pantalla — ya medido.** El acceso cuesta 4 px (la fila del Total pasa de 40 a 44).
  Lo que queda sin medir en dispositivo real es el teclado numérico abierto sobre el campo: el
  `scrollIntoView` y el `maxH` en dvh son la defensa, y un test de jsdom puede verificar que el
  contenedor tiene alto acotado, pero **ningún test de este repo puede simular el teclado de
  Android**. Se verifica a mano en la tableta antes de dar la feature por buena.
- **`RecalcOrderTotals` es la pieza silenciosa.** Si se olvida, el descuento se pierde en cuanto
  alguien agrega un producto — sin error, con el total simplemente subiendo de más.

## Complexity Tracking

Sin violaciones que justificar. La única exención es la del principio VIII (no registrar quién
financió el descuento), decidida por el dueño y documentada en el spec, no en este plan.
