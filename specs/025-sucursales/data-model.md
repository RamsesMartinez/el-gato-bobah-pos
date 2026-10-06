# Data Model: Sucursales

## branches (nueva)

| Columna | Tipo | Regla |
|---|---|---|
| id | bigint identity | PK |
| company_id | bigint not null | default de la sesión, como el resto; FK a `companies` on delete cascade |
| branch_number | int not null | consecutivo por empresa; único `(company_id, branch_number)` |
| code | citext not null | 1–10 caracteres `[A-Z0-9]`; único `(company_id, code)`; inmutable |
| name | text not null | 1–60 caracteres |
| is_headquarters | boolean not null default false | único parcial `(company_id) where is_headquarters` |
| is_active | boolean not null default true | no se desactiva la última activa |
| address, phone | text | copiados de `business_settings` para la matriz |
| postal_code | text | 5 dígitos si viene; lugar de expedición del CFDI |
| timezone | text | null = el de la empresa (`business_settings.timezone`) |
| created_at | timestamptz not null default now() | |

- Índice único `branches_tenant_key (company_id, id)` para las FK compuestas.
- RLS con `nullif`, como 0074; `select, insert, update` para `gatobobah_app`. Sin `delete`: una
  sucursal con historia no se borra.
- La matriz no se puede desactivar mientras sea la única activa (trigger `before update`).

## Columnas nuevas

| Tabla | Columna | Origen del valor | Restricción |
|---|---|---|---|
| cash_registers | branch_id not null | `default current_branch_id()` | FK compuesta; `cash_registers_one_primary` pasa a `(company_id, branch_id) where is_primary` |
| platform_connections | branch_id not null | `default current_branch_id()` | FK compuesta |
| orders | branch_id not null | sucursal de la caja del turno, en el `insert` | FK compuesta |
| stock_movements | branch_id not null | `default current_branch_id()` | FK compuesta |
| stock_levels | branch_id not null | el del movimiento (trigger) | únicos `(branch_id, ingredient_id)` y `(branch_id, product_id)` |
| expenses | branch_id null | ninguno: null = de toda la empresa | FK compuesta |

Heredan la sucursal por su padre y no llevan columna: `register_sessions`, conteos, movimientos y
transferencias de efectivo, renglones y pagos de pedido, devoluciones, liquidaciones de plataforma,
avisos de pedido entrante.

## Funciones y triggers

- `current_branch_id()`: lee los ajustes con `nullif(..., '')`; `app.branch_id` si está puesto; si no, la única sucursal activa de
  `app.company_id`; si hay cero o más de una, lanza `P0001` con mensaje `branch_ambiguous`.
- **Sin trigger en `companies`**: la matriz la crea `CreateCompany` en la misma consulta (un
  trigger duplicaría la matriz al restaurar un respaldo).
- `branches` `before insert`: asigna `branch_number` si viene nulo.
- `branches` `before update`: rechaza cambio de `code` o de `company_id`, y desactivar la última
  activa.
- Trigger de existencias (0009): el upsert usa la sucursal del movimiento.

## Relleno (dentro de 0076)

1. Una matriz por empresa existente.
2. Copiar dirección y teléfono de `business_settings` a su matriz.
3. `branch_id` = matriz **directa** en las seis tablas (no por el turno: hay pedidos viejos sin turno), luego `not null` donde aplica, luego las FK.
4. Comprobación final dentro de la migración: ninguna fila sin sucursal y una matriz por empresa;
   si falla, la migración aborta.
