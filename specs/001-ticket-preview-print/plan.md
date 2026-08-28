# Implementation Plan: Visualizador e impresión del ticket de venta

**Branch**: `001-ticket-preview-print` | **Date**: 2026-08-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-ticket-preview-print/spec.md`

## Summary

La vista previa y lo que se imprime son **el mismo documento**: `buildReceiptHtml()` produce un HTML
autocontenido de 80mm, ese HTML se monta en un `<iframe srcdoc>` dentro del modal, y el botón de
imprimir llama `print()` sobre **ese** iframe. No hay dos renders que puedan divergir (FR-002), no
hay popup que el bloqueador mate, y funciona igual en `localhost` y en el sitio publicado —front en
Cloudflare Pages, API detrás de Caddy— porque no depende de nada instalado en el equipo (FR-009).

El encabezado sale de `business_settings`, que gana cuatro columnas de identidad del negocio y el
logo como `bytea` en la misma fila. El logo viaja al ticket como **data URI**, resuelto antes de
abrir el modal: un `<img src>` remoto puede no haber cargado cuando se dispara `print()` y sale un
hueco blanco en el papel.

## Technical Context

**Language/Version**: Go 1.27 (server) · TypeScript 5 / React 19 (web)

**Primary Dependencies**: chi · pgx + sqlc · goose (embebidas) — Vite · Chakra UI v3 · TanStack
Query · Zustand

**Storage**: PostgreSQL 16. El logo va en `business_settings.logo_bytes` (bytea), no en disco — ver
[research.md](./research.md) D3.

**Testing**: `go test ./...` table-driven junto al código · vitest + @testing-library/react en web

**Target Platform**: API en contenedor Linux sobre VPS e2-micro (1 vCPU, 1 GB) · front PWA en
Edge/Chrome sobre tablet Windows de 7"

**Project Type**: Web — monorepo `server/` + `web/`

**Performance Goals**: la vista previa abre con la orden ya en memoria, sin ida al servidor en el
flujo de cierre · el logo se pide una vez por sesión y se cachea en TanStack Query

**Constraints**: nada instalado en el equipo del operador · logo ≤ 256 KB y ≤ 1024 px de lado ·
la generación del ticket queda como función pura para que la fase 2 (ESC/POS) reutilice los mismos
datos sin reescribir el contenido

**Scale/Scope**: un local, 2–4 tablets, decenas de tickets al día. 5 endpoints (3 nuevos, 2
extendidos), 3 migraciones, una sección nueva en el menú y 3 puntos de entrada al ticket (POS,
tablero y configuración)

## Constitution Check

*GATE: revisado antes de Phase 0 y de nuevo después de Phase 1. Ambas pasadas: PASS.*

| Principio | Cómo lo cumple este plan | Veredicto |
| --- | --- | --- |
| **I. Layering estricto** | `httpapi` decodifica multipart y mapea errores; `SettingsService` orquesta; sqlc hace el SQL; la validación de la imagen y el armado del ticket son funciones puras. Ninguna regla de negocio en el handler. | PASS |
| **II. Errores envueltos** | Sentinels nuevos en `domain` (`ErrLogoTooLarge`, `ErrLogoType`, `ErrLogoDimensions`) envueltos con `%w` y mapeados solo en `httpapi.Error`. `ctx` propagado hasta la query. | PASS |
| **III. Dinero** | El ticket **no** recalcula nada: imprime los importes que el pedido ya tiene (FR-014). Cero aritmética nueva sobre dinero. | PASS |
| **IV. Test-first** | La lógica pura (`ValidateLogo`, `buildReceiptHtml`) se prueba sin DB ni HTTP. Cada task de implementación va precedida de su task de test. | PASS |
| **V. Seguridad** | `RequireRole(admin, gerente)` en el router; `MaxBytesReader` antes de leer el cuerpo (patrón ya usado en `ExtractPurchaseDoc`); tipo verificado por **contenido**, no por el header; SVG rechazado; el binario se sirve con `nosniff`; `logging.SecurityEvent` en el rechazo por permiso; escape de datos de usuario ya cubierto por los tests de `buildReceiptHtml`. | PASS |
| **VI. YAGNI** | Sin capa de "storage provider" ni interfaz de un solo implementador: bytea directo. Sin motor de plantillas de ticket. Sin tabla nueva (columnas en `business_settings`, que ya trae su policy de RLS). | PASS |
| **VII. Comentarios del porqué** | Cada decisión no obvia — data URI en vez de `<img src>`, iframe en vez de popup, bytea en vez de disco — se comenta donde vive el código, no aquí. | PASS |

Sin violaciones que justificar → sección *Complexity Tracking* omitida.

## Project Structure

### Documentation (this feature)

```text
specs/001-ticket-preview-print/
├── plan.md              # este archivo
├── spec.md
├── research.md          # decisiones técnicas y alternativas descartadas
├── data-model.md        # migración, columnas, validaciones
├── quickstart.md        # cómo verificarlo de punta a punta
├── contracts/
│   └── api.md           # contrato de los endpoints
├── checklists/
│   └── requirements.md
└── tasks.md             # lo genera /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/
│   ├── 0033_ticket_business_info.sql      # identidad del negocio + logo
│   ├── 0034_ticket_notes_autoprint.sql    # texto superior + impresión automática + mimes
│   └── 0035_ticket_notes_block.sql        # textos a 400 caracteres + pie precargado
├── queries/
│   └── settings.sql                        # + GetTicketLogo, UpsertTicketLogo, DeleteTicketLogo,
│                                           #   UpdateBusinessInfo (y GetBusinessSettings ampliado)
├── internal/
│   ├── domain/
│   │   ├── logo.go                         # NUEVO: ValidateLogo puro + sentinels (PNG/JPEG)
│   │   ├── logo_test.go                    # NUEVO: table-driven
│   │   ├── businessinfo.go                 # NUEVO: ValidateBusinessInfo puro
│   │   └── businessinfo_test.go            # NUEVO: table-driven
│   ├── app/
│   │   └── settings.go                     # + SetBusinessInfo, SetLogo, Logo, ClearLogo
│   ├── httpapi/
│   │   ├── handlers_settings.go            # + UploadTicketLogo, TicketLogo, DeleteTicketLogo
│   │   ├── handlers_settings_test.go       # NUEVO: authz + límites
│   │   └── router.go                       # + rutas del logo bajo RequireRole
│   └── store/db/                           # regenerado por `make sqlc` (no se edita)

web/src/
├── utils/
│   ├── printReceipt.ts                     # buildReceiptHtml (encabezado, logo, marcas de
│   │                                       # reimpresión y prueba) + printFrame/printHtmlOffscreen
│   │                                       # + sampleTicketOrder. Adiós window.open
│   └── printReceipt.test.ts
├── features/tickets/                       # NUEVO, compartido por POS, tablero y configuración
│   ├── TicketPreview.tsx                   # modal con el iframe escalado y el botón de imprimir
│   ├── ticketBusinessInfo.ts               # ajustes + logo como data URI (hook + función pura)
│   ├── AutoPrintTicket.tsx                 # imprime al cerrar la venta, sin UI
│   ├── ReprintTicket.tsx                   # pide el pedido completo y abre la vista marcada
│   └── *.test.tsx / *.test.ts
├── features/pos/POSPage.tsx                # "Ver ticket" + AutoPrintTicket
├── features/orders/OrdersBoardPage.tsx     # + acción "Ticket" (reimpresión)
├── features/admin/PrintSettingsPage.tsx    # NUEVO: sección Impresión (logo, textos, interruptor,
│                                           # ticket de prueba y ayuda de impresión directa)
├── app/{App,AppShell,roles}.tsx            # ruta /impresion + entrada de menú + gate de rol
├── setupTests.ts                           # + stub de ResizeObserver (jsdom no lo trae)
└── api/{pos,client}.ts                     # endpoints del ticket + api.getRaw / api.putForm
```

**Structure Decision**: monorepo existente, con un directorio nuevo (`web/src/features/tickets/`,
compartido por POS, tablero y configuración) y una sección nueva en el menú (`/impresion`). La
configuración del ticket va en su propia pantalla y no dentro de Negocio: es donde el operador la
busca cuando el papel sale mal, y deja lugar a la impresora de cocina de la fase 2.

## Phase 0 — Research

Ver [research.md](./research.md). Nueve decisiones cerradas, cada una con su alternativa descartada.
D1–D6 salieron del diseño:
transporte de impresión (iframe), fidelidad preview/papel (mismo documento), almacenamiento del logo
(bytea), entrega del logo (data URI), validación de la imagen (por contenido) y forma de la subida
(multipart, reusando el patrón de `ExtractPurchaseDoc`). D7–D9 salieron de imprimir en papel:
legibilidad en térmica (nada de grises), escalar la vista en vez de comprimirla, y marcar el ticket
de prueba.

## Phase 1 — Design

- [data-model.md](./data-model.md) — la migración `0033`, las columnas nuevas con sus checks, las
  reglas de validación y el backfill del nombre desde `companies.name`.
- [contracts/api.md](./contracts/api.md) — los tres endpoints, sus códigos de respuesta y quién
  puede llamarlos.
- [quickstart.md](./quickstart.md) — cómo verificar cada historia de la spec, incluido el papel.
