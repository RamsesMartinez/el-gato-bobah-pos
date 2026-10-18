# Research: Subidas a producción sin corte

## R1. Por qué el apagado tarda 10 s y deja ERROR

- **Hallazgo**: `http.Server.Shutdown` espera a que las conexiones activas queden ociosas y **no cancela** el contexto de las peticiones. El handler `/events` solo termina con `r.Context().Done()`, así que cada stream SSE abierto espera los 10 s del plazo, y `Shutdown` devuelve `context deadline exceeded` → `slog.Error("shutdown")`.
- **Decision**: `Broker.Close()` cierra todos los canales de suscriptores; `Events` termina cuando su canal se cierra. Se cablea con `srv.RegisterOnShutdown(broker.Close)`, que corre al empezar `Shutdown`. Un `/events` que llega después responde 503 `SHUTTING_DOWN`, no un 200 vacío que haría reconectar en ciclo contra la copia que se va.
- **Alternatives**: `BaseContext` cancelable (cancela TODAS las peticiones, incluidos cobros en curso: no); bajar el plazo (esconde el problema).

## R2. Lecturas de menú en segundo plano

- **Hallazgo**: `MenusDePlataformaService` lanza `go s.correrLectura(...)` fuera de la petición. Al apagar, el proceso muere con la fila `en_curso`; `CerrarLecturasColgadas` solo la cierra si tiene más de 5 min y corre al arrancar o cada 24 h.
- **Decision**: un `sync.WaitGroup` en el servicio; el apagado las espera con su propio presupuesto (hasta los 20 s); si vence, las marca fallidas en paralelo con 5 s de tope y solo si siguen `en_curso` (`FailMenuRead` filtra por estado: una que terminó bien en medio no queda «fallida»). Un plazo único dejaba que una petición lenta se comiera el tiempo de las lecturas.
- **Traslape**: `CerrarLecturasColgadas` de la copia nueva no toca lecturas vivas de la vieja (umbral de 5 min > timeout de una lectura). Seguro.

## R3. Resumen diario con dos copias

- **Hallazgo**: `ClaimSummarySend` (`server/queries/daily_summary.sql`) ya es el candado: el insert con `on conflict … where` devuelve fila solo a quien gana; la otra recibe `ErrNoRows` y no manda.
- **Decision**: sin migración. Test de integración: dos `SendPending` concurrentes para la misma empresa y día, con un `send` que cuenta → exactamente 1.
- **Riesgo residual, que resultó real**: una copia que tomó la fila y se apaga antes de marcarla deja `pending`, y a los 10 min se re-toma y el correo sale otra vez. El ciclo usaba el contexto que el apagado cancela. Arreglo: la pasada corre con `context.WithoutCancel` y `main` espera los ciclos de fondo (`TestShutdownDuringASummaryPassLetsItFinish`, visto en rojo con la fila en `pending`).

## R4. Recorte de uso con dos copias

- `DeleteOldUsageDaily` y `DeleteOldTouchesDaily` son `delete … where fecha < corte`: idempotentes. Correr dos veces = mismo resultado. Sin cambio.

## R5. Avisos en vivo con dos copias

- **Hallazgo**: el `Broker` es en memoria por proceso. Durante el traslape un evento publicado en la vieja no llega a quien ya está en la nueva. Los hooks del web no recargan al reconectar.
- **Decision**: al `onopen` posterior a un `onerror`, invalidar las mismas consultas que invalida cada evento. Redis pub/sub queda fuera (YAGNI; el traslape dura segundos).

## R6. Cómo cambia Caddy de upstream sin cortar

- **Decision**: el Caddyfile hace `import /etc/caddy/upstream/activo*.caddy` (patrón: si falta el archivo Caddy arranca igual) dentro de `reverse_proxy`, el directorio `deploy/upstream/` se monta de solo lectura, y el script escribe el archivo (tmp + `mv`) y corre `docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile`. `caddy reload` aplica la config nueva sin soltar conexiones; las ya proxiadas siguen hasta que terminan. Excepción medida: la primera subida recrea Caddy (ver plan, *Primera subida*).
- **Alternatives**: dos upstreams con health checks activos y `lb_policy first` (el cambio depende del orden y de un intervalo, no de una decisión del despliegue; y la vieja seguiría recibiendo hasta fallar su check); variable de entorno en el Caddyfile (Caddy la lee al arrancar el proceso, no en el reload).

## R7. Esperar «lista» con imagen distroless

- **Decision**: flag `-readycheck` en el binario, gemelo de `-healthcheck`, que pide `/readyz` local; el script lo corre con `docker compose exec api-<color> /api -readycheck` en ciclo hasta 60 s.
- Además, al empezar el apagado `/readyz` responde 503: si algo vuelve a mandar tráfico a una copia que se va, se ve.

## R8. Medición del corte

- **Decision**: `scripts/medir-corte.sh URL DURACION` pide `/healthz` cada 200 ms con `curl --max-time 2`, cuenta totales, fallidas (≠200 o timeout) y la latencia máxima, y sale con código ≠0 si hubo alguna fallida. Se corre desde la máquina de quien programa contra `api-dev` mientras CI despliega.

## Resultado (2026-10-18)

- **R3 confirmado**: `TestTwoAPICopiesSendTheDailySummaryOnce` (8 pasadas simultáneas desde dos pools) manda 1 correo. Visto en rojo quitando la condición del `on conflict`: 8 correos.
- **R2 confirmado**: `TestStartupMaintenanceLeavesTheOtherCopysReadAlone` — el arranque de la copia nueva no toca una lectura viva de la vieja.
- **R4**: sin cambio; los dos `delete` son idempotentes.
- **Temporizadores** (`NewTicker` en `server/`): solo los tres conocidos (resumen, recorte, ping SSE por conexión).
- **Goroutine que el apagado no espera**: el correo de recuperar contraseña (`app/reset.go`, `go func()` fuera de la petición). Si la copia vieja se apaga en el milisegundo en que lo manda, el correo no sale y la persona lo vuelve a pedir; no hay dato que se pierda. Se deja así a propósito (YAGNI); si algún día ese envío importa, entra al mismo `WaitGroup` del apagado.
