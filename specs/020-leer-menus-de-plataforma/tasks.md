---

description: "Task list for feature 020 — leer los menús de plataforma y comparar"
---

# Tasks: Leer el menú de las plataformas y decir en qué difiere del nuestro

**Input**: [spec.md](spec.md) · [plan.md](plan.md) · [research.md](research.md) ·
[data-model.md](data-model.md) · [contracts/api.md](contracts/api.md) · [quickstart.md](quickstart.md)

## Los tests NO son opcionales en este repo

La plantilla de spec-kit los marca opcionales; la constitución los hace **principio IV, no
negociable**: primero el test que falla, después el código. Y una migración nueva sin su test de
integración **no se puede commitear** — lo bloquea
[migracion-con-test.sh](../../scripts/hooks/migracion-con-test.sh), que existe porque la regla ya
estaba escrita y aun así se rompió con la migración 0037.

Cada test de aquí dice **qué defecto concreto atrapa**. Un test que no contesta esa pregunta sobra.

## Formato: `[ID] [P?] [Story] Descripción con ruta`

- **[P]**: se puede hacer en paralelo (otro archivo, sin dependencias pendientes)
- **[US#]**: a qué historia pertenece

---

## Phase 1: Setup

**Propósito**: configuración y limpieza previa. Nada aquí depende de nada.

- [X] T001 Quitar `UBER_EATS_STORE_ID` de [deploy/.env](../../deploy/.env) — la tienda es una fila de `platform_connections`, no configuración, porque una empresa va a tener varias sucursales. Verificado el 2026-09-14: ningún archivo de `server/` ni `web/` lo lee, así que no rompe nada
- [X] T002 [P] Escribir el caso de `config.Validate` en [server/internal/config/config_test.go](../../server/internal/config/config_test.go) que falla si `UBER_EATS_CLIENT_SECRET` está puesto pero es un placeholder, y el que acepta las tres variables ausentes (la integración es opcional: sin credenciales el sistema arranca igual)
- [X] T003 Agregar `UberEatsClientID`, `UberEatsClientSecret` y `UberEatsEnv` a [server/internal/config/config.go](../../server/internal/config/config.go) con su validación fail-fast, siguiendo el patrón de `AnthropicAPIKey` (opcional, pero si viene, válida). Es **FR-021**: por entorno, nunca en el repositorio ni en la base

---

## Phase 2: Foundational (bloquea todo lo demás)

**⚠️ Ninguna historia puede empezar hasta terminar esta fase.**

### La migración, con su test ANTES

- [X] T004 Escribir [server/internal/integration/menus_plataforma_test.go](../../server/internal/integration/menus_plataforma_test.go) **antes de la migración**, y verlo fallar: cubre los cuatro `grant` a `gatobobah_app` bajo `appRoleStore`, el aislamiento entre las dos empresas del respaldo restaurado, que una FK no cruce empresas (los chequeos de integridad de Postgres **saltan RLS**), que `status='ok'` con `item_count=0` sea rechazado por `check` (FR-005), y que **dos tiendas de la misma plataforma quepan** en la llave única. Y los tres requisitos que descansan en las llaves, cada uno con su assert: **FR-012** (un segundo producto contra el mismo item de la plataforma es rechazado por la PK), **FR-010** (dos items distintos contra el **mismo** producto sí entran — es el caso real: una `Chamoyada` abajo, doce arriba) y **FR-014** (renombrar el producto del POS no toca la pareja). Sin estos asserts, los tres son promesas de un comentario
- [X] T005 Escribir [server/migrations/0071_menus_de_plataforma.sql](../../server/migrations/0071_menus_de_plataforma.sql) según [data-model.md](data-model.md): 2 enums, 4 tablas, RLS, grants solo a `gatobobah_app`, `set local lock_timeout = '3s'`, y `Down` que dropea tablas y tipos. **`platform_item_links.product_id` va `on delete restrict`**, no cascade — el precedente es `order_lines.product_id` en [0007](../../server/migrations/0007_orders.sql)
- [X] T006 Escribir [server/queries/menus_plataforma.sql](../../server/queries/menus_plataforma.sql) y correr `make sqlc`. **No nombres `company_id` en ninguna consulta**: RLS lo aplica solo. La poda **excluye la última lectura de cada conexión**

### La frontera del dinero (principio III)

- [X] T007 [P] Escribir `TestPesosDeCentavos` en [server/internal/domain/dinero_de_plataforma_test.go](../../server/internal/domain/dinero_de_plataforma_test.go): `13000 → 130.00`, el cero, el tope de `MaxMoney`, y un entero que desborda → `ErrValidation`. **Atrapa el defecto de que un precio absurdo de la plataforma entre como número raro en vez de rechazarse**
- [X] T008 [P] Implementar `PesosDeCentavos` en [server/internal/domain/dinero_de_plataforma.go](../../server/internal/domain/dinero_de_plataforma.go), con `Round2` y `ValidMoney`. Es **la única** conversión de centavos a pesos del sistema

### Los tipos puros

- [X] T009 [P] Crear los tipos y sentinels en [server/internal/domain/menu_de_plataforma.go](../../server/internal/domain/menu_de_plataforma.go): `ItemDePlataforma`, `ClaseDeItem`, `Diferencia`, `ClaseDeDiferencia`, `ClaseDeFallo` (los seis valores, espejo del `check` de la migración), `RetencionDeLecturasEnDias`, y `ErrLecturaVacia` · `ErrPlataformaSinCredenciales` · `ErrLecturaEnCurso` · `ErrSinLecturaValida` · `ErrParejaOcupada` · `ErrConexionDuplicada` · `ErrItemInexistente`
- [X] T010 [P] Escribir `TestLasClasesDeFalloCoincidenConElCheck` en [server/internal/domain/menu_de_plataforma_test.go](../../server/internal/domain/menu_de_plataforma_test.go), leyendo la lista del archivo de migración. **Atrapa que alguien agregue un valor en Go y no en la base**, o al revés — el síntoma sería un `23514` en producción al guardar una lectura fallida
- [X] T011 Mapear los sentinels nuevos a HTTP en [server/internal/httpapi/respond.go](../../server/internal/httpapi/respond.go) según la tabla de [contracts/api.md](contracts/api.md). El mapeo vive **solo** aquí (principio II)

**Checkpoint**: `make api-build && make api-test` en verde, y la migración aplica sobre el respaldo restaurado.

---

## Phase 3: User Story 4 — que nada pueda tocar la tienda (P1)

**Objetivo**: que escribir en una plataforma sea imposible, no improbable.

**Va primero a propósito**: si el cliente se escribe antes que su guardia, existe una ventana en la
que el guardia no está, y es justo cuando alguien prueba un `PUT` "solo para ver".

**Prueba independiente**: no existe en el sistema ningún camino que escriba contra una plataforma, y
un intento de agregarlo rompe una prueba.

- [X] T012 [US4] Escribir `TestElTransporteRechazaTodoVerboQueNoSeaGET` en [server/internal/uber/solo_lectura_test.go](../../server/internal/uber/solo_lectura_test.go) y verlo fallar: pide `POST`, `PUT`, `PATCH` y `DELETE` contra el host de menú y espera un error **antes de abrir el socket**, con el nombre de la plataforma y la operación en el mensaje
- [X] T013 [US4] Implementar el `http.RoundTripper` en [server/internal/uber/solo_lectura.go](../../server/internal/uber/solo_lectura.go). El `POST` del token va por un transporte aparte cuyo **único destino permitido es el host de autenticación**
- [X] T014 [US4] Escribir `TestNingunaEscrituraEnElPaquete` en [server/internal/uber/sin_escrituras_test.go](../../server/internal/uber/sin_escrituras_test.go): parsea el AST de `internal/uber` con `go/ast` y falla si aparece `http.MethodPost/Put/Patch/Delete` fuera de [solo_lectura.go](../../server/internal/uber/solo_lectura.go) y del cliente de token, **nombrando el archivo y la línea**. Es la alarma temprana de FR-007: falla en `go test`, no en producción
- [X] T015 [US4] Escribir `TestNingunaDependenciaDeNavegador` en [server/internal/uber/sin_escrituras_test.go](../../server/internal/uber/sin_escrituras_test.go) y su gemelo en [web/src/app/sin-automatizacion.test.ts](../../web/src/app/sin-automatizacion.test.ts): fallan si aparece una dependencia de automatización de navegador (playwright, puppeteer, selenium, chromedp, rod) fuera de las devDependencies de pruebas. **FR-008 es la misma clase de prohibición que FR-006 y hasta ahora era la única sin guarda** — y la que más caro sale: los términos mexicanos de Uber pactan terminación inmediata y sin causa

**Checkpoint**: la garantía de US4 existe antes que el primer request real.

---

## Phase 4: User Story 2 — leer el menú y emparejarlo (P1)

**Objetivo**: que exista una foto del menú publicado y que una persona la empareje con el catálogo,
una sola vez.

**Prueba independiente**: con un menú cuyos nombres coinciden parcialmente, el sistema propone las
evidentes, deja las dudosas sin decidir, y lo que la persona resuelve sobrevive a la siguiente
lectura.

**Por qué esta historia va antes que la US1**, aunque las dos sean P1: el propio spec lo dice — sin
emparejamiento, la comparación reporta que el 100% del menú difiere, que es cierto y es inútil.

### El fixture y el aplanado del menú

- [X] T016 [US2] Crear [server/internal/uber/testdata/menu.json](../../server/internal/uber/testdata/menu.json): un menú **con la forma real y datos inventados**. Conserva las trampas medidas —`type` ausente en la mayoría de las entidades, un modificador que es un item, ids truncados a 20 caracteres con acentos y emoji, precios en centavos, un item huérfano, un platillo en dos categorías, una categoría vacía— y **no** los platillos ni los precios del negocio: este repositorio es público (AGENTS.md §1)
- [X] T017 [US2] Escribir `TestAplanarElMenu` en [server/internal/uber/menu_test.go](../../server/internal/uber/menu_test.go) y verlo fallar. **Atrapa los dos defectos que ya costaron un conteo mal hecho**: que `type` ausente se trate como `ITEM` (contarlo al revés da 21 platillos donde hay 65) y que un modificador se cuente como platillo. Verifica también que el `external_id` se conserve **completo, con emoji y sin normalizar**
- [X] T018 [US2] Implementar los tipos del menú y `Aplanar` en [server/internal/uber/menu.go](../../server/internal/uber/menu.go): clasifica en **tres niveles** por procedencia de la referencia, no por un campo (FR-014a): `platillo` si lo referencia una categoría, `opcion` si lo referencia un grupo, y `grupo` para los `modifier_groups`. Si un item se referencia desde los dos lados, gana `platillo`. Una misma opción vive en varios grupos —medido: `Lemon_Pepper` en dos— así que se emite **una sola vez** por id

### El cliente

- [X] T019 [US2] Escribir `TestElTokenSeReusaYSeRenueva` en [server/internal/uber/uber_test.go](../../server/internal/uber/uber_test.go): dos llamadas seguidas piden **un** token, y uno a menos de 24 h de vencer se renueva. **Atrapa el defecto de pedir token por lectura**, que con el límite de 100 por hora invalida el que otro proceso está usando y produce 401 intermitentes irreproducibles
- [X] T020 [US2] Implementar `Client` en [server/internal/uber/uber.go](../../server/internal/uber/uber.go): token cacheado en memoria con mutex, `ListarTiendas` y `LeerMenu` con `Accept-Encoding: gzip`, sobre el transporte de T013. Marcar `// ponytail:` el techo de un token por proceso y su camino a Redis Cubre **FR-001**

### Las conexiones y la lectura

- [X] T021 [US2] Implementar el servicio de conexiones en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go): alta, baja y listado, con `ErrConexionDuplicada` desde la llave única
- [X] T022 [US2] Implementar la lectura en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go): corre en **goroutine con su propio `context.WithTimeout`**, no el del request (SC-006), y guarda la foto en una transacción. Rechaza la lectura vacía como `menu_vacio` y la truncada como `menu_truncado` **antes** de guardarla como `ok` Cubre **FR-002**
- [X] T023 [US2] Escribir `TestElClienteNoRegistraLaDireccionNiElCuerpo` en [server/internal/uber/uber_test.go](../../server/internal/uber/uber_test.go): ante un error de la plataforma, lo que llega a [logging.SecurityEvent](../../server/internal/logging/security.go) es **la clase de fallo y el host**, nunca la URL completa ni el cuerpo. **Atrapa FR-022 por el camino que el esquema no cubre**: el `check` de `failure_kind` protege la base, y el log quedaba abierto — DiDi transmite su `app_secret` en el *query string*, así que un `err.Error()` registrado escribe un secreto en la bitácora
- [X] T024 [US2] Escribir `TestElDisparoDeLecturaRespondeSinEsperar` en [server/internal/httpapi/handlers_menus_plataforma_test.go](../../server/internal/httpapi/handlers_menus_plataforma_test.go): con una plataforma simulada que tarda, el handler responde `202` de inmediato y la lectura termina después. **Es la única verificación automatizada de SC-006**, que es el criterio que protege el tiempo de respuesta del mostrador; hasta ahora solo se probaba a mano
- [X] T025 [US2] Escribir los handlers de conexiones y de lectura en [server/internal/httpapi/handlers_menus_plataforma.go](../../server/internal/httpapi/handlers_menus_plataforma.go), finos: decodifican, llaman al servicio, mapean el error
- [X] T026 [US2] Cablear las rutas en [server/internal/httpapi/router.go](../../server/internal/httpapi/router.go) bajo `RequireRole(RoleAdmin, RoleGerente)`, con `rateLimitUser` en el disparo de lectura — cada una consume cuota de un tercero

### El emparejamiento

- [X] T027 [US2] Escribir `TestProponerParejas` en [server/internal/domain/menu_de_plataforma_test.go](../../server/internal/domain/menu_de_plataforma_test.go): propone solo la coincidencia exacta de nombre, **marcada como propuesta**; con dos productos del POS del mismo nombre no propone ninguno y lo dice. **Atrapa el emparejamiento por aproximación silenciosa**, que con 9% de acierto por nombre inventaría el 91% de las parejas
- [X] T028 [US2] Implementar `ProponerParejas` puro en [server/internal/domain/menu_de_plataforma.go](../../server/internal/domain/menu_de_plataforma.go) Cubre **FR-011**
- [X] T029 [US2] Implementar confirmar, cambiar y deshacer pareja en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go). **Valida que el `externalId` exista en la última lectura `ok`** → `ErrItemInexistente`: como no hay FK hacia `platform_menu_items` —a propósito—, nada en la base lo impide, y un desajuste de codificación escribiría una fila que jamás empata con el `PUT` respondiendo 200 Cubre **FR-009** y **FR-013**
- [X] T030 [US2] Extender el emparejamiento a **opciones** (FR-014a) en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go): la pareja de una opción apunta a `modifier_options`, no a `products`, y `local_kind` nace en `'producto'` (FR-014b). **Sin esto un pedido de «Arma tu Crepa» no puede entrar solo al POS**: llegan el platillo y los ingredientes elegidos, y sin la pareja de cada ingrediente no se sabe qué se pidió
- [X] T031 [US2] Escribir `TestLaParejaDeUnaOpcionNoApuntaAUnProducto` en [server/internal/integration/emparejamiento_sobrevive_test.go](../../server/internal/integration/emparejamiento_sobrevive_test.go). **Atrapa el defecto de guardar el id de una opción en la columna de producto**: pasa los tipos y produce un mapeo que nunca empata
- [X] T032 [US2] Handlers de `pairing` y de `links` en [server/internal/httpapi/handlers_menus_plataforma.go](../../server/internal/httpapi/handlers_menus_plataforma.go), con el `{externalId}` url-decodificado y **guardado sin normalizar** (FR-020)

### Las dos puertas por donde se borra el emparejamiento

- [X] T033 [US2] Escribir [server/internal/integration/emparejamiento_sobrevive_test.go](../../server/internal/integration/emparejamiento_sobrevive_test.go) con **tres** casos, cada uno visto en rojo primero: `TestElEmparejamientoSobreviveALaPoda` (poda lecturas, las parejas siguen, y falla diciendo **cuántas se perdieron**), `TestBorrarUnProductoEmparejadoFalla` (`delete from products` sobre uno emparejado tiene que dar error de integridad — si vuelve a ser `cascade`, el reorg de datos de AGENTS.md §6 borra parejas sin avisar) y `TestLaPodaConservaLaUltimaLectura` (una conexión abandonada conserva su última lectura, o «hace cuatro meses» se vuelve «nunca» y FR-003 pide distinguirlos)

### La pantalla de emparejar

- [X] T034 [US2] [P] Agregar los tipos y las llamadas en [web/src/api/plataformas.ts](../../web/src/api/plataformas.ts)
- [X] T035 [US2] Escribir [web/src/features/admin/EmparejarPage.test.tsx](../../web/src/features/admin/EmparejarPage.test.tsx): una propuesta se ve distinta de una confirmada, confirmar avanza sola al siguiente pendiente, y **no hay ningún `<select>` en el árbol**
- [X] T036 [US2] Implementar [web/src/features/admin/EmparejarPage.tsx](../../web/src/features/admin/EmparejarPage.tsx) según la decisión 6 del plan: **una decisión a la vez, a ancho completo**, nombre a dos líneas sin truncar, y [Picker](../../web/src/components/Picker.tsx) para elegir producto — que ya trae buscador. La misma pantalla resuelve **platillos y opciones** (FR-014a), en ese orden: primero los 65 platillos, después los ingredientes que cuelguen de los ya emparejados. Dos columnas no caben en los 900 px útiles y truncan los nombres justo por donde se distinguen

**Checkpoint**: se lee el menú real del ambiente de pruebas y se empareja de punta a punta.

---

## Phase 5: User Story 1 — ver en qué difiere (P1) 🎯 la feature

**Objetivo**: la lista de diferencias.

**Prueba independiente**: con un menú que difiere del catálogo en un platillo de cada clase, la
pantalla nombra los tres y no inventa un cuarto.

**Depende de la fase 4**: necesita una foto y unas parejas. No es una dependencia inventada — el
spec la declara al justificar por qué US2 es P1.

- [X] T037 [US1] Escribir `TestComparar` en [server/internal/domain/menu_de_plataforma_test.go](../../server/internal/domain/menu_de_plataforma_test.go), table-driven, con los cinco escenarios del spec **y** los edge cases. Los que no pueden faltar: que dos productos idénticos **no aparezcan** (FR-018), que precio y disponibilidad salgan como clases distintas (FR-016), y que un producto solo en la plataforma **no traiga ninguna acción sugerida** (FR-019). Y el caso de **FR-014a**: cuando la plataforma cambia el id de un platillo, sale como «solo en la plataforma» y su producto como «solo en el catálogo» — **dos renglones visibles, nunca una pareja adivinada**
- [X] T038 [US1] Implementar `Comparar` puro en [server/internal/domain/menu_de_plataforma.go](../../server/internal/domain/menu_de_plataforma.go). **Sin DB, sin HTTP**: es lo que permite probar trece casos con una tabla
- [X] T039 [US1] Escribir `TestUnaLecturaVaciaNoLlegaAComparar` en [server/internal/app/menus_de_plataforma_test.go](../../server/internal/app/menus_de_plataforma_test.go). **Atrapa el peor reporte posible**: una lectura vacía tratada como dato válido dice que sobra todo el catálogo, que es justo lo que invita a una acción destructiva. El servicio la rechaza con `ErrLecturaVacia` antes de la función pura
- [X] T040 [US1] Implementar el cálculo de diferencias en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go), usando el **precio por plataforma** (excepción si existe, si no `base × (1 + markup)`) — la misma regla que ya resuelve `MenuDoc` en [app/menu.go](../../server/internal/app/menu.go), reusada y no duplicada Cubre **FR-015** y **FR-017**
- [X] T041 [US1] Handler de `differences` en [server/internal/httpapi/handlers_menus_plataforma.go](../../server/internal/httpapi/handlers_menus_plataforma.go), con el filtro `kind`. Un `kind` desconocido se **rechaza** con `ErrValidation`; nunca cae a un default en silencio (principio V)
- [X] T042 [US1] Escribir [web/src/features/admin/MenuDePlataformaPage.test.tsx](../../web/src/features/admin/MenuDePlataformaPage.test.tsx): abre mostrando solo `precio` y `disponibilidad`, y los «solo en un lado» viven en otra pestaña con el conteo en la etiqueta
- [X] T043 [US1] Implementar [web/src/features/admin/MenuDePlataformaPage.tsx](../../web/src/features/admin/MenuDePlataformaPage.tsx) según las decisiones 7 y 8 del plan: nombre **apilado y no en columna de ancho fijo**, y el borrado de conexión fuera del encabezado de uso diario, con el conteo de parejas que se pierden en su diálogo
- [X] T044 [US1] Agregar las dos pantallas a la lista blanca del **servidor**, en [server/internal/domain/uso.go](../../server/internal/domain/uso.go). **Va primero y no es opcional**: lo que no está ahí se descarta en el servidor y solo deja un `usage_descartado` en el log — la medición de las pantallas nuevas saldría vacía **sin un solo error visible**
- [X] T045 [US1] Agregar las mismas dos rutas a [web/src/app/rutas-medidas.ts](../../web/src/app/rutas-medidas.ts) y su etiqueta legible donde corresponda, más la entrada en el menú de administración. Instrumentar una pantalla son **dos lugares** (AGENTS.md §2), y este es el segundo

**Checkpoint**: la feature funciona. Es el MVP.

---

## Phase 6: User Story 3 — saber cuándo dejó de ser cierto (P2)

**Objetivo**: que ninguna comparación se presente sin decir de cuándo es.

**Prueba independiente**: con una lectura vieja y una reciente la pantalla distingue las dos; con
una fallida, dice que falló.

- [X] T046 [US3] Escribir `TestLosTresEstadosDeUnaLectura` en [server/internal/app/menus_de_plataforma_test.go](../../server/internal/app/menus_de_plataforma_test.go). **Atrapa el defecto de que «falló», «sin diferencias» y «nunca se ha leído» se vean igual** — significan cosas opuestas y una pantalla mal hecha los muestra idénticos
- [X] T047 [US3] Calcular `stale` y `readAt` en el servidor, en [server/internal/app/menus_de_plataforma.go](../../server/internal/app/menus_de_plataforma.go), y devolver `409` cuando la última lectura fue fallida. **Nunca** se entrega la anterior como si fuera de hoy (FR-004)
- [X] T048 [US3] [P] Traducir las seis clases de fallo a español en [web/src/features/admin/MenuDePlataformaPage.tsx](../../web/src/features/admin/MenuDePlataformaPage.tsx): «La lectura falló por tiempo agotado», nunca `failureKind: tiempo_agotado`. Y el campo de alta se etiqueta «ID de tienda en Uber Eats», con el cómo obtenerlo detrás de un icono de ayuda — nada de `externalStoreId` en pantalla

---

## Phase 7: Polish

- [X] T049 [P] Agregar a [AGENTS.md](../../AGENTS.md) §2 el renglón de esta feature: qué son las dos pantallas, que el `store_id` vive en tabla y no en el entorno, y **que el emparejamiento tiene dos vecinos que podrían borrarlo** — es lo que un agente futuro necesita saber antes de tocar el esquema
- [X] T050 [P] Actualizar [http/uber-eats.http](../../http/uber-eats.http) si algún endpoint cambió de forma al implementarlo
- [X] T051 Correr [quickstart.md](quickstart.md) completo contra el ambiente de pruebas, con los números esperados: **222 items** guardados (65 platillos + 157 opciones, no 233 ni 21) y el `POST` de lectura respondiendo en milisegundos
- [X] T052 Correr los gates: `cd server && go build ./... && go test ./...`, la suite de integración contra el respaldo restaurado, y `cd web && bun run lint && bun run vitest run && bun run build`

---

## Dependencias y orden

### Entre fases

- **Phase 1 (Setup)**: sin dependencias.
- **Phase 2 (Foundational)**: depende de Setup. **Bloquea todas las historias.**
- **Phase 3 (US4)**: depende de Foundational. **Va antes que la 4** — el guardia antes del cliente.
- **Phase 4 (US2)**: depende de 2 y 3.
- **Phase 5 (US1)**: depende de 4 (necesita foto y parejas).
- **Phase 6 (US3)**: depende de 4; se puede hacer en paralelo con la 5.
- **Phase 7**: al final.

### Dentro de cada historia

Test que falla → dominio puro → servicio → handler → ruta → pantalla. Sin saltarse el primero.

### Lo que se puede hacer en paralelo

- T002 con T001.
- T007+T008 con T009+T010 (dominio, archivos distintos), una vez aplicada la migración.
- T034 con T021–T033 (front y backend).
- Phase 5 y Phase 6 entre sí, después de la 4.
- T049 con T050.

---

## Estrategia de entrega

### MVP

Fases 1 → 2 → 3 → 4 → 5. Ahí la feature sirve: se lee el menú, se empareja y se ven las
diferencias. **Parar y validar** con el quickstart antes de seguir.

### Incremental

1. Fases 1–3 → existe la garantía de que nada escribe arriba.
2. Fase 4 → se lee y se empareja. Ya es útil sola: dice qué hay publicado.
3. Fase 5 → la lista de diferencias. **MVP.**
4. Fase 6 → la frescura, que es lo que hace que el día 2 no mienta.

### Lo que NO está en esta lista, a propósito

- Emparejar **opciones de modificador**. Producción tiene 546 y Uber 157; el spec no lo pide y la
  columna `kind` ya está para que agregarlo después no parta las filas existentes.
- Cualquier escritura hacia una plataforma.
- Un programador de tareas que lea solo.
- Las otras dos plataformas.

## Notas

- `[P]` = otro archivo, sin dependencias pendientes.
- Un commit por tarea o por grupo lógico, firmado y **sin `Co-Authored-By`**.
- Verificar que cada test se ve **en rojo por la razón correcta** antes de implementar. Un test que
  nunca se vio fallar no prueba nada.
