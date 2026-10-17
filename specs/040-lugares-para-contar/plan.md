# Implementation Plan: Lugares para contar

**Branch**: `040-lugares-para-contar` | **Date**: 2026-10-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/040-lugares-para-contar/spec.md`

## Summary

El lugar de un artículo deja de ser texto libre (`ingredients.count_place`, de la 038) y pasa a ser
una liga a `count_places`, una tabla de lugares por empresa con nombre único normalizado, orden y
archivo. La migración 0095 pasa los textos a la tabla, juntando los que solo difieren en mayúsculas,
acentos o espacios, y quita la columna de texto. El conteo ordena por la posición del lugar. En
pantalla, el campo de texto del artículo se vuelve un `Picker` con «Sin lugar» y alta escribiendo el nombre, y
Negocio › Inventario gana una sección de lugares para arrastrar, renombrar y archivar.

## Technical Context

**Language/Version**: Go 1.27.2 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query (sin dependencias nuevas)

**Storage**: PostgreSQL 16 con RLS por empresa; migración nueva `0095_count_places.sql`

**Testing**: dominio table-driven (normalización, reordenar); integración bajo `appRoleStore` con
`inTheThreeCases`; prueba de la migración sobre el respaldo restaurado con dos empresas; vitest de
las dos pantallas; Playwright a 1024×600 contra el ambiente de pruebas

**Target Platform**: tabletas de 7 a 10 pulgadas (1024×600)

**Project Type**: web (server + web)

**Performance Goals**: el conteo de ~600 insumos sigue en una sola consulta

**Constraints**: sin texto libre para el lugar; sin selector nativo; controles ≥ 44 px

**Scale/Scope**: decenas de lugares por empresa

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | Normalización y reglas de reordenar y archivar en `domain/count_places.go`; servicio en `app/count_places.go` con `store.WithTx`; handlers finos; SQL en `queries/count_places.sql` |
| II Errores | `ErrValidation` (nombre vacío o largo), `ErrConflict` con el id existente (nombre repetido), `ErrNotFound` (lugar de otra empresa) |
| III Dinero | No toca dinero |
| IV Tests | Dominio primero; integración en los tres casos; `TestEveryCompanyTableIsIsolated` recoge `count_places`; migración probada con dos empresas, datos repetidos con otra mayúscula o acento y artículos sin lugar |
| V Seguridad | `RequirePermission`: `inventory.count` para asignar y crear desde el artículo (lo que pide hoy el lugar), `inventory.configure` para renombrar, reordenar y archivar; listar acepta cualquiera de los dos, para que un rol propio con solo «configurar» vea su lista. Liga compuesta `(count_place_id, company_id)`: un lugar de otra empresa no se puede asignar |
| VI YAGNI | Sin existencias por lugar, sin traspasos; el arrastre con eventos de puntero propios, sin librería nueva |
| VII Idioma | `count_places`, `count_place_id` y `position` en inglés; texto de pantalla en español |
| VIII Puertas | Ver abajo |
| Roles | Solo permisos, ningún `RequireRole` nuevo |
| Tableta | `Picker` en vez de texto libre; agarradera de 52 px con espera de 500 ms |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Más de un almacén por sucursal** (#65): no se cierra. El lugar es de la empresa y solo ordena el
  recorrido; no guarda existencias. Un almacén será otra entidad, y nada de aquí lo asume: ni la
  pantalla ni las columnas llaman «almacén» a un lugar. Si dos sucursales necesitan recorridos
  distintos, se abre con `count_places.branch_id` nulable (nulo = todas) y el índice único pasa a
  `(company_id, coalesce(branch_id, 0), normalizado)`: un índice, sin tocar filas. No se construye hoy.
- **Snapshot**: un conteo cerrado no guarda el lugar. Hoy ninguna pantalla de conteos cerrados lo
  muestra; si alguna lo muestra después, renombrar reescribiría el pasado y tendrá que copiarlo.
- **Quitar la columna de texto**: la Down la recrea como en la 0092 y la llena con el nombre del
  lugar. **Pierde** el orden, los lugares sin artículos y los archivados, y la forma original de los
  textos que la subida juntó («refri »). Es aceptable: es el mismo dato con menos detalle.

## Pantallas y maquetas

| Pantalla | Dónde vive | Cómo se reparte el alto de 1024×600 |
|---|---|---|
| **Lugar del artículo** | Almacén › Artículos, en la hoja del artículo donde hoy está el campo de texto: un renglón «Lugar: Refri ›» (sin lugar: «Lugar: Sin lugar ›») que abre el `Picker` con `clearable` y `clearLabel="Sin lugar"`, los lugares en su orden y su alta propia (`onCreate`): con el buscador vacío dice «¿No está en la lista? Escribe su nombre y podrás crearlo aquí»; al escribir aparece «Crear «x»». El buscador **no** toma el foco al abrir, para que el teclado del sistema no tape la hoja en 600 px. Si el nombre ya existe con otra mayúscula, elige el existente y lo dice: «Ya tenías «Refri»; quedó elegido». | Un renglón de 52 px; la hoja es la del `Picker` |
| **Lugares** | Negocio › Inventario, sección nueva junto a «Motivos y tope»: título «Lugares» con «Agregar lugar» (44 px) a la derecha en la misma línea, una línea de 14 px «Mantén presionada ≡ para mover un lugar», y la lista en orden de recorrido; cada renglón en una sola línea: agarradera ≡ de 52 px, «1  Refri · 12 artículos» y Editar a la derecha. **Arrastre**: dnd-kit (ya está en el proyecto) con `PointerSensor` `delay: 500, tolerance: 8` como `CategoryRail` (no `distance` como `GroupOptions`); `touch-action: none`, `user-select: none` y `-webkit-touch-callout: none` **solo en la agarradera**, `pan-y` en el resto del renglón para que la lista se desplace; `autoScroll` de dnd-kit activo sobre el contenedor que desplaza (la página de Negocio), para llevar la fila 10 a la 1; `KeyboardSensor` con `sortableKeyboardCoordinates` y `aria-label="Mover {nombre}"`. Sin otra forma de reordenar (decisión del dueño, 2026-10-16: solo arrastrar). Editar abre una hoja con el nombre, Guardar, y «Archivar» rojo a ≥ 48 px de Guardar; sin artículos confirma en una línea («Se archiva «Refri». Ya no aparece en las listas»); con artículos pregunta a qué lugar pasan (o «Sin lugar»). | Sección dentro de una pantalla que ya existe: ~64 px de encabezado y renglones de 56 px |

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Lugar del artículo | https://claude.ai/artifact/M8QDid4VeEAS6zEFsVG58h | A: renglón que abre la hoja | Un solo renglón de alto y aguanta muchos lugares; se elige pocas veces |
| Lugares | https://claude.ai/artifact/DvUdfJLDJCm7pVZE3xpDZh | C: mantener y arrastrar | El dueño lo pidió: se entiende mejor que subir y bajar (A y B descartadas) |

## Capas

| Pieza | Capa | Dónde |
|---|---|---|
| Normalizar nombre (reusa `domain.NormalizeReasonName`), validar 1–30 letras, calcular el orden nuevo al mover a una posición | domain | `server/internal/domain/count_places.go` |
| `count_places`, `ingredients.count_place_id`, paso de los textos, quitar `count_place`, RLS y grants | migración | `server/migrations/0095_count_places.sql` |
| Consultas: listar, crear (23505 → id existente), renombrar, mover, archivar y mover artículos, asignar; `CountItems` hace `left join count_places p on p.id = i.count_place_id` y ordena `p.position is null, p.position, i.name, i.id` (sin nombrar `i.company_id`: sqlc no la conoce) | sqlc | `server/queries/count_places.sql`, `stock_counts.sql`, `ingredients.sql` |
| Servicio | app | `server/internal/app/count_places.go`; `SetCountPlace` pasa a recibir un id |
| Rutas | httpapi | `handlers_count_places.go`, `router.go` |
| Picker del lugar | web | `web/src/features/backoffice/CountPlaceField.tsx` |
| Sección de lugares | web | `web/src/features/admin/CountPlacesSettings.tsx` en `InventorySettingsSection.tsx` |

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| E1 | «Refri» y «refri » en los datos viejos | La migración agrupa por el nombre normalizado y toma el más usado como nombre | migración |
| E2 | El mismo texto en dos empresas | Se agrupa por `(company_id, normalizado)` | migración con dos empresas |
| E3 | Crear un nombre repetido con otra mayúscula | Índice único parcial normalizado → 409 con el id existente, y el front lo elige | integración + vitest |
| E4 | Asignar un lugar de otra empresa o archivado | FK compuesta → 404; la asignación toma el lugar `for share` y exige `archived_at is null` → si no, 409 | integración tres casos |
| E5 | Asignar mientras otro archiva el mismo lugar | Archivar toma el lugar `for update` antes de mover; la asignación lo toma `for share`: una espera a la otra y nunca queda un artículo en un lugar archivado | integración con dos transacciones |
| E6 | Archivar con artículos | Mover y archivar en la misma transacción; destino activo (`for share`), de la misma empresa, distinto del archivado, o nulo. Mueve también los artículos inactivos | integración |
| E7 | Crear, mover y archivar a la vez | Las tres toman primero `pg_advisory_xact_lock` por empresa (como el candado 28037 de insumos) y después, en otra sentencia, leen los activos `order by position, id` y renumeran 0..n-1. `toPosition` es el ordinal entre los activos | integración con alta y movimiento simultáneos |
| E8 | Conteo abierto mientras se reordena | El conteo lee el orden al cargarse; lo capturado es por artículo | integración |
| E9 | Bajar la migración | La Down recrea `count_place` como en la 0092, la llena con el nombre y borra `count_places`; pierde el orden (ver Pregunta VIII) | migración de ida y vuelta |
| E10 | Normalización distinta en la base y en Go | Prueba de paridad del índice contra `domain.NormalizeReasonName` con «Ñandú», «Café  » y espacio duro | integración |
| E11 | Arrastre accidental al desplazar la lista | Solo la agarradera captura el gesto, tras 500 ms; 8 px antes cancela | Playwright táctil a 1024×600 (jsdom no lo reproduce) |
| E12 | Lista más alta que la pantalla | `autoScroll` de dnd-kit; arrastrar de la fila 10 a la 1 | Playwright táctil a 1024×600 |
| E13 | `toPosition` fuera de rango | 400 | integración |

## Revisión after_plan (2026-10-16)

`/revision-de-arquitectura` con `db-architect` y `tablet-ui-reviewer`: se pidieron cambios y todos
quedaron aplicados en este plan, en `data-model.md`, `contracts/api.md` y el spec. Fueron:

- Las carreras de asignar contra archivar y de crear, mover y archivar a la vez (E5, E7).
- La Down, que pierde el orden (E9), y la paridad de la normalización (E10).
- La liga `no action` con su índice.
- Listar con cualquiera de los dos permisos.
- La alta del `Picker` sin «Otro lugar…» ni foco automático.
- El arrastre con `delay` y `touch-action` solo en la agarradera, y el auto-scroll (E11, E12).
- El aviso de nombre repetido y la confirmación al archivar sin artículos.

El respaldo con flechas que propuso el revisor de tableta para quien no puede arrastrar lo rechazó el
dueño: solo arrastrar.

## Al construir (2026-10-17)

- **El plan no vio el segundo lugar donde se elige**: el conteo (038, FR-015f) también pone el lugar
  desde cada renglón. Los dos usan ahora el mismo `PlaceChooser` de `CountPlaceField.tsx`, y la hoja
  del conteo trae `placeId`.
- **La prueba de paridad (E10) encontró un defecto de la 039**: `NormalizeReasonName` juntaba
  espacios con `\s` de regexp, que en Go no ve el espacio duro y en Postgres sí; un motivo pegado con
  espacio duro no encontraba el que ya existía. Ahora usa `strings.Fields`, con su test.
- Listar lugares acepta `inventory.count` o `inventory.configure` con un middleware nuevo,
  `RequireAnyPermission`, con su test.
- La prueba sin contexto (T022) no halló dónde se asigna el lugar fuera del conteo: Almacén ›
  Artículos ahora lo muestra junto a cómo llega y su botón dice «Cambiar o poner lugar ›» (T025).
  La agarradera usa el icono ≡ que nombra la instrucción.
- La segunda vuelta de T022 no halló cómo ordenar los lugares (viven en Negocio y se buscaban en
  Almacén): el conteo, antes de empezar, dice el orden del recorrido y lleva a Negocio › Lugares
  (T026). El botón del lugar mostraba el valor viejo tras elegir: la hoja del artículo guarda una
  copia al abrirse; ahora `PlaceChooser` muestra lo elegido de inmediato.
- El arrastre y las flechas del teclado no se pueden simular en jsdom (todo mide 0 px): vitest solo
  verifica que la agarradera se ofrece al teclado; el arrastre lo mide `web/e2e/lugares-para-contar.spec.ts`.

## Project Structure

```text
specs/040-lugares-para-contar/
├── plan.md, research.md, data-model.md, quickstart.md, contracts/api.md
└── tasks.md            # /speckit-tasks

server/
├── migrations/0095_count_places.sql
├── queries/count_places.sql, stock_counts.sql, ingredients.sql
├── internal/domain/count_places.go (+ _test)
├── internal/app/count_places.go, stock_counts.go, articles.go
├── internal/httpapi/handlers_count_places.go, router.go
└── internal/integration/count_places_test.go, migration_0095_test.go

web/src/
├── features/backoffice/CountPlaceField.tsx
├── features/admin/CountPlacesSettings.tsx, InventorySettingsSection.tsx
└── api/backoffice.ts
```

**Structure Decision**: monorepo existente, sin paquetes nuevos.

## Complexity Tracking

Sin violaciones.
