# Quickstart: validar la caja con asistencia

Todo en local, contra el `make start` de este worktree (puertos en `.env.worktree.local`), con una
base restaurada (`make db-restaurar`) y la API como `gatobobah_app`. Pantalla a 1024×600 con
`hasTouch`. Las capturas van a `test-results/`, nunca al repo.

## Preparación

1. Empresa `gatobobah` (filtrar por slug, no por id): Asistencia encendida en Configuración ›
   Asistencia.
2. Dos personas con PIN y «Lleva asistencia»: una con rol cajero (Beto) y una gerente (Gina). Una
   tercera sin «Lleva asistencia» (el dueño).
3. Ninguna caja abierta; sin entradas abiertas de esas personas.

## Escenarios

| # | Pasos | Resultado esperado | Spec |
|---|---|---|---|
| Q1 | Beto en sesión, caja cerrada, toca abrir | Antes de contar: «Para abrir la caja, primero marca tu entrada» y «Marcar entrada»; contar no está disponible | US1-2, FR-005 |
| Q2 | Marca entrada con su PIN desde la hoja | Entrada registrada; la hoja sigue a contar sin volver a empezar; la entrada aparece en Asistencia con su aparato | US1-3, FR-006 |
| Q3 | PIN equivocado 5 veces desde la hoja | Mismo bloqueo y mensaje que en la pantalla de Asistencia | Borde PIN |
| Q4 | Beto abre la caja | Se abre como hoy | US1-1 |
| Q5 | Con Beto aún dentro, Gina va a la caja abierta | Arriba, sin desplazar, el aviso «Beto abrió esta caja y no ha marcado su salida» con «Marcar salida de Beto» y «Cerrar sin la salida de Beto»; en el lugar del conteo, «Para cerrar la caja, primero resuelve el aviso de arriba sobre Beto»; no se ven la tabla del cierre, las terminales, el fondo, las notas ni «Cerrar caja» | US2-2, US3, FR-005 |
| Q6 | Beto marca su salida con su PIN desde la hoja | Salida registrada; el cierre sigue | US2-3 |
| Q7 | Reabrir con Beto (marcar entrada de nuevo); Beto se va sin marcar; Gina cierra con «Cerrar sin la salida de Beto» y motivo | Cierra; en Asistencia la entrada de Beto sale «sin salida»; en la bitácora, el cierre autorizado con Gina, Beto y el motivo | US3-1, US3-5 |
| Q8 | Igual que Q7 pero cierra un cajero sin el permiso | El mismo aviso en lugar del conteo, con «Si ya se fue, pide a alguien con permiso que cierre la caja» y sin «Cerrar sin la salida de Beto»; con `curl` mandando `authorizedClose`, 403 | US3-3, FR-008 |
| Q9 | Gina abre, no marca salida, intenta su propio cierre autorizado | No se le ofrece; se le pide su PIN. Con `curl`, 409 `CLOCK_OUT_WITH_PIN` | US3-7 |
| Q10 | El dueño (sin «Lleva asistencia») abre y cierra | Sin pasos nuevos | US4-2 |
| Q11 | Apagar Asistencia; abrir y cerrar con Beto sin entrada | Sin pasos nuevos | US4-1, SC-004 |
| Q12 | Con el front anterior (o `curl`), abrir sin entrada | 409 `CLOCK_IN_REQUIRED`; la caja no se abre | US1-5 |

Al terminar, todos los pedidos que se hayan creado se entregan y se cobran, y la caja queda cerrada.

## Gates

```bash
cd server && go build ./... && go test ./... && make test-integration
cd ../web && bun run lint && bun run vitest run && bun run build
```

E2E de caja (`contar-el-cajon`, `arqueo-ciego`, `caja-propinas-tarjeta`, `presupuesto-del-cierre`)
contra la base restaurada: si esa empresa tiene la asistencia encendida, ahora tienen que marcar
entrada antes de abrir (ver plan, «Riesgos»).
