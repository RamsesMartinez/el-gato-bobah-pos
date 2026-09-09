# Contratos: conteo de efectivo por denominaciones

Solo lo que cambia o nace. Todo bajo `/api/v1`, autenticado.

## Nace: `GET /cash/denominations`

Qué piezas se pueden contar en la moneda de esta caja.

```
GET /cash/denominations?currency=MXN
200 { "items": [
  { "id": 11, "value": "1000", "isCoin": false },
  { "id": 10, "value": "500",  "isCoin": false },
  ...
  { "id": 1,  "value": "0.50", "isCoin": true }
] }
```

Ordenadas de mayor a menor, que es como se cuenta un cajón. Solo activas.

## Cambia: `POST /cash/registers/{id}/open`

Hoy recibe el fondo como una cifra. Pasa a recibir **las piezas**, y el servidor suma.

```
POST /cash/registers/3/open
{ "counts": [ { "denominationId": 11, "pieces": 2 }, { "denominationId": 6, "pieces": 5 } ] }

201 { …la sesión, con openingCash ya calculado… }
```

O, por el otro camino:

```
{ "openingCash": 2350, "manualReason": "había un billete que no está en la lista" }
```

- **Los dos campos juntos se rechazan** (FR-015). No es un caso raro que valga la pena tolerar: son
  dos cifras del mismo dinero y no habría forma de saber cuál manda.
- **Ninguno de los dos** también se rechaza, salvo que las piezas vengan vacías, que significa abrir
  con el cajón en cero — una caja puede arrancar vacía y eso no es un error.
- `manualReason` es **obligatorio** cuando viene `openingCash`. Sin él, FR-016 se cae: habría cifras
  sin desglose y sin explicación.
- El total que mande el cliente junto a las piezas **se ignora**. El servidor recalcula.

## Cambia: `POST /cash/registers/{id}/close`

Igual, pero el conteo aplica **solo al método de efectivo**. Los demás siguen mandando su cifra.

```
POST /cash/registers/3/close
{
  "counts": [ { "denominationId": 11, "pieces": 1 }, … ],
  "declared": { "2": 4300, "3": 1200 },
  "notes": "…"
}
```

- `counts` sustituye lo declarado del método de efectivo; mandar **los dos** para ese método se
  rechaza.
- **`manualReason` es obligatorio cuando el efectivo viene en `declared` en vez de contado.** Este
  renglón faltaba y se agregó al implementar US2: FR-014 dice que escribir el total directamente
  exige un motivo, y FR-016 que NINGÚN arqueo queda con una cifra suelta — la apertura y el cierre
  son los dos arqueos del turno, así que la regla no puede aplicar solo a uno. Sin esto, todo lo que
  la apertura cerró se reabría por el otro lado.
- **El efectivo es el método con `kind = "efectivo"`**, que ahora viaja en cada renglón de `totals`.
  No se identifica por nombre: los métodos de plataforma en efectivo también tocan el cajón y solo
  `kind` los distingue — es la misma razón por la que el fondo de caja se suma a uno y no a cuatro.
- **Cerrar sin declarar efectivo no guarda conteo.** Una caja que no manejó efectivo no tiene arqueo
  que explicar, y un conteo en cero afirmaría "conté y estaba vacío", que es un hecho distinto de
  "nadie contó".
- Los métodos con `auto_declare` siguen sin pedir nada.
- El cierre sigue bloqueado por pedidos sin entregar, como hoy.

## Cambia: la vista del turno

Gana el desglose, para poder explicar una diferencia sin preguntarle al operador que cerró.

```
200 {
  …,
  "counts": {
    "apertura": { "total": "2350", "manualReason": null, "lines": [
      { "value": "1000", "pieces": 2, "subtotal": "2000" },
      { "value": "50",   "pieces": 7, "subtotal": "350"  }
    ]},
    "cierre": { "total": "4300", "manualReason": null, "lines": [ … ] }
  }
}
```

- **`counts` ausente o con un momento en null** = ese arqueo no se contó por denominaciones. Es lo
  que pasa con todos los cortes anteriores a esta funcionalidad, y la pantalla los muestra como
  siempre (FR-008).
- `subtotal` viaja calculado aunque sea derivable: lo lee un humano comparando contra su cajón, y
  hacer que la pantalla multiplique invita a que las dos multiplicaciones difieran.
