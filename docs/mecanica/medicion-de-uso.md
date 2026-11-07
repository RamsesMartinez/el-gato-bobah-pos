# Medición de uso y mapa de toques

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar instrumentar pantallas para la consola de plataforma. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **Medición de uso** (spec 017): el POS manda lotes a `POST /api/v1/usage` y la consola los lee en
  `GET /api/v1/platform/usage`. **Agregar una pantalla o una acción se hace en DOS lugares**: la
  lista blanca de [server/internal/domain/uso.go](../../server/internal/domain/uso.go) —lo que no está ahí
  se descarta en el servidor y queda un `usage_descartado` en el log— y el mapa de rutas de
  [web/src/app/rutas-medidas.ts](../../web/src/app/rutas-medidas.ts). La etiqueta que se ve en el mapa va
  en [web/src/consola/etiquetas-de-uso.ts](../../web/src/consola/etiquetas-de-uso.ts), que es una copia
  deliberada: la consola no importa del POS.
- **Mapa de toques por zona** (spec 019): los toques viajan en el MISMO request, en un arreglo
  `toques` aparte, y la consola los lee en `GET /api/v1/platform/touches`. **Instrumentar una
  pantalla también son dos lugares**: `pantallasConToque` en
  [server/internal/domain/toque.go](../../server/internal/domain/toque.go) y `PANTALLAS_CON_TOQUE` en
  [web/src/app/rutas-medidas.ts](../../web/src/app/rutas-medidas.ts). Tres cosas que no son obvias:
  - **La lista tiene que ser SUBCONJUNTO de `pantallasMedibles`** (la de la 017): el toque entra por
    el mismo endpoint y se valida contra aquélla. Una pantalla que no esté allá tiene todos sus
    toques descartados en silencio, y su rejilla sale vacía sin un solo error. Lo vigila
    `TestLasPantallasConToqueSonSubconjunto`.
  - **Cuesta disco, no solo una línea.** Medido: 16.5 MB por trimestre, por empresa y **por
    pantalla** —84 celdas × 2 orientaciones × 5 cortes de rol × 92 días— contra un techo de 20 MB.
    Instrumentar una segunda pantalla lo duplica: vuelve a correr `toques_volumen_test.go` y mueve
    el techo a propósito, o no la agregues.
  - **La leyenda se actualiza con cada rediseño del POS.** Vive en
    [web/src/consola/zonas-del-pos.ts](../../web/src/consola/zonas-del-pos.ts) con su `FECHA_DEL_LAYOUT`,
    y es lo que reemplaza a la captura que está prohibido pintar debajo. Ningún test puede saber si
    las bandas siguen siendo ciertas: el día que se mueva el panel de la cuenta, la leyenda vieja
    describe una pantalla que ya no existe.
