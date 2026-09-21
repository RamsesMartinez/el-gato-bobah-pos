# Implementation Plan: El panel del pedido se reacomoda

**Branch**: `023-el-panel-se-reacomoda` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/023-el-panel-se-reacomoda/spec.md`

## Summary

Front puro. **No hay esquema, no hay API nueva, no hay migración**: el endpoint que la historia 3
necesita —`PUT /orders/{id}/discount`— se construyó y se probó en la 022, y quedó sin pantalla a
propósito. Esta feature le pone la pantalla y, de paso, devuelve al panel los 52 px que gastaba en
lo que este negocio casi no usa.

La regla que ordena todo el cambio, y que es lo que hay que poder repetir dentro de seis meses:
**arriba lo que describe al pedido, abajo lo que mueve dinero.**

## Technical Context

**Language/Version**: TypeScript 5 + React 19 (solo `web/`)

**Primary Dependencies**: Chakra UI v3 (`MenuRoot`/`MenuContent`/`MenuItem` ya envueltos en
[components/ui/menu.tsx](../../web/src/components/ui/menu.tsx), usados hoy en `OrdersBoardPage` y
`ProductsAdminPage`), Zustand para la cuenta.

**Storage**: ninguno nuevo. El descuento y el cliente ya viven en la cuenta (`egb:ticket:v2`) y en
`orders`.

**Testing**: vitest + Testing Library. El caso de 1024×600 se simula como en la 022: mockeando
`useContainerWidth` y `matchMedia`, porque jsdom no hace layout.

**Target Platform**: tabletas de 7–10", presupuesto 1024 × 600.

**Constraints**: el panel mide ~330 px de ancho; el encabezado tiene que caber sin empujar nada
fuera. Ningún control tappable por debajo de 44 px.

**Scale/Scope**: cuatro archivos de `web/src`, ninguno de `server/`.

## Constitution Check

| Principio | Cómo lo cumple |
|---|---|
| **I. Layering** | No se toca backend. La regla de qué se puede editar (pedido no cobrado) ya vive en `domain` y en `app.SetDiscount`; la pantalla solo la refleja. |
| **III. Dinero** | Ninguna cifra cambia de fórmula. El descuento sigue restándose en el servidor; la hoja de cobro muestra lo que el servidor devuelve, nunca un total propio. |
| **IV. Test-first** | Los bordes están enumerados abajo con su test, y se escriben antes. |
| **V. Seguridad** | Sin superficie nueva: la pantalla llama al endpoint que ya existe, con su tope por usuario y su evento de seguridad. |
| **VI. YAGNI** | El menú lleva **tres** acciones, las que ya existían. No se inventa un centro de ajustes del pedido. |
| **VIII. Puerta abierta** | *¿Esto se puede agregar después al mismo costo?* Sí: es pantalla. No se cierra ninguna puerta — el nombre del cliente **sigue existiendo y viajando**, que es justo lo que pidió el dueño para el negocio que sí lo use. |
| **Restricciones del producto** | El tipo se alterna con un toque (no un `<select>`, no un `Picker` para dos opciones); los renglones del menú miden 48 px; el texto es de mostrador. |

**Resultado del gate: pasa.**

## Lo que cambia, archivo por archivo

```text
web/src/features/pos/
├── Ticket.tsx              el encabezado nuevo, la zona baja sin controles, el menú
├── Ticket.test.tsx         los casos de abajo
└── POSPage.test.tsx        que las tres superficies sigan diciendo lo mismo

web/src/shared/
├── CobrarSheet.tsx         corregir el descuento de un pedido ya creado
└── CobrarSheet.test.tsx

web/src/api/pos.ts          `setOrderDiscount(id, {amount|percent})` → PUT /orders/{id}/discount
```

## Decisiones de diseño

### 1. El encabezado, y el ancho que de verdad hay

```text
[ocultar]  Colorpoint Sho…      [Mostrador]  [⋮]
```

**Medido, no supuesto**: el panel es `clamp(300px, 32%, 380px)` — a 1024 px da **328**, menos 24 de
padding = **304 útiles**. Los controles fijos (ocultar 48, tipo ~112, menú 44, tres gaps de 8) se
llevan 228, así que al nombre del pedido le quedan **~76 px, unos 10 caracteres**.

Y el nombre no es corto: **el esquema por omisión es `razas`, no `animales`**
([folio.go](../../server/internal/domain/folio.go), `EsquemaPorDefecto = EsquemaRazas`, y el
`default 'razas'` de [0046](../../server/migrations/0046_folio_con_nombre.sql)). «Colorpoint
Shorthair» son 20 caracteres, ~150 px. Contra eso, dos decisiones escritas:

- **Se quita la palabra «Pedido»** del encabezado. Con el ancho como recurso escaso, esa palabra es
  puro adorno: lo que identifica al pedido es el nombre.
- **Lo que cede es el ancho del nombre, que trunca con puntos suspensivos. Nunca la altura de un
  control.** Se dice aquí porque es exactamente la situación en la que alguien baja un botón de 44 a
  36 px para ganar ancho, y el dedo deja de acertar. El nombre completo sigue estando en la barra de
  cuentas, en el ticket y en la comanda.

- **El tipo** es un botón que alterna mostrador ↔ domicilio. Muestra el activo con su icono y su
  palabra. En un pedido de plataforma **no se pinta**: ese pedido es a domicilio por definición y
  ofrecer el cambio sería ofrecer algo que el servidor rechaza.
- **El menú** (`⋮`) abre: *Nombre del cliente*, *Descuento*, *Vaciar el pedido*. Renglones de 48 px.
  Vaciar va abajo, separado por una línea y en rojo — sigue pidiendo confirmación.
- **`Vaciar` sale del encabezado** como botón de texto. Era el control que más estorbaba: destructivo
  y poco usado, compitiendo por ancho con lo que sí se usa.

### 2. Dónde se captura lo que el menú abre

Un campo **inline, temporal, entre el encabezado y la lista de renglones** — fuera de la caja de
totales, que tiene `maxH="60dvh"`. Aparece al elegirlo del menú y se va al confirmar.

**Fuera de esa caja, y no es un detalle**: el aviso de «ya no están en el menú» vivía adentro y
mandó el botón COBRAR a un scroll interno; por eso se sacó ([Ticket.tsx, el comentario del
aviso](../../web/src/features/pos/Ticket.tsx)). Metido ahí, el campo repetiría el mismo defecto. En
su lugar correcto, lo único que se encoge es la lista —que ya tiene su scroll— y el pie queda
anclado: **los botones no se mueven bajo el dedo**. No es una hoja inferior porque escribir un nombre no merece un viaje de pantalla completa,
y no es permanente porque entonces no habríamos ganado nada.

El resultado sí es permanente donde importa: **un descuento aplicado se lee en la zona de totales**,
en el renglón que la 022 ya pinta bajo el Total. Es dinero; esconderlo sería cobrar de menos sin
decir por qué.

### 3. La zona baja

```text
Total                                   $385.00
  $385.00 − $50.00 de descuento          (solo si hay)
Envío  [____]                            (solo domicilio propio)
[ Enviar a cocina ]        [ COBRAR ]
Cobrar también manda el pedido a cocina
```

Sin un solo control tocable salvo el envío y los dos botones. **El envío se queda**: es un importe
que suma al total y se lee junto a él, no un atributo que se esconda.

### 4. Corregir el descuento en la hoja de cobro

Un renglón en el cuerpo de la hoja, **solo cuando el pedido ya existe**: muestra el descuento vigente
y un control para cambiarlo, o «Agregar descuento» si no hay. Llama a `PUT /orders/{id}/discount` y
repinta con lo que el servidor devuelve — nunca con una cifra calculada aquí.

El rechazo del servidor (pedido ya cobrado, descuento mayor que la venta) se muestra tal cual: esos
mensajes ya vienen redactados para el operador y con el máximo adentro.

## Bordes → dónde queda su test

| Borde | Qué debe pasar | Test |
|---|---|---|
| Pedido de mostrador, sin cliente ni descuento | La zona baja NO trae fila de tipo ni campo de cliente | `Ticket.test.tsx` |
| Pedido de plataforma | El botón de tipo no se pinta | `Ticket.test.tsx` |
| Menú abierto | Las tres acciones, todas ≥48 px | `Ticket.test.tsx` |
| Vaciar desde el menú | Sigue pidiendo confirmación | `Ticket.test.tsx` |
| Descuento aplicado | Se ve en la zona de totales **con el menú cerrado** | `Ticket.test.tsx` |
| Cliente capturado desde el menú | Llega al cuerpo del pedido que se manda | `pedido.test.ts` |
| Las tres superficies | Panel, píldora y barra siguen diciendo la misma cifra | `POSPage.test.tsx` (anchos 500 y 1024) |
| Hoja de cobro, pedido con descuento | Se puede corregir y el total repinta con lo del servidor | `CobrarSheet.test.tsx` |
| Hoja de cobro, pedido cobrado | El rechazo del servidor se muestra; ninguna cifra se mueve | `CobrarSheet.test.tsx` |
| Hoja de cobro, pedido sin descuento | No hay renglón en $0.00, pero sí se puede agregar | `CobrarSheet.test.tsx` |
| Cuenta guardada por la versión anterior | Sigue hidratando sin campos nuevos | `cuentaGuardadaAntes.test.ts` (la guardia de clase ya existente) |
| Nombre de folio del esquema **razas** (20 caracteres) | El nombre trunca; el tipo y el menú NO se comprimen ni bajan de 44 px | `Ticket.test.tsx` |
| El panel se colapsa con el menú abierto | El menú se va con él; no queda flotando sobre el catálogo | `POSPage.test.tsx` (cruzando el umbral de ancho) |
| Carrito vacío | «Vaciar el pedido» no se ofrece en el menú, como hoy no se ofrece el botón | `Ticket.test.tsx` |
| El campo inline abierto | Los botones de acción NO se mueven: el que se encoge es el scroll de la lista | `Ticket.test.tsx` |

## Riesgos conocidos

- **El ancho del encabezado — ya medido, no resuelto del todo.** Con el esquema `razas` el nombre
  trunca a ~10 caracteres. Es tolerable porque el nombre completo vive en la barra de cuentas, en el
  ticket y en la comanda, pero si en la tableta se lee mal, la salida es que el tipo muestre solo su
  icono y no la palabra. Se decide viéndolo, no antes.
- **El menú sobre un panel que se colapsa.** A 1024×600 el panel puede ocultarse con el menú abierto;
  el menú tiene que morir con él y no quedar flotando sobre el catálogo.
- **Un toque menos, no uno más.** SC-002 dice que el caso de todos los días no puede costar un toque
  extra. Medido: el pedido de mostrador sin cliente ni descuento cuesta **cero toques** en esos tres
  controles, antes y después, porque el tipo nace en «mostrador» y el diseño solo lo muestra.
- **Lo que sí cuesta más, dicho con su número**: capturar el nombre del cliente pasa de **1 toque**
  (enfocar el campo siempre visible) a **2** (abrir el menú y elegirlo). Es la contraparte aceptada a
  cambio de los 52 px, y es la razón por la que el dueño pidió mover justo ese campo: en este negocio
  casi no se usa.

## Complexity Tracking

Sin violaciones que justificar.
