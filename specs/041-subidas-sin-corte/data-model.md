# Data Model: Subidas a producción sin corte

Sin migración ni tabla nueva.

## Lado activo de la API (archivo, no base)

`deploy/upstream/activo.caddy` en la VM, ignorado por git.

| Contenido | Significado |
| --- | --- |
| `to api-blue:8080` | activo azul |
| `to api-green:8080` | activo verde |
| (no existe) | instalación anterior: activo `api:8080` (contenedor `deploy-api-1`) |

Transiciones: lo escribe solo el script, solo después de que la otra copia respondió lista.

## Envío del resumen diario (existente)

`daily_summary_sends (company_id, business_date)` único; `status` `pending → sent | failed`. El `insert … on conflict` de `ClaimSummarySend` es la exclusión entre copias. Sin cambios.

## Lectura de menú (existente)

`en_curso → terminada | fallida`. Nuevo: al apagar, una lectura que no terminó en el plazo pasa a `fallida` en vez de quedarse `en_curso`.
