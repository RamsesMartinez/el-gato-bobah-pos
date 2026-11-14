# Research: Menú lateral en dos niveles

## R1. Dónde vive la estructura del menú
- **Decision**: un módulo puro `web/src/app/nav.ts` (reescrito) con los módulos y una función `visibleModules(user, { attendanceEnabled })`.
- **Rationale**: lo que se prueba por rol es una función sin DOM; `AppShell` solo pinta. Es el mismo reparto que `routes.ts` (`canOpen`).
- **Alternatives**: filtrar dentro del componente (como hoy). Se descarta: la prueba por rol tendría que montar el shell entero cuatro veces.

## R2. Qué pasa al tocar un módulo sin pantalla propia
- **Decision**: con una subopción visible, abre esa subopción; con varias, muestra las subopciones (la forma la decide la maqueta).
- **Rationale**: el cajero ve Inventario con una sola subopción; hacerlo tocar dos veces es un toque de más en cada turno.
- **Alternatives**: una pantalla índice por módulo. Se descarta: sería una ruta nueva y el issue prohíbe cambiar rutas.

## R3. Cómo se vuelve al menú anterior
- **Decision**: no se vuelve. El dueño descartó el interruptor y quitó el criterio (2026-11-14).
- **Alternatives**: interruptor por equipo en Interfaz (descartado por el dueño); runbook de volver a subir la versión anterior (descartado).

## R4. Interfaz e Integraciones en la medición
- **Decision**: `apariencia` (sin permiso) e `integraciones` (`integrations.manage`) en `pantallasMedibles` y `permissionByScreen`; `/apariencia` y `/integraciones` en `rutas-medidas.ts`; etiquetas en la consola.
- **Rationale**: el issue pide medirlas ya; sin el permiso en `permissionByScreen` un mesero con token propio inflaría Integraciones.
- **Tolerancia a la versión anterior**: la API vieja las descarta (`usage_descartado`) durante el traslape; no rompe nada.

## R5. Accesos rápidos
- **Decision**: ninguno (decisión del dueño): solo Vender y Pedidos tienen protagonismo, como botones directos del primer nivel. Sin acción nueva en la medición.

## R6. Resolución
- **Decision**: diseño y pruebas a 1280×854 y 2560×1440. La enmienda (constitución 1.29.0) la trae la sesión del #291; esta rama no la toca y no se mergea antes.
