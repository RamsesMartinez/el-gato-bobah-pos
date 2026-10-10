# Quickstart — validar que los pedidos entran

**Feature**: 021 · Recibir los pedidos de Uber Eats

Esto se valida **sin depender de Uber** (FR-031). Uber todavía no documenta cómo generar un pedido
de prueba y hay un ticket abierto; el diseño no puede quedar esperando esa respuesta.

---

## Lo que hace falta

- El ambiente de pruebas arriba (`api-dev.elgatobobah.com`). Es una VM spot: puede aparecer apagada
  sin que nadie la apagara.
- Una tienda conectada en `Plataformas` (la del selector de la 020).
- Una llave de firma capturada para esa empresa y plataforma.
- Un usuario que pueda aceptar pedidos.

## 1 · La firma es obligatoria, y el rechazo no dice por qué

Lo primero que hay que ver **en rojo**: un aviso sin firma no entra.

```bash
curl -i -X POST https://api-dev.elgatobobah.com/api/v1/webhooks/uber-eats \
  -H 'Content-Type: application/json' -H 'X-Environment: sandbox' \
  -d '{"event_type":"orders.notification","event_id":"sin-firma-1"}'
```

**Se espera**: 401. El mismo cuerpo que con una firma incorrecta. Y un evento de seguridad en el
log, **sin** el cuerpo y **sin** la llave.

Repetir con `X-Uber-Signature: 00` y comprobar que la respuesta es **idéntica**: si las dos difieren
en algo, se puede saber por tanteo cuándo se acertó el formato.

## 2 · Un pedido entra solo

Se firma el cuerpo con la misma llave que se capturó:

```bash
CUERPO=$(cat evento-pedido.json)
FIRMA=$(printf '%s' "$CUERPO" | openssl dgst -sha256 -hmac "$LLAVE" -hex | sed 's/.* //')
curl -i -X POST https://api-dev.elgatobobah.com/api/v1/webhooks/uber-eats \
  -H 'Content-Type: application/json' -H 'X-Environment: sandbox' \
  -H "X-Uber-Signature: $FIRMA" --data-binary "$CUERPO"
```

**Ojo con el cuerpo**: la firma se calcula sobre los **bytes exactos** que se envían. `--data-binary`
y no `-d`: `-d` recorta saltos de línea y la firma deja de cuadrar.

**Se espera**:
- 200 con cuerpo vacío.
- El pedido visible en la tableta **en menos de 10 segundos**, sin recargar (SC-001).
- Sus renglones con nombre y precio de la plataforma; los que no tengan pareja, señalados.

## 3 · El mismo aviso cinco veces sigue siendo un pedido

Repetir el comando anterior **cinco veces sin cambiar nada**.

**Se espera**: 200 las cinco, y **un** pedido en la pantalla (SC-003). Es el defecto que hace que la
cocina prepare dos veces y que no truena por ningún lado.

## 4 · Aceptar cuesta un toque

Con el pedido en pantalla, tocar **Aceptar**.

**Se espera**:
- Se imprime el ticket de cocina, sin pasos extra (SC-002).
- El pedido aparece en el tablero como cualquier otro.
- **No** aparece como dinero por cobrar: ya lo cobró la plataforma.
- El total del POS **cuadra al centavo** con el de la plataforma (SC-007).

Tocar Aceptar otra vez: se espera 409 con un mensaje que lo diga.

## 4b · El aviso se ve aunque haya una hoja abierta

Con el **ticket abierto a pantalla completa** —capturando una venta de mostrador— mandar otro aviso
firmado.

**Se espera**: el aviso se ve **por encima** del ticket. Es el escenario que más importa: el pedido
llega justo cuando hay alguien ocupado. Si se pinta debajo, la feature no sirve para el único
momento en que hace falta.

Repetirlo con la hoja de modificadores y con la de cobro abiertas.

## 4c · Tres pedidos a la vez

Mandar tres avisos con distinto `decide_before`.

**Se espera**: se ve **el más urgente** con Aceptar y Rechazar directos, y un contador con los otros
dos. Aceptar el más urgente sigue costando un toque.

## 5 · Aceptar sin turno de caja abierto

Con la caja **cerrada**, mandar otro pedido y aceptarlo.

**Se espera**: se acepta igual y se imprime — la cocina no espera a que alguien abra caja (FR-022).
Al abrir el siguiente turno, ese pedido **se ve de un vistazo** como ya considerado en la apertura
(FR-023). Sin esa señal aparece dinero que nadie recuerda haber cobrado.

## 6 · Rechazar

Sobre un pedido pendiente, tocar **Rechazar** y elegir un motivo.

**Se espera**: queda rechazado con su motivo, con quién y cuándo, y no cuenta como venta.
Con un código de motivo inventado, 422 — **no** cae a "otro" en silencio.

## 7 · Un pedido que no se puede leer no se confirma

Firmar un aviso cuyo `resource_href` apunte a algo que no responde.

**Se espera**: 5xx, **no** 200 — es lo que hace que la plataforma reintente. El intento queda
registrado con su clase de fallo, y el **aviso** crudo guardado. Ningún pedido en la pantalla.

Es el caso que más caro sale si está al revés: confirmar lo que falló pierde el pedido en silencio.

## 8 · No se cruzan las empresas, ni siquiera con el mismo id de tienda

Con dos empresas y sus dos tiendas, firmar un aviso de la tienda de **A** con la llave de **B**.

**Se espera**: rechazado. Y el caso inverso —aviso de A firmado con la llave de A— no deja rastro
alguno en B. Es lo que un unitario no puede ver: **exige integración con dos empresas**.

Y el caso que de verdad importa: **registrar el MISMO id de tienda en las dos empresas** (pasa en el
ambiente de pruebas, donde se reparten tiendas de demostración compartidas) y mandar un aviso
firmado con la llave de A. Se espera que caiga en A y solo en A. Quien resuelve la ambigüedad es la
firma, no el cuerpo.

## 9 · El ambiente equivocado no pasa

Mandar un aviso con `X-Environment: production` contra el ambiente de pruebas.

**Se espera**: rechazado. Procesarlo mezclaría un pedido real con datos de prueba.

---

## Antes de reportar que quedó

- **No dejar pedidos de prueba abiertos.** El ambiente se comparte con una persona: un pedido de
  prueba vivo aparece en la barra del POS, suma a "por cobrar" y bloquea el cierre de caja. Lo que
  se crea probando se entrega y se cobra.
- **Los gates completos**, no solo los tocados: `go build`, `go test`, la suite de integración,
  `bun run lint`, `vitest` y `bun run build`.
- **Lo que quedó sin verificar se nombra "no verificado"**, empezando por lo que dependa del ticket
  con Uber.

## Lo que este quickstart NO prueba

- **La forma real del pedido que manda Uber.** Los avisos son nuestros y con la forma documentada.
  El primer pedido real puede traer algo distinto; para eso se guarda **el detalle** crudo
  (`raw_detail`), que es donde vive esa diferencia — no el aviso, que son cuatro campos y una liga.
- **Que `accept_pos_order` funcione.** Exige ser la app gestora de pedidos de la tienda, permiso que
  hoy no tenemos y que está en el ticket.
- **La robollamada a los 90 segundos ni el corte a los 11.5 minutos.** Los dispara Uber.
