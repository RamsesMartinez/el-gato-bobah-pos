# Contrato: correo de aviso

Destinatario `ALERT_EMAIL_TO`; remitente `MAIL_FROM`; HTML UTF-8, todo valor escapado.

**Asunto** (inmediato): `[POS <host>] Error 500: <METHOD> <route>`
**Asunto** (resumen): `[POS <host>] Error 500 repetido <N> veces: <METHOD> <route>`
(`[POS <host>] Panic: …` cuando fue panic.)

**Cuerpo**, renglones en este orden:

| Renglón | Ejemplo |
| --- | --- |
| Hora (CDMX) | 2026-11-08 14:03:12 CST |
| Petición | POST /api/v1/orders/{id}/pay |
| Usuario | #12 Ana (o «sin sesión») |
| Empresa | #2 El Gato Bobah (o «#2») |
| Id de request | 5f0c… |
| Tipo | pg:23505 · *errors.errorString · panic:runtime.boundsError |
| Mensaje | texto enmascarado |
| Repeticiones | solo en resumen: «N en los últimos 15 min» y, si aplica, «más M anteriores sin reportar» |
| Descartados | solo si > 0: «N avisos descartados por ráfaga desde el último correo» |
| Traza | ≤ 12 marcos del proyecto |

**Nunca**: cuerpo de petición o respuesta, encabezados, query, IP, contraseñas, PIN, tokens, datos de
pago. Lo verifica `TestAlertEmailCarriesNoSecrets` (httpapi) sobre asunto y cuerpo.
