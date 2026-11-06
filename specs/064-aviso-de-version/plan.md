# Implementation Plan: El aviso de versión nueva no tapa la pantalla

**Branch**: `064-aviso-de-version` | **Date**: 2026-11-05 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/064-aviso-de-version/spec.md`

## Summary

El aviso «Nueva versión disponible» deja de ser un toast flotante arriba al centro y pasa a un
renglón fijo del riel lateral, que existe en todas las pantallas del marco y no es área de trabajo.
Se quita el toast; el estado que ya existe (`useAppUpdate.needRefresh`) es la única fuente. Solo
frontend: sin servidor, sin base, sin migración. Detalle de las decisiones en
[research.md](research.md).

## Technical Context

**Language/Version**: TypeScript, React 19

**Primary Dependencies**: Chakra UI v3, zustand, vite-plugin-pwa (sin cambios de dependencias)

**Storage**: N/A (estado en memoria de la pestaña, como hoy)

**Testing**: vitest + Testing Library (componente), Playwright a 1024×600 contra `make start`
(disposición)

**Target Platform**: tabletas de 7 a 10 pulgadas, ~1024×600, Chromium

**Project Type**: web (solo `web/`)

**Performance Goals**: N/A

**Constraints**: cero píxeles de alto quitados al área de trabajo; control ≥ 44 px; no sacar de la
vista la opción activa ni «Salir»

**Scale/Scope**: un componente del riel, `registerPwa.ts`, `AppShell.tsx`, una e2e

## Constitution Check

| Principio | Aplica | Cómo se cumple |
|---|---|---|
| I. Layering | No (sin backend) | — |
| II. Errores | No | — |
| III. Dinero | No | — |
| IV. Test-first | Sí | Test de componente y e2e de disposición antes del cambio; los bordes (riel lleno, sin aviso, pantallas fuera del marco) están en el spec antes del código |
| IV. Migración tolera versión anterior | No hay migración | — |
| V. Seguridad | No | El gancho de simulación es solo de desarrollo y se verifica ausente en el build |
| VI. YAGNI | Sí | Se reusa el store existente; sin componente genérico de «avisos del riel» |
| VII. Comentarios / idioma | Sí | Identificadores nuevos en inglés; texto de pantalla en español para quien opera |
| VIII. Puertas | No toca esquema | Ninguna puerta en juego |
| Tableta 1024×600 | Sí | Es el objetivo de la feature; medido con Playwright |
| Un solo estilo | Sí | Botón y tokens del tema, como «Salir» |
| Texto para quien opera | Sí | «Actualizar», sin «service worker» ni «versión del frontend» |

Sin violaciones. Re-chequeo tras el diseño: igual.

## Pantallas y maquetas

Pantalla cambiada: **el riel lateral** (marco de todas las pantallas). Ninguna pantalla de trabajo
cambia de disposición.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Riel lateral con versión nueva | [maquetas-064](https://claude.ai/artifact/78sh66tXCtcvBb6TN9ZZ5A) | A · en lugar del logo | Se ve primero, un toque en 52 px, y le quita solo 8 px a la lista; B pone «recargar» junto a «Salir» y le quita ~60 px; C no se entiende como botón |

**Elegida por la sesión por delegación del dueño, 2026-11-05.** Un subagente puntuó las tres contra
1024×600, el spec y la constitución (1 a 5 en siete criterios): A 31, B 26, C 25.

### Cómo queda (opción A)

- **Sin versión nueva**: el riel es el de hoy, al píxel (FR-006).
- **Con versión nueva**: el logo (44 px + 8 de margen) se reemplaza por un botón verde de 64×44 px
  (`w="64px"`, `minH="44px"`, `px={0}`, icono de 18 px y rótulo de 10 px, como las opciones del riel),
  con icono de «subir de versión» (flecha hacia arriba en círculo, no la flecha circular de
  refrescar, que se lee como «recargar los datos de esta pantalla») y el rótulo «Actualizar». Se
  conservan los 8 px de margen con «Vender». Nombre accesible: «Actualizar a la versión nueva».
  Un toque llama `useAppUpdate.apply()`.
- **El toast de versión se quita** (FR-005). El punto verde de 6 px del pie también: sería una
  segunda señal del mismo hecho. «Actualizar ahora» dentro de «Detalles del sistema» se queda: no es
  un aviso, es el lugar donde se ve la versión.
- **La lista del riel no pierde alto**: el botón ocupa exactamente el alto del logo (44 + 8), así que
  con el aviso el riel mide lo mismo que sin él (revisión de tableta, MEDIUM 1: con rol admin a 600 px
  la lista tiene ~6 px de holgura y la maqueta, con 52 px, recortaba la 8.ª opción). Se mide en la
  e2e: con aviso, la lista mide lo mismo y la opción activa y «Salir» siguen completas. Aun así, el
  efecto que lleva la opción activa a la vista y la medida de la flecha «hay más» se vuelven a correr
  al cambiar `needRefresh`, por si una fuente o un zoom vuelven distinto el alto.
- **Distancia a «Vender»** (revisión de tableta, MEDIUM 2, aceptado): quedan los 8 px de margen que
  hoy tiene el logo, el doble de los 4 px que separan las opciones del riel entre sí. Más aire le
  quitaría alto a la lista. Un toque errado recarga pero no pierde la cuenta en captura (US2-3).
- Componentes: `Button` del tema (`colorPalette="green"`, radio del tema), icono de `react-icons/lu`
  como el resto del riel.

## Project Structure

### Documentation (this feature)

```text
specs/064-aviso-de-version/
├── plan.md
├── research.md
├── quickstart.md
├── checklists/requirements.md
└── tasks.md          # /speckit-tasks
```

Sin `data-model.md` ni `contracts/`: no hay datos guardados ni interfaz con otro sistema. El único
estado es `useAppUpdate` (`needRefresh: boolean`, `apply()`), que ya existe y no cambia de forma.

### Source Code (repository root)

```text
web/src/
├── shared/pwa/registerPwa.ts      # quita el toast de versión; gancho solo-dev para simular
├── stores/appUpdate.ts            # sin cambios de forma
├── app/AppShell.tsx               # logo o botón según needRefresh; re-medir el riel al cambiar
├── app/UpdateRailButton.tsx       # nuevo: el botón «Actualizar» en el lugar del logo
├── app/UpdateRailButton.test.tsx  # nuevo
└── app/SystemInfo.tsx             # se quita el punto verde; el bloque del diálogo se queda
web/e2e/
└── aviso-de-version.spec.ts       # nuevo: disposición a 1024×600 en Vender, Pedidos, Negocio y Caja
```

**Structure Decision**: solo `web/`, capa de presentación. El estado sigue en el store de zustand.

## Complexity Tracking

Sin violaciones que justificar.
