# Implementation Plan: Ajustes de artículos y conteo tras la prueba en tableta

**Branch**: `044-ajustes-de-articulos-y-conteo` | **Date**: 2026-10-21 | **Spec**: [spec.md](spec.md)

## Summary

Seis ajustes de pantalla tras la prueba del dueño (#124) y los límites del conteo configurables por
empresa (#126):

- «← Volver a Artículos» en la ficha; «Revisar» legible en la lista.
- La gráfica de costos rediseñada con el SVG propio, sin librería.
- El teclado del conteo acepta el teclado físico.
- **Límites del conteo por empresa**: tabla nueva `count_limits` (una fila por empresa; por clase de
  unidad, el máximo por conteo y la cifra para pedir confirmar), sembrada al migrar y al crear empresa;
  editable en Configuración del negocio › Almacén; el conteo y su aviso la leen en lugar de los valores
  fijos; el mensaje «Máximo N <unidad> por conteo».

## Technical Context

**Language/Version**: Go 1.27.3 (server), TypeScript + React 19 (web)

**Primary Dependencies**: chi, pgx + sqlc, goose; Chakra UI v3, TanStack Query. Sin dependencias nuevas.

**Storage**: PostgreSQL 16 con RLS. Migración nueva `0098_count_limits.sql`.

**Testing**:
- Dominio: `CountSanity` con la cifra de confirmar de la empresa; validar límites; el mensaje del máximo.
- Integración: la migración con dos empresas (relleno, defaults, Down); leer y cambiar límites en los
  tres casos de aislamiento; el permiso; la bitácora; la captura rechazada con el mensaje y aceptada tras
  subir el máximo; una empresa nueva nace con su fila.
- vitest: volver, «Revisar», la gráfica (0, 1 y varios puntos, tocar), el teclado físico, el aviso del
  máximo antes de mandar, la sección de límites.
- Playwright **en local** (:3044) a 1024×600: la lista sigue con 6 renglones; la sección de límites mide 44 px por control; recorrido del almacén en claro y oscuro con contraste medido y capturas.
- **Todo se prueba en local** (constitución 1.25.0); el dueño no pidió ambiente de pruebas para la 044, así que no se valida en el ambiente de pruebas. Lo que solo se ve desplegado (build de producción, CSP) queda sin verificar hasta develop; para acercarse, la prueba local corre también contra `bun run build && bun run preview`.

**Target Platform**: tabletas de 7 a 10 pulgadas (1024×600)

**Project Type**: web (server + web)

**Constraints**: controles de 44 px o más, sin selector nativo; la migración tolera a la API anterior.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I Layering | `domain/count_limits.go`: tipo, defaults, validación y mensaje. `CountSanity` recibe la cifra de confirmar. Servicio en `app/count_limits.go` con `store.WithTx`. Handler fino; SQL en `queries/count_limits.sql` |
| II Errores | Límites inválidos y cifra sobre el máximo: `ErrValidation` envuelto con el texto que se muestra |
| III Dinero | No toca dinero |
| IV Tests | Prueba primero en cada tarea; migración probada con dos empresas; aislamiento con `inTheThreeCases`; `TestEveryCompanyTableIsIsolated` recoge `count_limits`; `TestEveryAuditActionIsProduced` la acción nueva |
| V Seguridad | Leer: autenticado con `inventory.count` o `inventory.configure`; cambiar: `RequirePermission(inventory.configure)`. Cifras con tope en la frontera (400, no 500) |
| VI YAGNI | Sin librería de gráficas. Una fila por empresa, sin historial propio (lo guarda la bitácora) |
| VII Idioma | Tabla, columnas, rutas y acción en inglés; texto de pantalla en español |
| VIII Puertas | Ver abajo |
| Tableta | Inputs de 44 px con `inputMode="decimal"`; nada nativo |
| Migración tolerante | Solo agrega una tabla: la API anterior no la lee y sigue con sus valores fijos, que son los defaults |

### Pregunta VIII: ¿se puede agregar después al mismo costo?

- **Varios almacenes (#65)**: no se cierra. Los límites son por empresa; por almacén sería una columna
  más y copiar la fila de la empresa.
- **Roles por empresa**: los controles preguntan por permiso.

## Pantallas y maquetas

| Pantalla | Qué cambia | Alto a 1024×600 |
|---|---|---|
| **Ficha del artículo** | «← Volver a Artículos» con borde. Gráfica: línea suavizada, área con degradado, guías tenues, último costo en etiqueta oscura, correcciones como punto hueco, fecha y costo al tocar | Igual; la gráfica ~190 px |
| **Lista de artículos** | «Revisar» como etiqueta roja del tamaño de la cifra | Filas de 44 px, 6 renglones |
| **Teclado del conteo** | Teclado físico; «Máximo N <unidad> por conteo» antes de mandar | Igual |
| **Existencias** (US7) | Solo colores: los fijos pasan a tokens del tema | Igual; sin maqueta |
| **Todo el almacén** (US8) | Repaso visual en claro y oscuro; se corrige lo que salga, sin cambiar disposiciones | Igual |
| **Configuración del negocio › Almacén** | Sección «Límites del conteo»: 4 clases × (máximo, confirmar desde) en kg, L, m y piezas; «Descartar» y «Guardar límites» | ~370 px, dentro de la caja Almacén, antes de «Se echó a perder»; la página se desplaza |

### Maquetas

Decisión del dueño: una sola opción por pantalla. Aprobadas el 2026-10-21.

| Pantalla | Maqueta | Elegida | Por qué |
|----------|---------|---------|---------|
| Gráfica de costos de la ficha | https://claude.ai/artifact/LEPLrLZU1Zb2u3cLyXA4Hx | Única: SVG propio rediseñado | Decisión del dueño: SVG antes que librería |
| Límites del conteo | https://claude.ai/artifact/MfLUsnnCX9UmR2tFK5VQzH | Única: tabla por clase con «Guardar límites» | Decisión del dueño (#126) |

El botón de volver, «Revisar», el teclado físico, Existencias en oscuro y lo que corrija el repaso no cambian la disposición: no llevan maqueta.

## Capas

| Pieza | Capa | Dónde |
|---|---|---|
| `count_limits` (company_id pk con FK a companies on delete cascade; `max_mass`, `max_volume`, `max_length`, `max_piece`, `confirm_mass`, `confirm_volume`, `confirm_length`, `confirm_piece`, numeric(14,4) en unidad base, `check` > 0 y `max > confirm`; `updated_at`), RLS con `nullif`, grants select/insert/update a `gatobobah_app`, relleno de empresas existentes | migración | `server/migrations/0098_count_limits.sql` |
| `CreateCompany` siembra la fila | sqlc | `server/queries/companies.sql` |
| `CountLimits` (defaults, `ForKind`, `Validate`, `MaxMessage`) y `CountSanity` con la cifra de confirmar | domain | `domain/count_limits.go`, `domain/count_sanity.go` |
| Leer (con fallback a defaults) y guardar (upsert) con evento `settings.count_limits_changed` | sqlc + app | `queries/count_limits.sql`, `app/count_limits.go` |
| La captura del conteo usa el máximo y la cifra de la empresa | app | `app/stock_counts.go` |
| `GET` y `PUT /settings/count-limits` | httpapi | `handlers_settings.go`, `router.go` |
| Sección «Límites del conteo» | web | `features/admin/CountLimitsSettings.tsx` en `InventorySettingsSection.tsx` |
| Aviso del máximo y teclado físico | web | `StockCountTab.tsx`, `components/NumericKeypad.tsx` |
| Volver, «Revisar», gráfica | web | `ArticleSheet.tsx`, `ArticlesTab.tsx`, `CostTrend.tsx` |
| Colores de Existencias y lo que encuentre el repaso | web | `StockPage.tsx` y los componentes que salgan |
| Recorrido visual del almacén (claro/oscuro, 1024×600) | prueba local | `web/e2e/almacen-claro-oscuro.spec.ts`, corrida con `E2E_BASE_URL=http://localhost:3044 E2E_API_URL=http://localhost:8044/api/v1 E2E_SLUG=bobah-pruebas E2E_PASSWORD=…` |
| Etiqueta de la acción nueva | web | `shared/audit/labels.ts` |

### Decisiones de la revisión de base de datos

- **Defaults en la columna, una sola vez**: máximo 1,000,000 en unidad base en las cuatro clases (igual
  que `MaxStockQty` hoy) y confirmar = `countCaps` de hoy. El relleno es `insert into count_limits
  (company_id) select id from companies on conflict do nothing` y la siembra de `CreateCompany` solo pasa
  el id. Un test de paridad compara la fila sembrada con `domain.DefaultCountLimits`.
- **Checks con nombre**: `> 0`, `max > confirm` por clase y `<= 1000000000` (techo de almacenamiento) en la columna.
- **El máximo sube o baja** (decisión del dueño, 2026-10-21: «que no quede fijo a nivel código, pero sí como un valor por default»). El conteo deja de validar contra `MaxStockQty` y valida contra el límite de la empresa; el único techo es `domain.MaxCountLimit` = 1,000,000,000 en unidad base (1,000 t), que existe solo para que la cifra quepa en `numeric(14,4)` y en los ajustes que genera. Compras, recetas y ajustes siguen con `MaxStockQty`.
- `company_id` con default `nullif(current_setting('app.company_id', true), '')::bigint`, como la 0095.
  Sin grant a `gatobobah_platform`. Sin índice extra.
- `updated_at` lo fija el upsert con `now()`. La lectura nunca escribe.
- **Down**: `drop table`; pierde los límites personalizados (es configuración, no un hecho). Lo dice su
  comentario.
- **Traslape**: un cambio de límites durante los segundos de la subida no lo respeta la API vieja.
  Aceptado.
- **Ocho columnas fijas**: son las cuatro clases de `unit_kind`; una clase nueva costaría una migración de
  columnas, que es el mismo costo que agregar la clase.

### Decisiones de la revisión de tableta

- **Límites**: pie con «Guardar límites» fijo (sticky) dentro de la sección y `scrollIntoView` al enfocar, para que el teclado del sistema no lo tape. Cifras con separador de miles al salir del campo. Texto: «Lo más que se puede registrar de un artículo en un conteo» y «Si lo contado pasa de esta cifra, el conteo pide confirmar».
- **Mensaje del máximo**: el cliente usa los límites de `GET /settings/count-limits` (se refresca al abrir el conteo); si el GET falla, usa los defaults y deja decidir al servidor; ante un 400 muestra el texto del servidor tal cual.
- **Teclado físico**: un solo listener en `StockCountTab`, no en cada `NumericKeypad`; se apaga con hojas, diálogos o Picker abiertos y con el foco en un input o un botón (Enter sobre un botón ya hace su clic). Tab hace `preventDefault` solo si hay otra casilla.
- **Gráfica**: el toque elige el punto más cercano por `x` (con 20 puntos no caben zonas de 44 px); la etiqueta se acomoda para no salirse del SVG ni tapar el punto.
- **«Revisar»**: celda con `whiteSpace="nowrap"` y ancho fijo; sin `title`.
- **Volver**: se mide a 800 px; si no cabe, «← Artículos» con borde. Flecha con `aria-hidden`.

### CI más corto (US9)

- **Repartir la integración**: matriz de 3 en el job `integration` de `.github/workflows/ci.yml`. Un script
  (`scripts/ci-shard-tests.sh`) lista las pruebas con `go test -list` y las asigna por turno al shard N;
  cada shard corre `go test -run '^(…)$'` con su Postgres de servicio. El script también verifica que la
  unión de los shards es la lista completa y que no hay repetidas: si no, el job falla. El paquete
  `internal/httpapi` va entero en un shard. Prueba del script en `scripts/tests/ci-shard-tests-test.sh`.
- **Imagen sin recompilar**: el caché de BuildKit (`type=gha`) no guarda los `--mount=type=cache` del
  Dockerfile, por eso cada merge recompila todo. Se compila el binario en el job `image` con `setup-go` (su
  caché sí persiste) y un `Dockerfile` con `ARG PREBUILT` copia ese binario; sin el argumento, el Dockerfile
  compila como hoy (para `make deploy`). Sin acciones de terceros nuevas.
- **Las pruebas lentas**: `go test -json` en el CI guarda la duración de cada prueba; las 10 más lentas van a
  «Al construir» con su causa. Lo barato (`t.Parallel` donde no toca estado global) se corrige aquí.
- **Lo que no cambia**: qué pruebas corren y cuándo; el pre-push; los despliegues.

## Bordes (se piensan antes del código)

| # | Borde | Cómo se cierra | Test |
|---|---|---|---|
| E1 | Una empresa sin fila (restauración vieja, alta a mano) | La lectura cae a los defaults del dominio; guardar hace upsert | Integración |
| E2 | Máximo ≤ confirmar, cero, negativo, NaN, sobre `MaxCountLimit` | 400 con el porqué | Dominio e integración |
| E3 | Otra empresa lee o cambia los límites | RLS; los tres casos | Integración |
| E4 | Bajar el máximo con un conteo abierto | Lo capturado no se toca; aplica a la siguiente captura | Integración |
| E5 | API anterior durante la subida | Sigue con sus valores fijos, que son los defaults | Por diseño |
| E6 | El aviso de cifra absurda sin otra referencia | Usa la cifra de confirmar de la empresa; no revela nada | Dominio |
| E7 | Kilos capturados en un artículo en gramos | El máximo se compara en unidad base tras convertir | Integración |
| E8 | Coma, letras, Enter con «Guardar» apagado, Enter con el aviso abierto, foco en otro input | Coma = punto; letras se ignoran; Enter no hace nada sin captura ni confirma el aviso; con foco en un input de texto el teclado no se apropia de las teclas | vitest |
| E9 | Gráfica con 0, 1 o 20 puntos; tocar fuera de un punto | 0: nada; 1: un punto con su cifra; tocar fuera cierra la etiqueta | vitest |
| E11 | Un color fijo que se ve en claro y desaparece en oscuro | Repaso con contraste medido (4.5:1 texto, 3:1 controles) en los dos temas | Playwright local |
| E12 | Una prueba nueva que ningún shard toma, o que dos toman | El script compara la unión con `go test -list` y falla | Script con prueba |
| E13 | Un shard sin pruebas (lista corta) | El shard termina en verde sin correr nada | Script con prueba |
| E14 | La imagen con un binario de otra arquitectura | El binario se compila `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, igual que hoy | El job verifica `file` del binario |
| E10 | Quien cuenta sin `inventory.configure` | Lee los límites (los necesita el aviso), no los cambia (403) | Integración |

## Al construir

(Se llena al implementar.)

## Project Structure

```text
server/migrations/0098_count_limits.sql
server/queries/count_limits.sql, companies.sql
server/internal/domain/count_limits.go, count_sanity.go, audit.go
server/internal/app/count_limits.go, stock_counts.go
server/internal/httpapi/handlers_settings.go, router.go
server/internal/integration/count_limits_test.go
web/src/features/admin/CountLimitsSettings.tsx, InventorySettingsSection.tsx
web/src/features/backoffice/StockCountTab.tsx
web/src/features/backoffice/articles/ArticleSheet.tsx, ArticlesTab.tsx, CostTrend.tsx
web/src/components/NumericKeypad.tsx
web/src/shared/audit/labels.ts
.github/workflows/ci.yml, server/Dockerfile
scripts/ci-shard-tests.sh, scripts/tests/ci-shard-tests-test.sh, lefthook.yml
```

**Structure Decision**: monorepo existente, sin paquetes nuevos.

## Complexity Tracking

Sin excepciones.
