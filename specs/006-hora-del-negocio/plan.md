# Implementation Plan: La hora del negocio manda

**Branch**: `006-hora-del-negocio` | **Date**: 2026-09-01 | **Spec**: [spec.md](./spec.md)

**Input**: [spec.md](./spec.md)

## Summary

El negocio ya tiene su zona horaria guardada, validada y viajando al frontend. **Ninguna de las doce
pantallas que muestran una hora la usa**: todas dicen la hora del navegador de esa tableta, y eso
incluye el ticket que se lleva el cliente. Del mismo desfase sale que "Entregados hoy" se vacíe a las
18:00 locales, porque filtra por el día del servidor, que corre en UTC.

Cuatro piezas, y la primera es la que hace imposible que el problema vuelva:

1. **Un solo lugar convierte a la zona del negocio**, y de él cuelgan las doce pantallas y los dos
   papeles. Doce formateos sueltos es exactamente cómo esto se desincronizó la primera vez.
2. **Los pedidos activos se ven siempre**, sin filtro de fecha, marcados con su día para que el
   rezago se distinga del trabajo de hoy.
3. **"Entregados hoy" se vacía cuando el negocio dice**: medianoche del local por default,
   configurable a turno o a cierre de caja.
4. **La fecha de negocio deja de caer a UTC** cuando no se puede leer la zona. Es lo que dejó dos
   pedidos en el arqueo equivocado en la cuenta de pruebas.

## Technical Context

**Language/Version**: Go 1.27 (backend), TypeScript / React 19 (frontend)

**Primary Dependencies**: chi, pgx + sqlc, goose (backend); Chakra UI v3, TanStack Query (frontend).
La conversión de zona la hace la API de internacionalización del navegador — **sin librería nueva**.

**Storage**: PostgreSQL con RLS por empresa. Una columna en `business_settings`. **Ninguna migración
de datos**: se midió que el negocio real tiene sus 21 pedidos con la fecha correcta.

**Testing**: `go test` (unitarios en `domain` para el cálculo del corte, integración con **dos
empresas**), vitest

**Target Platform**: tabletas de 7 a 10 pulgadas, ~1024×600

**Project Type**: aplicación web, monorepo

**Performance Goals**: ninguna nueva. Formatear ~20 fechas por pantalla con zona es lo que la API del
navegador hace por diseño.

**Constraints**: ≥44 px; prohibido `<select>` nativo. **La primera hora que se pinta ya tiene que ser
la correcta**: una hora que se corrige sola enseña al operador a no confiar en lo que lee.

**Scale/Scope**: 2 empresas, 2 estaciones, 12 sitios de formateo, 11 pedidos de rezago. Ninguna
pantalla nueva: cambian las que ya muestran horas, más un ajuste en la de Negocio.

## Constitution Check

*GATE: revisado antes de Fase 0 y de nuevo tras el diseño de Fase 1.*

| Principio | Cómo lo cumple este plan |
| --- | --- |
| **I. Layering estricto** | El cálculo del corte según el modo va a `domain` como función pura sobre (modo, instante, zona, turno). El servicio orquesta; el handler mapea. SQL solo por sqlc |
| **II. Errores envueltos** | Un `corteDeVista` fuera de los tres valores es `domain.ErrValidation` envuelto con `%w`; el mapeo sigue viviendo solo en `httpapi.Error` |
| **III. Dinero** | **La feature no toca dinero**, y esa es su restricción más importante: cambia qué se muestra, no en qué día cae una venta. `business_date` no se escribe en ningún lado nuevo, y el quickstart compara las cifras de un arqueo cerrado antes y después |
| **IV. Test-first, bordes primero** | Los bordes están en el spec antes que el código. El cálculo del corte —incluido el día del cambio de horario en Tijuana— es unitario en `domain`; las dos consultas y la columna son de integración con dos empresas |
| **V. Seguridad adversarial** | Detalle abajo |
| **VI. YAGNI** | Sin librería de fechas: la API del navegador alcanza. Sin tabla nueva. Sin zona por usuario ni por sucursal. Los tres modos de corte se construyen a pedido explícito del dueño y su costo queda anotado |
| **VII. Comentarios del porqué** | Por qué la medianoche se calcula en la zona y no restando 24 horas, por qué el instante viaja en UTC y se formatea en la vista, y por qué el fallback dejó de ser UTC |

### Principio V, en detalle

| Pregunta adversarial | Respuesta concreta |
| --- | --- |
| ¿Puedo ver los pedidos de otra empresa al quitar el filtro de fecha? | No. RLS acota `orders` por empresa y el test de integración corre con dos |
| ¿Una zona inválida tumba la pantalla? | No: cae al default del producto, sigue funcionando y deja constancia |
| ¿Puedo mover dinero de un arqueo a otro cambiando la zona? | No. `business_date` ya está escrito en cada fila y esta feature no lo reescribe. El quickstart lo verifica leyendo un arqueo cerrado antes y después |
| ¿Puedo poner un `corteDeVista` que no existe? | No: valor conocido o `ErrValidation`. El default es para el campo ausente, nunca para el presente y malformado |
| ¿Se filtra algo por el campo nuevo? | No lleva datos de nadie: es una preferencia de pantalla del negocio |

**Sin violaciones que justificar.**

## Project Structure

### Documentation (this feature)

```text
specs/006-hora-del-negocio/
├── plan.md              # Este archivo
├── research.md          # Fase 0 — ocho hallazgos, todos medidos
├── data-model.md        # Fase 1 — una columna, cero migraciones de datos
├── quickstart.md        # Fase 1 — un recorrido por historia, con su fallo esperado
├── contracts/api.md     # Fase 1 — un campo nuevo, dos endpoints que cambian
├── checklists/
└── tasks.md             # Lo crea /speckit-tasks
```

### Source Code

```text
server/
├── migrations/
│   └── 00NN_corte_de_vista.sql              # una columna en business_settings
├── queries/
│   └── orders.sql                            # en curso sin fecha; entregados desde el corte
├── internal/
│   ├── domain/
│   │   ├── businessdate.go                   # desdeCuandoSeVen(modo, ahora, zona, turno)
│   │   └── businessdate_test.go              # incluye el día del cambio de horario
│   ├── app/
│   │   ├── orders.go                         # Open sin fecha; DeliveredToday con el corte
│   │   ├── backoffice.go                     # el fallback deja de ser UTC
│   │   └── settings.go                       # el ajuste nuevo
│   └── integration/
│       ├── corte_de_vista_test.go            # con DOS empresas
│       └── pedidos_viejos_se_ven_test.go

web/src/
├── hooks/
│   └── useHoraDelNegocio.ts                  # EL único lugar que convierte
├── utils/
│   ├── format.ts                             # el formateo cuelga de la zona
│   ├── printReceipt.ts                       # el papel también
│   └── printKitchen.ts
└── features/
    ├── admin/BusinessSettingsPage.tsx        # el ajuste + el aviso al cambiar de zona
    ├── backoffice/ · sales/ · orders/        # dejan de formatear por su cuenta
    └── pos/PedidosEnCurso.tsx                # marca los de días anteriores
```

**Structure Decision**: monorepo existente. Un archivo nuevo en `web/src/hooks/` y una migración.
Todo lo demás es sustituir doce formateos por uno.

## Fases

### Fase 0 — Investigación ✅

Ver [research.md](./research.md). Ocho hallazgos. Los tres que cambian el plan:

- **El histórico está limpio.** El dueño autorizó reconstruir desde respaldos y **no hace falta**:
  se midió antes de usar el permiso. Los dos pedidos mal fechados son de la cuenta de pruebas.
- **La causa de esos dos sigue viva**: la fecha de negocio cae a UTC cuando no se puede leer la zona,
  en vez de caer al default del producto. Eso sí se corrige.
- **El horario de verano no murió del todo**: `America/Tijuana` está en la lista que el producto
  ofrece y sí cambia. La medianoche se calcula en la zona, nunca restando 24 horas.

### Fase 1 — Diseño ✅

- [data-model.md](./data-model.md) — una columna, y el costo honesto de los dos modos que hoy nadie usa
- [contracts/api.md](./contracts/api.md) — por qué el instante viaja en UTC y se formatea en la vista
- [quickstart.md](./quickstart.md) — arranca cambiando la zona del sistema operativo, o pasa en falso

### Fase 2 — Tareas

La genera `/speckit-tasks`. El orden que sugiere el diseño:

1. **El fallback de la fecha de negocio.** Es un defecto vivo que mueve dinero de día; no depende de
   nada más y va primero.
2. **US1, la hora en toda la interfaz.** Es la causa raíz y la que entrega valor sola.
3. **US2, los pedidos activos siempre visibles.** Independiente de la 1.
4. **US3, el corte configurable.** Necesita la columna.
5. **US4, el aviso al cambiar de zona.** P3, encima de la 1.

## Riesgos, y qué hace el plan con cada uno

| Riesgo | Qué haría | Cómo se cierra |
| --- | --- | --- |
| Aplicar la zona dos veces | Una hora corrida seis horas de más se ve plausible y nadie la reporta | El instante viaja en UTC y se convierte en un solo lugar. El quickstart compara pantallas entre sí, no contra una expectativa |
| Un formateo suelto que no se migra | Vuelve el problema por una pantalla | El helper único, y un test que falle si alguien vuelve a llamar a `toLocaleString` sin zona |
| La hora parpadea al cargar | El operador ve la hora cambiar y deja de confiar en ella | No se pinta hora hasta conocer la zona. Es SC-003 |
| Quitar el filtro de fecha trae once pedidos de julio | La barra se llena de rezago | Es el objetivo. Se distinguen a la vista y la barra ya scrollea sola desde la feature 005 |
| Restar 24 horas para el corte | Se desfasa el día del cambio de horario en Tijuana | Se calcula la medianoche de esa fecha en esa zona, y el unitario cubre ese día |
| Usar el permiso de tocar históricos sin necesidad | Se movería dinero que hoy está bien puesto | Se midió primero: 21 de 21 correctos en el negocio real. Ninguna migración de datos |

## Complexity Tracking

Sin violaciones a la constitución. Nada que justificar.
