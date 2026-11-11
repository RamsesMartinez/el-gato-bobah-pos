---
description: "Tareas de la 074: empaques de compra y edición de compras guardadas por MCP"
---

# Tasks: Empaques de compra y edición de compras guardadas desde el servidor MCP

**Input**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md),
[contracts/mcp-tools.md](contracts/mcp-tools.md), [quickstart.md](quickstart.md)

**Tests**: obligatorios (constitución IV y encargo: TDD estricto). Cada test se corre y **se ve en
rojo por la razón correcta** antes de escribir el código que lo pone en verde; el rojo se anota en el
commit.

**Orden de historias**: la US2 (crear el empaque) va antes que la US1 (corregir la compra) porque el
caso de la US1 necesita `presentation_create`. Las dos son P1.

## Phase 1: Setup

- [X] T001 Restaurar el respaldo más reciente en la base del worktree (`make db-restaurar dump=backups/prod/gatobobah-20261111-0045.dump`) y comprobar `readyz` de la API local en `http://localhost:8074/readyz` (hecho al especificar; repetir si la base se recreó)

## Phase 2: Foundational (catálogo y pistas de error)

- [X] T002 Test de tabla en server/internal/domain/apitoken_test.go: `MCPToolByName` encuentra `presentation_create`, `presentation_update` (alcance `write:inventory`, permiso `inventory.configure`) y `expense_edit` (`write:expenses`, `expenses.edit`), las tres `Write`; y `ToolErrorHintFor` con `EXPENSE_EDIT_STALE` dice «vista previa» y con `DECISION_REQUIRED` dice «decisions». Correrlo y verlo en rojo
- [X] T003 Agregar los tres renglones al catálogo `mcpTools` y los dos códigos a `toolFixByCode` en server/internal/domain/apitoken.go; T002 en verde (`go test ./internal/domain/`)

## Phase 3: User Story 2 — Crear el empaque de una marca nueva (P1)

**Goal**: el agente crea y edita empaques, y la vista previa de una compra sugiere crear el que falta.

**Independent Test**: vista previa de compra con un renglón de insumo sin empaque trae `create_presentation`; crear y editar un empaque por MCP con los rechazos de la pantalla.

- [X] T004 [US2] Escribir server/internal/integration/mcp_074_test.go, `TestMCPPresentations`: vista previa sin escribir (lista los empaques del insumo); aplicar crea con marca; diez veces la misma clave = un empaque; 409 marca repetida en el mismo insumo; 400 marca de 41 letras; `presentation_update` cambiando solo la marca conserva nombre y contenido; `brand: ""` la quita; `active: false` conserva la marca; editar el nombre de un aproximado conserva su unidad de compra; evento de la llamada aplicada con su antes y después; token con solo `write:catalog` no ve ni `presentation_create` ni `presentation_update` y la llamada directa se niega nombrando el alcance; rol sin `inventory.configure` (con alcance completo) no ve ninguna de las dos; token de otra empresa → «No existe». Correr con `make test-integration` y verlo en rojo (herramienta desconocida)
- [X] T005 [US2] En el mismo archivo, `TestMCPPurchaseSuggestsCreatingThePresentation`: renglón con insumo y sin empaque → `presentations` con `brand` y `create_presentation` con `ingredient_id` y `unit_id` llenos y `ask` con marca y contenido; renglón con `unit_id` → nota «sin marca ni empaque». Verlo en rojo
- [X] T006 [US2] Crear server/internal/httpapi/mcp_tools_074.go con rutas (lista blanca y `mcpListTool`) y `presentation_create`/`presentation_update` según contracts/mcp-tools.md y research R4 (update parte del empaque actual; unidad base por `GET /units`); T004 en verde
- [X] T007 [US2] Cambiar la vista previa de `purchase_create` en server/internal/httpapi/mcp_tools_073.go: `presentationsOf` trae `brand`; sugerencia `create_presentation` y nota de unidad (research R5); agregar `GET /stock/articles/{id}` y `GET /units` a su lista blanca; T005 en verde y `TestMCPPurchases` sigue verde

## Phase 4: User Story 1 — Corregir una compra que entró en la unidad base (P1) — MVP

**Goal**: `expense_edit` con el protocolo de la ruta; el caso del encargo de punta a punta.

**Independent Test**: compra en gramos → empaque → edición; existencias, costo e importe iguales a los de una compra de referencia capturada bien.

- [X] T008 [US1] En server/internal/integration/mcp_074_test.go, `TestMCPExpenseEditFixesAPurchase`: dos insumos gemelos con el mismo costo inicial; uno comprado bien (empaque 375 g), otro en gramos y corregido con `presentation_create` + `expense_edit` (`remove` + `add` con `arrived_on`); al final, misma existencia, mismo costo vigente, mismo importe y estado, y el renglón vivo con empaque y marca. Más: vista previa no escribe y trae `fingerprint`, `inventory`, `costs`, `money`; aplicar sin `fingerprint` se rechaza antes de despachar; huella vencida (cambiar el gasto entre vista previa y aplicar) → error que pide la vista previa y nada cambia; reintento con la misma clave y la huella nueva aplica; diez veces la misma clave = una edición; decisión faltante (conteo cerrado después de la llegada) → error que nombra `count:<id>` y nada cambia, y con `keep` aplica; evento `expense.edited` con el token en el dispositivo. Verlo en rojo
- [X] T009 [US1] En el mismo archivo, `TestMCPExpenseEditGates`: token con solo `write:purchases` no la ve; rol con alcance y sin `expenses.edit` no la ve; rol con `expenses.edit` y sin `inventory.receive` quitando un renglón recibido → rechazo que nombra el permiso; token de otra empresa → «No existe»; `arrived_on` con forma inválida se rechaza en el esquema; sin `arrived_on` el renglón agregado queda por recibir y la vista previa lo dice. Verlo en rojo
- [X] T010 [US1] Agregar `expense_edit` a server/internal/httpapi/mcp_tools_074.go (research R1–R3; `arrived_on` en los campos de fecha del esquema): vista previa con `?preview=1`, aplicar con `fingerprint` y `decisions`, antes y después con `GET /expenses/{id}`, notas de renglones que quedan por recibir; T008, T009 y T012 en verde
- [X] T011 [US1] Ampliar `TestDispatcherRefusesRoutesOutsideCatalog` en server/internal/httpapi/mcp_dispatch_test.go: `expense_edit` no despacha `/expenses/{id}/deliveries` ni `/expenses/{id}/exclude`

## Phase 5: User Story 3 — Encabezado y renglones de un gasto (P2)

- [X] T012 [US3] En server/internal/integration/mcp_074_test.go, `TestMCPExpenseEditHeader`: cambiar proveedor (y `clear_supplier`) y categoría muestra antes y después; agregar un renglón sin mercancía a un gasto pagado sube el importe y pide la decisión `money`, los pagos no cambian; gasto anulado → rechazo con la causa. Escribirlo junto con T008 y T009, antes de T010, y verlo en rojo
- [X] T013 [US3] Ajustar server/internal/httpapi/mcp_tools_074.go si T012 lo pide (`supplier_id` / `clear_supplier` mutuamente excluyentes); T012 en verde

## Phase 6: Polish & Cross-Cutting

- [X] T014 [P] Test en web/src/features/admin/IntegrationsPage.test.tsx: el alcance Gastos dice «corregir» y Almacén «empaques de compra». Verlo en rojo
- [X] T015 [P] Texto de los dos alcances en web/src/features/admin/IntegrationsPage.tsx (plan, «Pantallas y maquetas»); T014 en verde; `bun run lint` y `bun run vitest run`
- [X] T016 [P] docs/mecanica/personal-y-permisos.md: sección «Lo que agregó la spec 074» (herramientas, alcances, ensanche de tokens, protocolo de huella por MCP, cambiar renglón = quitar y agregar)
- [X] T017 `cd server && go build ./... && go test ./...` y `make test-integration` completos en verde
- [X] T018 Correr `/revision-de-codigo`; cada hallazgo que se acepte, con su test en rojo antes del arreglo
- [X] T019 Prueba con cliente MCP real contra la API local del worktree (quickstart.md): `whoami` con la fila del token «Gestión Claude» (secreto de prueba solo en la base local) lista las tres herramientas; caso de aceptación con un insumo de harina del negocio; rechazos; limpieza (anular la compra y desactivar el empaque de prueba). Lo observado, en texto en el PR

  **Observado el 2026-11-11** (cliente del SDK oficial de MCP por HTTP contra `make start` del
  worktree, base restaurada del respaldo de esa madrugada; sin cifras del negocio):
  - `whoami` con la fila del token «Gestión Claude» (secreto de prueba puesto solo en la base local):
    trae los siete alcances y lista `presentation_create`, `presentation_update` y `expense_edit`
    (52 herramientas). Cubre las nuevas sin regenerarlo.
  - Compra en gramos del insumo de harina pastel del negocio: la vista previa sugirió
    `presentation_create` con insumo y unidad llenos, pidió marca y contenido, y avisó que el renglón
    entraba sin marca ni empaque. Aplicada así, reproduce el incidente.
  - `presentation_create` (Bolsa, 375 g, Betty Crocker): la vista previa listó los empaques del insumo
    (ninguno); aplicada, creó el empaque. Repetirlo dio «ya hay un empaque que se llama así» y una
    marca de 41 letras «la marca va de 1 a 40 letras».
  - `expense_edit` (quitar el renglón en gramos y agregar el mismo con el empaque, llegada del mismo
    día): la vista previa dijo que existencias y costo no cambian y que el importe y el estado
    quedan iguales, sin decisiones. Aplicar sin `fingerprint` se rechazó antes de despachar; con una
    huella vieja, «algo cambió desde la vista previa»; con la buena, aplicó, y repetirla con la misma
    clave devolvió lo guardado. Al final la existencia y el costo vigente del insumo son los mismos
    que dejó la compra en gramos, y el renglón vivo tiene el empaque y la marca.
  - Repetido tras las correcciones de la revisión: igual, y reaplicar con clave nueva y la misma huella
    ya no duplica (la ruta responde que el renglón quitado no existe).
  - Limpieza: las dos compras de prueba anuladas por `expense_cancel` y el empaque desactivado.
- [X] T020 `make ci-local` en verde; empujar; PR a `develop` que cierra #268; esperar CI y mergear en verde
- [ ] T021 PR de release `develop` → `main`, CI verde, merge; seguir el deploy hasta `readyz` 200 en producción (solo salud)
- [ ] T022 `make cerrar-worktree dir=../egb-074-mcp-presentaciones datos=1`, borrar la rama remota, comentar y cerrar #268 con lo que entró, lo que quedó fuera (`ingredient_create`) y el ensanche de tokens

## Dependencies

- T002 → T003 → todo lo demás.
- US2 (T004–T007) antes que US1 (T008–T011): el caso de US1 usa `presentation_create`.
- T012 (US3) se escribe antes de T010, con T008 y T009; T013 después de T010.
- T014–T016 en paralelo con US1/US3.
- T017 → T018 → T019 → T020 → T021 → T022.

## Parallel Example

```text
T014 (test web) · T016 (doc) mientras corre T008–T010
```

## Implementation Strategy

MVP = Foundational + US2 + US1: es el caso del encargo. US3 reusa la misma herramienta y solo agrega
cobertura de encabezado.
