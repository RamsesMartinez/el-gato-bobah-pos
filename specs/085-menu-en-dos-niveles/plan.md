# Implementation Plan: Menú lateral en dos niveles

**Branch**: `085-menu-en-dos-niveles` | **Date**: 2026-11-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/085-menu-en-dos-niveles/spec.md`

## Summary

El riel de 17 opciones planas pasa a un primer nivel de hasta 8 botones: Vender y Pedidos directos y
seis módulos que abren un panel con sus destinos (maqueta C ajustada, elegida por el dueño). Cambia
solo la presentación: las rutas, las redirecciones, los nombres de pantalla y los permisos se quedan
como están. La estructura del menú vive en un módulo puro (`web/src/app/nav.ts`, reescrito) que, dados los
permisos de la sesión y si la asistencia está encendida, devuelve qué módulos y subopciones se ven;
ese módulo es lo que prueban los tests por rol. Sin interruptor ni periodo con el menú anterior
(decisión del dueño). En el servidor solo crece la lista blanca de la medición (Interfaz e
Integraciones).

## Technical Context

**Language/Version**: TypeScript 5 / React 19 (web); Go 1.27.4 (una lista en `domain`)

**Primary Dependencies**: Chakra UI v3, react-router 7, Zustand (persist), TanStack Query

**Storage**: Ninguna

**Testing**: vitest + Testing Library (unitarios del menú y del shell), Playwright a 1280×854 y
2560×1440 (e2e), `go test` para la lista de medición

**Target Platform**: el equipo del mostrador de 1280×854 (constitución 1.29.0) y pantallas 2K

**Project Type**: web (POS) con API Go

**Performance Goals**: el riel se pinta sin peticiones nuevas (reusa la del roster de asistencia y la
de mermas por aprobar que ya hace)

**Constraints**: primer nivel ≥ 72 px por botón, renglones del panel ≥ 52 px, sin desplazar a
1280×854 con el pie completo y un lugar de 44 px para la campana del #291 (medido en la maqueta: el
último botón del admin termina a 829 de 854 px)

**Scale/Scope**: 8 botones, 18 destinos, 4 roles sugeridos más roles propios

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Layering | Servidor: solo `domain/uso.go` (lista blanca pura). Front: lógica pura en `nav.ts`, pintado en `AppShell` |
| II. Errores | Sin rutas nuevas ni errores nuevos |
| III. Dinero | No toca dinero |
| IV. TDD | `nav.test.ts` por rol antes del código; `uso_test.go` para las pantallas nuevas; e2e de rutas por URL directa |
| IV. Migraciones | Ninguna. La lista blanca nueva tolera a la API anterior: la versión vieja descarta `apariencia` e `integraciones` como hoy (solo deja `usage_descartado` durante el traslape) |
| V. Seguridad | Ninguna barrera cambia; el menú es espejo de `ROUTE_PERMISSIONS` y se filtra con `can()`/`canOpen()`. `permissionByScreen` gana `integraciones → integrations.manage` para que un rol sin el permiso no infle el conteo |
| VI. YAGNI | Sin config remota del menú, sin interruptor, sin accesos rápidos (decisión del dueño) |
| VII. Comentarios / idioma | Identificadores nuevos en inglés (`MENU_MODULES`, `visibleModules`, `moduleOf`, `MenuRail`); texto de pantalla en español |
| VIII. Puertas | Ninguna puerta se toca: no hay esquema |
| Restricciones de producto | Controles ≥ 44 px (primer nivel 72 px), sin `<select>`, un solo estilo (tokens de `theme/ui.ts`), texto para quien opera |
| Visibilidad por permiso | Nada pregunta por nombre de rol; lo vigilan `sin-nombre-de-rol.test.ts` y `TestNoRouteIsGatedByRoleName` |

Sin violaciones; la tabla de complejidad no aplica.

## Diseño

### Dónde aterriza cada pieza

| Pieza | Capa | Archivo |
|---|---|---|
| Módulos, destinos, orden y regla de visibilidad | front, puro | `web/src/app/nav.ts` (se reescribe: de lista plana a módulos) |
| Riel con panel al tocar y vista expandida en pantalla ancha | front, componente | `web/src/app/MenuRail.tsx` (nuevo), montado por `AppShell.tsx` en el lugar de la lista de hoy |
| Pantallas medibles nuevas | servidor, dominio | `server/internal/domain/uso.go` (`apariencia`, `integraciones`) |
| Mismo nombre en el front y la consola | front | `web/src/app/rutas-medidas.ts`, `web/src/consola/etiquetas-de-uso.ts` |

### Regla de visibilidad (`visibleModules`)

1. Un destino se ve si `canOpen(ruta)` y, si declara permiso propio, `can(permiso)`; Asistencia
   además exige la asistencia encendida. Es exactamente el filtro de hoy en `AppShell`.
2. Un módulo se ve si tiene al menos un destino visible. Caja y Menú tienen su pantalla propia como
   primer destino; Vender, Pedidos y Reportes son módulos de un solo destino.
3. Con un solo destino visible, tocar el módulo navega a él; con varios, abre el panel.
4. `moduleOf(pathname)` dice qué módulo y qué destino están activos (por prefijo, para
   `/catalogo/productos`, `/plataformas/3/emparejar` y `/caja/administrar`).

### Panel (C ajustada)

- Botón de módulo de 96 px de ancho y 72 px de alto, ícono de 26 px y rótulo de 12 px; los que abren
  panel llevan un chevron y `aria-expanded`.
- Línea divisoria después de Pedidos.
- Panel de 240 px junto al riel, sobre el área de trabajo con un velo que lo cierra al tocar; título
  del módulo y renglones de 52 px (`role="menu"`/`menuitem`, foco al primer renglón, flechas y
  Escape). Se alinea con su botón y, si no cabe abajo, se ancla al fondo.
- A ≥ 1920 px de ancho el riel se expande (260 px): módulos con sus destinos en lista, sin panel.
- El riel pierde su `overflowY` con desplazamiento: a 1280×854 todo cabe (el piso de la constitución
  1.29.0). Debajo de ese alto conserva el desplazamiento como red, no como diseño.

### Lo que no cambia

- `App.tsx` (rutas y redirecciones), `routes.ts` (`ROUTE_PERMISSIONS`), `RequirePermission`.
- El pie del riel: `TemaRapido`, nombre, Salir, `SystemInfo`, y `UpdateRailButton` en el lugar del
  logo. Se deja un hueco de 44 px declarado para la campana del #291 (no se pinta nada en él hasta
  que exista).
- El número de mermas por aprobar: va sobre Inventario en el primer nivel y sobre «Se echó a perder»
  dentro del panel.

### «Se acabó» (#291)

Su renglón se declara en `nav.ts` (dentro de Inventario, sin acceso rápido) con su ruta y su permiso
solo cuando esa ruta exista en develop. Si al mergear no existe, no se declara (sería un enlace roto)
y se reporta pendiente: es un renglón en `nav.ts`.

## Pantallas y maquetas

Una sola pieza cambia de disposición: el riel, presente en todas las pantallas del POS.

| Pantalla | Maquetas | Elegida | Por qué |
|----------|----------|---------|---------|
| Riel lateral (primer nivel, panel, vista ancha) | [Menú en dos niveles](https://claude.ai/artifact/KeVmDvNgxhnCS5kFXb3UdN) (versión 2) | C ajustada | El dueño eligió C y la ajustó: Vender y Pedidos son los únicos con protagonismo; sin accesos rápidos ni interruptor |

## Project Structure

### Documentation (this feature)

```text
specs/085-menu-en-dos-niveles/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/menu.md
└── tasks.md
```

### Source Code

```text
web/src/app/
├── nav.ts             # reescrito: módulos, destinos, visibleModules, moduleOf
├── nav.test.ts        # lo que ve cada rol; Menú e Inventario contiguos
├── MenuRail.tsx       # nuevo: riel con panel y vista ancha
├── MenuRail.test.tsx  # nuevo: panel, teclado, destino único
├── AppShell.tsx       # monta MenuRail
├── railScroll.ts      # se borra si el riel deja de desplazar
├── rutas-medidas.ts
web/src/consola/etiquetas-de-uso.ts
web/e2e/                 # cabe-en-la-tableta, aviso-de-version, enlaces Asistencia/Almacén, menu-por-rol (nuevo)
server/internal/domain/uso.go, uso_test.go
docs/mecanica/medicion-de-uso.md   # Interfaz e Integraciones medidas
```

**Structure Decision**: web + una lista del dominio Go; sin migración ni endpoint nuevo.
