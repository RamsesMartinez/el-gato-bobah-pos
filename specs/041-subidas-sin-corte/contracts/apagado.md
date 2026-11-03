# Contrato: apagado de la API

Al recibir SIGTERM/SIGINT, en este orden:

1. `/readyz` responde 503 `{"status":"shutting_down"}`; `/healthz` sigue 200. Un `/events` nuevo responde 503 `SHUTTING_DOWN`.
2. Se detienen los ciclos de fondo (recorte y resumen diario); una pasada del resumen a medio envío termina con su propio contexto.
3. `Shutdown` con 15 s: deja de aceptar; `RegisterOnShutdown` cierra el broker → todos los `/events` terminan.
4. Lecturas de menú en segundo plano y ciclos de fondo: hasta los 20 s desde la señal (al menos 3 s propios).
5. Lo que no terminó: las lecturas vivas se marcan `fallida` en paralelo, con 5 s de tope, y solo si siguen `en_curso`.
6. Si todo terminó: `slog.Info("shutdown complete", "duration_ms", …)`. Si algo venció: `slog.Error("shutdown", …)`.

Peor caso: 25 s, debajo del `stop -t 30` del despliegue: la API siempre decide antes que Docker la mate.
