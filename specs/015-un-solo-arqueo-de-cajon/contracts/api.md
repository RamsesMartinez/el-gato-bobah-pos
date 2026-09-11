# Contratos: un solo arqueo de cajón

Solo lo que cambia. Todo bajo `/api/v1`, autenticado.

## Cambia: `POST /cash-sessions/close`

El cuerpo es el mismo de la 003, con una regla nueva: **`declared` solo acepta métodos cuyo dinero
NO está en el cajón**.

```
POST /cash-sessions/close
{
  "registerId": 1,
  "counts": [ { "denominationId": 11, "pieces": 1 }, … ],   // el cajón, contado
  "declared": { "2": 4300, "3": 1200 },                     // tarjeta, transferencia, plataforma en línea
  "notes": "…"
}
```

O, por el camino manual del cajón:

```
{ "countedCash": 835, "manualReason": "había un billete que no está en la lista", … }
```

`countedCash` va **aparte de `declared`** y no dentro, por lo mismo que `openingCash` en la
apertura: el dinero del cajón no se declara por método. Los dos caminos del cajón —piezas o cifra
con motivo— siguen siendo excluyentes.

- **Un método de cajón en `declared` se RECHAZA con 400 nombrándolo.** No se ignora: ignorarlo
  dejaría al operador creyendo que su cifra se guardó. El caso realista no es un atacante sino una
  tableta con el front viejo en caché —la app es una PWA con service worker— así que el mensaje tiene
  que decir qué hacer: recargar.
- El efectivo se declara **una sola vez**, por `counts` o por el camino manual de la 003
  (`manualReason` con la cifra). Los dos juntos siguen siendo `ErrConteoAmbiguo`.
- Los métodos con `auto_declare` siguen sin pedir nada.
- El cierre sigue bloqueado por pedidos sin entregar.

## Cambia: la vista del turno y el detalle del corte

Gana el arqueo del cajón como objeto propio, al lado de los totales por método.

```
200 {
  …,
  "drawer": {
    "expected": "8340.00",      // fondo + efectivo que llega al cajón + entradas − salidas
    "counted": "8290.00",       // null mientras el turno esté abierto
    "difference": "-50.00",     // null mientras el turno esté abierto
    "methodIds": [1, 8, 9, 10], // los métodos cuyo dinero está en este cajón
    "requiresCount": true       // si el cierre exige contarlo
  },
  "totals": [
    { …, "requiresEntry": false }  // si ESE método exige una cifra capturada
  ]
}
```

- **`drawer` ausente** = ese turno no tiene arqueo de cajón que reportar: es un corte anterior a
  esta feature, una caja que no maneja efectivo, **o un turno abierto consultado por id** —
  `register_session_totals` se escribe al cerrar, así que `GET /cash-sessions/{id}` sobre un turno
  vivo devuelve cero métodos y ningún arqueo; el turno abierto se lee por `/current`. La pantalla
  lo muestra como siempre y **no** inventa un cero.
- **`methodIds`** viaja para que la pantalla sepa a qué renglones no pedirles cifra, sin volver a
  decidirlo por su cuenta: dos derivaciones de la misma regla son dos pantallas que pueden diferir.
- Con el **arqueo ciego** encendido y el turno abierto, **`drawer.expected` y todos los
  `totals[].expected` viajan en null — en `GET /cash-sessions/current` Y en
  `GET /cash-sessions/{id}`**. Los dos, porque el segundo está abierto a rol cajero y acepta el id
  del turno abierto: nulificar uno solo deja la cifra a un request de distancia. Con el turno ya
  cerrado las cifras viajan, que es cuando la diferencia se muestra. Lo que la pantalla no debe mostrar no se le manda: ocultarlo
  en el cliente deja la cifra en la respuesta, legible con las herramientas del navegador, y el
  control dejaría de serlo. Aplica a todos los métodos y no solo al efectivo — ver el esperado de la
  tarjeta permite el mismo acomodo.
- **`methodIds` incluye a todos los métodos configurados para el cajón, hayan vendido o no**: si
  mañana cobran, su dinero cae en el mismo montón. Son renglones informativos del corte, y la
  pantalla no les pide cifra.
- **Y por eso cada renglón gana `requiresEntry`**, que dice si ese método exige captura para poder
  cerrar. Es el hallazgo más grave de la revisión de arquitectura: hoy la pantalla lo deduce de si el
  esperado es cero, así que con el esperado en null concluiría que **nada** falta por capturar y
  dejaría firmar un cierre vacío. Quién exige captura lo decide el servidor, que es el único que
  sigue viendo las cifras con el arqueo ciego encendido.

## Cambia: `PATCH /payment-methods/{id}`

Hoy solo acepta `autoDeclare`. Gana los dos interruptores, y **los tres son opcionales de verdad**:
en el cuerpo van como booleanos que pueden faltar, no como booleanos con default.

Es una corrección de la revisión de arquitectura, y el defecto que evita es concreto: el handler de
hoy declara `AutoDeclare bool` sin puntero, donde un campo ausente y un `false` explícito son
indistinguibles. Copiado tal cual para tres interruptores, un `PATCH {"isActive": false}` **resetea
`affectsCashDrawer` a falso** y saca del arqueo el dinero de ese método — plata reescrita por el tipo
de dato, no por el negocio.

```
PATCH /payment-methods/8
{ "isActive": false }
{ "affectsCashDrawer": false }
{ "autoDeclare": true }
```

- Rol de administración, como hoy.
- **Cada cambio deja evento de seguridad** con quién y cuándo: es configuración que mueve la
  reconciliación del dinero. El handler ya lo hace para `autoDeclare`; los dos campos nuevos pasan
  por el mismo camino y no por una rama nueva.
- **Desactivar un método no borra su dinero**: el esperado del turno abierto sigue contando lo ya
  cobrado con él, y las cifras de un corte cerrado no cambian.
- Un cuerpo sin ninguno de los tres campos se rechaza: un PATCH que no pide nada es un cliente roto,
  no una operación válida.

## Cambia: `GET` y `PATCH` de los ajustes del negocio

```
GET  /settings  → 200 { …, "blindCashCount": false }
PATCH /settings   { "blindCashCount": true }
```

- Rol de administración.
- **Aplica al siguiente cierre.** Un cierre a medio capturar no puede pasar de mostrar la diferencia
  a ocultarla: la pantalla lee el ajuste al abrir el cierre y lo conserva hasta terminarlo.
