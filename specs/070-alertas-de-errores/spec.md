# Feature Specification: Aviso por correo de cada error interno de la API

**Feature Branch**: `070-alertas-de-errores`

**Created**: 2026-11-08

**Status**: Draft

**Input**: Encargo del dueño (2026-11-08): «hoy un 500 en producción pasa sin que nadie se entere.
Cada 500 de la API manda un correo».

## Contexto

Cuando la API responde un error interno (500), la tableta muestra «Error interno del servidor» y en
la bitácora del servidor queda una línea. Nadie mira esa bitácora a diario: el dueño se entera
cuando quien opera se queja, horas o días después, y para entonces el renglón del error ya se
mezcló con miles de peticiones normales. Ya pasó con `/catalogo/opciones` respondiendo 500 durante
horas en producción (AGENTS.md, «Un agregado que se puede vaciar va con coalesce»).

Lo que existe hoy y se reúsa:

- **Envío de correo**: el sistema ya manda correo para la recuperación de contraseña, por SMTP.
  En producción sale por Zoho Mail (`contacto@elgatobobah.com`); en local cae en mailpit.
- **Agrupamiento compartido**: Redis ya está en producción (caché de menú y limitador de intentos)
  y lo comparten los dos procesos de la API durante una subida azul/verde.
- **Id de request** (`X-Request-Id`) en cada respuesta y en cada renglón de bitácora.

## Decisiones del dueño

- Destinatario: `ramses.mtz96@gmail.com`.
- **Se usa el correo que ya existe (Zoho Mail), no ZeptoMail** (respuesta del 2026-11-08: en el
  proyecto no había nada de ZeptoMail).
- Uno por ruta y tipo de error cada 15 minutos, con el conteo de repeticiones.
- Sin datos sensibles; el test lo verifica sobre el correo generado.

## Decisiones tomadas en este spec (recomendación, sin preguntar)

| Duda | Decisión | Por qué |
| --- | --- | --- |
| ¿Qué cuenta como «un 500»? | Solo el estado 500 exacto, venga de un error que nadie mapeó o de un panic | 501 (función no configurada) y 503 (plataforma caída, copia que se apaga) son estados previstos con su propio mensaje en pantalla; avisar de ellos es ruido |
| ¿Cómo se ve la ruta? | El **patrón** de la ruta (`/api/v1/orders/{id}`), nunca la URL real | La URL real puede traer `?token=` (los avisos en vivo lo usan) y agruparía cada id como un error distinto |
| ¿Qué es «tipo de error»? | El tipo del error de fondo (el último de la cadena) y, si viene de la base, su código; para un panic, el tipo del valor | El texto del mensaje trae ids y montos: agrupar por texto mandaría un correo por pedido |
| ¿Qué pasa con las repeticiones? | El primer error de una ventana avisa al instante. Si en los siguientes 15 min se repite, al cerrar la ventana llega **un** correo con cuántas veces se repitió, y la ventana se renueva mientras siga repitiéndose | Avisar al instante es lo que el dueño pidió; contar sin mandar el conteo nunca dejaría ver una ráfaga |
| ¿Quién recibe? | Una variable de ambiente con el destinatario; vacía = avisos apagados, y la API lo dice al arrancar | El producto se vende: otro despliegue avisa a otra persona. Apagado por omisión evita mandar correos desde la máquina de quien programa |
| ¿Empresa por nombre o por id? | Las dos; si el nombre no se puede leer (la base es justo lo que falló), solo el id | `company_id = 1` no es El Gato Bobah: el id solo invita a confundirse |
| ¿Usuario? | Id y nombre de quien hizo la petición; «sin sesión» si no había | Es personal del negocio, no dato de cliente, y es lo primero que se pregunta |

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Me entero de un error interno en minutos (Priority: P1)

Una petición de la tableta falla con un error interno. Al dueño le llega un correo con lo
necesario para encontrar el error: cuándo pasó (hora del centro de México), qué petición era
(método y ruta), quién la hizo y en qué empresa, el id de request para buscarla en la bitácora, el
mensaje del error y una traza corta del código.

**Why this priority**: es la feature entera; sin esto el resto no tiene a quién avisar.

**Independent Test**: forzar un 500 en local y ver el correo en mailpit con todos los campos.

**Acceptance Scenarios**:

1. **Given** avisos encendidos, **When** una petición autenticada termina en 500, **Then** llega un
   correo con hora CDMX, método, patrón de ruta, id y nombre de usuario, id y nombre de empresa, id
   de request, mensaje del error y traza resumida.
2. **Given** avisos encendidos, **When** un handler entra en panic, **Then** la tableta recibe el
   500 de siempre (con el mismo cuerpo `INTERNAL`) y llega un correo marcado como panic con su traza.
3. **Given** avisos encendidos, **When** la petición responde 400, 404, 409, 501 o 503, **Then** no
   se manda correo.
4. **Given** la petición se cortó del lado del cliente (abort deliberado del servidor de HTTP),
   **When** termina, **Then** no se manda correo.

---

### User Story 2 - Una ráfaga no me llena el buzón (Priority: P1)

El mismo error se repite cien veces en cinco minutos (un endpoint roto que la tableta reintenta).
Llega un correo al primer error y, al cerrar la ventana de 15 minutos, otro con el número de
repeticiones. Errores distintos (otra ruta u otro tipo) avisan por separado.

**Why this priority**: sin agrupar, el primer incidente real vuelve inútil el aviso y el buzón.

**Independent Test**: provocar el mismo 500 N veces con la ventana acortada en el test y contar los
correos generados.

**Acceptance Scenarios**:

1. **Given** una ventana nueva, **When** ocurre el primer error de una ruta y tipo, **Then** sale un
   correo al instante.
2. **Given** ya avisado, **When** el mismo error se repite 99 veces más dentro de la ventana,
   **Then** no sale otro correo hasta que cierra la ventana, y ese dice «se repitió 99 veces».
3. **Given** la ventana cerró sin repeticiones, **When** pasa el tiempo, **Then** no sale nada.
4. **Given** dos procesos de la API vivos (subida azul/verde), **When** los dos ven el mismo error en
   la misma ventana, **Then** sale un solo correo inmediato y las repeticiones de los dos se suman.

---

### User Story 3 - El aviso nunca empeora el error (Priority: P1)

Avisar es secundario. Si el correo no sale (SMTP caído, credencial vencida, red lenta) o Redis no
responde, la tableta recibe su respuesta igual de rápido que hoy, y la API sigue atendiendo.

**Why this priority**: un aviso que tumba o atrasa la caja es peor que no tener aviso.

**Independent Test**: con un enviador que tarda o falla y Redis apagado, la respuesta 500 llega sin
esperar al envío, y el agrupamiento sigue funcionando en memoria.

**Acceptance Scenarios**:

1. **Given** un enviador que tarda 30 s, **When** ocurre un 500, **Then** la respuesta sale sin
   esperarlo.
2. **Given** el enviador falla, **When** ocurre un 500, **Then** se escribe un renglón en la
   bitácora con la causa (sin el contenido del correo) y no se reintenta en bucle.
3. **Given** Redis caído, **When** ocurren 500 repetidos, **Then** el agrupamiento sigue en la
   memoria del proceso (un correo por ventana por proceso) y no truena nada.
4. **Given** una ráfaga que llena la cola de avisos pendientes, **When** llegan más, **Then** se
   descartan los que no caben, se cuenta cuántos, y la petición no espera.
5. **Given** la API se está apagando (subida), **When** quedan conteos de repeticiones sin mandar,
   **Then** se mandan antes de terminar, con un plazo corto que no alarga el apagado más de unos
   segundos.

---

### User Story 4 - El correo no filtra nada sensible (Priority: P1)

El correo sale del servidor a un buzón externo. No lleva contraseñas, PIN, tokens, cuerpos de
petición o de respuesta, datos de pago ni la URL con su query.

**Why this priority**: principio V; un correo con un token es una credencial en el buzón de alguien.

**Independent Test**: provocar un 500 con contraseña y PIN en el cuerpo, token en el encabezado y en
la query, y un mensaje de error que contiene un JWT, un número de tarjeta y `password=...`; el test
revisa el correo generado completo (asunto y cuerpo) y no encuentra ninguno.

**Acceptance Scenarios**:

1. **Given** una petición con secretos en cuerpo, encabezados y query, **When** termina en 500,
   **Then** ninguno aparece en el correo.
2. **Given** un error cuyo texto contiene un JWT, un `Bearer …`, `password=…`/`"token":"…"` o una
   secuencia de 13 a 19 dígitos, **When** se arma el correo, **Then** aparecen enmascarados.
3. **Given** un mensaje de error de varios kilobytes, **When** se arma el correo, **Then** se recorta.

### Edge Cases

- **El envío falla o tarda**: la petición no espera; el fallo queda en la bitácora; ese aviso se
  pierde (no hay reintentos ni cola persistente). Ver Supuestos.
- **Redis caído o no configurado** (local): agrupamiento en memoria del proceso. Con Redis caído y
  dos procesos vivos puede llegar un correo duplicado por proceso; se acepta.
- **Redis se cae a media ventana**: el proceso pasa a memoria y puede mandar un aviso inmediato de
  un error que en Redis ya estaba avisado; se acepta (duplicar es mejor que callar).
- **Azul/verde**: el agrupamiento vive en Redis compartido; la ventana la «gana» un solo proceso. Si
  el proceso que ganó se apaga antes de cerrar la ventana, manda el conteo al apagarse.
- **Proceso que muere sin apagado ordenado** (OOM, kill): el conteo pendiente de su ventana queda en
  Redis y el siguiente aviso de ese error lo suma como «repeticiones anteriores sin reportar».
- **Ráfaga de muchos errores distintos**: la cola acotada descarta; el siguiente correo que sí sale
  dice cuántos avisos se descartaron.
- **Panic después de haber empezado a escribir la respuesta**: no se puede cambiar el estado ya
  enviado; se avisa igual (el panic es el error).
- **`http.ErrAbortHandler`**: se deja pasar sin avisar (es la forma deliberada de cortar una
  respuesta, no un error).
- **Petición sin sesión** (login, webhook): el correo dice «sin sesión» en usuario y empresa.
- **El 500 es porque la base no responde**: leer el nombre de la empresa también falla; el correo
  sale con el id, sin esperar más de un par de segundos.
- **Avisos apagados** (sin destinatario o sin SMTP): no se arma nada, y el arranque lo registra.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Toda respuesta con estado 500 de la API MUST generar un aviso, venga de un error no
  mapeado o de un panic recuperado.
- **FR-002**: Un panic recuperado MUST responder a la tableta el mismo 500 con cuerpo `INTERNAL` que
  un error no mapeado, y MUST registrarse en la bitácora con su traza.
- **FR-003**: Las respuestas con estado distinto de 500 y los cortes deliberados de respuesta MUST
  NOT generar aviso.
- **FR-004**: El correo MUST llevar: hora en zona America/Mexico_City, método, patrón de ruta, id y
  nombre del usuario (o «sin sesión»), id y nombre de la empresa (o solo el id si el nombre no se
  pudo leer en 2 s), id de request, mensaje del error, si fue panic, traza resumida (solo marcos del
  código del proyecto, máximo 12) y el ambiente (producción, pruebas, desarrollo).
- **FR-005**: Los avisos MUST agruparse por (patrón de ruta, método, tipo de error) en ventanas de
  15 minutos: el primero de una ventana sale al instante; las repeticiones se cuentan y, al cerrar
  la ventana, si hubo alguna, sale un correo con el conteo y la ventana se renueva.
- **FR-006**: El agrupamiento MUST compartirse entre procesos de la API vía Redis, de modo que dos
  procesos vivos no dupliquen el aviso inmediato.
- **FR-007**: Si Redis no está configurado o una operación falla, el agrupamiento MUST seguir en
  memoria del proceso, sin detener ni retrasar nada.
- **FR-008**: Generar el aviso MUST NOT bloquear ni retrasar la respuesta: el request solo encola en
  una cola acotada; si está llena, el aviso se descarta y se cuenta.
- **FR-009**: Un fallo de envío MUST quedar en la bitácora con clave estable y sin el contenido del
  correo, y MUST NOT reintentarse.
- **FR-010**: El correo MUST NOT contener contraseñas, PIN, tokens, encabezados de autorización,
  cuerpos de petición o respuesta, query de la URL ni datos de pago; el texto del error y del panic
  MUST pasar por un enmascarado (JWT, `Bearer`, pares clave-valor sensibles, secuencias de 13 a 19
  dígitos) y recortarse.
- **FR-011**: Al apagarse, la API MUST mandar los conteos pendientes de las ventanas que ganó, con
  un plazo máximo de 5 s.
- **FR-012**: El destinatario MUST configurarse por ambiente; sin destinatario o sin SMTP los avisos
  quedan apagados y el arranque lo registra.
- **FR-013**: Un conteo pendiente que ningún proceso mandó (proceso muerto) MUST sumarse en el
  siguiente aviso de ese error como repeticiones anteriores.

### Key Entities

- **Aviso de error**: lo que se sabe de un 500 al terminar la petición — cuándo, qué petición, quién,
  qué error, traza. Vive en memoria hasta mandarse; no se guarda en la base.
- **Ventana de agrupamiento**: llave (ruta, método, tipo), quién la ganó y cuántas repeticiones
  lleva. Vive en Redis (o en memoria) y vence sola.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Un 500 en producción llega al buzón del dueño en menos de 2 minutos desde que ocurre.
- **SC-002**: 100 errores iguales en 15 minutos producen como máximo 2 correos (el inmediato y el del
  conteo).
- **SC-003**: Con el envío de correo tardando 30 s o fallando, la respuesta 500 llega al cliente en
  el mismo tiempo que sin avisos (diferencia menor a 50 ms en el test).
- **SC-004**: El correo generado por una petición cargada de secretos contiene cero de ellos
  (verificado por test sobre asunto y cuerpo completos).
- **SC-005**: Con Redis apagado, un error repetido sigue produciendo un correo por ventana por
  proceso, y ninguna petición falla por eso.

## Assumptions

- **Sin reintentos ni cola persistente** (principio VI): un aviso que no sale por SMTP caído se
  pierde; queda su renglón en la bitácora de 30 días. Una cola persistente compra poco frente a un
  SMTP que casi nunca falla.
- **Volumen**: Zoho Mail aguanta cientos de correos al día; con el agrupamiento, un incidente cuesta
  pocos correos.
- **Solo la API del POS y la consola de plataforma** (el mismo router). Los errores de procesos en
  segundo plano (lectura de menús de plataforma, webhooks después de responder) quedan fuera.
- **Sin pantalla**: nada cambia en `web/`.
- **Sin migración**: nada se guarda en la base.
- El ambiente de pruebas no tiene SMTP hoy; para la prueba con correo real se le configura el mismo
  Zoho Mail que usa producción (autorizado por el dueño en el encargo).
