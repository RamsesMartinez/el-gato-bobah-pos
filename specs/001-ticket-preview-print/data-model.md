# Phase 1 — Data Model: Visualizador e impresión del ticket

**Feature**: 001-ticket-preview-print · **Date**: 2026-08-27

## Entidades

### Ajustes del negocio (`business_settings`, extendida)

Una fila por empresa (PK = `company_id` desde
[0023](../../server/migrations/0023_tenant_columns.sql)), aislada por la policy `tenant_isolation`
de [0024](../../server/migrations/0024_tenant_rls.sql). Las columnas nuevas heredan ese aislamiento
sin trabajo extra.

| Columna | Tipo | Nulo | Regla |
| --- | --- | --- | --- |
| `delivery_fee` | `numeric(10,2)` | no | ya existía |
| `business_name` | `text` | no | 1–60 caracteres tras recortar espacios; se siembra desde `companies.name` |
| `address` | `text` | sí | ≤ 120 caracteres; vacío = no se imprime el renglón |
| `phone` | `text` | sí | ≤ 30 caracteres; vacío = no se imprime el renglón |
| `footer_note` | `text` | sí | ≤ **400** caracteres, **varias líneas**; vacío = el ticket cierra con "¡Gracias!" |
| `header_note` | `text` | sí | ≤ **400** caracteres, varias líneas; vacío = no se imprime. Va arriba del detalle |
| `auto_print_on_close` | `boolean` | no | default `false`; enciende la impresión sin toque al cerrar venta |
| `logo_bytes` | `bytea` | sí | ≤ 256 KB (ver D5 en [research.md](./research.md)); nulo = logo por default |
| `logo_mime` | `text` | sí | `image/png` o `image/jpeg`; nulo si y solo si `logo_bytes` es nulo |
| `logo_updated_at` | `timestamptz` | sí | sirve de versión para el `ETag` del binario |

**Invariante**: `logo_bytes` y `logo_mime` son nulos juntos o no nulos juntos. Se fuerza con un
check, no con disciplina del código — una fila con bytes sin mime hace que el navegador adivine el
tipo, que es justo lo que D5 cierra.

### Migraciones

Son tres y en ese orden. **Ninguna se edita una vez aplicada**: cambiar una migración que ya corrió
deja esquemas distintos entre máquinas según quién migró antes.

| Migración | Qué hace |
| --- | --- |
| [`0033_ticket_business_info.sql`](../../server/migrations/0033_ticket_business_info.sql) | Las siete columnas de identidad y logo, con sus `check`. Siembra `business_name` desde `companies.name` y solo entonces la pone `not null`: agregarla ya `not null` sin default reventaría con las filas existentes |
| [`0034_ticket_notes_autoprint.sql`](../../server/migrations/0034_ticket_notes_autoprint.sql) | `header_note`, `auto_print_on_close`, y aprieta la lista de mimes a PNG/JPEG para que coincida con lo que la app valida |
| [`0035_ticket_notes_block.sql`](../../server/migrations/0035_ticket_notes_block.sql) | Sube los textos de 120 a 400 caracteres y **siembra el pie por default** (aviso de "sin valor fiscal" y cómo pedir factura) donde no hay nada configurado |

El `Down` de cada una es gemelo. El de la 0035 tiene un detalle que no es opcional: antes de volver
el check a 120 hay que vaciar los textos que ya no cabrían, o el `alter` falla y la migración queda
a medias.

Los separadores del pie sembrado miden **32 caracteres** porque ése es el ancho real del papel de
80mm con la fuente del ticket: uno más largo se parte en dos renglones y rompe el recuadro.

No hay tabla nueva, así que **no hay que registrar nada en la policy de RLS**. Si alguien cambia de
opinión y saca el logo a su propia tabla, ese registro deja de ser opcional.

### Logo del ticket

No es una entidad propia: son las tres columnas `logo_*` de arriba. Uno por empresa. Ausente por
default.

### Ticket de venta

**No se persiste.** Se deriva del pedido (`orders` + `order_lines` + `order_line_modifiers`, ya
existentes) y de los ajustes vigentes al momento de imprimir. Consecuencia aceptada y documentada en
la spec: si el negocio cambia su dirección, una reimpresión vieja sale con la dirección nueva. Para
un ticket de venta —que no es comprobante fiscal— eso es correcto; guardar un snapshot por ticket
sería una tabla y una política de retención para un problema que nadie tiene.

## Validaciones

Todas viven en `domain`, sin I/O, y se prueban table-driven ahí mismo (principio IV).

| Regla | Dónde | Error |
| --- | --- | --- |
| Tamaño del logo ≤ 256 KB | `domain.ValidateLogo` | `ErrLogoTooLarge` |
| Tipo real ∈ {PNG, JPEG} | `domain.ValidateLogo` (por contenido, no por header) | `ErrLogoType` |
| Lado ≤ 1024 px | `domain.ValidateLogo` (`image.DecodeConfig`) | `ErrLogoDimensions` |
| `Name` no vacío, ≤ 60 | `domain.BusinessInfo.Validate` | `ErrValidation` |
| `Address` ≤ 120, `Phone` ≤ 30 | `domain.BusinessInfo.Validate` | `ErrValidation` |
| `HeaderNote`/`FooterNote` ≤ 400 | `domain.BusinessInfo.Validate` | `ErrValidation` |

Los largos se miden en **caracteres y no en bytes** (`utf8.RuneCountInString`): rechazar un nombre
por llevar acentos no tiene nada que ver con cómo se ve en el papel.

El mapeo a HTTP se hace **solo** en `httpapi.Error` con `errors.Is`, como manda el principio II. Los
tres sentinels nuevos caen en 400/422; ninguno debe poder producir un 500.

## Queries sqlc (`server/queries/settings.sql`)

Sin `WHERE`: RLS acota a la fila de la empresa actual, igual que las dos que ya existen.

| Nombre | Tipo | Para qué |
| --- | --- | --- |
| `GetBusinessSettings` | `:one` | Se amplía con los cuatro campos de identidad + `logo_mime` + `logo_updated_at`. **No** selecciona `logo_bytes` |
| `UpdateBusinessInfo` | `:one` | Guarda nombre, dirección, teléfono y leyenda |
| `GetTicketLogo` | `:one` | `logo_bytes`, `logo_mime`, `logo_updated_at` — la única que trae el binario |
| `SetTicketLogo` | `:one` | Guarda bytes + mime + `now()` |
| `ClearTicketLogo` | `:exec` | Deja las tres columnas en nulo |

Que `GetBusinessSettings` no traiga el binario es deliberado: esa query corre en el camino del cobro
([CheckoutSheet.tsx](../../web/src/features/pos/CheckoutSheet.tsx) la usa para el costo de envío) y
no tiene por qué mover 256 KB por cada pedido.

## Tipos en el front

`BusinessSettings` (en `web/src/types/pos.ts`) gana `businessName`, `address`, `phone`,
`footerNote`, `hasLogo` y `logoUpdatedAt`. `hasLogo` y `logoUpdatedAt` son lo que le dice al front
si tiene que pedir el binario y si su copia en caché sigue vigente.

`buildReceiptHtml` pasa a recibir tres argumentos: el pedido, la información del negocio (con el
logo ya como data URI) y las opciones (`{ reprint: boolean }`). Sigue siendo pura y sin I/O, que es
lo que la hace reutilizable cuando la fase 2 emita los mismos datos como ESC/POS.

## Regla de la marca de reimpresión

`reprint` lo decide **quién abre la vista previa**, no la base:

- Abierta desde el modal de confirmación del POS → `reprint: false`.
- Abierta desde el tablero de órdenes → `reprint: true`.

No se guarda un contador de impresiones. Consecuencia conocida: imprimir dos veces seguidas desde la
confirmación produce dos tickets sin marca. Se acepta — el operador está viendo el pedido recién
cerrado, y contar impresiones exige una tabla y una escritura en el camino más caliente del sistema
para prevenir algo que la marca del tablero ya cubre en el caso que importa.
