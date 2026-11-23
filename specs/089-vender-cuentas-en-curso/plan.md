# Implementation Plan: Vender — canales compactos, más cuentas a la vista y qué le falta a cada una

**Branch**: `089-vender-cuentas-en-curso` | **Date**: 2026-11-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/089-vender-cuentas-en-curso/spec.md`

## Summary

Solo front. La fila de cuentas deja de decir el estado interno («Pagada · en cocina pagada») y dice
lo que le falta a la cuenta para quitarse —**Cobrar $X**, **Entregar**, **Cobrar $X y entregar**,
**Sin enviar**—, traduciendo el estado que ya manda el servidor (`domain.AccountState`). Desde la
cuenta se entrega (`POST /orders/:id/deliver`, ya existe) y se abre el cobro (hoja existente). La
parte de arriba se reacomoda según la maqueta que elija el dueño, para que los canales gasten menos
alto (y no crezcan al elegir plataforma) y quepan más cuentas.

## Technical Context

**Language/Version**: TypeScript 5 / React 19 (web). Go 1.27 sin cambios.

**Primary Dependencies**: Chakra UI v3, TanStack Query (ya en el repo). Nada nuevo.

**Storage**: N/A — sin migración, sin columnas.

**Testing**: vitest + Testing Library (unitarios de la traducción y de la fila), Playwright contra el
`make start` del worktree para medidas y el flujo de entregar/cobrar.

**Target Platform**: equipo de mostrador táctil. Presupuesto de esta feature 1024×600 (pedido expreso
del dueño); revisión a 1280×854 (mínimo de la constitución) y 2K.

**Project Type**: web application (monorepo `server/` + `web/`); solo cambia `web/`.

**Performance Goals**: la fila se repinta con el evento del servidor como hoy; entregar responde en
el tiempo de una petición.

**Constraints**: 44 px por control; sin `<select>` nativo; franjas del shell de 52 px (entrantes) y
60 px (agotados) restan alto cuando aparecen.

**Scale/Scope**: 5 a 25 cuentas vivas en hora pico; 4 canales.

## Constitution Check

| Principio | Cómo se cumple |
| --- | --- |
| I. Layering | Sin cambios de servidor. El estado sigue en `domain.AccountState`; el front solo lo nombra en `web/src/domain/cuentas.ts`. |
| II. Errores | Errores de entregar con `mensajeDeError` (sin internals), como en Pedidos. |
| III. Dinero | El monto que falta es `outstanding` del servidor; el front no suma ni redondea. |
| IV. Test-first | Unitarios de la traducción estado→«lo que falta» (tabla, los seis estados) y de la fila (acciones solo donde aplican, doble toque, error) antes del código. E2E de medida y de entregar/cobrar. Sin migración: no aplica el test de integración. |
| V. Seguridad | Sin control nuevo: entregar y cobrar ya están gateados en el servidor (tenant + sesión). La fila no inventa barrera. |
| VI. YAGNI | Sin endpoint nuevo ni campo nuevo: «lo que falta» es una traducción de estado, no un hecho. |
| VII. Comentarios | El porqué de la disposición en el componente; textos para quien opera. |
| VIII. Puertas | Ninguna se toca: no hay esquema. «Más de una caja vendiendo» no se afecta (la fila ya es de todas las tabletas). |
| Restricciones de pantalla | Maqueta elegida antes de implementar; 44 px; un solo estilo (componentes de `web/src/components` y botones de Chakra del tema). |

Resultado: pasa sin excepciones.

## Pantallas y maquetas

Una pantalla cambia: **Vender** (la parte de arriba del catálogo, las cuentas en curso, la franja «le
falta» del ticket y la hoja «Todas las cuentas»). Lo que no cambia: la cuadrícula de productos, las
categorías, la hoja de cobro.

### Maquetas

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Vender (parte de arriba y cuentas) | https://claude.ai/artifact/GmkrZHAxCFV2QbYmZsPu9Q | B · Columna de cuentas | Elegida por el dueño el 2026-11-23: 7 cuentas a la vista a 1024×600 (hoy 2), igual con franjas; entregar o cobrar de un toque |

### Disposición elegida (B), medida en el arnés con componentes reales

Modo ancho (contenedor ≥ 900 px, el de 1024×600 y mayores):

- **Renglón 1** (44 px, 12 arriba): canales como grupo pegado (Mostrador · DiDi · Rappi · Uber Eats,
  44 px de alto, sin ícono); con plataforma, el folio en el mismo renglón y el canal en naranja. Se
  quitan los renglones «Cobrando con precios de…» y «Mantén presionado…»: el ticket ya dice el canal
  bajo el nombre, y la indicación pasa a un botón de ayuda (44 px) junto al folio. A la derecha:
  buscar (plegado a botón cuando el ticket está abierto o hay folio), precios y editar.
- **Columna de cuentas** a la izquierda del catálogo, 248 px, de y≈60 al fondo, con desplazamiento
  vertical propio (`flex="1" minH={0}`, patrón de `Page fill`). Encabezado «Cuentas (N)» y «+ Nueva».
  Un renglón por cuenta (56 px): nombre y lo que le falta (color por R-1). Cuando le falta UNA sola
  cosa, el renglón trae su botón: **Entregar** (pagada sin entregar) o **Cobrar** (entregada que
  debe). Con las dos, el renglón no trae botón: se toca, se abre su ticket y ahí están las dos.
  Orden estable por antigüedad (la más vieja arriba): en una lista vertical, reordenar al tocar hace
  que la cuenta se mueva bajo el dedo. La cuenta abierta se marca y se desplaza a la vista.
  La cuenta nueva sin productos se pinta como primer renglón «Cuenta nueva».
- **Categorías y productos** a la derecha de la columna, como hoy.
- **Ticket**: bajo el encabezado, para un pedido, una franja «Falta …» con **Entregar** cuando le
  falta entregar (el cobrar ya está abajo).
- Cifras medidas (1024×600, ticket abierto): productos desde y=138 (hoy 196; 287 con plataforma),
  7 cuentas a la vista (hoy 2), 7 con las dos franjas; 8 productos completos (hoy 9), 6 con franjas.
  1280×854: 12 cuentas (hoy 4). Ningún control bajo 44 px.

Modo angosto (< 900 px, sin panel lateral): la fila horizontal de hoy con el texto nuevo de «lo que
falta»; la columna no cabe.

Defecto encontrado al medir, entra en esta feature: el aviso de agotado se coloca debajo de
`[data-testid="aviso-de-pedido-entrante"]`, que ya no existe desde la spec 093 (la franja se llama
`franja-de-entrantes`): hoy el aviso tapa la franja de pedidos por aceptar.

## Project Structure

### Documentation (this feature)

```text
specs/089-vender-cuentas-en-curso/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/ui.md
├── checklists/requirements.md
└── tasks.md            (/speckit-tasks)
```

### Source Code (repository root)

```text
web/src/
├── domain/cuentas.ts                 # ESTADO + loQueFalta (texto y acciones por estado)
├── features/pos/
│   ├── estadosDeCuenta.ts            # medidas de la fila, orden
│   ├── ColumnaDeCuentas.tsx          # columna B (modo ancho)
│   ├── FilaDeCuentas.tsx             # fila horizontal (modo angosto) con «lo que falta»
│   ├── TodasLasCuentasSheet.tsx      # renglones con lo que falta + entregar/cobrar
│   ├── PlatformPicker.tsx            # canales compactos, sin renglones de texto
│   ├── POSPage.tsx                   # disposición, entregar (mutación) y cobrar desde la cuenta
│   └── useEntregarCuenta.ts          # mutación de entregar con invalidación y mensaje de error
├── features/caja/turno.tsx           # usa el mismo texto de «lo que falta»
└── features/notificaciones/FranjaDeAgotado.tsx  # se coloca debajo de la franja de entrantes
web/e2e/vender-cuentas.spec.ts        # medidas a 1024×600/1280×854 y entregar/cobrar
```

**Structure Decision**: web application; solo `web/`.

## Complexity Tracking

Sin violaciones.
