# Implementation Plan: Sucursales en la base de datos

**Branch**: `025-sucursales` | **Date**: 2026-10-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/025-sucursales/spec.md`

## Summary

Nueva tabla `branches` (una matriz por empresa, número consecutivo por empresa, código corto
inmutable, datos del lugar). Cajas, tiendas de plataforma, pedidos e inventario guardan su
sucursal con llave foránea compuesta `(company_id, branch_id)`. Una función SQL,
`current_branch_id()`, resuelve «la sucursal» mientras no haya selector: devuelve la única activa
de la empresa de la sesión y **truena** si hay cero o más de una. Sirve de valor por omisión en las
columnas nuevas, así que casi ninguna consulta de escritura cambia. Una sola migración, la 0076, con
relleno de datos. No cambia ninguna pantalla ni ninguna API.

## Technical Context

**Language/Version**: Go 1.27 (server), SQL de Postgres (migración goose embebida)

**Primary Dependencies**: pgx + sqlc, goose. Sin dependencias nuevas.

**Storage**: PostgreSQL con RLS por empresa (`gatobobah_app`)

**Testing**: `go test ./...`; integración contra Postgres real con `appRoleStore` e `inTheThreeCases`

**Target Platform**: VM Linux (`pos-vps`), contenedor Docker

**Project Type**: web-service (backend). Sin cambios en `web/`

**Performance Goals**: el relleno de `orders.branch_id` sobre producción termina en segundos
(estimado: la base pesa 19 MB en total)

**Constraints**: cero cambios visibles para quien opera; las cifras de dinero idénticas antes y
después sobre el mismo respaldo (SC-005)

**Scale/Scope**: 1 tabla nueva, 6 tablas con columna nueva, 1 trigger reescrito, 4 llamadores de
`GetOpenPrimarySession`, 2 consultas de inventario, 5 tests con inserts como owner, 1 script de
corte

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principio | Cómo se cumple |
|---|---|
| I. Capas | Esquema y resolución de sucursal en la migración (SQL). `GetOpenPrimarySession` cambia en `queries/cash.sql` + `make sqlc`. Servicios en `app/` sin lógica nueva salvo pasar la sucursal de la tienda al aceptar un pedido de plataforma. Nada en `httpapi` ni en `domain` |
| II. Errores | El error de `current_branch_id()` (cero o varias sucursales) se mapea a un sentinel de `domain` (`ErrBranchAmbiguous`) en `httpapi.Error`, no a un 500 genérico |
| III. Dinero | No toca cálculos. SC-005 lo vigila un test sobre el corte y las ventas antes y después de migrar |
| IV. Test-first | Test de la migración sobre respaldo real con dos empresas; aislamiento de `branches` en los tres casos; FK compuesta que rechaza sucursal ajena; dos cajas principales abiertas en dos sucursales; existencias por sucursal. Todos antes del código |
| IV. Local = producción | Las pruebas de sucursal van bajo `appRoleStore`; nada se prueba como owner salvo la migración misma |
| V. Seguridad | La FK compuesta es la barrera contra ligar algo a una sucursal ajena (las FK saltan RLS). Su test intenta el ataque con un `branch_id` de otra empresa |
| VI. YAGNI | Sin pantallas, sin JWT con sucursal, sin tabla usuario↔sucursal, sin precio por sucursal. Todo eso se agrega después al mismo costo |
| VII. Idioma | Identificadores nuevos en inglés (`branches`, `branch_id`, `current_branch_id`, `is_headquarters`) |
| VIII. Puertas | Cruza «Más de una sucursal». Respuesta a «¿se puede agregar después al mismo costo?»: **no** para pedidos, cajas, tiendas e inventario, porque con dos sucursales lo no registrado queda ambiguo para siempre. Por eso van ya. **Sí** para catálogo, empleados y precios, que quedan fuera |

Sin violaciones. Re-check tras el diseño: igual.

## Decisiones de diseño (Phase 0)

Detalle y fuentes en [research.md](research.md). Resumen:

1. **`current_branch_id()` como única fuente de «la sucursal»**. Función SQL `stable`: primero
   lee `app.branch_id` de la sesión, si existe (hoy nunca; es la puerta para el selector futuro). Si
   no, devuelve la única sucursal activa de `app.company_id`, y si hay cero o más de una, lanza un
   error con código propio. **Los dos ajustes se leen con `nullif(..., '')`** como en 0074: una
   conexión reciclada trae cadena vacía y sin eso truena con 22P02 en vez de `branch_ambiguous`.
   La función tiene su propio test en los tres casos (revisión de diseño, 2026-10-01). *Alternativas descartadas*: un helper en Go repetido en cada servicio
   (cuatro lugares que olvidar), y escoger la matriz cuando hay varias (adivinar en silencio, lo que
   prohíbe el principio V).
2. **Valor por omisión en las columnas** (`cash_registers`, `platform_connections`,
   `stock_movements`): `default current_branch_id()`. Así las consultas de escritura actuales no
   cambian y FR-006 (la tienda se asigna sola) sale gratis. *Descartado*: agregar el parámetro a
   cada `insert`, que son decenas de cambios sin ganancia mientras haya una sucursal.
3. **`orders.branch_id` sale del turno**, en el mismo `insert`: subconsulta a
   `register_sessions → cash_registers.branch_id`. Todo pedido **nuevo** tiene turno, incluidos los
   de plataforma. **El relleno de los históricos asigna la matriz directo**, sin pasar por el turno:
   hay pedidos viejos con `register_session_id` nulo (0061:63) y la subconsulta los dejaría sin
   sucursal. Al aceptar un pedido de plataforma, el turno se busca **en la sucursal de la tienda**,
   no en «la» de la empresa.
4. **Existencias por sucursal**: `stock_levels` cambia sus únicos a `(branch_id, ingredient_id)` y
   `(branch_id, product_id)`, y el trigger de 0009 se reescribe con **el mismo destino en su
   `on conflict`** (si no, cada venta falla con «no unique constraint matching»). `ListIngredients`
   y `ListStockLevels` filtran por `current_branch_id()`: sin eso, con dos sucursales, cada insumo
   saldría repetido sin decir de qué sucursal es.
5. **Alta de empresa**: la matriz se crea **en la misma consulta** que la empresa
   (`CreateCompany` con un CTE), no con un trigger. Un trigger `after insert on companies` se
   dispararía también al restaurar un respaldo (`pg_restore` carga con `COPY`) y crearía una matriz
   que choca con la real: `make db-restaurar` abortaría. El arnés (`makeCompany`) y
   `docs/corte-produccion/01_nueva_empresa.sql` se actualizan a mano.
9. **Inserts como owner con otra empresa**: varios tests insertan con el pool del owner y un
   `company_id` explícito distinto del de la sesión (`fecha_y_folio_separados_test.go:367`,
   `migracion_conteo_de_efectivo_test.go:50`, `menus_plataforma_test.go:195,264`,
   `pedidos_de_plataforma_test.go:117`). El valor por omisión les daría la sucursal de otra empresa
   y la FK compuesta los rechazaría. Pasan `branch_id` explícito. `01_nueva_empresa.sql` agrega
   `branches` a su lista de tablas para que el corte no copie una sucursal ajena.
6. **Número de sucursal**: `max(branch_number)+1` por empresa con `for update` sobre la empresa
   dentro del alta. No se usa secuencia global.
7. **Código de la matriz**: el slug sin guiones, en mayúsculas, hasta 10 caracteres. Inmutable por
   trigger `before update`.
8. **Datos del ticket**: dirección y teléfono de `business_settings` se copian a la matriz; el
   ticket sigue leyendo `business_settings` (FR-013).

## Project Structure

### Documentation (this feature)

```text
specs/025-sucursales/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── tasks.md           # /speckit-tasks
```

Sin `contracts/`: no cambia ninguna API ni ninguna pantalla.

### Source Code (repository root)

```text
server/
├── migrations/
│   ├── 0076_branches.sql                 # tabla, función, triggers, columnas, relleno, FKs
│   └── version.go                        # versión esperada → 76
├── queries/
│   ├── cash.sql                          # GetOpenPrimarySession filtra por sucursal
│   └── orders.sql                        # insert de pedido toma la sucursal del turno
├── internal/
│   ├── domain/branch.go (+ _test)        # ErrBranchAmbiguous y el código derivado del slug
│   ├── httpapi/respond.go                # mapeo del sentinel
│   ├── app/orders.go, pedidos_de_plataforma.go, backoffice.go   # llamadores
│   └── integration/
│       ├── branches_test.go              # FR-001..011 bajo appRoleStore, tres casos
│       └── migracion_0076_test.go        # relleno sobre respaldo con dos empresas, SC-001 y SC-005
docs/emparejamiento-de-plataformas.md     # renglón: la sucursal ya existe
.specify/memory/constitution.md           # puerta «Más de una sucursal» → cruzada
```

## Riesgos

- **El relleno reescribe `orders`.** La base completa pesa 19 MB (medido el 2026-09-29), así que
  es cuestión de segundos. Se corre con el respaldo de las 00:45 ya hecho.
- **Bajada (Down)**: se niega con `raise exception` si alguna empresa ya tiene dos sucursales.
- **Prueba en el ambiente de pruebas con un respaldo de producción** antes de dar la entrega por
  buena (instrucción del dueño, 2026-10-02): restaurar el respaldo más reciente en `pos-vps-dev`,
  desplegar la rama y recorrer el POS.
- **Va con la integración de Uber**: no llega a producción antes que ella.
