# Research: aviso de versión nueva (064)

## Lo que hay hoy (medido en el código, 2026-11-05)

- `web/src/shared/pwa/registerPwa.ts` → `onNeedRefresh` hace dos cosas: prende
  `useAppUpdate.needRefresh` (store de zustand) y crea un toast «Nueva versión disponible» con
  `duration: ∞` y acción «Actualizar».
- El toaster (`components/ui/toaster.tsx`) va arriba al centro (`placement: "top"`), con
  `pointer-events: none` en el cuerpo y `auto` en sus botones. El cuerpo deja pasar el toque, pero
  **se pinta encima** del control y su botón «Actualizar» sí captura: el control queda tapado a la
  vista y, en la franja del botón, también al dedo. Es el caso del issue (Negocio, «Agregar lugar»).
- `app/SystemInfo.tsx` (pie del riel) ya lee `needRefresh`: un punto verde de 6 px sobre la
  versión, y «Actualizar ahora» dentro del diálogo «Detalles del sistema». Son dos toques y el punto
  no se ve.
- El riel mide 76 px de ancho. De arriba abajo: logo (44 px + 8 de margen), lista de opciones con
  scroll (`flex: 1`), flecha de «hay más» (16 px), y el pie (nombre, «Salir» 44 px, versión).

## Decisión 1 — dónde vive el aviso

**Decisión**: en el riel lateral, que es marco fijo de todas las pantallas y no le pertenece a
ninguna. La forma exacta (qué renglón del riel ocupa) la elige `/maquetas`; las candidatas son
renglones del riel. Ver la sección «Maquetas» de plan.md.

**Rationale**: el riel es la única zona de la app que existe en todas las pantallas del marco y no
es área de trabajo; su ancho no cambia con el aviso, y el área de trabajo (columna derecha) no
pierde ni un píxel (FR-001, FR-002).

**Alternativas descartadas**:

- *Toast en otra esquina*: abajo a la derecha tapa «Cobrar» y «Enviar» (por eso se movió arriba);
  abajo a la izquierda tapa la lista de productos. Cualquier esquina flotante tapa algo en alguna
  pantalla.
- *Franja arriba del área de trabajo* (como la de mermas por aprobar): le quita ~44 px de alto a
  todas las pantallas mientras dure el aviso. Viola FR-002.
- *Solo el punto del pie, más grande*: sigue siendo dos toques (abre el diálogo) y FR-004 pide uno.

## Decisión 2 — el toast se quita

**Decisión**: `onNeedRefresh` deja de crear el toast; solo prende el store. FR-005.

**Rationale**: con el aviso fijo en el riel, el toast duplica el mismo hecho y es justo el que tapa.

## Decisión 3 — aviso de «Instalar app» fuera

**Decisión**: no se toca. Ver spec, *Decisiones de la sesión*.

## Decisión 4 — cómo se prueba sin un service worker de verdad

**Decisión**: el componente se prueba con vitest poniendo `needRefresh` en el store. La disposición
(nada tapado, mismo alto del área de trabajo) se mide con Playwright a 1024×600 contra `make start`
(Vite en modo desarrollo), donde un gancho **solo de desarrollo** en `registerPwa.ts`/`main.tsx`
expone `window.__egbSimularVersionNueva()` que prende el store como lo haría `onNeedRefresh`.

**Rationale**: en desarrollo no hay service worker (`registerSW` es no-op), y producir una versión
nueva de verdad exige dos builds servidos. El gancho vive bajo `import.meta.env.DEV`, así que Vite
lo elimina del build de producción; la vuelta final contra `bun run preview` comprueba además que
no existe ahí.

**Alternativas descartadas**: exponer el store en `window` también en producción (superficie sin
uso); simular con `page.route` un `sw.js` nuevo (frágil y lento, y prueba al plugin, no al aviso).

## Decisión 5 — el riel cambia de alto disponible

Si el aviso ocupa un renglón que hoy usa la lista de opciones, la lista se encoge y la opción activa
puede quedar fuera de la vista (FR-007). El efecto que la lleva a la vista corre hoy con
`[pathname, nav.length]`; tendría que correr también al cambiar `needRefresh`, y la flecha de «hay
más» tendría que volver a medirse. Si la maqueta elegida no le quita alto a la lista, este punto no
aplica.

**Resuelto con la maqueta A, ajustada tras la revisión de tableta**: el botón mide 64×44, el mismo
alto que el logo, así que la lista no pierde alto. Se agrega `needRefresh` a las dependencias del
efecto de la opción activa y de la medida de «hay más» como red de seguridad; no hace falta un
`ResizeObserver` (jsdom no lo trae y el único cambio de alto previsto ya no existe).
