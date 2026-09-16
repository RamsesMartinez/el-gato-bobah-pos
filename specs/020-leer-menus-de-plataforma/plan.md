# Implementation Plan: Leer el menú de las plataformas y decir en qué difiere del nuestro

**Branch**: `020-leer-menus-de-plataforma` | **Date**: 2026-09-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/020-leer-menus-de-plataforma/spec.md`

## Summary

Se lee el menú publicado de una plataforma, se guarda como una foto fechada, y se compara contra el
catálogo del POS usando un **emparejamiento que una persona confirmó una sola vez**. La pantalla
lista solo lo que difiere. Nada del sistema puede escribir en la plataforma, y eso se garantiza en
el transporte, no en una regla de estilo.

La primera plataforma es **Uber Eats**, porque es la única con acceso concedido hoy (ambiente de
pruebas activo desde el 2026-09-14). El diseño no la privilegia: lo específico de cada plataforma
vive en un paquete propio detrás de una frontera; el resto —foto, emparejamiento, comparación— no
sabe de cuál vino.

### Lo que se midió contra datos reales antes de escribir este plan

No son supuestos: salen del menú real de la tienda y del respaldo de producción del 2026-09-14.

| Hecho medido | Consecuencia de diseño |
| --- | --- |
| `external_data` viene **vacío en los 233 items** de Uber | El emparejamiento **no puede** apoyarse en que la plataforma traiga nuestro id. US2 no es opcional |
| Emparejar por nombre exacto acierta **6 de 65** platillos | La propuesta automática es una ayuda, no el mecanismo. FR-011 manda |
| `categories[].entities[].type` viene **ausente en 60 de 81** referencias, y su default es `ITEM` | Un lector que solo cuente los explícitos reporta 21 platillos donde hay 65 |
| Un **modificador ES un item**: 65 platillos + 157 opciones + 11 huérfanos = 233 | El lector distingue platillo de opción por **dónde lo referencian**, no por un campo |
| 16 platillos aparecen en **más de una categoría** | El id de la plataforma es la llave; la categoría no identifica |
| Producción: **76 grupos y 557 opciones** contra 42 y 157 en Uber | El hueco de opciones es mayor que el de platillos |
| Producción: **7 excepciones** en `product_platform_prices` sobre 174 productos | Hoy el precio por plataforma de FR-017 es siempre `base × (1 + markup)`. El requisito se cumple igual, pero el lado del POS es una fórmula |
| Uber entrega precios como **enteros en centavos** | Frontera de dinero con su prueba (principio III) |

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 / React 19 (front)

**Primary Dependencies**: chi, pgx + sqlc, goose (nada nuevo — el cliente HTTP es `net/http` de la
stdlib, como [hibp](../../server/internal/hibp/hibp.go))

**Storage**: PostgreSQL 16 con RLS por empresa. Cuatro tablas nuevas, migración `0071`

**Testing**: `go test` unitario en `domain` (comparación pura, sin I/O) · integración contra
Postgres real bajo `appRoleStore` para RLS y grants · vitest en el front

**Target Platform**: El POS corre en tabletas de ~1024×600. Esta pantalla vive en la
**administración del POS**, no en la consola de plataforma: la usa quien administra el catálogo del
restaurante, no quien vende el sistema

**Project Type**: Web (backend Go + front React en el mismo monorepo)

**Performance Goals**: La lectura de un menú no bloquea ningún request del mostrador (SC-006). El
menú real de la tienda pesa **211 KB sin comprimir / 23.8 KB con gzip**, medido

**Constraints**: Uber permite **100 tokens por hora y el 101 invalida el más viejo**; el token dura
30 días. El token se cachea o la integración se rompe sola

**Scale/Scope**: Decenas a cientos de productos por plataforma. Una empresa, **varias tiendas**

## Constitution Check

*GATE: pasa antes de Phase 0. Re-evaluado después de Phase 1.*

| Principio | Cómo lo cumple este plan |
| --- | --- |
| **I. Layering** | `httpapi` handlers finos → `app.MenusDePlataforma` (orquesta + tx) → `store/db` (sqlc). La comparación es **pura y vive en `domain`**. El cliente de Uber es un paquete propio `internal/uber`, sin DB ni lógica de negocio, como `internal/hibp` |
| **II. Errores** | Sentinels en `domain` (`ErrLecturaVacia`, `ErrPlataformaSinCredenciales`, `ErrParejaOcupada`), envueltos con `%w`, mapeados a HTTP solo en `httpapi.Error`. La goroutine de lectura tiene contexto con timeout propio: termina siempre |
| **III. Dinero** | Uber entrega **centavos enteros**; el POS usa pesos `float64` con `Round2`. La conversión es **una sola función en `domain` con su prueba**, y valida con `ValidMoney` antes de comparar |
| **IV. Test-first** | La comparación es una función pura table-driven. La migración lleva su test de integración **antes** (lo exige el hook [migracion-con-test.sh](../../scripts/hooks/migracion-con-test.sh)) |
| **V. Seguridad** | Credenciales por env validadas al arranque (fail-fast en `config.Validate`), nunca en la base ni en el repo. El `store_id` **no es secreto** y va en tabla. Los logs nunca llevan token ni secreto — incluido el caso de DiDi, que lo manda en el *query string* |
| **VI. YAGNI** | Sin programador de tareas, sin historial de menú más allá de la retención, sin abstracción de "proveedor de plataforma" con una sola implementación. El segundo conector decidirá si hace falta una interfaz |
| **VIII. Puertas** | Ver la sección siguiente. Es donde este plan se juega |

### Las puertas, nombradas

Tres decisiones de esquema que no se recuperan, y qué se hace con cada una:

1. **Varias tiendas por empresa.** *(Corregido en revisión: la primera versión de este plan tenía
   el `store_id` en una variable de entorno.)* Una empresa tendrá varias sucursales, y cada una es
   una tienda distinta en la plataforma. Un `UBER_EATS_STORE_ID` en el entorno hace imposible la
   segunda sin tocar el despliegue, y peor: ata la foto del menú a "la" tienda, sin forma de saber
   después de cuál era. **La conexión es una fila** con su `external_store_id`, y su índice único
   es `(company_id, delivery_platform_id, external_store_id)` — **no** `(company_id,
   delivery_platform_id)`, que es justo el "único por `company_id` que en realidad debería ser por
   sucursal" que la constitución nombra. `branches` no existe todavía; cuando exista, se agrega un
   `branch_id` nullable y nada se reparte.
2. **La llave del producto hacia afuera.** El emparejamiento se guarda contra el **identificador
   que la plataforma entrega**, textual y completo (FR-020), nunca contra el nombre. Los ids de
   Uber están **truncados a 20 caracteres y derivados del nombre** (`Chamoyada_de_Maracuy`), así
   que son frágiles del lado de allá — razón de más para guardarlos tal cual y no derivarlos.
3. **Promociones de plataforma.** El spec exige cruzar la puerta de "una lista de productos por
   plataforma" sin cerrar la de promociones. La foto guarda **el precio publicado tal como viene**,
   sin interpretarlo como "precio normal": el día que haya promoción, el dato de que ese día el
   precio publicado era otro sigue ahí. No se construye nada de promociones hoy.

### Lo que este plan NO cierra y deja anotado

- **Credenciales por env, no por empresa.** Hoy un despliegue sirve a una empresa. Cuando haya
  varias con su propia cuenta de Uber, las credenciales se mueven a una tabla cifrada. Eso es
  configuración, no un hecho histórico: **se agrega después al mismo costo**, y por eso gana el
  principio VI. La tabla de conexiones ya lleva `company_id`, así que nada hay que reparticionar.
- **Se emparejan platillos Y opciones; los grupos no** *(decidido el 2026-09-15; la primera versión
  de este plan dejaba las opciones fuera)*. Producción tiene 76 grupos y 557 opciones; Uber, 42 y
  157.

  La razón del cambio es el destino, no el gusto: **sin la pareja de cada ingrediente, un pedido de
  «Arma tu Crepa» no puede entrar solo al POS.** Llega el platillo y los ingredientes que el cliente
  eligió, y si esos ingredientes no están emparejados no se sabe qué se pidió — habría que repetir
  entera la sesión de emparejamiento cuando se construya la recepción de pedidos.

  El nivel de **grupo** queda sin usar, pero el enum lo contempla: agregarlo después obligaría a
  decidir de qué clase eran las filas ya escritas, y eso solo se puede adivinar.

## Decisiones de diseño

### 1. Que no pueda escribir: se garantiza en el transporte

FR-007 pide una prueba que falle si alguien agrega una escritura. Una revisión de código no basta y
un comentario menos. Dos capas:

- **Un `http.RoundTripper` que solo deja pasar `GET`**, envuelto alrededor del cliente de menú.
  Cualquier otro verbo devuelve un error nombrando la plataforma y la operación, **antes de abrir
  el socket**. El `POST` del token va por un cliente aparte, cuyo único destino permitido es el
  host de autenticación. Esto cumple el escenario 3 de la US4: aunque la credencial tenga permisos
  de escritura concedidos por Uber, el sistema no ejerce ninguno.
- **Una prueba que parsea el AST del paquete `internal/uber`** y falla si aparece cualquier
  `http.MethodPost/Put/Patch/Delete` fuera del cliente de token. Es la alarma temprana: falla en
  `go test`, no en producción.

El guardia de transporte es la garantía; la prueba de AST es lo que la hace fallar pronto.

### 2. La lectura no corre dentro del request

SC-006 lo exige y el edge case de "la plataforma no responde" lo confirma. El endpoint que dispara
la lectura **encola y responde de inmediato**; la lectura corre en una goroutine con su propio
`context.WithTimeout` (no el del request, que muere al responder) y escribe su resultado en el
registro de lecturas. La pantalla lee ese registro.

Sin programador de tareas: la lectura se dispara a petición. FR-001 dice "sin intervención humana
**en el momento de la lectura**" —o sea, sin pegar un token a mano—, no que tenga que ser
automática. Un cron es fácil de agregar después y hoy no cambia nada.

### 3. El token se cachea en memoria, y por qué eso tiene techo

100 tokens por hora, y el 101 invalida el más viejo — o sea que pedir uno por lectura es una bomba
de tiempo, no un desperdicio. El token vive **en memoria del proceso**, con mutex, y se renueva
cuando le quedan menos de 24 horas de los 30 días.

`// ponytail:` el techo es **un token por proceso**: con N réplicas de la API son N tokens vivos.
Con 100/hora eso aguanta de sobra hoy (una réplica). El camino de upgrade, si algún día hay varias,
es mover el token a Redis como caché compartida —que es exactamente para lo que Redis ya se usa— y
no cambiar nada más.

### 4. La foto se guarda por lectura, no se sobrescribe

Los items pertenecen a **la lectura que los trajo**, con retención acotada (constante en `domain`,
como `RetencionDeToquesEnDias` de la 019). Sobrescribir "el menú actual" sería más simple y
perdería para siempre el hecho de qué publicaba la plataforma antes — un hecho que no se
reconstruye.

Volumen medido: 233 items por lectura. Con una lectura diaria y 92 días de retención son ~21 mil
renglones por conexión. No es un problema.

**El emparejamiento NO cuelga de la foto.** Cuelga de la conexión y guarda el id externo como
texto. Si colgara de `platform_menu_items`, podar lecturas viejas se llevaría por delante el
trabajo manual de emparejar — el peor defecto posible de esta feature, y silencioso.

### 5. La comparación es una función pura

`domain.Comparar(itemsDeArriba, productosDelPos, parejas) → []Diferencia`. Sin DB, sin HTTP. Es lo
que permite probar los cinco escenarios de la US1 y los ocho edge cases con tablas, y es la razón
de que `domain` no tenga I/O.

Una lectura fallida o vacía **no llega a `Comparar`**: el servicio la rechaza antes (FR-005). La
función pura no puede distinguir "menú vacío" de "lectura rota", y darle esa decisión sería
esconderla donde no se ve.

### 6. El emparejamiento es UNA decisión a la vez, no dos listas lado a lado

*(Decisión tomada tras la revisión de arquitectura; la primera versión de este plan no tenía
ninguna decisión de pantalla, que es justo cómo esta clase de regla se pierde.)*

El spec dice *"ve las dos listas lado a lado"* y **eso no cabe**. Medido contra el repo: el rail de
[AppShell](../../web/src/app/AppShell.tsx) son 76 px y el `p={6}` de `Page` otros 48, así que quedan
**900 × 552 px útiles**. Dos columnas dan ~418 px netos cada una, y el truncado de Chakra corta al
final — con los nombres reales, «Chamoyada de Mango», «de Mora» y «de Maracuyá» se pintan las tres
como *«Chamoyada de M…»*. En una pantalla cuyo único trabajo es distinguir platillos, eso la
inutiliza.

Y hay un agujero peor que el ancho: **con dos listas, encontrar el producto correcto es scroll**.
174 productos en filas de 48 px son 9 visibles; hasta 19 pantallas de scroll por cada una de las 59
parejas manuales, contra un spec (SC-002) que exige terminarlas en una sesión.

La disposición que sí cabe, a ancho completo y con el alto contado:

| Franja | Alto |
| --- | --- |
| Padding de `Page` | 24 px |
| Progreso — «12 de 65 resueltas» | 52 px |
| El item de la plataforma, nombre **a dos líneas, nunca truncado** | 88 px |
| La acción: `[Confirmar] [Buscar otro]`, o un botón que abre el `Picker` | 44 px |
| Padding | 24 px |
| **Total** | **~232 px** de 552 |

- **El buscador no se diseña: ya existe.** [Picker](../../web/src/components/Picker.tsx) abre una
  hoja inferior con buscador automático arriba de 6 opciones y filas grandes, y ahí el nombre tiene
  ~850 px — ninguno de los reales se acerca a truncarse. **Prohibido el `<select>` nativo**, para
  elegir plataforma y para elegir producto. Esta regla vive aquí y no solo en el checklist de
  pruebas, porque la constitución ya la perdió una vez por vivir en el comentario de un componente.
- **Confirmar avanza sola al siguiente pendiente**, sin un tap de «Siguiente». Medido contra la
  disposición implícita del spec: de ~195 taps a ~124.
- **Los tres estados no compiten en la misma vista.** `sin pareja`, `propuesta` y `confirmada` son
  tres, pero con una decisión a la vez solo hay uno en pantalla, así que no hace falta inventar tres
  estilos legibles a la vez. Lo ya resuelto vive en una pestaña «Confirmadas (n)».
- **Confirmar en lote solo de lo que se ve.** Con 6 propuestas, un botón «Confirmar las 6» que
  dispare 6 `PUT` es aceptable **si las 6 están a la vista con nombre y precio de los dos lados** —
  eso es lo que lo separa de aceptar a ciegas, que FR-011 prohíbe. Lleva deshacer de un solo tap.

### 7. La lista de diferencias muestra lo accionable, no todo

Con 109 items sin pareja, meterlos en la misma lista que las diferencias de precio ahoga lo que sí
hay que corregir — y **la mayoría nunca se van a emparejar a propósito**: el POS tiene 174 productos
activos y Uber 65 platillos porque muchos nunca se publicaron. Es ruido estructural permanente, no
trabajo pendiente.

- Filtro por omisión: **`precio` + `disponibilidad`**.
- `solo_en_plataforma` y `solo_en_catalogo` viven en otra pestaña, con el conteo en la etiqueta
  («Solo en un lado (109)»), nunca expandida por omisión.
- Eso además recupera los ~58 px que se iba a comer un aviso propio: el número vive en la pestaña.
  Queda presupuesto para ~11 renglones de contenido en vez de ~8.
- **El nombre no va en columna de ancho fijo.** Apilado —principal en negrita, segunda línea gris
  con el otro lado— para que el truncado del punto 6 no reaparezca aquí.

### 8. Dónde vive lo que destruye trabajo

`DELETE /connections/{id}` se lleva en cascada las lecturas **y hasta 65 parejas confirmadas**. No
va en el encabezado de uso diario, junto a «Leer ahora»: va en la configuración de la conexión, y su
diálogo dice **cuántas parejas se pierden** antes de confirmar. Es la regla de separar las acciones
destructivas, aplicada al caso concreto.

## Project Structure

### Documentation (this feature)

```text
specs/020-leer-menus-de-plataforma/
├── plan.md              # Este archivo
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/
│   └── api.md           # Phase 1
├── checklists/
│   └── requirements.md  # Ya existe (de /speckit-specify)
└── tasks.md             # Lo crea /speckit-tasks
```

### Source Code (repository root)

```text
server/
├── migrations/
│   └── 0071_menus_de_plataforma.sql        # 4 tablas + RLS + grants
├── queries/
│   └── menus_plataforma.sql                # sqlc
└── internal/
    ├── uber/                               # NUEVO. Cliente de solo lectura, sin DB
    │   ├── uber.go                          #   token cacheado, GET de menú y tiendas
    │   ├── solo_lectura.go                  #   el RoundTripper que bloquea todo verbo != GET
    │   ├── menu.go                          #   tipos del menú y el aplanado a items
    │   ├── uber_test.go
    │   └── sin_escrituras_test.go           #   la prueba de AST (FR-007)
    ├── domain/
    │   ├── menu_de_plataforma.go            # Comparar, clases de diferencia, retención
    │   ├── dinero_de_plataforma.go          # centavos -> pesos, con ValidMoney
    │   └── *_test.go                        # table-driven
    ├── app/
    │   └── menus_de_plataforma.go           # leer (goroutine), guardar foto, emparejar, comparar
    ├── httpapi/
    │   ├── handlers_menus_plataforma.go
    │   └── router.go                        # rutas nuevas bajo /admin
    └── integration/
        ├── menus_plataforma_test.go         # RLS, grants, la migración
        └── emparejamiento_sobrevive_test.go # los DOS lados por donde se borra una pareja:
                                             #   la poda de lecturas y el borrado de un producto

web/src/
├── api/plataformas.ts
└── features/admin/
    ├── MenuDePlataformaPage.tsx             # la lista de diferencias (US1, US3)
    ├── EmparejarPage.tsx                    # el emparejamiento (US2)
    └── *.test.tsx
```

**Structure Decision**: monorepo existente, sin proyecto nuevo. El único paquete nuevo es
`internal/uber`, y existe por el principio I: el conocimiento de una API ajena no se mezcla con
`app`. Sigue el precedente de `internal/hibp`, que hace lo mismo con Have I Been Pwned.

**No se crea una interfaz `PlataformaLectora`.** Hay una sola implementación; el principio VI la
prohíbe hasta que exista un segundo consumidor real. Cuando DiDi se conecte, la interfaz se extrae
de dos implementaciones concretas, que es cuando se sabe qué firma necesita.

## Re-evaluación de la constitución, después del diseño

Lo que el diseño de Phase 1 cambió respecto de la primera evaluación:

- **Principio VIII, reforzado en tres puntos** que no estaban antes de escribir el modelo: el
  `external_store_id` dentro de la llave única (varias tiendas), la columna `kind` desde el día uno
  (para no tener que adivinar después de qué clase eran las filas viejas), y sobre todo que
  `platform_item_links` **no referencia la foto**: colgarla de ahí habría hecho que podar lecturas
  viejas borrara el emparejamiento en silencio, semanas después.
- **Principio IV, dos requisitos movidos al esquema.** FR-005 («una lectura vacía es una lectura
  fallida») quedó como `check` de base, y FR-012 («un item de la plataforma no apunta a dos
  productos») como llave primaria. Un `check` no se olvida en una rama nueva del servicio.
- **Principio V, un hallazgo del diseño**: `failure_kind` es una clase cerrada y no el mensaje de
  la plataforma, porque DiDi transmite su `app_secret` en el *query string* y guardar el error
  crudo escribiría un secreto en la base por un camino que nadie está mirando (FR-022).
- **Principio I, sin cambios**: el paquete `internal/uber` no importa `store` ni `app`, y `domain`
  sigue sin I/O.
- **Principio VI**: se resistió la tentación de una interfaz de plataforma y la de un programador
  de tareas. Ninguna de las dos tiene un segundo consumidor real hoy.

Sin violaciones nuevas que justificar.

### Lo que la revisión de arquitectura cambió (2026-09-14)

`db-architect` y `tablet-ui-reviewer` sobre este plan, antes de una sola línea de código. Once
correcciones; las dos primeras eran defectos reales del diseño, no matices:

| # | Hallazgo | Qué se cambió |
| --- | --- | --- |
| 1 | **`platform_item_links.product_id` con `on delete cascade`.** El documento protegía el emparejamiento de la poda de lecturas llamándola «el peor defecto posible» y lo dejaba colgando de `products` con cascade. En este repo los productos **sí se borran**: [docs/reorg/14_rollback.sql](../../docs/reorg/14_rollback.sql) lo hace | `on delete restrict`, con el precedente de `order_lines.product_id` citado, más su prueba |
| 2 | **El emparejamiento no cabe en 1024×600.** Dos columnas truncan los nombres justo por donde se distinguen, y sin buscador son 19 pantallas de scroll por pareja | Decisión 6: una decisión a la vez, con `Picker` |
| 3 | `failure_kind` como `text` a secas, siendo la columna de la que depende FR-022 | `check` con la lista cerrada |
| 4 | **El plan no tenía ninguna decisión de pantalla**; la prohibición del `<select>` vivía solo en el checklist de pruebas | Decisiones 6, 7 y 8 |
| 5 | La poda borraba la última lectura de una conexión abandonada, volviendo «hace 4 meses» indistinguible de «nunca» (FR-003) | La poda conserva siempre la más reciente |
| 6 | El edge case del menú truncado no tenía valor de fallo | `menu_truncado`, con su razón de ser valor propio |
| 7 | Los 109 sin pareja ahogaban lo accionable | Decisión 7: filtro por omisión |
| 8 | El `DELETE` de conexión, destructivo, sin lugar asignado | Decisión 8 |
| 9 | `PUT` de link sin validar que el `externalId` exista | Al contrato, con su 404 |
| 10 | La etiqueta del `externalStoreId` en pantalla podía delatar el nombre del campo de la API | Al contrato |
| 11 | El índice compuesto no sirve al chequeo de integridad, porque **las cascadas de FK saltan RLS** (medido contra Postgres real en la revisión) | Índice liso por `product_id`, con la razón escrita |

Los dos revisores dieron por buenas: las PK sin `company_id`, la llave única con
`external_store_id` —la puerta de sucursales queda abierta de verdad—, `text` y no `citext` para
los ids externos, los tipos, los grants, y que `gatobobah_platform` no reciba nada.

## Complexity Tracking

Sin violaciones que justificar. Las dos decisiones que podrían parecerlo, y por qué no lo son:

| Decisión | Por qué no es complejidad especulativa |
| --- | --- |
| Paquete nuevo `internal/uber` | Principio I: es I/O contra un tercero y no puede vivir en `app`. Precedente directo: `internal/hibp` |
| Guardar la foto por lectura en vez de sobrescribir | Principio VIII: qué publicaba la plataforma ayer es un hecho que no se reconstruye. La retención acota el costo |
