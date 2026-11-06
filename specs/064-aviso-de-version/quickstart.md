# Quickstart: validar el aviso de versión nueva (064)

## Requisitos

- `make start` arriba en este worktree (web en `http://localhost:3064`, API en `:8064`).
- Contraseña y PIN del admin: `make reset-admin` si no se conocen.

## Unitarios

```bash
cd web && bun run vitest run src/app
```

Esperado: las pruebas del aviso en el riel pasan (aparece con `needRefresh`, un toque llama
`apply`, sin `needRefresh` el riel es el de hoy) y `registerPwa` ya no crea un toast de versión.

## Disposición a 1024×600 (Playwright, local)

```bash
cd web && E2E_BASE_URL=http://localhost:3064 E2E_API_URL=http://localhost:8064/api/v1 \
  E2E_SLUG=… E2E_PASSWORD=… E2E_PIN=… bunx playwright test e2e/aviso-de-version.spec.ts
```

Esperado, en Vender, Pedidos, Negocio y Caja:

1. Antes del aviso se mide el área de trabajo; se llama `window.__egbSimularVersionNueva()`.
2. El aviso se ve en el riel y su control mide ≥ 44 px de alto.
3. El área de trabajo mide lo mismo, al píxel (SC-002).
4. Ningún control visible del área de trabajo se cruza con la caja del aviso (SC-001).
5. La opción activa del riel y «Salir» siguen completas a la vista (FR-007).

## Build servido

```bash
cd web && bun run build && bun run preview
```

Esperado: `window.__egbSimularVersionNueva` es `undefined` (el gancho no viaja a producción).
