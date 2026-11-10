# Contrato: gestión de tokens

Todas con `RequirePermission(integrations.manage)`, bajo `/api/v1`, con sesión normal de la pantalla
(JWT). **Ninguna es una herramienta MCP**: un token no puede crear ni revocar tokens.

## `GET /integrations/tokens`

Lista los tokens de la empresa, vigentes y no vigentes (los revocados o vencidos hace más de 90 días
no se listan).

```json
{"items": [{"id": 3, "name": "Claude de la laptop", "prefix": "egbpat_2_3_Xa9q", "ownerId": 7,
  "ownerName": "Ramsés", "scopes": ["read", "write:expenses"], "createdAt": "…",
  "expiresAt": "…", "lastUsedAt": "…|null", "revokedAt": "…|null", "status": "active|revoked|expired"}]}
```

## `POST /integrations/tokens`

Cuerpo: `{"name": "…", "scopes": ["write:expenses"], "expiresInDays": 90, "password": "…"}`. La
contraseña actual de quien llama es obligatoria (un PIN visto en la tableta no basta); los intentos
fallidos cuentan en el mismo bloqueo que el login. Un `userId` en el cuerpo se ignora. `read` se agrega solo.
`expiresInDays` ∈ {30, 90, 180}; nombre de 1 a 60 caracteres; alcance desconocido → 400. El dueño es
quien llama. Respuesta 201: el registro de la lista más `"token": "egbpat_…"` (única vez) y
`"mcpUrl"`.

## `POST /integrations/tokens/{id}/revoke`

204. Idempotente (revocar uno revocado es 204). Token de otra empresa → 404.
