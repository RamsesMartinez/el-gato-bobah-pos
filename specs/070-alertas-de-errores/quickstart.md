# Quickstart: probar el aviso

## Local (mailpit)

1. `deploy/.env` del worktree: `ALERT_EMAIL_TO=ramses.mtz96@gmail.com` (mailpit lo atrapa; no sale a
   internet). `make start`.
2. Forzar un 500: con la API arriba, `docker stop egb70-postgres-1` y pedir cualquier endpoint
   autenticado (o `go test ./internal/httpapi -run ServerError` para el camino del panic).
3. mailpit `http://localhost:8270`: un correo con todos los renglones del
   [contrato](contracts/alert-email.md). Repetir 10 veces: no llega otro hasta el cierre de ventana.
4. `docker stop egb70-redis-1` y repetir: sigue avisando (memoria), la API responde igual.

## Ambiente de pruebas (autorizado por el dueño)

1. En `pos-vps-dev`, `deploy/.env`: `SMTP_*`/`MAIL_FROM` de Zoho Mail (los de producción) y
   `ALERT_EMAIL_TO`. Desplegar develop.
2. Forzar un 500 real sin escribir datos (p. ej. detener postgres un momento en la VM de pruebas y
   pedir `/api/v1/me`), y confirmar que el correo llega al buzón.

## Producción

Agregar `ALERT_EMAIL_TO` al `deploy/.env` de `pos-vps` antes de subir. Después solo salud (`readyz`,
logs sin 5xx, y en el log de arranque que los avisos están encendidos).
