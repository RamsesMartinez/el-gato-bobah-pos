# Implementation Plan: Recibir los pedidos de Uber Eats

**Branch**: `021-recibir-pedidos-uber` | **Date**: 2026-09-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/021-recibir-pedidos-uber/spec.md`

## Summary

Un pedido hecho en Uber Eats entra solo al POS y quien atiende lo acepta de un toque.

La plataforma entrega un **aviso firmado** a una URL pública; el aviso no trae el pedido, trae una
referencia. El sistema verifica la firma, resuelve de qué empresa es esa tienda, trae el detalle,
lo registra **fuera** de `orders` y avisa a la tableta por el canal en vivo que ya existe. Al
aceptar, se crea la fila en `orders` con quien aceptó como `opened_by`, ya pagada por la plataforma,
y esa tableta imprime.

Las decisiones y su porqué están en [research.md](research.md); aquí va dónde aterriza cada pieza.

## Technical Context

**Language/Version**: Go 1.27 (backend) · TypeScript 5 / React 19 (front)

**Primary Dependencies**: chi · pgx + sqlc · goose (embebido) · Redis (rate limit) · Chakra UI v3.
Cero dependencias nuevas: HMAC-SHA256 es `crypto/hmac` + `crypto/sha256` de la stdlib.

**Storage**: PostgreSQL 16 con RLS por empresa. Migración nueva: `0072`.

**Testing**: `go test` (unitarios en `domain`, integración contra Postgres real bajo build tag
`integration`) · vitest (front). El arnés de integración clona una plantilla ya migrada; quien use
`appRoleStore(t)` debe llamar antes a `newTestStore(t)`.

**Target Platform**: API en contenedor tras Caddy (TLS). Front en tabletas de 1024×600.

**Project Type**: Servicio web con front propio (monorepo existente).

**Performance Goals**: contestar el aviso en **menos de 5 s p95**, con presupuesto duro para la
llamada a Uber. El pedido visible en la tableta en **menos de 10 s** desde que el cliente pidió
(SC-001).

**Constraints**:
- El aviso se responde 200 con cuerpo vacío, y **solo** cuando quedó procesado (FR-005).
- Hay **11.5 minutos** para aceptar o rechazar; a los **90 s** Uber llama por teléfono al local.
- El mismo aviso puede llegar dos veces y **sin orden garantizado**.
- Ningún secreto ni dato de cliente en los registros de operación.

**Scale/Scope**: una empresa hoy, varias por diseño. Un local, varias sucursales por diseño. Volumen
real medido: decenas de pedidos al día, no miles.

## Constitution Check

*GATE: pasa antes de Phase 0, se re-evalúa tras Phase 1.*

| Principio | Cómo lo cumple este plan |
|---|---|
| **I · Layering** | `httpapi` valida firma y forma y responde; `app` orquesta y transacciona; `domain` decide (clasificar el evento, verificar la firma, mapear renglones, decidir si se puede aceptar); `store/db` es sqlc. El cliente de Uber vive en `internal/uber/` como el de la 020. |
| **II · Errores envueltos** | Sentinels nuevos en `domain` (`ErrFirmaInvalida`, `ErrAvisoRepetido`, `ErrPedidoYaDecidido`, `ErrTiendaDesconocida`); el mapeo a HTTP solo en `httpapi.Error`. `ctx` propagado hasta la query y hasta la llamada saliente, con su presupuesto. |
| **III · Dinero** | El precio lo manda la plataforma y se redondea con `Round2` en la frontera. El pedido pendiente **no** está en `orders`: no puede sumar a ningún total antes de existir. Al aceptar se registra **ya pagado por la plataforma**, nunca como deuda por cobrar. Deja test que falla nombrando el concepto duplicado. |
| **IV · Test-first** | Cada control deja su test antes que el código. La verificación de firma, la deduplicación y el mapeo de renglones son lógica pura y se prueban en `domain` sin base. Lo que un unitario no puede ver —grants de las tablas nuevas, aislamiento entre empresas, que la consulta sin tenant no filtre— va a integración con **dos empresas**. La migración se prueba con su test, no después. |
| **V · Seguridad** | Es una **puerta pública nueva**, la primera del negocio sin sesión: firma obligatoria, rechazo sin pistas de qué falló, tope de tamaño antes de interpretar, límite de frecuencia, evento de seguridad con clave estable y sin PII, y comparación de firma en **tiempo constante**. Detalle abajo. |
| **VI · YAGNI** | Sin cola de trabajos (Uber ya reintenta), sin segundo canal en vivo (el broker existe), sin dependencias nuevas, sin aceptación automática. |
| **VII · Comentarios** | El porqué de la consulta sin tenant, del presupuesto de tiempo y del `check` relajado va en el código, no en la respuesta. |
| **VIII · Puertas** | Ver abajo. |

### Puertas del principio VIII

| Puerta | Qué haría este plan | Veredicto |
|---|---|---|
| **Varias sucursales por empresa** | El aviso se resuelve por `external_store_id`, y `platform_connections` ya es por tienda | **Abierta**. No se agrega ningún contador ni único "por empresa" que debiera ser por sucursal |
| **Varias empresas** | La llave de firma va **en la base, por empresa** y no en el entorno: una URL sirve a todas | **Parcialmente abierta.** Recibir sí; decidir no. Ver la nota de abajo |
| **Más de una caja vendiendo** | Nada asume "la" caja: el pedido nace sin turno y se enlaza al que se abra | **Abierta** |
| **De quién es un pedido** | `opened_by` = quien aceptó, una persona real, no un usuario de sistema | **Abierta**, y mejorada: se sabe quién aceptó y cuándo |
| **Promociones de plataforma** | El aviso crudo se guarda tal cual, así que lo que Uber mande sobre promociones queda registrado aunque hoy no se interprete | **No se cierra** |
| **Costear con recetas** | Los renglones copian nombre y precio del momento | **No se cierra** |

**Por qué "parcialmente" y no "abierta"** (corregido tras la revisión de `db-architect`):
`UBER_EATS_CLIENT_ID` y `UBER_EATS_CLIENT_SECRET` siguen en el entorno, porque los usa el camino
**saliente** que construyó la 020. La firma de **entrada** queda por empresa, pero **aceptar y
rechazar** —la mitad operativa de la feature— salen con la identidad global.

Y el riesgo no es solo "no funciona para el segundo cliente": el `accept_pos_order` de la empresa B
saldría bajo la aplicación de la empresa A. Si Uber no rechaza limpio ese cruce de tienda contra
aplicación, una empresa estaría aceptando pedidos en la cuenta de otra. Eso es peor que un fallo.

Es una exención consciente —moverlo es refactorizar la 020 y no cabe en esta feature— pero queda
escrito así para que quien lea solo la tabla no concluya que la 021 deja el negocio listo para el
segundo cliente. Lo deja listo para **recibir**, no para **decidir**.

### Seguridad — lo que hay que responder, no en teoría

Es la primera ruta del negocio **sin sesión de usuario**. Las preguntas y sus respuestas:

- *¿Un atacante que conoce la URL mete un pedido?* No: sin la firma correcta se rechaza antes de
  interpretar el cuerpo. La URL no es un secreto y el diseño no supone que lo sea.
- *¿Puede saber si acertó?* No: el rechazo es el mismo mensaje para firma ausente, mal formada o
  incorrecta, y la comparación es en tiempo constante (`hmac.Equal`).
- *¿Puede tumbar la recepción a base de basura?* Hay tope de tamaño antes de leer el cuerpo y
  límite de frecuencia. El límite **no puede** dejar fuera los pedidos legítimos: se dimensiona muy
  por encima del volumen real y, como Redis es fail-open en este repo, un hiccup del caché nunca
  bloquea un pedido.
- *¿Puede hacer que un pedido caiga en otra empresa?* No: la empresa sale de la tienda, y la firma
  que valida es la de **esa** empresa. Un aviso firmado con la llave de A que declare una tienda de
  B no valida.
- *¿Se filtra algo en los registros?* El aviso crudo se guarda en la base, no en el log. En el log
  van clave estable e identificadores, nunca el cuerpo, nunca la llave, nunca datos del cliente.
- *¿Y los datos del cliente?* Se conservan solo lo que hace falta para preparar y entregar, y fuera
  de los registros de operación (FR-016). El aviso crudo, que sí los trae, vive en una tabla con su
  propia poda.

## Project Structure

### Documentation (this feature)

```text
specs/021-recibir-pedidos-uber/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── webhook.md
└── checklists/requirements.md
```

### Source Code (repository root)

```text
server/
├── migrations/
│   └── 0073_pedidos_de_plataforma.sql      # tablas nuevas, llaves de firma, relajar el check
├── queries/
│   └── pedidos_de_plataforma.sql           # sqlc
├── internal/
│   ├── domain/
│   │   ├── aviso_de_plataforma.go          # clasificar, verificar firma, decidir  (PURO)
│   │   └── pedido_entrante.go              # mapear renglones contra el catálogo   (PURO)
│   ├── uber/
│   │   ├── pedidos.go                      # traer el detalle, aceptar, rechazar
│   │   └── solo_lectura.go                 # YA EXISTE — hay que abrirle la puerta a 2 POST
│   ├── app/
│   │   └── pedidos_de_plataforma.go        # orquesta: resolver, traer, registrar, decidir
│   └── httpapi/
│       ├── handlers_webhook_plataforma.go  # la puerta pública
│       └── router.go                       # la ruta, fuera de RequireAuth y de WithTenant
└── internal/integration/
    └── pedidos_de_plataforma_test.go       # dos empresas, rol app, migración

web/src/
├── api/pedidosDePlataforma.ts
├── features/pos/AvisoDePedidoEntrante.tsx    # el aviso, en su propio overlay (ver abajo)
├── features/pos/PedidosEntrantesSheet.tsx    # la lista cuando hay más de uno
├── features/pos/motivosDeRechazo.ts          # código de la plataforma → frase en español
├── features/admin/PedidosDePlataformaPage.tsx # qué llegó y qué pasó (historia 4)
└── features/backoffice/CashPage.tsx           # YA EXISTE — una línea en la apertura (FR-023)
```

**Structure Decision**: la de siempre en este repo — nada nuevo. La única pieza que rompe el molde
es la ruta pública, y por eso va **fuera** de los dos grupos de middleware del router y con su
propio archivo de handler, para que se vea que no es una ruta más del negocio.

## Decisiones de diseño que el plan fija

Las nueve decisiones con su alternativa rechazada están en [research.md](research.md). Lo que hay
que tener presente al leer el resto:

1. **Síncrono con presupuesto** (D1): se contesta 200 solo si el detalle se trajo y se guardó.
2. **Una consulta sin tenant, aislada y con nombre** (D2): traduce tienda → empresa. Es la única.
3. **La llave de firma en la base, por empresa, y dos a la vez** (D3).
4. **El pedido pendiente vive fuera de `orders`** (D4); la fila de `orders` nace al aceptar.
5. **Aceptar no exige turno** (D5): `register_session_id` nace NULL y se enlaza después.
6. **Hay que relajar un `check` de la 0007** (D6): hoy un pedido de plataforma solo puede ser a
   domicilio, y Uber manda pedidos para recoger.
7. **El aviso a la tableta va por el broker que ya existe** (D7).
8. **Imprime la tableta que aceptó** (D8).
9. **Si Uber no concede el permiso del aviso de cierre de tienda, se consulta el estado** (D9).
10. **Los cuerpos crudos se podan a los 60 días** y la fila se queda. El ciclo de pago de la
    plataforma es mensual: pasado ese ciclo el cuerpo ya no sirve para conciliar nada y solo carga
    datos de clientes.

### Las pantallas, fijadas por la revisión de `tablet-ui-reviewer`

El plan no resolvía disposición y eso era el hueco. Lo que queda decidido:

- **El aviso vive en su PROPIO overlay, por encima de cualquier hoja abierta.** Las hojas del POS
  —ticket a pantalla completa, modificadores, cobro— se montan en un portal fuera del árbol de la
  página. Un aviso pintado como parte normal de la pantalla queda **debajo** de la hoja que se abra
  después: tapado justo durante una captura de mostrador, que es el escenario que se quería
  proteger. Y esa familia de defecto ya mordió una vez en este repo con el apilamiento de hojas de
  Chakra. **Va con su escenario de aceptación**: el aviso llega con el ticket abierto y se ve.
- **Nunca en la barra superior.** Esa fila ya se desborda ~55 px en 1024×600 antes de agregarle
  nada. El aviso va como franja horizontal fuera de ella, como `AvisoDeTurnoViejo`.
- **Con varios pendientes se muestra el más urgente**, con Aceptar y Rechazar directos, más un
  contador «+N» que abre la lista — el mismo patrón de `PedidosEnCurso`, no un componente nuevo.
  Aceptar el más urgente cuesta **un toque** (SC-002); aceptar el segundo cuesta dos, y se cuenta
  así a propósito.
- **Sin segundero.** Entre el segundo 47 y el 46 no hay ninguna decisión distinta que tomar. Se
  pintan minutos y color, con un umbral a los 90 s —cuando ya está sonando el teléfono del local— y
  otro cerca del final.
- **Aceptar y Rechazar no son dos botones iguales.** Aceptar grande y en la posición dominante;
  Rechazar chico, contorneado y separado. El patrón ya está resuelto en `PedidosEnCursoSheet`, que
  separa tres acciones con dos irreversibles.
- **El motivo de rechazo se elige con `Picker`, nunca `<select>`**, y con su frase en español:
  `ITEM_AVAILABILITY` → «Se acabó un ingrediente». El código de la plataforma no se pinta jamás.
- **En la apertura de caja va UNA línea**, no una tabla: «3 pedidos de Uber ya aceptados · $342.00
  quedan en este turno». Esa pantalla ya mide 1,494 px en 1024×600 y una rejilla nueva nace 200 px
  debajo del pliegue.

### Lo que hay que abrirle a `solo_lectura.go`, con cuidado

El transporte de `internal/uber/` **rechaza todo verbo distinto de GET** antes de abrir el socket, y
un test parsea el AST del paquete para que el intento falle en `go test`. Esa garantía existe porque
el `PUT` de menú reemplaza el menú publicado sin deshacer.

Esta feature necesita **exactamente dos** escrituras: aceptar y rechazar un pedido. Ninguna toca el
menú ni la tienda. La garantía no se quita: se convierte en **lista blanca de rutas**, de modo que
el `PUT` de menú y el `DELETE` de `pos_data` sigan siendo imposibles de disparar por accidente. El
test del AST se actualiza para verificar la lista blanca, no para desaparecer.

## Riesgos, con nombre

| Riesgo | Qué pasa si se cumple | Qué lo contiene |
|---|---|---|
| El secreto de la firma no es el que creemos | Ningún aviso valida y no entra ni un pedido | La llave es configurable (D3); se determina empíricamente con el primer aviso real |
| Uber no concede el permiso del aviso de cierre de tienda | FR-028 no se puede cumplir por ese camino | D9: se consulta el estado, que ya sabemos leer |
| No se puede generar un pedido de prueba | No hay validación de punta a punta contra Uber | FR-031: la feature se valida disparando avisos firmados contra el propio ambiente de pruebas |
| El detalle del pedido trae una forma distinta de la documentada | El mapeo falla o mapea mal en el primer pedido real | **El detalle crudo** se guarda byte por byte (`raw_detail`): se puede reconstruir y corregir días después, cuando Uber ya no lo entregue |
| La ruta pública se vuelve superficie de ataque | Ruido, o peor | Firma, tope de tamaño, límite de frecuencia y evento de seguridad, todos con su test |

## Complexity Tracking

| Violación | Por qué hace falta | Alternativa simple, y por qué se rechaza |
|---|---|---|
| **Una consulta que NO usa `store.QC(ctx)`** | El webhook no tiene empresa: la empresa es el resultado de la consulta, no su entrada | Usar `QC` es imposible, no solo peor: no hay `app.company_id` que fijar. Se acota devolviendo solo ids y con test de aislamiento |
| **Relajar un `check` de la 0007** | Uber manda pedidos para recoger en tienda, y hoy la tabla los rechaza | Forzar `domicilio` en todo pedido de plataforma: haría que el POS mienta sobre cómo se entregó, y rompe el camino de prueba más barato |
| **Verificar la firma contra varias candidatas** cuando dos empresas registran el mismo id de tienda | En el ambiente de pruebas las plataformas reparten ids de tienda compartidos, y demostrar la feature por empresa es FR-031 | Un único global de `(plataforma, tienda)`: apuesta la prueba del segundo cliente contra una coincidencia de sandbox, y deja que la empresa la determine un dato que cualquiera escribe en el cuerpo en vez de quién pudo firmarlo |
| **Abrir dos escrituras en un transporte de solo lectura** | Aceptar y rechazar son escrituras, y sin ellas no hay feature | Un segundo cliente HTTP sin la garantía: duplicaría el código y dejaría la puerta abierta de par en par. La lista blanca conserva la garantía donde importa |
