# Contratos: los toques por zona

## `POST /api/v1/usage` — el mismo de la 017, con un campo más

```
{ "eventos": [ { "pantalla": "pos", "celda": 37, "orientacion": "horizontal" },
                { "pantalla": "caja", "accion": "cerrar-turno" } ] }
→ 204 No Content   (siempre)
```

- **Un evento con `celda` es un toque**; sin ella es lo de siempre (apertura o acción). Los dos
  viajan en el mismo lote, por la misma cola y con el mismo limitador: lo que hace que la medición
  no estorbe ya está escrito y probado, y un camino nuevo habría que volver a demostrarlo.
- **`celda` es un número de 0 a 83**, ya calculado en la tableta. **Nunca se manda `x` ni `y`**: un
  punto con precisión de píxel que viaja existe en el cuerpo, en el log de un proxy y en la memoria
  del servidor aunque después se redondee.
- **`orientacion`** es `horizontal` o `vertical`. Un toque sin orientación válida se descarta: la
  misma celda es otro lugar en cada forma.
- **Sin instante y sin elemento.** El día lo pone el servidor con la zona del negocio, y qué control
  se tocó no se guarda — esa pregunta la responde la 017 con las acciones con nombre.
- Un toque de una pantalla que **no está instrumentada** se descarta, como cualquier valor fuera de
  la lista blanca, y queda contado en el log del servidor.

## `GET /api/v1/platform/touches?pantalla=…&desde=…&hasta=…&empresa=…`

Grupo de plataforma, detrás de `RequireOperador`. Con una sesión del negocio: **401**.

```
→ 200 {
    "pantalla": "pos",
    "orientacion": "horizontal",
    "rejilla": { "columnas": 12, "filas": 7 },
    "periodo": { "desde": "2026-09-01", "hasta": "2026-09-12" },
    "celdas": [ { "celda": 0, "veces": 0 }, … , { "celda": 37, "veces": 412 } ],
    "porRol": [ { "rol": "cajero", "veces": 900 }, { "rol": null, "veces": 12 } ]
  }
```

- **Las 84 celdas viajan siempre**, incluidas las de cero: «qué parte no toca nadie» es la mitad de
  la pregunta, y una celda que se omite por no tener filas se pinta como un hueco.
- `orientacion` por defecto `horizontal`; se puede pedir la otra. **Nunca se suman las dos.**
- `porRol` reparte **el total de la pantalla**, no de una celda, y `rol: null` significa «sin
  corte»: ese uso existe pero atribuirlo identificaría a una persona.
- **Ninguna imagen, ningún texto de la pantalla, ninguna cifra de dinero.**
- Rango máximo: 92 días, que es lo que se conserva. Más viejo se **rechaza**; no se recorta en
  silencio a lo que hay.

## Lo que no existe, a propósito

- Ninguna ruta que devuelva toques sueltos: no se guardan.
- Ninguna que acepte ni devuelva imágenes.
- Ninguna pantalla del negocio que muestre esto.

## Configuración nueva

**Ninguna.** Ni una variable de entorno. La resolución de la rejilla y la lista de pantallas
instrumentadas viven en el código, donde un cambio se ve en el diff.
