# Contratos de API — 008

Dos endpoints existentes que ganan campos. **Ninguno nuevo.**

---

## `GET /api/v1/cash-status`

Lo consulta el POS para saber si puede cobrar. Gana la información del turno viejo (FR-012).

**Antes**

```json
{ "open": true }
```

**Después**

```json
{
  "open": true,
  "openedAt": "2026-08-31T18:29:22Z",
  "businessDate": "2026-08-31",
  "deOtroDia": true
}
```

| Campo | Tipo | Significado |
|---|---|---|
| `open` | `bool` | Sin cambios. Si la caja que recibe ventas tiene turno abierto. |
| `openedAt` | `string?` | Cuándo abrió ese turno. Ausente si no hay turno abierto. |
| `businessDate` | `string?` | El día con el que se abrió el turno. Ausente si no hay turno abierto. |
| `deOtroDia` | `bool` | **Lo decide el servidor**: si el día de apertura del turno ya no es hoy en la zona del negocio. `false` cuando no hay turno abierto. |

Notas de contrato:

- `deOtroDia` **no se calcula en la pantalla**. La zona del negocio vive en el servidor; que cada
  tableta compare con su propio reloj es la familia de defectos que esta feature cierra.
- Compara **días de calendario**, no horas transcurridas: un turno abierto ayer a las 23:00 es de
  otro día aunque lleve una hora.
- Agregar campos es compatible hacia atrás: un front viejo ignora lo que no conoce.

---

## `GET /api/v1/cash-sessions/{id}`

Detalle de un corte. Gana la lista de sus ventas (FR-009, FR-010, FR-011).

**Se agrega al objeto de respuesta**

```json
{
  "sales": [
    {
      "id": 169,
      "dailyNumber": 158,
      "folioName": "Chartreux",
      "openedAt": "2026-09-04T13:16:10Z",
      "status": "entregada",
      "serviceType": "mostrador",
      "total": "42.00",
      "refund": "0.00"
    }
  ],
  "salesCount": 158,
  "salesShown": 158,
  "salesTotal": "6664.00"
}
```

| Campo | Tipo | Significado |
|---|---|---|
| `sales` | `array` | Las ventas que este corte cobró, más recientes primero. Incluye canceladas y reembolsadas, con su estado. |
| `salesCount` | `int` | **Cuántas hay en total**, no cuántas se mandaron. |
| `salesShown` | `int` | Cuántas trae `sales`. Menor que `salesCount` cuando se alcanzó el tope. |
| `salesTotal` | `string` | Suma de las ventas del corte **excluyendo canceladas, reembolsadas y propinas**. |

Notas de contrato:

- **`salesCount` no es opcional.** Un recorte silencioso se lee como "esto es todo": la pantalla
  tiene que poder decir cuántas hay.
- Tope de 200 filas (`MaxListLimit`, el que ya existe).
- `salesTotal` declara qué excluye, y la pantalla lo repite. Una cifra agregada que no dice qué
  incluye invita a sumarla con otra y a reportar dinero que el negocio no tuvo.
- Un corte sin ventas devuelve `sales: []`, `salesCount: 0`, `salesTotal: "0.00"` — nunca `null`.

---

## Sin cambios de contrato

- `GET /api/v1/sales` — **no gana filtro por corte**, a propósito (FR-014).
- `POST /api/v1/orders` — misma entrada y misma salida. Lo que cambia es de dónde salen
  `businessDate` y `dailyNumber`, que el cliente no manda ni puede influir.
- Todo lo del arqueo: agrupa por turno y no lo toca esta feature.
