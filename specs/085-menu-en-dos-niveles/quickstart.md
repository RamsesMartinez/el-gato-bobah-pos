# Quickstart: validar el menú en dos niveles

Prerrequisitos: `make start` del worktree arriba (API :8085, web :3085) con base restaurada o admin creado.

1. `cd web && bun run vitest run src/app` — `nav.test.ts` (lo que ve cada rol), `MenuRail.test.tsx`, `AppShell.test.tsx`.
2. `cd server && go test ./internal/domain/ -run Uso` — Interfaz e Integraciones se aceptan.
3. e2e a 1280×854 y 2K: `E2E_BASE_URL=http://localhost:3085 E2E_API_URL=http://localhost:8085/api/v1 E2E_SLUG=… E2E_PASSWORD=… E2E_PIN=… bun run e2e -- menu-por-rol cabe-en-la-tableta aviso-de-version`.
   - Esperado: el riel no desplaza; admin ve 8 botones, cajero 6 (asistencia encendida); cada ruta abre por URL directa.
4. Medición: abrir Interfaz e Integraciones; en la consola de plataforma aparecen con conteo.
