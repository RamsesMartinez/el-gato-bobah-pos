# Feature Specification: Menú lateral en dos niveles

**Feature Branch**: `085-menu-en-dos-niveles`

**Created**: 2026-11-14

**Status**: Maqueta elegida (2026-11-14): opción C ajustada

**Input**: Issue #293. Relacionados: #290 (Caja), #291 (notificaciones de inventario y «Se acabó»,
otra sesión, worktree `egb-084`).

## Contexto

El riel lateral tiene 17 opciones al mismo nivel. En el equipo del mostrador (1280×854, el único que
opera casi todo el día) obliga a desplazar, deja botones de 44 px con rótulos de 10 px y no tiene
dónde poner un acceso rápido como «Se acabó» (#291).

Uso medido del 12-sep al 13-nov de 2026 (telemetría propia del POS, anónima):

| Uso | Opciones |
|---|---|
| A diario (≥ 60 de 62 días) | Vender, Pedidos, Caja, Ventas |
| Semanal | Gastos, Asistencia, Menú |
| Por rachas | Almacén, Se echó a perder, Plataformas |
| Casi no se usan | Reportes, Empleados, Negocio, Impresión, Mi cuenta |
| Sin medir | Interfaz e Integraciones |

Bajo uso no es prescindible: el cierre de caja es crítico aunque se haga una vez por turno, y
Plataformas, Negocio y Empleados son configuración.

## Decisiones ya tomadas (issue #293 y el encargo, no se rediscuten)

| Tema | Decisión |
|---|---|
| Rutas, nombres internos y permisos | No cambian. Solo cambia cómo se presenta el menú |
| Visibilidad | Por permiso, nunca por nombre de rol |
| Resolución de diseño y pruebas | 1280×854 (constitución 1.29.0, la enmienda la trae la sesión del #291) y 2K (2560×1440) |
| Pie del riel | Se queda como está (tema rápido, nombre de quien entró, Salir, versión) y deja lugar para la campana de notificaciones del #291 |
| «Se acabó» | Es del #291. Este menú le reserva su lugar dentro de Inventario (no es acceso rápido); se conecta si su ruta ya está en develop |

### Primer nivel y subopciones (decidido por el dueño en las maquetas, 2026-11-14)

Vender y Pedidos son los únicos con protagonismo: van primero, a un toque, separados del resto por
una línea. Los demás son módulos que, al tocarlos, abren un panel con sus destinos.

| Módulo | Destinos del panel (ruta que ya existe) |
|---|---|
| Vender (`/pos`) | — (abre directo) |
| Pedidos (`/pedidos`) | — (abre directo) |
| Caja | Caja (`/caja`), Ventas (`/ventas`), Gastos (`/gastos`) |
| Inventario | Almacén (`/almacen`), Se echó a perder (`/merma`), Se acabó (#291) |
| Menú | Menú (`/catalogo`), Plataformas (`/plataformas`) |
| Personal | Asistencia (`/asistencia`), Empleados (`/empleados`) |
| Reportes (`/reportes`) | — (abre directo) |
| Ajustes | Negocio (`/negocio`), Impresión (`/impresion`), Integraciones (`/integraciones`), Interfaz (`/apariencia`), Mi cuenta (`/cuenta`) |

Un módulo con un solo destino visible para esa persona abre ese destino directo, sin panel.

Lo que ve cada rol sugerido con sus permisos de hoy (medido en `system_role_permissions`, 0108):

| Rol | Módulos | Subopciones visibles |
|---|---|---|
| Admin | Los 8 | Todas |
| Gerente | Los 8 | Todas menos Empleados e Integraciones |
| Cajero | Vender, Pedidos, Caja (directo), Inventario, Personal*, Ajustes | Se echó a perder (y Se acabó); Asistencia*; Interfaz, Mi cuenta |
| Mesero | Vender, Pedidos, Inventario, Personal*, Ajustes | Se echó a perder (y Se acabó); Asistencia*; Interfaz, Mi cuenta |

\* Personal y Asistencia solo con la asistencia encendida en la empresa.

## Dudas del issue: lo que decidió el dueño (2026-11-14, en las maquetas)

| Duda | Decisión |
|---|---|
| Accesos rápidos además de «Se acabó» | Ninguno. «Se acabó» tampoco es acceso rápido: vive dentro de Inventario. Solo Vender y Pedidos tienen protagonismo |
| Vista de «todas las opciones» para el admin | No hace falta pantalla aparte: en pantalla ancha el menú se muestra expandido, filtrado por permiso |
| Duración de la transición y cómo volver | Sin interruptor ni periodo con el menú anterior. Se quita el criterio de aceptación «forma documentada de volver al menú anterior»; si algo falla, se corrige hacia adelante |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - El cajero encuentra lo de todos los días sin desplazar (Priority: P1)

En el equipo del mostrador, el cajero ve en el riel seis botones grandes. Vender y Pedidos van
arriba, a un toque, separados del resto. Toca Caja y, como Caja es su único destino ahí, abre su caja
sin panel.

**Why this priority**: es el uso diario de todo el local y el mayor riesgo es la memoria muscular.

**Independent Test**: entrar como cajero a 1280×854, contar los botones del primer nivel, llegar a
Vender, Pedidos y Caja en un toque cada uno, sin desplazar el riel.

**Acceptance Scenarios**:

1. **Given** un cajero con los permisos sugeridos y la asistencia encendida, **When** entra al POS,
   **Then** el primer nivel muestra Vender, Pedidos, Caja, Inventario, Personal y Ajustes, en ese
   orden, y ningún otro.
2. **Given** el cajero en cualquier pantalla, **When** toca Pedidos, **Then** abre Pedidos sin panel.
3. **Given** cualquier rol a 1280×854, **When** se pinta el riel, **Then** los botones del primer
   nivel caben sin desplazar y el pie del riel queda completo.

---

### User Story 2 - Quien administra llega a cada pantalla de configuración (Priority: P1)

El gerente de otra fonda, sin que nadie le explique, encuentra Gastos (dentro de Caja), Asistencia
(dentro de Personal) y los ajustes del negocio (dentro de Ajustes).

**Why this priority**: agrupar esconde; si alguien no encuentra una pantalla, la reorganización
empeora el producto.

**Independent Test**: un usuario nuevo (agente sin contexto) como gerente, a 1280×854 y a 2K,
encuentra Pedidos, Gastos, Asistencia y Ajustes sin ayuda.

**Acceptance Scenarios**:

1. **Given** el gerente, **When** toca un módulo con más de un destino, **Then** se abre su panel con
   todos los destinos a los que tiene permiso, con su nombre de siempre; al elegir uno se abre esa
   pantalla y el panel se cierra; tocar fuera o Escape lo cierra sin navegar.
2. **Given** una pantalla abierta que vive en un módulo (por ejemplo Gastos), **When** se pinta el
   riel, **Then** el módulo que la contiene se ve activo y la subopción marcada.
3. **Given** una pantalla ancha (2K), **When** se pinta el menú, **Then** se ven los módulos con sus
   subopciones sin tocar nada (vista expandida).

---

### User Story 3 - Nada que ya funcionaba se rompe (Priority: P1)

Cada ruta, enlace interno y redirección de hoy sigue abriendo la misma pantalla: `/productos` y
`/opciones` llevan al catálogo, `/almacen?tab=conteo`, `/negocio#lugares`, `/merma`, `/caja`,
`/caja/administrar` y el aterrizaje en `/pos`.

**Why this priority**: es la regla 1 del issue; un enlace roto en el mostrador es un turno detenido.

**Independent Test**: abrir cada ruta por URL directa con cada rol que puede abrirla y ver su
pantalla; con un rol que no puede, ver lo mismo que hoy.

**Acceptance Scenarios**:

1. **Given** cualquier ruta de la tabla de módulos, **When** se abre por URL directa, **Then** abre la
   misma pantalla que antes del cambio.
2. **Given** una ruta vieja con redirección, **When** se abre, **Then** redirige igual que hoy.

---

### User Story 4 - La medición dice qué se usa del menú nuevo (Priority: P2)

Quien vende el sistema ve en la consola de plataforma cuántas veces se abre cada pantalla, incluidas
Interfaz e Integraciones.

**Why this priority**: con el menú nuevo, la medición es lo que dice si algo quedó escondido.

**Independent Test**: abrir Interfaz e Integraciones; ver los conteos en la consola.

**Acceptance Scenarios**:

1. **Given** una sesión con permiso, **When** abre Interfaz o Integraciones, **Then** la apertura
   queda contada y no descartada.

### Edge Cases

- **Módulo sin ningún destino visible** (por ejemplo Personal con la asistencia apagada para un
  cajero): el módulo no aparece.
- **Módulo con un solo destino visible** (Caja para el cajero; Inventario para el mesero mientras no
  exista «Se acabó»): al tocarlo abre ese destino directo, sin panel.
- **Rol propio con un permiso suelto** (por ejemplo solo «ver ventas» sin «operar caja»): Caja
  aparece con Ventas como su único destino; nunca ofrece una pantalla que respondería 403.
- **Aviso de mermas por aprobar**: el número que hoy va en «Se echó a perder» sigue visible en el
  primer nivel (sobre Inventario) y, dentro del panel, sobre «Se echó a perder».
- **Panel abierto y la persona toca otro módulo**: se cierra uno y se abre el otro (o navega, si el
  otro abre directo).
- **Panel abierto cerca del pie**: se acomoda para no salirse de la pantalla (Ajustes, al fondo).
- **Ventana más baja que 854 px** (1280×768, barra del navegador): la lista de módulos desplaza como
  hoy, con el pie fijo y el aviso de «hay más»; Salir y el módulo activo quedan a la vista.
- **Aviso de versión nueva**: sigue en el lugar del logo, sin quitar alto al menú.
- **Permisos que cambian a media sesión** (se recargan al volver al frente): el menú se repinta con
  lo nuevo sin cerrar sesión.
- **Pantalla abierta que no está en ningún módulo** (por ejemplo `/plataformas/3/emparejar`): se
  marca el módulo de su pantalla madre (Menú › Plataformas).
- **Teclado y lector de pantalla**: cada módulo es un botón con nombre; el submenú se abre con
  Enter/Espacio, se recorre con Tab y se cierra con Escape; nada depende de arrastrar.
- **«Se acabó» todavía no está en develop**: su renglón no se pinta (no hay ruta a la cual llevar) y
  se reporta como pendiente de conectar.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: El riel MUST mostrar, en este orden: Vender y Pedidos (directos, separados del resto por
  una línea), y los módulos Caja, Inventario, Menú, Personal, Reportes y Ajustes, con los destinos de
  la tabla de arriba.
- **FR-002**: Cada botón del primer nivel MUST medir al menos 72 px de alto y cada renglón del panel
  al menos 52 px.
- **FR-003**: Un destino MUST verse solo si la persona puede abrir su pantalla (mismo criterio por
  permiso que hoy); un módulo MUST verse solo si tiene al menos un destino visible. Ningún control
  pregunta por el nombre de un rol.
- **FR-004**: Asistencia MUST seguir apareciendo solo con la asistencia encendida en la empresa.
- **FR-005**: A 1280×854 el primer nivel y el pie del riel MUST caber sin desplazar para cualquier
  rol, con lugar reservado para la campana del #291 en el pie.
- **FR-006**: Tocar un módulo con más de un destino visible MUST abrir su panel junto al riel; elegir
  un destino MUST abrir esa pantalla y cerrar el panel; tocar fuera o Escape MUST cerrarlo sin
  navegar. Con un solo destino visible, tocarlo MUST abrir ese destino directo.
- **FR-007**: El módulo que contiene la pantalla abierta MUST verse activo, y dentro del panel el
  destino abierto marcado.
- **FR-008**: En pantalla ancha y alta (≥ 1920×1300; medido: a 1920×1080 no cabe) el menú MUST mostrarse expandido, con los
  módulos y sus destinos visibles sin tocar nada.
- **FR-009**: Ninguna ruta, redirección, nombre de pantalla ni permiso MUST cambiar; cada ruta de hoy
  MUST abrir por URL directa la misma pantalla.
- **FR-010**: Menú e Inventario MUST quedar contiguos en el primer nivel (las recetas descuentan del
  almacén).
- **FR-011**: El número de mermas por aprobar MUST seguir visible en el primer nivel.
- **FR-012**: Interfaz e Integraciones MUST medirse como pantallas.
- **FR-013**: El menú MUST poder usarse con teclado y lector de pantalla y no depender de arrastrar.
- **FR-014**: El pie del riel (tema rápido, nombre, Salir, versión) MUST quedar igual.

### Key Entities

- **Módulo**: botón del primer nivel; tiene nombre, ícono y destinos (uno o varios).
- **Destino**: una pantalla que ya existe, con su ruta y el permiso con que se abre hoy.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A 1280×854, con cualquiera de los cuatro roles sugeridos, el riel no desplaza y todos
  sus módulos y el pie se ven completos.
- **SC-002**: Un usuario nuevo como cajero y como gerente de otra fonda encuentra Pedidos, Gastos,
  Asistencia y Ajustes sin ayuda, a 1280×854 y a 2K.
- **SC-003**: El 100 % de las rutas de hoy abre por URL directa la misma pantalla para cada rol que
  puede abrirla.
- **SC-004**: Lo que ve cada rol sugerido coincide con la tabla de esta spec, verificado por prueba.
- **SC-005**: Vender y Pedidos quedan a un toque desde cualquier pantalla.
- **SC-006**: Interfaz e Integraciones aparecen con conteo en la consola de plataforma en lugar de
  descartarse.

## Assumptions

- La resolución mínima es 1280×854 con la constitución 1.29.0 de la sesión del #291; esta rama no
  enmienda la constitución y no se mergea a develop antes que esa enmienda.
- Vender, Pedidos y Reportes abren directo. Caja y Menú tienen pantalla propia, que es el primer
  renglón de su panel.
- No hay periodo con el menú anterior: el dueño quitó ese criterio (2026-11-14).

## Fuera de alcance

- Cambiar rutas, nombres internos o permisos.
- Rediseñar cada pantalla (Caja es el #290).
- La pantalla y el permiso de «Se acabó» (#291).
- La campana de notificaciones (#291); aquí solo se le deja lugar.
