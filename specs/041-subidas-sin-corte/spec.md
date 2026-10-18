# Feature Specification: Subidas a producción sin corte

**Feature Branch**: `041-subidas-sin-corte`

**Created**: 2026-10-18

**Status**: Draft

**Input**: Pedido del dueño (2026-10-17) tras medir la subida a producción de ese día: la API dejó de responder ~15 s (02:52:25 «shutting down», 02:52:35 «shutdown: context deadline exceeded», 02:52:40 «listening»). 10 de esos 15 s son la espera de las conexiones de avisos en vivo, que nunca cierran solas, y cada subida deja un ERROR falso en el log. Decisión del dueño: azul/verde en la misma máquina, precedido por un apagado ordenado rápido.

## Contexto medido

| Hecho | Valor |
| --- | --- |
| Corte en la subida del 2026-10-17 | ~15 s sin respuesta |
| Parte del corte que es esperar los avisos en vivo | 10 s (el plazo completo del apagado) |
| Memoria de una copia de la API | ~10 MB |
| Memoria libre en la máquina de producción | ~468 MB (caben dos copias) |
| Cómo sube hoy | se apaga la copia vieja y después arranca la nueva |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Quien opera no nota la subida (Priority: P1)

El mostrador cobra, la cocina recibe comandas y llegan pedidos de plataforma mientras se sube una versión nueva. Ninguna petición falla: la versión nueva arranca junto a la vieja, recibe el tráfico solo cuando ya está sana, y la vieja termina lo que traía antes de apagarse.

**Why this priority**: es el pedido. Hoy se sube con el local operando (sin ventana) y cada subida es un hueco de ~15 s donde un cobro o un aviso de pedido puede fallar.

**Independent Test**: en el ambiente de pruebas, pedir la salud de la API en ciclo durante una subida completa y contar las peticiones fallidas.

**Acceptance Scenarios**:

1. **Given** la API atendiendo y una petición en ciclo contra su salud, **When** se sube una versión nueva, **Then** ninguna petición falla ni tarda más de 2 s.
2. **Given** una petición larga en curso en la copia vieja (p. ej. un cobro), **When** el tráfico pasa a la copia nueva, **Then** esa petición termina con su respuesta normal antes de que la vieja se apague.
3. **Given** una tableta con el tablero de pedidos abierto, **When** se sube una versión nueva, **Then** sus avisos en vivo se reconectan solos a la copia nueva y el tablero queda al día (no se pierde un pedido creado durante el traslape).

---

### User Story 2 - Una versión rota no tumba al local (Priority: P1)

Si la versión nueva no arranca o no queda sana (falta una variable, no alcanza la base, truena al iniciar), el tráfico nunca se mueve: la copia vieja sigue atendiendo, el despliegue termina en rojo y lo dice.

**Why this priority**: hoy una versión rota deja la API abajo hasta que alguien la arregla. Con dos copias, la red de seguridad sale gratis y es la mitad del valor.

**Independent Test**: en el ambiente de pruebas, subir a propósito una versión que no queda sana y verificar que la salud sigue respondiendo con la versión anterior y que el despliegue falla.

**Acceptance Scenarios**:

1. **Given** una versión nueva que no queda sana en el plazo de espera, **When** corre el despliegue, **Then** el tráfico sigue en la copia vieja, la copia nueva se detiene y el despliegue termina en error con el motivo.
2. **Given** un despliegue que falló así, **When** se corrige y se vuelve a subir, **Then** sube normal sin intervención manual en la máquina.

---

### User Story 3 - Apagar la API es rápido y limpio (Priority: P2)

Al apagarse una copia de la API, cierra primero las conexiones de avisos en vivo y después espera solo lo que de verdad está en curso. El apagado tarda lo que tardan las peticiones vivas, no el plazo completo, y no deja un ERROR en el log.

**Why this priority**: es la mitad barata del arreglo y vale por sí sola (incluso sin azul/verde quita 10 de los 15 s); además, sin ella la copia vieja del azul/verde tardaría siempre el plazo completo en irse.

**Independent Test**: con una tableta conectada a los avisos en vivo, apagar la API local y medir cuánto tarda y qué deja en el log.

**Acceptance Scenarios**:

1. **Given** conexiones de avisos en vivo abiertas y ninguna otra petición, **When** se apaga la API, **Then** termina en menos de 1 s y el log no tiene ningún renglón de error.
2. **Given** una petición normal en curso, **When** se apaga la API, **Then** esa petición termina con su respuesta y después se apaga.
3. **Given** una petición que no termina dentro del plazo, **When** vence el plazo, **Then** el log lo dice con su nivel de error (ese sí es un error real).

---

### User Story 4 - Nada sale dos veces durante el traslape (Priority: P2)

Mientras dos copias corren a la vez, ningún trabajo de fondo se hace doble: el resumen diario por correo sale una sola vez por empresa y día, y el mantenimiento que corre al arrancar no le pisa trabajo vivo a la otra copia.

**Why this priority**: el traslape es nuevo; sin esto, el dueño recibe dos correos o una lectura de menú en curso queda marcada como colgada.

**Independent Test**: arrancar dos copias de la API contra la misma base a la hora del resumen y contar correos enviados.

**Acceptance Scenarios**:

1. **Given** dos copias corriendo a la hora del resumen diario, **When** las dos lo intentan, **Then** sale un solo correo por empresa y día.
2. **Given** una lectura de menú de plataforma en curso en la copia vieja, **When** arranca la copia nueva y corre su mantenimiento de arranque, **Then** esa lectura no se marca como colgada.
3. **Given** el recorte periódico de la medición de uso corriendo en las dos copias, **When** coinciden, **Then** el resultado es el mismo que con una sola (sin error ni doble efecto).

---

### User Story 5 - Las migraciones toleran a la versión anterior (Priority: P3)

Como las dos versiones corren contra la misma base durante el traslape, la constitución fija la regla: toda migración funciona con la versión anterior de la API corriendo al mismo tiempo. Se agrega en una subida y se quita en otra, nunca las dos cosas en la misma.

**Why this priority**: sin la regla, el azul/verde convierte una migración que quita o renombra una columna en errores de la copia vieja durante el traslape. Es una regla, no código, pero es la que hace seguro todo lo demás.

**Independent Test**: la regla aparece en la constitución con su ejemplo, y `/speckit-analyze` la puede medir en planes futuros.

**Acceptance Scenarios**:

1. **Given** la constitución enmendada, **When** un plan propone quitar o renombrar una columna que la versión en producción lee, en la misma subida, **Then** es un hallazgo de revisión.

### Edge Cases

- **La nueva queda sana y después truena** (sana en el primer chequeo, cae segundos después): el despliegue revisa de nuevo tras mover el tráfico; si cae y la vieja sigue viva, regresa el tráfico a la vieja.
- **El despliegue se interrumpe a la mitad** (se cae la conexión de CI, se cancela el job): la configuración del proxy y las copias vivas quedan en un estado del que el siguiente despliegue parte sin intervención; nunca queda el proxy apuntando a una copia apagada.
- **Primera subida con el esquema nuevo**: la máquina hoy tiene una sola copia con el nombre viejo, y su proxy nació sin el directorio de upstream y ve el archivo de configuración anterior (lo reemplaza `git checkout`). Hay que recrear el proxy una vez, y eso corta: medido en local, ~10 s con la espera por defecto de Docker (el proxy espera a los avisos en vivo) y ~1.5 s con espera de 1 s. Es un corte único por máquina, se dice y se mide (SC-006).
- **El resumen diario se cortaba a medio envío** (descubierto en la revisión de código, ya existía antes de esta feature): si la API se apagaba después de mandar el correo y antes de marcarlo, el envío quedaba «pendiente» y a los 10 minutos otra pasada lo volvía a mandar. El apagado detiene el ciclo, pero la pasada en curso termina.
- **La migración corre al arrancar la copia nueva mientras la vieja atiende**: la vieja debe seguir funcionando con el esquema ya migrado (regla de la User Story 5); si una migración no puede cumplirla, el despliegue lo declara y se acepta el corte por escrito.
- **Avisos en vivo durante el traslape**: cada copia sólo avisa a sus propias conexiones; un pedido creado en la vieja mientras la tableta ya está en la nueva no le llega como aviso. La tableta recupera el estado completo al reconectarse.
- **Memoria**: dos copias más Postgres, Redis y el proxy en 1 GB; si la máquina no tiene memoria para la segunda, el despliegue falla antes de mover tráfico.
- **Recarga del proxy con conexiones largas abiertas**: recargar la configuración no corta las peticiones ni los avisos en vivo que ya pasan por él más allá de lo que el apagado de la copia vieja decide. La excepción es la primera subida (ver «Primera subida con el esquema nuevo»), que recrea el proxy y corta los avisos en vivo; la tableta reconecta sola.
- **Los dos lados con la misma versión** (reintentar un despliegue del mismo commit): funciona igual que uno normal.
- **Ambiente local y de pruebas**: el mismo mecanismo de despliegue corre en el ambiente de pruebas; local no necesita azul/verde pero sí el apagado ordenado.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Al apagarse, la API MUST cerrar primero todas las conexiones de avisos en vivo y después esperar solo a las peticiones en curso, dentro de un plazo.
- **FR-002**: Un apagado en el que todo terminó dentro del plazo MUST NOT dejar renglones de nivel error en el log; uno que venció el plazo MUST dejarlo.
- **FR-003**: La producción MUST tener dos lados para la API (azul y verde), y el proxy MUST enviar el tráfico a uno solo a la vez.
- **FR-004**: El despliegue MUST arrancar la versión nueva en el lado libre sin detener la vieja, y esperar a que reporte estar lista (incluida la base) antes de mover tráfico.
- **FR-005**: El despliegue MUST mover el tráfico recargando la configuración del proxy sin reiniciarlo ni cortar conexiones, salvo la primera subida de cada máquina, que recrea el proxy una sola vez (SC-006).
- **FR-006**: Tras mover el tráfico, el despliegue MUST apagar la copia vieja de forma ordenada (FR-001), dejando terminar lo que traía.
- **FR-007**: Si la copia nueva no queda lista dentro del plazo, el despliegue MUST dejar el tráfico en la vieja, detener la nueva y terminar en error con el motivo.
- **FR-008**: El despliegue MUST poder correr de nuevo desde cualquier estado en que lo deje un despliegue interrumpido, sin pasos manuales en la máquina.
- **FR-009**: El resumen diario por correo MUST salir una sola vez por empresa y día aunque haya dos copias corriendo, con la exclusión resuelta en la base.
- **FR-010**: El mantenimiento que corre al arrancar (cerrar lecturas de menú colgadas, recortes) MUST NOT afectar el trabajo vivo de la otra copia durante el traslape.
- **FR-011**: Cualquier otro proceso de fondo con temporizador MUST quedar revisado y documentado como seguro o protegido para dos copias.
- **FR-012**: La tableta MUST reconectarse sola a los avisos en vivo tras el cambio de copia y recuperar el estado completo del tablero al reconectarse.
- **FR-013**: La constitución MUST incluir la regla de migraciones compatibles con la versión anterior (agregar y quitar en subidas distintas), con su porqué.
- **FR-014**: MUST existir una prueba que pida la salud de la API en ciclo durante una subida en el ambiente de pruebas y reporte peticiones totales, fallidas y la mayor latencia.
- **FR-015**: El despliegue de producción MUST seguir pidiendo el OK explícito del dueño y terminar verificando la salud, como hoy.
- **FR-016**: La documentación operativa (AGENTS.md y el runbook de despliegue) MUST describir el nuevo despliegue, cómo saber qué lado está activo y cómo regresar a la versión anterior.

### Key Entities

- **Lado de la API**: uno de dos (azul, verde); tiene una versión, un estado (activo, en espera, apagado) y es el destino del proxy cuando está activo.
- **Envío del resumen diario**: el registro de que el resumen de una empresa para un día ya salió (o está saliendo); es la exclusión entre copias.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: En una subida en el ambiente de pruebas que no sea la primera de esa máquina (ver SC-006), 0 peticiones fallidas de salud en ciclo (al menos una cada 200 ms durante toda la subida), contra ~15 s de corte medidos en producción el 2026-10-17.
- **SC-002**: Ninguna petición de salud durante la subida tarda más de 2 s.
- **SC-003**: El apagado de una copia con solo avisos en vivo abiertos tarda menos de 1 s (hoy: 10 s), y deja 0 renglones de error en el log.
- **SC-004**: Una versión que no queda sana deja el servicio respondiendo con la anterior: 0 peticiones fallidas durante ese despliegue fallido.
- **SC-005**: Con dos copias corriendo a la hora del resumen, sale exactamente 1 correo por empresa y día.
- **SC-006**: La primera subida de cada máquina (paso de una copia a dos lados) corta una sola vez, 2 s o menos, medido con la misma prueba de SC-001.

## Assumptions

- Una sola máquina sigue siendo la producción; la alta disponibilidad entre máquinas (dos VMs, balanceador de GCP, Cloud SQL) queda fuera, igual que Kamal, Swarm y Kubernetes.
- El proxy que ya está delante de la API (Caddy) es el que hace el cambio, con su recarga sin corte.
- Redis y Postgres se comparten entre las dos copias; Redis sigue siendo solo caché y contadores, así que compartirlo es seguro.
- Los avisos en vivo siguen siendo por copia (en memoria); se acepta que durante el traslape un aviso no cruce de copia, porque la tableta recupera el estado al reconectarse (FR-012).
- Las migraciones siguen corriendo al arrancar la API; la regla de la User Story 5 es lo que vuelve seguro que corran mientras la vieja atiende.
- Las vueltas se iteran en local (el ambiente de este worktree ya corre: API en :8041, web en :3041); el ambiente de pruebas se usa una vez con la versión final para la medición de SC-001, SC-004 y SC-006; en producción no se prueba, solo salud después de subir, con OK explícito del dueño.
- No hay pantalla nueva: no aplica maqueta.
