# Research

## R1. Transporte y SDK

- **Decisión**: HTTP streamable **sin sesión** (`Stateless: true`, `JSONResponse: true`) del SDK
  oficial `github.com/modelcontextprotocol/go-sdk` v1.8.0, montado en `/mcp` de la misma API.
- **Por qué**: probado el 2026-11-10 en un módulo aparte: `initialize`, `tools/list` y `tools/call`
  responden por POST sin `Mcp-Session-Id`; el esquema de entrada se infiere del struct (con
  `required` y `additionalProperties: false`), un argumento de tipo equivocado se rechaza antes de
  llegar a la herramienta, y el servidor se construye **por petición** (`getServer(*http.Request)`),
  lo que permite que `tools/list` dependa del token. Sin sesión, la VM no guarda estado entre
  llamadas y el despliegue azul/verde no corta a nadie. Caddy ya enruta todo el dominio de la API.
- **Alternativas**: servidor JSON-RPC propio (rechazado: el protocolo cambia por versiones, ver plan);
  un binario aparte `pos-mcp` por stdio que hable con la API (rechazado: duplica despliegue y
  credenciales, y Claude Code de cualquier máquina tendría que instalarlo); SSE heredado (deprecado).
- **Costo**: dependencias transitivas (`jsonschema-go`, `segmentio/encoding`, `uritemplate`,
  `x/oauth2`, `x/time`). `govulncheck` en el pre-push y en CI las cubre.

## R2. Formato y verificación del token

- **Decisión**: `egbpat_<empresa>_<id>_<secreto>` con secreto de 32 bytes `crypto/rand` en
  hex (base64url mete `_`, el separador); se guarda `sha256(secreto)`. Comparación con `subtle.ConstantTimeCompare`.
- **Por qué**: RLS exige saber la empresa antes de leer; ponerla en el token evita una vista o rol
  con bypass (como el del webhook). Si alguien cambia la empresa, la búsqueda bajo esa empresa no
  encuentra el id y es 401. bcrypt no aporta con 256 bits de entropía y costaría ~50 ms por llamada.
- **Alternativas**: JWT firmado (no se revoca sin lista); token opaco con búsqueda global (exige
  saltar RLS).

## R3. Despachar al router

- **Decisión**: la herramienta arma una petición interna (método, ruta, query, cuerpo JSON) y la
  sirve el mismo `chi.Router` con un principal interno en el contexto.
- **Por qué**: ver plan, decisión 1. Verificado en el código: `SetProductPlatformPrice` registra el
  evento de seguridad y invalida el menú en el handler; `AdminUpdateProduct` publica `menu.updated`
  en el handler; `CancelExpense` calcula `canAdjust` en el handler.

## R4. Rate limit

- **Decisión**: dos limitadores nuevos sobre el `rateLimiter` existente: `mcpIPs` (300/min por IP,
  antes de autenticar, cuenta todas) y `mcpTokens` (120/min por token, después). Redis caído:
  `blocked` falla abierto como el resto.

## R5. Bitácora

- **Decisión**: un evento por llamada en una transacción aparte, escrito después de despachar, con
  el resultado. El de la operación lo escribe su servicio en su transacción, como siempre.
- **Por qué**: si la operación falla, su transacción se revierte, pero el intento debe quedar.

## R6. Conexión desde Claude Code

- **Decisión**: el comando que muestra la pantalla:

  ```bash
  export POS_MCP_TOKEN='egbpat_…'          # en el perfil del shell, no en el repo
  claude mcp add --transport http --scope user pos-gatobobah https://api.elgatobobah.com/mcp \
    --header 'Authorization: Bearer ${POS_MCP_TOKEN}'
  ```

  Comillas simples para que se guarde la referencia `${POS_MCP_TOKEN}` y no el valor. Se verifica en
  el quickstart que Claude Code la expande al conectar; si en el alcance `user` no la expande, la
  alternativa documentada es `.mcp.json` del proyecto, que sí expande variables.
