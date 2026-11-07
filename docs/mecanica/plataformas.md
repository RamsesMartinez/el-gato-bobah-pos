# Plataformas de reparto

Notas de mecánica de esta área: lo que no se ve en el código ni en la pantalla y ya costó un defecto. Se leen **antes** de tocar precios por plataforma, emparejamiento, menús publicados, pedidos crudos y credenciales. Vienen de `AGENTS.md` (se movieron el 2026-11-07 porque las instrucciones que carga cada sesión pasaron de 150 mil caracteres); los principios siguen en la constitución.

- **El precio de `product_platform_prices` NO es el precio al que se vende en la plataforma.** Es
  una copia que se captura **después**, a mano, para que el ticket del POS cuadre con lo que la
  plataforma **ya cobró**. La dirección de la verdad es de arriba hacia abajo: manda lo publicado en
  Uber, DiDi o Rappi, y el POS va detrás.

  De ahí se siguen tres cosas que se han interpretado al revés y cuestan números falsos:
  - **El markup por plataforma (`delivery_platforms.price_markup_pct`, hoy 35%) es solo un valor por
    omisión para lo que nadie ha capturado.** No describe lo que el negocio cobra. Cualquier cálculo
    de margen o de comisión hecho sobre él describe una tienda hipotética.
  - **Una diferencia de precio contra la plataforma casi siempre significa «el POS está
    desactualizado», no «la plataforma está mal».** La acción normal es actualizar abajo.
  - **Un precio de plataforma muy por debajo del de mostrador es un dedazo de captura, no una
    promoción**, y hoy nada lo detecta. Medido el 2026-09-15: `Sodas explosivas` se publica en Uber
    a $110.00 y el POS la tenía capturada a $34.75 — dos ventas registradas a menos de un tercio.
  - **Con la API de la plataforma conectada, el precio lo pone la plataforma y se sobrescribe solo**
    (decidido el 2026-09-28; regla en la constitución, *Restricciones del producto*). La captura a
    mano queda para las plataformas no conectadas, y la pantalla lo dice: bloqueado con «Lo pone
    Uber» en el POS y un aviso fijo en la tienda. Construido en la spec 026: la mecánica está en
    «Emparejar con la plataforma» más abajo y las decisiones en
    [docs/emparejamiento-de-plataformas.md](../emparejamiento-de-plataformas.md).
- **Emparejar con la plataforma** (spec 026, 0077). Cuatro cosas que no se ven en la pantalla:
  - **Una pareja apunta a un producto O a una opción** (`product_id` / `modifier_option_id` según
    `local_kind`, con `check`). Antes la opción se guardaba en `product_id`; la 0077 aborta si
    encuentra una así en vez de adivinar.
  - **El precio por plataforma lo escribe la lectura del menú** cuando la tienda está conectada
    (`source = platform`); la captura a mano de esa fila responde 409 `PLATFORM_PRICE_MANAGED`. Con
    dos tiendas de la misma plataforma no se copia nada, porque el precio es por plataforma y una
    pisaría a la otra (log `platform_price_sync_failed reason=several_stores_same_platform`).
  - **La copia de precios avisa al POS** por `OnPricesSynced`, que httpapi conecta al caché del
    menú y al canal de avisos. Un camino nuevo que escriba precios fuera de un handler tiene que
    avisar igual, o las tabletas cobran con el precio viejo hasta 24 horas.
  - **El producto genérico** (`products.system_kind = 'platform_unpaired'`, inactivo, sale en la
    comanda) recibe los renglones de pedidos de plataforma sin pareja. Se resuelve con
    `ensure_platform_unpaired_product(empresa)`, que filtra por la empresa explícita: con un
    `where system_kind = …` a secas, como owner devolvía el de otra empresa.
- **Menús de plataforma** (spec 020): se lee el menú publicado de una tienda y se compara contra el
  catálogo. Cuatro cosas que no son obvias y que cuestan caro si se olvidan:
  - **La garantía de que nada escribe en la plataforma vive en el TRANSPORTE**, no en una revisión:
    [solo_lectura.go](../../server/internal/uber/solo_lectura.go) rechaza todo verbo distinto de `GET`
    antes de abrir el socket, y `sin_escrituras_test.go` parsea el AST del paquete para que el
    intento falle en `go test`. El `PUT` de menú de Uber es reemplazo total y no hay deshacer.
  - **El emparejamiento tiene DOS vecinos que podrían borrarlo**, y los dos están cerrados:
    `platform_item_links` **no** referencia `platform_menu_items` (para que podar lecturas no se
    lleve el trabajo manual) y su FK a `products` es `on delete restrict` (porque el reorg de datos
    del §6 sí borra productos). Los cubre `emparejamiento_sobrevive_test.go`, y su fallo sería
    silencioso y semanas después.
  - **El id de la tienda vive en `platform_connections`, NUNCA en el entorno.** Una empresa tendrá
    varias sucursales y cada una es una tienda distinta arriba; la llave única incluye
    `external_store_id` por eso. **Y NO se teclea**: `GET /admin/platform-menus/available-stores`
    lo trae de la plataforma y la pantalla ofrece "Nombre — Ciudad". Es el único dato del alta que
    una persona no puede producir ni verificar de memoria, y ese endpoint es el MISMO que servirá
    cuando exista el consentimiento del comerciante — ahí solo cambia de dónde sale el token, no la
    pantalla. Deja el campo escrito a mano como salida, nunca como el camino.
  - **Instrumentar sus pantallas son TRES lugares**, no dos: `pantallasMedibles` y
    `rolesPorPantalla` en [uso.go](../../server/internal/domain/uso.go), más `PANTALLAS` en
    [rutas-medidas.ts](../../web/src/app/rutas-medidas.ts). Falta el mapa de roles y los eventos se
    descartan todos, en silencio.
- **Lo que llega crudo de una plataforma NO se poda** (decidido el 2026-10-22). El aviso y el
  detalle de cada pedido (`raw_body`, `raw_detail`) y el nombre del cliente se guardan para
  siempre: el POS los va a mostrar. Una poda se construyó (#130) y se revirtió, porque lo borrado no
  se recupera. Fuera de producción los quita `scripts/respaldo-anonimo.sh`, que trunca esas tablas.
- **Credenciales de las plataformas** (0075, ampliación de la 021): el Client ID y el Client Secret
  de la app de Uber **ya no van en el entorno**. Se capturan en Plataformas → «Acceso a la app», por
  empresa, y se guardan solo si Uber entrega un token con ellas. Cuatro cosas que cuestan caro:
  - **El secreto se guarda cifrado, y en producción solo con Cloud KMS.** El paquete
    [internal/secrets](../../server/internal/secrets/) habla con KMS por REST con el token del metadata
    server de la VM; no hay SDK ni archivo de llave de Google. Cada VM tiene su cuenta de servicio
    (`pos-api-dev`, `pos-api-prod`) con un solo permiso: cifrar y descifrar con SU llave
    (`keyRings/pos-dev` o `pos-prod`, `cryptoKeys/credenciales`). En `deploy/.env` del servidor va
    `CREDENTIALS_KMS_KEY` con el nombre de la llave; **sin ella la API de producción no arranca**,
    así que se pone ANTES de desplegar. En tu máquina va `CREDENTIALS_LOCAL_KEY`, que
    `make check-env` genera y que producción rechaza.
  - **Un respaldo de producción restaurado en local trae las credenciales ilegibles**: es la
    protección funcionando. La pantalla dice «hay que volver a capturar el acceso», no un 500
    (`TestABackupFromAnotherEnvironmentNeedsRecapture`). Lo mismo con la llave de firma.
  - **El cliente armado se reusa por empresa, y no es optimización**: guarda su token, y Uber da 100
    por hora e invalida el más viejo a partir del 101. El que se comprobó al guardar es el que se
    queda atendiendo. Un servicio de credenciales por proceso, compartido por menús y pedidos.
  - **La AAD de cada cifrado es `credential|company|platform|kind`** ([domain](../../server/internal/domain/platform_credentials.go)).
    No se cambia su formato: todo lo ya guardado se cifró con él. La llave de firma usa el mismo
    tipo para la primaria y la secundaria porque rotar mueve el cifrado entre columnas.
