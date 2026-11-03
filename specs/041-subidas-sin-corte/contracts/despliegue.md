# Contrato: script de despliegue azul/verde

`scripts/deploy-azul-verde.sh` — lo llama `make deploy-image` (misma entrada que hoy: `API_IMAGE`).

| Paso | Si falla |
| --- | --- |
| 0. Si no existe `activo.caddy`, escribirlo con `to api:8080` (tmp + mv) | — |
| 1. `check-env`, `MemAvailable ≥ 100 MB` | sale ≠0, nada cambió |
| 2. Lee el activo de `deploy/upstream/activo.caddy`; el otro color es el destino | — |
| 3. `pull` del destino; `up -d --no-deps --force-recreate api-<destino>` | detiene el destino, sale ≠0 |
| 4. `/api -readycheck` en ciclo, hasta 60 s | detiene el destino, sale ≠0, tráfico intacto |
| 5. Escribe `activo.caddy` (tmp + mv) y `caddy reload` | restaura el archivo anterior, detiene el destino, sale ≠0 |
| 6. Revisa `-readycheck` del destino otra vez | regresa el archivo y recarga; sale ≠0 |
| 7. `stop -t 30` de la vieja (y en la primera subida, quita `${COMPOSE_PROJECT_NAME:-deploy}-api-1`) | aviso, sale 0 (el tráfico ya está en la nueva) |
| 8. `docker image prune -f` | ignorado |

El Caddyfile importa `activo*.caddy` (glob): si el archivo falta, Caddy arranca igual en vez de dejar la API sin proxy. Prueba: `scripts/tests/deploy-azul-verde-test.sh`.

Salida: una línea por paso con el color y el SHA; código 0 solo si el tráfico quedó en la nueva y sana.
