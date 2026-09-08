# Research: el folio con el que llegó el pedido, y lo que la plataforma se quedó

**Fecha**: 2026-09-07 · **Rama**: `014-folio-y-comision-de-plataforma`

Fase 0 del plan. Aquí van las decisiones que el spec dejó al diseño, cada una con su porqué y con
lo que se descartó. **Los números medidos no se re-derivan**: se citan de `docs/`.

## Lo que ya existe y no hay que construir

| Pieza | Dónde está | Qué aporta a esta feature |
|---|---|---|
| `orders.delivery_platform_id` (nullable, FK compuesta con `company_id`) | [0007](../../server/migrations/0007_orders.sql), [0041](../../server/migrations/0041_fk_compuesta_tenant.sql) | Ya distingue un pedido de plataforma de uno de mostrador. El folio cuelga de él |
| Selector de lista activa | [PlatformPicker](../../web/src/features/pos/PlatformPicker.tsx) | El bloque de pantalla donde entra el campo, y el que ya sabe si hay plataforma |
| Pantalla de Ventas con lista + resumen del **mismo `where`** | [sales.sql](../../server/queries/sales.sql), [SalesFilter](../../server/internal/domain/sales.go) | El patrón de filtro nuevo se copia, no se inventa |
| Índice único **parcial** sobre columna nullable | [0057](../../server/migrations/0057_cobro_idempotente.sql) | Es exactamente la forma del índice del folio |
| Rastro `updated_by not null` en una tabla que escribe el cajero | [0037](../../server/migrations/0037_platform_prices.sql) | El precedente que justifica guardar quién escribió el folio |
| Tabla per-tenant nueva: RLS + policy + `grant` explícito | 0037 §2 | Sin el `grant`, producción devuelve `42501` en el primer request y dev nunca lo ve |
| Presupuesto de alto del POS medido a 1024×600 | [presupuesto-de-pantalla-1024x600.md](../../docs/presupuesto-de-pantalla-1024x600.md) | Decide dónde cabe el campo sin costar un renglón |

**Lo que NO existe y este plan tampoco construye**: entidad de depósito, entidad de documento de
pago, cliente HTTP contra ninguna plataforma, estimación de comisión al capturar, producto por
plataforma, importador del histórico de FUDO.

---

## D-1. El folio es una columna de `orders`, texto, nullable

**Decisión**: `orders.platform_order_ref text`, nullable, con tope de longitud y sin normalizar más
allá de recortar los extremos.

**Por qué**: lo fija [apis-de-plataformas.md §7.2](../../docs/apis-de-plataformas.md) — Uber es un
UUID de 36 caracteres, DiDi un entero de 19 dígitos y Rappi uno de 10. Un `bigint` no los cubre a
los tres y un `smallint`/`int` tampoco a uno solo. Guardar el código corto de 5 caracteres que ve el
personal es irrecuperable: del corto no se reconstruye el completo.

**Tope de longitud: 64.** El más largo conocido son los 36 del UUID de Uber; 64 deja lugar a un
prefijo o a un formato que hoy no se conoce, y sigue siendo cota de cordura contra un pegado
accidental de media pantalla. No es una regla de negocio, es la misma clase de guarda que
`MaxOrderQty`.

**Alternativas descartadas**:

- *Tabla aparte `order_platform_refs` (1:1)*: una fila por pedido de plataforma para una sola
  columna de texto. Obliga a un join en la lista de Ventas —la consulta más caliente de la pantalla—
  para una columna que se lee siempre que se lee el pedido. La liquidación sí va aparte (D-5), y por
  razones que aquí no aplican.
- *Reusar `orders.notes`*: el folio dejaría de ser buscable y de poder ser único.

## D-2. La unicidad es parcial, por `(company_id, delivery_platform_id, platform_order_ref)`

**Decisión**: `create unique index … where platform_order_ref is not null`, copiando la forma de
[0057](../../server/migrations/0057_cobro_idempotente.sql).

**Por qué el `where`**: dos NULL nunca chocan en un índice único, así que sin él el índice cargaría
con todo el histórico de pedidos de mostrador —la enorme mayoría— sin servir para nada.

**Por qué lleva la plataforma**: el mismo número de pedido puede existir en dos plataformas
distintas. Un índice por `(company_id, ref)` rechazaría una captura legítima, y
[§7.2](../../docs/apis-de-plataformas.md) marca eso como "barato de agregar, caro de quitar si ya
rechazó capturas legítimas".

**Por qué lleva `company_id`**: es requisito del spec (dos empresas pueden usar el mismo folio) y
además es el patrón de índices del repo — RLS agrega `company_id` a toda consulta del rol de app, y
un índice que no arranca por ahí descarta filas de otras empresas dentro del scan
([AGENTS.md §1](../../AGENTS.md)).

## D-3. La cadena vacía se rechaza en tres lugares, no en uno

El spec dice que `""` **no es** lo mismo que no tener folio. La defensa va en capas porque cada una
falla por su lado:

| Capa | Qué hace | Qué falla sin ella |
|---|---|---|
| `domain.NormalizePlatformRef` | Recorta extremos; cadena vacía tras recortar → `ErrValidation` | El handler decidiría, y el siguiente handler lo olvidaría |
| Check en Postgres | `platform_order_ref <> ''` y `= btrim(…)` | Un camino nuevo (una migración de datos, un script) mete `''` y rompe la unicidad en silencio |
| Índice único parcial | `where platform_order_ref is not null` | Dos `''` chocarían entre sí en vez de convivir como dos pedidos sin folio |

**La única normalización es recortar los extremos** (Assumption 4 del spec, y regla operativa de
[§7.1](../../docs/apis-de-plataformas.md): *se teclea tal como aparece en el reporte de pago*).
Mayúsculas, guiones y longitud se conservan: cualquier otra transformación destruye el valor que
sirve para conciliar. `strings.TrimSpace` recorta espacio Unicode; el check de Postgres usa
`btrim(x, E' \t\n\r')` para que la red de abajo cubra la misma clase de caracteres y no una más
angosta.

## D-4. El descuento se guarda como **total + parte de la plataforma**

**Decisión**: `discount_total` y `discount_platform` son los dos hechos que se copian del documento.
La parte del restaurante es `discount_total − discount_platform`, se calcula en `domain` y viaja en
la respuesta, pero **no se almacena**.

**Por qué**: es el dato que Rappi cambia por campaña
([plataformas-digitales.md §4-bis](../../docs/plataformas-digitales.md)): a veces absorbe un
porcentaje, a veces nada, y sobre 8 pedidos apareció en 3. Guardar los tres números deja dos
verdades sobre el mismo hecho, y en cuanto un documento corregido mueve una y no la otra, la fila se
contradice a sí misma y nadie sabe cuál mitad creer — el mismo modo de falla que la constitución
nombra en el principio III.

Con esta forma, el rechazo que pide el escenario 6 del spec ("parte de la plataforma mayor que el
descuento total") es una comparación real y no una imposibilidad por construcción.

> **Desviación declarada**: FR-012 lista "descuento financiado por el restaurante" entre lo que el
> sistema MUST guardar. Este plan lo **deriva** en vez de almacenarlo. Se declara aquí para que
> `/speckit-analyze` lo vea y el dueño lo confirme o lo rechace, no para resolverlo en silencio.

## D-5. La liquidación es su propia tabla, 1:1, con `order_id` como llave primaria

**Decisión**: `platform_settlements (order_id primary key references orders(id), …)`.

**Por qué tabla y no columnas** (Assumption 5 del spec, y aquí el detalle de esquema): la presencia
de la fila **es** el estado del dinero. Con columnas nullables en `orders`, "todavía no llega el
documento" y "el documento dice cero" se ven igual, que es justo lo que FR-015 prohíbe. Y el pedido
de mostrador —la enorme mayoría de las filas— cargaría ocho columnas nulas en la tabla que toca cada
venta.

**Por qué `order_id` es la PK y no un `id` propio**: FR-014 dice a lo más una liquidación por
pedido. Con PK en `order_id` eso lo garantiza el motor y el "reemplaza, no duplica" es un
`insert … on conflict (order_id) do update`, que es una sola instrucción y no un borra-e-inserta con
ventana de inconsistencia.

**Sin `on delete cascade`**: no existe ningún `delete from orders` en el repo —un pedido se cancela,
no se borra— así que la cláusula nunca dispararía. Lo que sí haría es sugerir que borrar un pedido
es un camino soportado.

## D-6. El neto puede ser negativo, y por eso hace falta un validador nuevo

`domain.ValidMoney` exige no negativo, así que **no sirve** para el neto: el escenario 4 del spec
—promoción financiada por el restaurante— produce netos negativos reales y hay que aceptarlos.

**Decisión**: `domain.ValidSignedMoney(v)` en [limits.go](../../server/internal/domain/limits.go):
`escalaSana(v)` y `|v| ≤ MaxMoney`. Los demás importes (bruto reportado, comisión, descuentos,
retenciones) siguen con `ValidMoney(v, true)`, que ya rechaza el negativo que el spec quiere
rechazar.

La guarda de `escalaSana` no es opcional: sin ella un literal `1e100000000` de 47 bytes quema ~25 s
de CPU y 279 MiB antes de que nadie valide nada.

## D-7. La comisión se guarda como **monto y tasa**, y la tasa es nullable

**Decisión**: `commission_amount numeric(10,2) not null` y `commission_pct numeric(5,2)` **nullable**.

**Por qué snapshot y no cálculo**: FR-013, y [§7.2](../../docs/apis-de-plataformas.md) lo marca
irrecuperable — recalcular el pasado con la tasa de hoy reescribe la historia. Las tasas medidas
(Uber 30%, DiDi 30%, Rappi 20%) son material para **proponer precios**, que es otra feature.

**Por qué la tasa es nullable y el monto no**: hay documentos que dan el monto sin declarar la tasa.
Un `0` en su lugar afirmaría "la plataforma cobró 0%", que es una mentira medible. Es el mismo
principio de FR-015 aplicado a una columna: la ausencia se representa como ausencia.

**Qué NO se guarda**: la base sobre la que la plataforma calculó la comisión — y resulta menos
irreversible de lo que parecía: siempre que `commission_pct` no sea nula,
`base = commission_amount / (commission_pct/100)` se reconstruye desde las dos columnas que sí se
guardan. El único caso genuinamente perdido es el documento que no declara tasa, y ahí ninguna
columna lo habría salvado, porque el dato nunca existió en el origen. En DiDi está resuelta
(30% del precio sin promoción, medido en 13 pedidos); **en Uber sigue sin resolverse**
([§4-bis](../../docs/plataformas-digitales.md)) y no se cierra con una columna que hoy nadie puede
llenar bien. Se agrega después al mismo costo cuando un pedido con promoción distinta de 50% lo
decida.

## D-8. El filtro de pendientes aplica a las cinco consultas, y son **cinco pares**

Las tres consultas del resumen hoy **no** llevan el filtro de estado, a propósito (el resumen tiene
que poder decir cuánto se canceló aunque la tabla esté filtrada a entregadas), pero **sí** llevan el
de tipo de venta.

**El filtro nuevo va en la familia del tipo de venta**: el escenario 2 de la US-2 exige que el
resumen describa el mismo conjunto que la lista **"sin excepción"**. Va, palabra por palabra, en
`ListSales`, `CountSales`, `SalesTotalsByStatus`, `SalesTotalsByMethod` y `SalesCancelledLines`.

**Forma del parámetro, y aquí está el detalle que cuesta caro**: NO se usa el patrón
`sqlc.narg('x')::boolean is null or (…)` con el que viajan `status` y `service_type`. Se usan **dos
variantes de cada consulta** —diez en total— y `app/sales.go` elige cuál llamar: una lleva el
predicado `o.delivery_platform_id is not null and o.platform_order_ref is null` **literal en el texto
SQL**, la otra no lo lleva.

**Por qué, medido contra Postgres real con 150k pedidos**: con el patrón OR y **plan genérico**
Postgres no puede probar que el predicado del índice parcial se cumple, porque el valor del parámetro
es desconocido al planear, y el plan cae a `Bitmap Index Scan on orders_date_status` filtrando a
mano — **incluso con el parámetro en `true`**. Y el plan genérico no es un caso raro: pgx usa
statements con nombre por default (`QueryExecModeCacheStatement`) y no hay override en
[store.go](../../server/internal/store/store.go), así que un statement que se repite en la misma
conexión del pool termina ahí solo. Con el predicado literal, el índice parcial se usa hasta con
`plan_cache_mode = force_generic_plan` forzado.

Sigue siendo **100% parametrizado**: son dos textos de consulta distintos, no SQL concatenado. Y no
rompe la regla de que las cinco viven juntas y se editan en la misma pasada — son cinco pares en el
mismo archivo, con el `where` de cada par idéntico salvo esa línea.

**Por qué se puede hacer aquí y no con `status`**: este filtro es **booleano**. `status` y
`service_type` son enums multivaluados, y dos variantes por valor serían una explosión combinatoria;
por eso allá el patrón OR es el correcto y aquí no.

**Por qué no un enum de tres valores** (`pendiente`/`capturado`/todos): "capturado" no lo pidió
nadie y sería la extensión especulativa que prohíbe el VI — y además convertiría este filtro en
multivaluado, que es justo lo que hace que el patrón literal deje de servir. El día que se pida, se
agrega al mismo costo.

**En la frontera HTTP** es `?folioPlataforma=pendiente`, con whitelist en
`domain.SalesFilter.Validate`: un valor presente y desconocido es `ErrValidation`, nunca un default
silencioso (FR-010, y principio V).

## D-9. Dónde cabe el campo en la pantalla del POS, medido

Del [presupuesto de pantalla](../../docs/presupuesto-de-pantalla-1024x600.md): el mosaico tiene un
paso de renglón de **114 px**, el bloque del `PlatformPicker` mide **40 px** sin plataforma y
**83 px** con una elegida, y el estado más apretado —plataforma activa, sin el aviso de caja— deja
**3 renglones y 25 px de sobra**. El piso táctil de la constitución es 44 px, así que **nada nuevo
cabe en esos 25 px si se apila**.

| Colocación | Alto que agrega | Renglones del mosaico | Veredicto |
|---|---|---|---|
| Renglón propio bajo los botones | +48 px (44 + gap) | 3 → **2** | Cumple SC-007 por los pelos; se usa solo como caída |
| **En el mismo `HStack` de los botones**, cuando hay plataforma activa | **+4 px** (el renglón pasa de 40 a 44) | **3 → 3** | **Elegida** |
| En la hoja de cobro | 0 px | 3 → 3 | Descartada: al cobrar, la tablet de la plataforma ya se movió (Assumption 1) |

Los cuatro botones de hoy (`Mostrador` + tres plataformas) ocupan ~440 px de los ~916 px útiles a
1024 px de ancho, así que un campo de ~200 px entra en el mismo renglón sin envolver. Con más
plataformas configuradas el `flexWrap` lo baja solo, y ahí cuesta el renglón — que SC-007 permite.
**Esto se mide, no se supone**: el quickstart trae el script y el resultado se anota en el documento
de presupuesto.

**El teclado del sistema**: el campo queda arriba del mosaico, así que el teclado —que sube desde
abajo— no lo tapa. Es la razón adicional para no ponerlo en una hoja inferior.

## D-10. La exigencia del folio es una hoja al mandar, con una sola salida

FR-005 pide el dato al capturar y **una** salida explícita. Con el campo en línea ya lleno, mandar
cuesta lo mismo que hoy: cero toques extra. Con el campo vacío, se interpone una hoja con el campo
enfocado y dos botones — *Guardar y mandar* / *Mandar sin folio* — que es **un toque más** en el
peor caso, exactamente lo que declara SC-003.

**Por qué no un campo opcional sin hoja**: un campo opcional no se llena y la feature se vuelve
decorativa (Assumption 2). **Por qué no obligatorio sin salida**: detiene la operación el día que el
dato de verdad no está, en hora pico, con el repartidor enfrente.

**Los dos botones van en un footer FIJO, dentro de un contenedor dimensionado con `dvh`** — el
patrón de [ModifierSheet](../../web/src/features/pos/ModifierSheet.tsx). No es cosmético: el campo
llega enfocado, así que el teclado del sistema sube solo, y si *Mandar sin folio* vive dentro del
contenido scrolleable el teclado lo tapa. La salida explícita pasaría a costar dos toques —cerrar el
teclado y luego tocarla— y SC-003 dejaría de cumplirse justo en el camino que la feature promete que
es barato. El quickstart lo comprueba con el teclado abierto, no solo con la hoja montada.

## D-11. Corregir el folio no toca dinero, y eso se prueba contra Postgres

El endpoint de corrección escribe **una** columna (más su rastro) y nada más. Que no mueva ninguna
cifra no se razona: se mide. El test de integración fotografía el corte de caja, el arqueo cerrado y
el resumen de ventas antes y después, y falla nombrando la cifra que se movió — la forma de
[`TestElFondoDeCajaSeCuentaUnaSolaVez`](../../server/internal/integration/corte_plataformas_test.go).

Ningún reporte de dinero del repo lee `orders.updated_at` (verificado con `grep` sobre
`server/queries/`), así que tocarlo es seguro.

## D-12. Guardar quién escribió el folio y cuándo

**Decisión**: `platform_ref_set_by bigint references users(id)` y `platform_ref_set_at timestamptz`,
ambos nullable, escritos junto con el folio.

**Por qué**: es el mismo trato que [0037](../../server/migrations/0037_platform_prices.sql) le da a
`product_platform_prices.updated_by` — *"es el rastro que justifica dejar que un cajero escriba
precios"*. Aquí el cajero escribe el valor con el que después se concilia un depósito, y un folio mal
tecleado se descubre semanas más tarde, cuando ya nadie recuerda quién capturó.

**Por qué ahora y no después**: la columna se agrega después al mismo costo; **el hecho, no**. Es la
pregunta del principio VIII, y la respuesta la vuelve barata hoy y perdida mañana.

**Las tres columnas van todo-o-nada**, con su check en el esquema: o están las tres o no está
ninguna. Un rastro que un camino futuro puede dejar a medias no es un rastro en el que se pueda
confiar meses después, que es cuando se usa.

**Lo que esta columna NO resuelve, para que no sorprenda**: `platform_ref_set_by` hereda la misma
ambigüedad que `orders.opened_by` — dos tabletas comparten la misma cuenta, así que dice *qué
cuenta*, no *qué estación*. Esa puerta ya estaba abierta en la tabla del principio VIII antes de
esta feature y no se cierra más; el día que se resuelva para `opened_by`, esta columna necesita el
mismo movimiento.

**Y no hay borrado**: el endpoint de corrección **sobrescribe**, nunca deja el folio en `null`. El
folio es el único dato irrecuperable de la feature, y ningún caso de uso que sobrescribir no
resuelva justifica un camino que lo destruye.

## D-13. La pantalla de Ventas: un toggle, un folio truncado y nada de columna nueva

Tres decisiones chicas que juntas deciden si la pantalla sigue siendo usable a 1024×600.

**El filtro de pendientes es un chip/toggle, no un `Picker`.** El valor es booleano, y como `Picker`
el ciclo completo cuesta cuatro toques (abrir hoja + elegir para encender, y lo mismo para apagar)
contra dos con un toggle. La vara de UX del POS es minimizar taps, y aquí el gesto es de ida y
vuelta: se enciende para trabajar los pendientes y se apaga para volver.

**El folio se muestra bajo el nombre de la plataforma, en la celda "Tipo", y truncado con elipsis.**
Una columna nueva no cabe: el contenedor de la tabla en
[SalesPage](../../web/src/features/sales/SalesPage.tsx) tiene `overflowY` pero **no** `overflowX`, así
que una columna de más desbordaría la página entera y no la tabla. Y sin truncar tampoco sirve: el
folio llega a 64 caracteres —el de Uber son 36— en una celda angosta, así que envolvería a dos o tres
líneas **en casi todos los renglones** de la vista filtrada a plataforma, que es justo el caso de uso
de la US-2. El valor completo vive en el detalle, que es donde además se corrige.

**Los botones del `PlatformPicker` suben de 40 px a 44 px.** Están por debajo del piso táctil desde
que se escribieron, y en el estado "Mostrador" —donde el campo de folio no existe— nada los estira.
El archivo ya se toca por esta feature: arreglarlo aquí cuesta una propiedad, y dejarlo cuesta otro
turno.

## D-14. El formulario de la liquidación son nueve campos en 600 px de alto

**Apilados en una columna no caben**: nueve veces (etiqueta ~18 px + control 44 px + separación
8 px) son ~630 px, más que la pantalla completa, antes del encabezado, de los botones y de los
~250 px que se lleva el teclado numérico al abrirse.

**Decisión**: los siete campos de dinero en **dos columnas** (etiqueta encima del control, dos por
renglón) y los dos de texto —referencia del depósito y documento— al final a ancho completo. Con eso
son cuatro renglones de dinero (~280 px) más dos de texto (~140 px), que caben con el footer fijo.

**El footer de guardar es fijo y el contenedor va en `dvh`**, por el mismo motivo que D-10: con el
teclado abierto, un botón de guardar dentro del scroll deja de existir.

Esto se mide, no se supone, y el conteo se anota junto al del POS.

## D-15. El resumen de plataformas: cada tile dice sobre cuántos pedidos habla

Las tres cifras de SC-006 cubren **conjuntos distintos** —en el ejemplo del contrato, 96 pedidos
contra 74— y el contrato ya lo declara con `orders`, `incluye` y `excluye`. Pero eso vive en el JSON,
y **quien lee la pantalla no lee el JSON**: tres tiles hermanos con tres cifras se restan a ojo, que
es la forma exacta del fondo de caja que dejó un turno con $4,500 de faltante.

**Decisión**: cada tile nuevo lleva su conteo de pedidos como nota, igual que ya hacen *Canceladas* y
*Reembolsadas* en [SalesSummaryTiles](../../web/src/features/sales/SalesSummaryTiles.tsx). El dato ya
viaja en la respuesta; lo único que faltaba era usarlo.

**`incluye`/`excluye` van detrás de un icono de ayuda, no como texto permanente.** Cuatro líneas de
prosa por tarjeta en cada refresco gastan el alto que la tabla necesita, y son la explicación de un
cálculo — justo lo que la constitución manda guardar detrás de la ayuda.

**Pendiente antes de aprobar esta pieza**: la pantalla de Ventas **nunca se midió**. El presupuesto
documentado es solo del POS. Estimado —y se llama estimado— el chrome ronda 340–350 px sobre los
~552 px netos de `Page(fill)`, o sea unos 3–4 renglones de tabla, y esta fila resta otros ~72–78 px
en cuanto el periodo tenga liquidaciones, que deja de ser el caso raro apenas se adopte la feature.
Se mide con el método del POS antes de darla por buena.

## D-16. La búsqueda por folio, exacta y solo del folio de plataforma

**Decisión**: la pantalla de Ventas gana un campo de búsqueda que hace **igualdad exacta** contra
`platform_order_ref`, y contra nada más.

**Por qué existe**: sin ella el folio se guarda y sigue costando lo mismo encontrar el pedido. SC-002
pide localizar cualquier renglón del documento de pago "en un solo paso, sin comparar montos ni
fechas", y la prueba independiente de la US-1 es literalmente *"se busca ese pedido por el folio"*.
El plan de la primera pasada lo pasó por alto: agregaba el filtro de pendientes y la columna, pero
ninguna forma de buscar.

**Por qué exacta y no parcial**: el caso de uso es pegar el identificador que trae el documento. Una
búsqueda parcial sobre 64 caracteres devuelve varios candidatos y obliga a comparar, que es
justamente lo que se viene a eliminar.

**Por qué solo el folio de plataforma, y no también el número o el nombre interno**: son cosas
distintas que hoy comparten la palabra "folio", y mezclarlas en un solo buscador haría que "187"
devolviera el pedido #187 y también cualquier folio de Rappi que contenga 187. Ver D-18.

**Índice**: la búsqueda no puede usar el índice único, que arranca por `(company_id,
delivery_platform_id, …)` y deja la plataforma en medio; una búsqueda no la conoce. Por eso la
migración lleva **su propio** índice parcial `(company_id, platform_order_ref)
where platform_order_ref is not null`. Cuesta nada hoy y se paga solo desde el primer mes de
conciliación.

**Forma del parámetro**: `?folio=<exacto>`, y **puede combinarse** con el filtro de pendientes — la
combinación devuelve vacío por construcción (un pendiente no tiene folio) y eso es correcto, no un
defecto que haya que impedir.

## D-17. Los tests de migración y de dinero corren contra un respaldo real, anonimizado

**Decisión**: se agrega un camino que baja un respaldo de la base de producción, le borra los datos
personales y lo restaura en la base de pruebas. Los tests de la migración y los de "no se movió
ninguna cifra" corren contra eso.

**Por qué**: la constitución ya lo exige —*"el test corre contra una base restaurada de un respaldo
real y con al menos dos empresas"*— y hoy **no se está cumpliendo**: el harness de integración hace
`drop schema public cascade` y migra desde cero. La regla existe porque una base sembrada limpia
vuelve no-op todo camino "por cada otra empresa", y porque las formas que muerden las dejó la
historia: pedidos viejos sin nombre de folio, pedidos sin sesión de caja, los 158 que se archivaron
con la fecha equivocada. Nada de eso lo produce una siembra.

Y SC-005 lo pide por su lado: los totales se verifican **contra una copia de los datos de
producción**.

**De dónde sale**: el repo ya llega a la base de producción con `make prod-db-tunnel`
([Makefile](../../Makefile)), que abre un túnel por `gcloud compute ssh` a `pos-vps`. El camino nuevo
se apoya en ese, no inventa acceso.

**Anonimizado, no crudo**: el respaldo trae nombres de cliente y notas de pedido, que son PII. Una
base de pruebas que corre en CI no es lugar para eso, y el principio V ya trata a `customerName` y
las notas como datos que no salen ni a los logs. Se borran al restaurar, no después.

**Lo que NO se toca al anonimizar**: importes, fechas, estados, sesiones de caja y plataformas. Ese
es justo el material que estos tests vienen a probar.

## D-18. "Folio" ya significa dos cosas, y en pantalla hay que desambiguarlo

En este sistema la palabra ya está tomada: la columna **Folio** de la pantalla de Ventas es el
número del turno (`daily_number`) con su nombre cantable (`folio_name`, "Tigre"). El identificador de
la plataforma es otra cosa completamente.

**Decisión**: en la interfaz **nunca se le llama "folio" a secas**. Se le nombra por su plataforma
—*Folio de Uber Eats*, *Folio de Rappi*— o *Folio de la plataforma* cuando no hay una sola. El
encabezado de la columna existente no cambia.

**Por qué importa**: quien opera lee "Folio" en dos lugares de la misma pantalla y asume que es el
mismo dato. Es el tipo de ambigüedad que no rompe nada en el código y hace que alguien busque el
número equivocado con el documento de pago en la mano.

En el código y en la base sí se llama `platform_order_ref`, que no se confunde con nada.

## D-19. Lo que este plan deliberadamente NO cierra

- **Base de cálculo de la comisión de Uber**: sin resolver, y no se inventa columna (D-7).
- **Pedido de plataforma sin turno de caja abierto**: sigue exigiendo turno, igual que hoy. Este plan
  no lo empeora ni lo resuelve; el spec ya lo declara por su nombre.
- **Producto de plataforma ≠ producto del POS**: nada de este plan asume 1:1.
- **Depósito y documento de pago como entidades**: la referencia viaja como texto porque **expira**
  (Rappi conserva 3 meses, Uber 31 días); la estructura se agrega después al mismo costo.
- **Los 19 pedidos de plataforma de agosto de 2026 que viven solo en el respaldo de FUDO**: su propia
  feature, y nunca dentro de `orders`.
