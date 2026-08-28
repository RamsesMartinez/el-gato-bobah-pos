---

description: "Lista de tasks: visualizador e impresión del ticket de venta"
---

# Tasks: Visualizador e impresión del ticket de venta

**Input**: Design documents from `/specs/001-ticket-preview-print/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/api.md](./contracts/api.md)

**Tests**: Incluidos y **no opcionales**. El principio IV de la constitución es TDD no negociable:
cada task de implementación va precedida de la task del test que falla.

**Organization**: Agrupadas por historia de usuario para que cada una se pueda implementar, probar y
entregar sola.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Se puede correr en paralelo (archivo distinto, sin depender de una task pendiente)
- **[Story]**: A qué historia pertenece (US1, US2, US3)
- Cada task lleva su ruta de archivo exacta

## Path Conventions

Monorepo del repo: backend en `server/`, frontend en `web/`. Sin directorios nuevos salvo
`web/src/features/tickets/`.

---

## Phase 1: Setup

**Purpose**: lo mínimo para que el resto tenga dónde aterrizar

- [X] T001 Crear el directorio `web/src/features/tickets/` para el componente compartido entre POS y tablero
- [X] T002 [P] Exponer el logo por default como data URI importando `web/src/assets/logo.webp?inline` (Vite ya declara `*?inline` en su client.d.ts: no hizo falta tocar `web/src/vite-env.d.ts`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: la migración, el camino de LECTURA de los datos del negocio y el generador del ticket.
Las tres historias dependen de esto.

**⚠️ CRITICAL**: ninguna historia arranca hasta que esta fase cierre

### Backend — esquema y lectura

- [X] T003 Escribir la migración `server/migrations/0033_ticket_business_info.sql` con las siete columnas de [data-model.md](./data-model.md), el check que amarra `logo_bytes` con `logo_mime`, el backfill de `business_name` desde `companies.name` y el `Down` gemelo que las quita todas
- [X] T004 Ampliar `GetBusinessSettings` y agregar `GetTicketLogo` en `server/queries/settings.sql`, sin `WHERE` (RLS acota al tenant), y regenerar con `make sqlc`
- [X] T005 Agregar al test de integración `server/internal/integration/settings_test.go` el caso de que `GET /business-settings` devuelva los campos nuevos y **no** el binario, y correrlo para verlo fallar antes de T006
- [X] T006 Ampliar el struct `BusinessSettings` y `SettingsService.Get` en `server/internal/app/settings.go` con nombre, dirección, teléfono, leyenda, `hasLogo` y `logoUpdatedAt`
- [X] T007 Ampliar la respuesta de `Handlers.BusinessSettings` en `server/internal/httpapi/handlers_settings.go` según [contracts/api.md](./contracts/api.md), sin incluir nunca los bytes del logo (quedó cubierto por T006: el handler serializa el struct del servicio)
- [X] T008 Agregar el test de `GET /business-settings/ticket-logo` en `server/internal/integration/settings_test.go` (404 sin logo, 200 con `Content-Type` guardado, `X-Content-Type-Options: nosniff` y `ETag`, 304 con `If-None-Match`) y correrlo para verlo fallar antes de T009
- [X] T009 Implementar el handler `TicketLogo` en `server/internal/httpapi/handlers_settings.go` y su ruta autenticada en `server/internal/httpapi/router.go`

### Frontend — datos y generador del ticket

- [X] T010 [P] Ampliar `BusinessSettings` en `web/src/api/pos.ts` (ahí vive la interfaz, no en types/) con `businessName`, `address`, `phone`, `footerNote`, `hasLogo` y `logoUpdatedAt`
- [X] T011 [P] Agregar `ticketLogo()` en `web/src/api/pos.ts` apuntando a `/business-settings/ticket-logo`
- [X] T012 Crear `web/src/features/tickets/useTicketBusinessInfo.ts`: junta los ajustes con el logo ya convertido a data URI y cae al default cuando la respuesta es 404
- [X] T013 Escribir los casos que fallan en `web/src/utils/printReceipt.test.ts`: encabezado con y sin cada campo opcional, logo default vs. subido, marca de reimpresión, que los casos de escape de XSS que ya existen sigan pasando, que los cuatro campos del negocio pasen por `esc()` (un `<img onerror>` en `businessName` no debe dejar markup vivo: el documento es `srcdoc` y por lo tanto same-origin), y que los importes impresos sean exactamente los del pedido, sin recálculo (FR-014)
- [X] T014 Ampliar `buildReceiptHtml(order, business, opts)` en `web/src/utils/printReceipt.ts` para el encabezado, el logo como data URI y la marca de reimpresión, pasando nombre, dirección, teléfono y leyenda por `esc()` igual que los datos del pedido, y manteniéndola pura
- [X] T015 Escribir el test de `printHtml` en `web/src/utils/printReceipt.test.ts`: monta el documento en un iframe y dos toques seguidos producen una sola impresión
- [X] T016 Implementar `printHtml(html)` por iframe en `web/src/utils/printReceipt.ts` y eliminar el `window.open` con su `if (!w) return` que fallaba en silencio. La impresión se dispara desde el padre con `iframe.contentWindow.print()`: **nunca** con un `<script>` dentro del `srcdoc`, que la CSP de producción (`script-src 'self'`) bloquea aunque en dev funcione

**Checkpoint**: el ticket se genera con encabezado y logo, y se puede imprimir sin popup

---

## Phase 3: User Story 1 — Ver el ticket y mandarlo a imprimir al cerrar el pedido (Priority: P1) 🎯 MVP

**Goal**: el cajero ve el ticket antes de gastar papel y lo imprime desde ahí mismo.

**Independent Test**: cerrar un pedido con dos líneas y un modificador, comparar la pantalla contra
el papel campo por campo ([quickstart.md](./quickstart.md) §US1).

### Tests for User Story 1

- [X] T017 [P] [US1] Escribir `web/src/features/tickets/TicketPreview.test.tsx`: el modal monta el documento del ticket, el botón imprime una sola vez ante dos toques, y cerrar sin imprimir no dispara impresión

### Implementation for User Story 1

- [X] T018 [US1] Implementar `web/src/features/tickets/TicketPreview.tsx`: modal de Chakra con el iframe a 80mm reales, escalado con `transform` solo para pantalla, y las acciones imprimir y cerrar alcanzables sin scroll en 7"
- [X] T019 [US1] Cablear en `web/src/features/pos/POSPage.tsx`: el botón del modal de confirmación abre `TicketPreview` con `reprint: false` en lugar de llamar a `printReceipt` a ciegas
- [X] T020 [US1] Recorrer a mano [quickstart.md](./quickstart.md) §US1 incluida la verificación en papel, y anotar en el mismo archivo cualquier diferencia entre pantalla y papel

**Checkpoint**: US1 entregable sola. El POS ya no imprime a ciegas.

---

## Phase 4: User Story 2 — Reimprimir el ticket de un pedido ya cerrado (Priority: P2)

**Goal**: recuperar el ticket de un pedido cerrado, marcado como reimpresión.

**Independent Test**: cerrar un pedido, ir al tablero y reimprimirlo; el papel sale marcado
([quickstart.md](./quickstart.md) §US2).

### Tests for User Story 2

- [X] T021 [P] [US2] Escribir `web/src/features/orders/OrdersBoardPage.test.tsx`: la acción de ticket pide el pedido completo y abre la vista previa con `reprint: true`

### Implementation for User Story 2

- [X] T022 [US2] Agregar la acción "Ticket" en `web/src/features/orders/OrdersBoardPage.tsx` que llama `posApi.order(id)` y abre `TicketPreview` con `reprint: true`, disponible también en la sección de entregados
- [X] T023 [US2] Recorrer a mano [quickstart.md](./quickstart.md) §US2, incluido el caso del pedido cancelado o reembolsado

**Checkpoint**: US1 y US2 funcionan de forma independiente

---

## Phase 5: User Story 3 — Configurar los datos del negocio y el logo (Priority: P3)

**Goal**: que el ticket deje de estar escrito en el código.

**Independent Test**: cambiar nombre y logo y ver el cambio en el siguiente ticket sin reiniciar
([quickstart.md](./quickstart.md) §US3).

### Tests for User Story 3

- [X] T024 [P] [US3] Escribir `server/internal/domain/logo_test.go` table-driven: tamaño sobre 256 KB, tipo real fuera de la lista blanca, JPEG renombrado a `.png` clasificado por contenido, SVG con `<script>` rechazado, y lado mayor a 1024 px
- [X] T025 [P] [US3] Escribir los casos de `ValidateBusinessInfo` en `server/internal/domain/businessinfo_test.go`: nombre vacío, nombre de 61 caracteres y cada campo opcional en su límite
- [X] T026 [P] [US3] Escribir `server/internal/httpapi/handlers_settings_test.go`: un cajero recibe 403 al subir logo o cambiar datos, y el rechazo deja el evento `forbidden`

### Implementation for User Story 3

- [X] T027 [US3] Implementar `server/internal/domain/logo.go` con los sentinels `ErrLogoTooLarge`, `ErrLogoType`, `ErrLogoDimensions`, la función pura `ValidateLogo` (tipo por contenido con `http.DetectContentType`, dimensiones con `image.DecodeConfig`), y `ValidateBusinessInfo` en `server/internal/domain/businessinfo.go`
- [X] T028 [US3] Mapear los tres sentinels nuevos en `server/internal/httpapi/respond.go` para que caigan en 400/422 y nunca en 500 — **sin cambios**: los sentinels envuelven `ErrValidation`, que el switch ya mapea a 400 con el mensaje real
- [X] T029 [US3] Agregar `UpdateBusinessInfo`, `SetTicketLogo` y `ClearTicketLogo` en `server/queries/settings.sql` y regenerar con `make sqlc`
- [X] T030 [US3] Agregar `SetBusinessInfo`, `SetLogo` y `ClearLogo` en `server/internal/app/settings.go`, validando en `domain` antes de tocar el store
- [X] T031 [US3] Implementar `UpdateBusinessSettings` ampliado, `UploadTicketLogo` y `DeleteTicketLogo` en `server/internal/httpapi/handlers_settings.go`, reusando la secuencia `MaxBytesReader` → `ParseMultipartForm` → `RemoveAll` de `server/internal/httpapi/handlers_purchasedoc.go`
- [X] T032 [US3] Registrar las rutas nuevas bajo `RequireRole(admin, gerente)` en `server/internal/httpapi/router.go`
- [X] T033 [P] [US3] Agregar los campos del negocio y el control de subir/quitar logo en `web/src/features/admin/BusinessSettingsPage.tsx`, con vista previa de la imagen y el mensaje de rechazo que diga qué se aceptaba
- [X] T034 [US3] Recorrer a mano [quickstart.md](./quickstart.md) §US3, incluido el `curl` con token de cajero que debe responder 403


### Ampliación de US3 — textos del ticket e interruptor de impresión automática

> Los IDs se agregan al final y no se renumeran: renumerar movería tasks ya marcadas y rompería
> las referencias de los commits.

- [X] T040 [US3] Escribir la migración `server/migrations/0034_ticket_notes_autoprint.sql` con `header_note text` (check ≤ 120) y `auto_print_on_close boolean not null default false`, más su `Down` gemelo. **En archivo aparte y no dentro de la 0033**, que ya corrió en dev: editar una migración aplicada deja esquemas distintos según quién migró antes
- [X] T041 [P] [US3] Agregar a `server/internal/domain/businessinfo_test.go` los casos de `headerNote` (vacío permitido, 120 y 121 caracteres)
- [X] T042 [US3] Ampliar `ValidateBusinessInfo` en `server/internal/domain/businessinfo.go` con `headerNote`
- [X] T043 [US3] Llevar `headerNote` y `autoPrintOnClose` por las tres capas: `server/queries/settings.sql` (+ `make sqlc`), `server/internal/app/settings.go` y `server/internal/httpapi/handlers_settings.go`, según [contracts/api.md](./contracts/api.md)
- [X] T044 [US3] Escribir en `web/src/utils/printReceipt.test.ts` los casos del texto superior: se imprime arriba del detalle, se omite cuando está vacío, y pasa por `esc()`
- [X] T045 [US3] Renderizar `headerNote` en `buildReceiptHtml` (`web/src/utils/printReceipt.ts`), arriba del detalle del pedido
- [X] T046 [US3] Agregar en `web/src/features/admin/BusinessSettingsPage.tsx` los dos campos de texto y el interruptor de impresión automática, con el aviso de que el interruptor solo sirve si el navegador imprime sin diálogo

---


### Ampliación — ticket de prueba y bloques de texto

- [X] T053 [US3] Subir los textos del ticket de 120 a 400 caracteres y respetar los saltos de línea (`white-space: pre-line`), con sus casos en `web/src/utils/printReceipt.test.ts` y la migración `server/migrations/0035_ticket_notes_block.sql`
- [X] T054 [US3] Sembrar en la `0035` el pie por default (aviso de "sin valor fiscal" y cómo pedir factura) solo donde no haya nada configurado, con separadores de 32 caracteres para que no se partan en el papel
- [X] T055 [US3] Agregar `sampleTicketOrder()` y la marca `** TICKET DE PRUEBA **` en `web/src/utils/printReceipt.ts`, con sus tests
- [X] T056 [US3] Botón "Ticket de prueba" en `web/src/features/admin/PrintSettingsPage.tsx` que abre la vista previa con el pedido de muestra
- [X] T057 [US1] Escalar la vista previa con `transform` en vez de comprimir el iframe, y dejar de cerrar el diálogo al tocar fuera: comprimirlo metía scroll horizontal y arrastrarlo cerraba el ticket a media revisión (`web/src/features/tickets/TicketPreview.tsx`)
- [X] T058 Stub de `ResizeObserver` en `web/src/setupTests.ts`: jsdom no lo implementa y lo usa `useContainerWidth`

---

## Phase 5b: User Story 4 — Imprimir solo al cerrar la venta (Priority: P2)

**Goal**: quitar el toque que más se repite en el día.

**Independent Test**: encender la opción, cerrar un pedido y ver salir el papel sin tocar nada;
apagarla y comprobar que vuelve a pedir el toque ([quickstart.md](./quickstart.md) §US4).

### Tests for User Story 4

- [X] T047 [P] [US4] Escribir en `web/src/utils/printReceipt.test.ts` los casos de `printHtmlOffscreen`: monta un iframe fuera de pantalla, imprime **después** de que el documento cargó, y lo desmonta al terminar aunque la impresión falle
- [X] T048 [P] [US4] Escribir en `web/src/features/tickets/TicketPreview.test.tsx` (o su propio archivo) el caso de que con `autoPrintOnClose` encendido se imprime sin abrir la vista previa, y apagado no se imprime nada

### Implementation for User Story 4

- [X] T049 [US4] Implementar `printHtmlOffscreen(html)` en `web/src/utils/printReceipt.ts`, reusando el candado anti-doble-toque de `printFrame`. Esperar el `load` del iframe antes de imprimir: sin eso el papel sale en blanco, que es exactamente el modo de fallo que ya nos costó una tarde
- [X] T050 [US4] Cablear en `web/src/features/pos/POSPage.tsx`: al cerrar un pedido, si `autoPrintOnClose` está encendido, imprimir sin abrir la vista previa. El botón "Ver ticket" **se queda** (FR-025)
- [X] T051 [US4] Verificar que una impresión fallida (impresora apagada) no pierde ni bloquea la venta: el pedido queda registrado y se puede reimprimir desde el tablero
- [X] T052 [US4] En vez de una verificación suelta: guía numerada dentro de la app (Impresión → ayuda del interruptor) con los pasos para dejar cualquier equipo Windows imprimiendo directo, incluido el destino del acceso directo con botón de copiar

**Checkpoint**: las cuatro historias funcionan de forma independiente

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T035 Dejar verdes los gates: `make api-build && make api-test && make lint` y, en `web/`, `bun run lint && bun run typecheck && bun run test`
- [X] T036 Verificar la persistencia del logo tras `make stop && make start` según [quickstart.md](./quickstart.md), que es lo que prueba que no se guardó en disco del contenedor
- [ ] T037 Verificar la paridad local ↔ desplegado de [quickstart.md](./quickstart.md) **una vez que el deploy lleve este código**: el mismo recorrido de US1 contra la instancia desplegada, sin instalar nada. Lo verificable antes del deploy ya está: el ticket no depende de nada instalado y el logo va como data URI porque la CSP de producción (`img-src 'self' data:`) bloquearía un `<img src>` a la API, que vive en otro dominio
- [X] T038 Verificar la legibilidad en la tablet de 7" según [quickstart.md](./quickstart.md) y ajustar el escalado del iframe en `web/src/features/tickets/TicketPreview.tsx` si hace falta zoom
- [X] T039 Runbook en [docs/impresion-tickets.md](../../docs/impresion-tickets.md) + su renglón en el índice `docs/README.md`, como pide AGENTS.md §6. Escribir el runbook (impresora como default, arranque de Edge con `--kiosk-printing` y el gotcha del `--user-data-dir`, ajustes del driver que hacen el corte) y agregar su renglón en `docs/README.md`, como pide AGENTS.md §6

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001–T002)** → sin dependencias.
- **Foundational (T003–T016)** → depende de Setup. **Bloquea todo lo demás.**
- **US1 (T017–T020)** → depende de Foundational.
- **US2 (T021–T023)** → depende de Foundational y de `TicketPreview` (T018). No depende de US3.
- **US3 (T024–T034)** → depende de Foundational. **No depende de US1 ni de US2**: el backend del
  camino de escritura es independiente del componente de vista previa.
- **Polish (T035–T039)** → depende de todo lo anterior.

### User Story Dependencies

- **US1** es independiente y es el MVP.
- **US2** reusa el componente de US1; si se entrega US1 sola, US2 es un cableado más.
- **US3** es independiente de las dos: se puede construir en paralelo por otra persona en cuanto
  cierre Foundational, porque toca archivos distintos (backend + página de admin).

### Within Each User Story

Test primero, implementación después. En US3, `domain` antes que `app`, `app` antes que `httpapi`, y
el front al final: es el orden de capas del principio I.

### Parallel Opportunities

- T010 y T011 en paralelo con el backend de T003–T009: archivos distintos, sin dependencia.
- T024, T025 y T026 en paralelo entre sí: tres archivos de test distintos.
- T033 en paralelo con T027–T032 mientras el contrato de [contracts/api.md](./contracts/api.md) se
  respete.
- **US3 completa en paralelo con US1 y US2** una vez cerrada Foundational.

## Parallel Example: User Story 3

```text
En cuanto cierre Foundational, tres frentes a la vez:
  A) T024 + T025  → tests de domain (Go)
  B) T026         → test de authz en httpapi (Go)
  C) T033         → UI de ajustes (React), contra el contrato ya escrito
Convergen en T027–T032, que sí van en orden de capas.
```

## Implementation Strategy

### MVP First (User Story 1 Only)

Setup + Foundational + US1 = **T001–T020**. Con eso el POS deja de imprimir a ciegas, que es el
problema que duele todos los días. El ticket sale con el nombre sembrado desde `companies.name` y el
logo del Gato Bobah: nadie tiene que configurar nada para que funcione.

### Incremental Delivery

1. **T001–T020** → MVP: ver e imprimir al cerrar. Entregable y usable.
2. **T021–T023** → reimpresión desde el tablero. Cierra el hueco del papel atorado.
3. **T024–T034** → datos y logo configurables. El ticket deja de estar en el código.
4. **T035–T039** → gates y verificaciones de campo.

### Parallel Team Strategy

Con dos personas: una toma US1+US2 (todo front) y otra toma US3 (backend + página de admin). No se
pisan un solo archivo. El punto de sincronización es Foundational, que conviene hacer de a uno para
no pelear con `make sqlc`.

## Notes

- **La verificación en papel no es opcional.** SC-002 pide comparación campo por campo entre
  pantalla y papel; un test de vitest no puede probarlo. Las tasks T020, T023 y T034 son manuales a
  propósito.
- **Ningún task usa `--no-verify`.** Si un hook truena, se arregla la causa (Quality gates de la
  constitución).
- Los sentinels nuevos se mapean en `httpapi.Error` y en ningún otro lugar: el principio II prohíbe
  repartir `http.Error` por los handlers.
