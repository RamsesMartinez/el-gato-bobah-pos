# Phase 0 — Investigación y decisiones

**Fecha**: 2026-09-14 · Feature [020](spec.md)

La investigación de mercado y de APIs **no se repite aquí**: vive en
[docs/apis-de-plataformas.md](../../docs/apis-de-plataformas.md) y su §7.2 es la lista de
decisiones de esquema que cierran puertas. Este documento registra lo que se decidió para esta
feature, con qué evidencia, y qué se descartó.

Lo que sigue se midió contra la tienda real el 2026-09-14 (ambiente de pruebas de Uber, `HTTP 200`
en las seis lecturas) y contra el respaldo de producción del mismo día.

---

## 1. La forma real del menú de Uber

`GET /v2/eats/stores/{id}/menus` devuelve **cuatro listas al mismo nivel**: `menus`, `categories`,
`items`, `modifier_groups`. No es un árbol.

Medido en la tienda: 1 menú · 12 categorías · **233 items** · 42 grupos de modificadores ·
**211,218 bytes** sin comprimir (23.8 KB con `Accept-Encoding: gzip`).

Los 233 items se reparten así, y la repartición no está en ningún campo:

| | Cuántos | Cómo se reconoce |
| --- | ---: | --- |
| Platillos | **65** | Referenciados desde `categories[].entities[]` |
| Opciones de modificador | **157** | Referenciados desde `modifier_groups[].modifier_options[]` |
| Huérfanos | **11** | Nadie los referencia — existen y no se ven en la app |

### Dos trampas que ya costaron un conteo mal hecho

- **Un modificador ES un item.** No hay tipo "opción" aparte. Un lector que pida "los items" y los
  cuente como platillos reporta 233 donde hay 65.
- **`categories[].entities[].type` viene AUSENTE en 60 de 81 referencias**, y su default es `ITEM`.
  Contar solo los que lo traen explícito da **21 platillos donde hay 65**. Es el error que este
  proyecto ya cometió una vez, y el lector tiene que tratar el campo ausente como `ITEM`.

**Decisión**: el aplanado a "items de plataforma" se hace por **procedencia de la referencia**, no
por un campo, y deja una prueba con un fixture del menú real recortado.

**Alternativa descartada**: confiar en `type`. Falla en el 74% de los casos reales.

---

## 2. El identificador externo: qué es y qué no

**`external_data` está VACÍO en los 233 items.** Es el campo que Uber reserva para la llave del
PDV, y como el menú se capturó a mano en el portal, nadie lo llenó. Llenarlo exige un `PUT`, que
esta feature no hace.

El id que sí viene (`items[].id`) lo genera Uber **derivándolo del nombre y truncándolo a 20
caracteres**: `Chamoyada_de_Maracuy`, `Chamoyada_de_Mojit🌿`. De 233 items, **59 llegan al tope**
y hay **20 prefijos repetidos**.

**Decisión**: el emparejamiento se guarda contra `items[].id`, **textual, completo y sin
normalizar** (FR-020). Es la única llave estable que existe hoy de ese lado.

**Riesgo descartado con medición (2026-09-15)**: se temía que renombrar un platillo en el portal
cambiara su id y rompiera la pareja. **No pasa.** Uber asigna el id al crear la entidad y lo deja
quieto: **111 de las entidades del menú tienen un id derivado de un nombre que ya no existe** —
`Elige_tu_mejor_opció` hoy se llama «Arma tu crepa de 2 ingredientes», `¿Agregar_crema_batid` se
llama «Extras para tu frappé». El id es una llave estable de verdad, no un slug vivo.

**Alternativa descartada**: emparejar por nombre en cada lectura. Medido: **6 de 65 (9%)**. Y
`Hot Chicken 🔥🔥🔥🔥` contra `Hot Chicken - Buldak` no empata ni con similitud razonable.

---

## 3. El dinero viene en centavos enteros

`price_info.price` es un entero en unidades menores: `13000` = $130.00 MXN.

**Decisión**: la conversión vive en **una sola función de `domain`**, valida con `ValidMoney` antes
de devolver, y tiene su prueba con los casos de borde (0, el tope de `MaxMoney`, un valor que
desborda). El paquete `internal/uber` devuelve **centavos**, fiel a la API: traducir en dos lugares
es cómo se cuela un redondeo distinto en cada uno.

Detalle observado que **no** se usa hoy: `tax_info.vat_rate_percentage: 15` en los items, cuando el
IVA de alimentos preparados en México es 16%. No afecta a esta feature —no se calcula impuesto—
pero queda anotado: cualquier spec futuro que use ese campo tiene que verificarlo antes.

---

## 4. Precios con contexto: el menú de Uber es un grafo, no un árbol

`price_info.overrides` permite que **el mismo item cueste distinto según el grupo de modificadores
en que aparezca** (`context_type: MODIFIER_GROUP`). Lo mismo con `quantity_info.overrides`.

**Decisión para esta feature**: la comparación usa el **precio base** del item y guarda los
overrides tal como vienen, sin interpretarlos. Comparar un precio con contexto contra un precio del
POS sin contexto reportaría diferencias que no son.

**Por qué importa aunque no se use hoy**: [docs/apis-de-plataformas.md](../../docs/apis-de-plataformas.md)
lo llama *"el error estructural más caro de esta familia"* — modelar el menú como árbol y después
intentar mapearlo a Uber. Guardar los overlays crudos desde el principio es lo que evita ese error
cuando toque escribir.

---

## 5. El token: 100 por hora y el 101 mata al más viejo

Medido: `expires_in` **2,592,000 segundos** (30 días), `scope: eats.store`. El límite documentado es
100 tokens por hora **y el número 101 invalida el más antiguo**.

**Decisión**: caché en memoria del proceso con mutex, renovación cuando queden menos de 24 horas.
Marcado `// ponytail:` con su techo —un token por proceso— y su camino de upgrade: Redis como caché
compartida, que es para lo que ya se usa.

**Alternativa descartada**: pedir token por lectura. Con lecturas frecuentes desde varios procesos
se invalida el token que otro está usando, y el síntoma es un 401 intermitente imposible de
reproducir.

**Verificado y vale la pena saberlo**: con las mismas credenciales contra el host de producción, el
token responde **401 `unauthorized_client`**. Los dos ambientes están separados de verdad del lado
de la autenticación.

---

## 6. El ambiente de pruebas devuelve la tienda REAL

`test-api.uber.com` devolvió la tienda del negocio, su `store_id`, su menú publicado completo y sus
horarios. La documentación de Uber, en cambio, dice que los datos de prueba *se reinician
periódicamente*. **Las dos cosas se contradicen y no está resuelto cuál manda.**

**Decisión**: es irrelevante para la lectura —leer la tienda real es exactamente lo que se
quiere— y **bloqueante para cualquier escritura**, que esta feature no hace. Queda como pregunta
abierta a Uber por `t.uber.com/integration-support` antes de que exista un spec que escriba.

---

## 7. Varias tiendas: la corrección que cambió el esquema

La primera versión de este plan ponía el `store_id` en una variable de entorno
(`UBER_EATS_STORE_ID`). **Está mal** y se corrigió antes de escribir código: una empresa va a tener
varias sucursales, cada una es una tienda distinta arriba, y un valor de entorno:

1. hace imposible la segunda tienda sin tocar el despliegue, y
2. ata cada foto de menú a "la" tienda, sin dejar registrado de cuál era. Ese segundo punto es el
   irrecuperable: es exactamente el *"único por `company_id` que en realidad debería ser por
   sucursal"* que la constitución nombra en su tabla de puertas.

**Decisión**: la conexión con una plataforma es **una fila**, con su `external_store_id` textual, y
el índice único incluye esa columna. `branches` no existe todavía; cuando exista, se agrega un
`branch_id` nullable y ninguna fila hay que repartir.

Verificado antes de decidirlo: **ningún archivo de `server/` ni de `web/` lee `UBER_EATS_*`**, así
que quitar esa variable del entorno no rompe nada.

---

## 8. El lado del POS de la comparación es casi siempre una fórmula

> **Corregido el 2026-09-15.** La primera versión de esta sección midió `company_id = 1`, que es
> **«Bobah Pruebas»** y no el negocio. El real es **`company_id = 2`, slug `gatobobah`**. La
> diferencia no era cosmética: 172 productos en vez de 174, cero excepciones de precio en vez de 7,
> y un catálogo aparentemente quieto desde julio cuando en realidad se editó el 13 de septiembre.
> Nada falló; salieron números plausibles y falsos. **Filtra por `slug`, no por el id.**

Medido en el respaldo de producción del 2026-09-14, empresa `gatobobah`:

- `product_platform_prices`: **7 excepciones** — 6 de DiDi y **1 sola de Uber Eats**.
- `delivery_platforms.price_markup_pct`: **35.00** para Uber Eats, DiDi y Rappi; 0 para "Propio".

O sea que el precio del catálogo "para esa plataforma" que exige FR-017 es hoy, para **173 de los
174** productos activos, `base × 1.35`. La excepción de Uber es una sola, y está mal capturada:
`Soda Explosiva` tiene precio base $35.00 y precio de Uber **$34.75** — más barato arriba que en el
mostrador, cuando la plataforma cobra 34.80% de comisión. Es el único producto que se ha vendido
por Uber.

**Decisión**: la comparación usa el precio efectivo por plataforma —excepción si existe, fórmula si
no—, que es como ya lo resuelve `MenuDoc` en [app/menu.go](../../server/internal/app/menu.go). No
se duplica esa regla: se reusa.

**Consecuencia que la pantalla debe decir**: con casi cero excepciones, casi toda diferencia de precio
que aparezca es "el markup por defecto no coincide con lo publicado", no "alguien capturó mal un
precio". Son dos mensajes distintos y confundirlos manda a buscar al lugar equivocado.

---

## 9. Modificadores: sí se emparejan, y por qué se cambió de opinión

Producción tiene **76 grupos de modificadores y 557 opciones**; Uber, 42 y 157.

**Primera decisión (2026-09-14): fuera del alcance.** El argumento era el costo: emparejar 557
opciones es más trabajo que emparejar 174 platillos, y el spec solo pedía comparar.

**Decisión corregida (2026-09-15): platillos y opciones, sí; grupos, no.** Lo que cambió no fue el
costo sino el destino. El objetivo declarado es que **un pedido de la plataforma entre solo al POS y
el operador solo lo acepte e imprima**. Un pedido trae el platillo **y los ingredientes que el
cliente eligió**; sin la pareja de cada ingrediente no se sabe qué se pidió, y el pedido no se puede
registrar. Dejarlo para después significa repetir entera la sesión de emparejamiento.

El nivel de **grupo** sigue sin usarse —la opción se identifica sola por su id, aparezca en el grupo
que aparezca— pero el enum lo contempla desde el día uno: agregarlo después obligaría a decidir de
qué clase eran las filas ya escritas, y eso solo se puede adivinar.

Dato relevante para cuando toque: según la tabla de incompatibilidades que publica Toast, **Uber es
la única de las cuatro grandes que soporta subgrupos anidados**. "Arma tu Crepa" cae de lleno ahí.

---

## 10. Lo que se descartó del alcance, con su razón

| Descartado | Razón |
| --- | --- |
| Programador de tareas para leer solo | FR-001 pide que la lectura no necesite a una persona **en el momento**, no que sea periódica. Un cron se agrega después sin tocar nada |
| Interfaz `PlataformaLectora` | Principio VI: una sola implementación. La interfaz se extrae cuando haya dos y se sepa qué firma necesitan |
| Historial completo de menús sin poda | El costo crece sin techo. Con retención acotada la puerta queda abierta al mismo costo |
| Emparejar automático y confirmar después | FR-011 lo prohíbe, y los datos le dan la razón: 9% de acierto por nombre significa que el 91% de las parejas automáticas serían inventadas |
| Leer las otras dos plataformas ahora | No hay acceso concedido. El diseño no las estorba: lo específico vive detrás de una frontera |
