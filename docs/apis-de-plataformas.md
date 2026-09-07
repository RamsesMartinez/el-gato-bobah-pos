# APIs de las plataformas de delivery — qué se puede y qué no, para El Gato Bobah

Documento de referencia previo al spec de "conectarse a las APIs de las plataformas". No es un plan: es lo que se sabe, con qué respaldo, y qué decisiones de esquema **de otros specs** dependen de esto.

**Niveles de evidencia** — cada renglón accionable trae el suyo:

| Marca | Significa |
| --- | --- |
| **[D]** | Documentado por la plataforma o por el proveedor, leído en fuente primaria |
| **[R]** | Reportado por terceros (comparativos, notas, catálogos de API) — no verificado en fuente primaria |
| **[I]** | Inferido a partir de lo anterior. No es un hecho: es un razonamiento con nombre |

Cuando dos investigaciones se contradicen, el renglón lo dice. Las mediciones de red traen su fecha; la más reciente es del 2026-09-06.

---

## 1. La respuesta corta

**Hoy, para este negocio, la integración directa con las tres plataformas NO es alcanzable, y la vía middleware tampoco está confirmada como alcanzable.** Lo que sí es alcanzable hoy, sin permiso de nadie y sin código de integración, es cerrar el objetivo (1) —el folio— capturándolo a mano, y avanzar buena parte del objetivo (3) importando los CSV de pago que las plataformas ya entregan por su portal.

| Objetivo del negocio | ¿Alcanzable hoy? | Por dónde |
| --- | --- | --- |
| (1) Que el folio de la plataforma llegue solo | **No por API.** Sí a mano | Campo de folio en la captura manual (§7) |
| (2) Que el pedido entre al POS sin recaptura | **No** | Exige API directa (bloqueada) o middleware (sin confirmar) |
| (3) Que los reportes de pago y comisión se bajen solos | **No por API.** Sí en semiautomático | Importar el CSV que ya se descarga del portal (§7) |

Por qué está bloqueada la vía directa, en una línea por plataforma:

| Plataforma | Estado de la puerta | Evidencia |
| --- | --- | --- |
| **Uber Eats** | Cerrada: exige NDA, contrato de licencia de *POS Provider*, un *partner manager* asignado y whitelisting manual de la app. El grant es `client_credentials`, así que **el comercio no puede autorizarse a sí mismo** | [D] [authentication](https://developer.uber.com/docs/eats/guides/authentication), [getting-started](https://developer.uber.com/docs/eats/guides/getting-started) |
| **Rappi** | Cerrada, pero es la menos cerrada: documentación pública y completa, credenciales solo por alta manual del punto de contacto comercial | [D] [dev-portal.rappi.com](https://dev-portal.rappi.com/en/api-reference/) |
| **DiDi Food** | Cerrada: el primer paso del proceso es una negociación donde DiDi pregunta literalmente cuántas tiendas y cuántos pedidos al día se manejan, más NDA y auditoría | [D] nodo 1890 del portal de DiDi |
| **Middleware** (Deliverect, Otter) | Sin confirmar. Su API hacia POS de terceros existe, pero está detrás de un **programa de partners con llamadas de certificación**, escrito para vendedores de POS con cartera de clientes | [D] [Deliverect partners](https://developers.deliverect.com/docs/become-an-integration-partner), [Otter scenarios-pos](https://connect.tryotter.com/docs/scenarios-pos/) |

Y el dato que ordena las prioridades: **el webhook del pedido nunca trae el dinero final**. Uber pide tratar como *provisional* todo lo que venga de la Orders API, avisa que los datos financieros tardan hasta 72 horas en asentar y que las disputas aparecen hasta 30 días después [D — [reporting](https://developer.uber.com/docs/eats/guides/reporting)]. Es decir: aun con la API concedida, conciliar exige un **segundo canal** de reportes. El canal de reportes ya existe hoy, sin API, en el portal de cada plataforma.

---

## 2. Lo que NO se pudo averiguar

Esto va primero a propósito: son los huecos que pueden invertir el veredicto o volver inútil un diseño.

### 2.1 Huecos que pueden matar el proyecto

| Hueco | Por qué no se cerró | Qué cambia si se resuelve |
| --- | --- | --- |
| **Si Deliverect u Otter aceptan certificar un POS propio de UN local con ~45 pedidos de plataforma al mes** | Ninguno publica mínimo de locales ni costo de certificación; los dos programas están redactados para vendedores de POS ("your customers") | Es el único hueco que decide si la vía middleware existe. Hay un reporte en el foro de desarrolladores de Deliverect (19-mar-2026) de un solicitante filtrado en el umbral de *Locations Served* [R] |
| **Si Uber otorga acceso en México y a un comercio que es su propio proveedor** | La única copia del *Uber Eats API Licensing Agreement* que se pudo leer (ejecutada en 2022) define Territory como EE.UU., África, Australia, Canadá, Europa, Medio Oriente, Nueva Zelanda y Japón. **México y Latinoamérica no aparecen** [D, documento de 2022, vigencia no verificada] | Si el territorio sigue así, la vía directa a Uber no existe en México, sin importar el volumen |
| **Si el `order_id` de la API es el mismo identificador que el del documento de pago** | En las tres plataformas queda sin declarar. Uber solo define su columna como "Order ID as listed in Uber Eats Manager"; Rappi no declara la equivalencia entre Orders y Financial; DiDi tipa uno como string corto y otro como entero de 19 dígitos | Si no empatan, el folio llega pero **no concilia**, que es justo el problema que se quiere resolver. Es verificación empírica obligatoria, no de lectura |
| **Si el Payment Details Report de México trae las mismas columnas que el de EE.UU.** | Las columnas documentadas son las de EE.UU.; el propio artículo aclara que Canadá tiene un juego distinto por impuestos locales. Nada verificado sobre IVA, ISR ni retenciones del régimen de plataformas digitales | Decide si el importador de CSV de §7 se puede diseñar contra la documentación o hay que abrir un archivo real primero |

### 2.2 Huecos de acceso y costo

| Hueco | Detalle |
| --- | --- |
| **Criterio de elegibilidad por volumen** | Ninguna de las tres publica un mínimo. El 98% de inyección en 3 días de Uber es criterio de **expansión a más tiendas**, no de entrada [D]. Que no esté documentado no prueba que no exista como criterio informal |
| **Tiempos de alta** | Cero SLA publicados en las tres. DiDi describe cuatro fases (negociación, desarrollo y QA, piloto, expansión) sin un solo plazo [D] |
| **Costo de la API** | El contrato de Uber de 2022 dice que las APIs se proveen "free of charge, as-is, and with no warranty of any kind" [D]. Rappi y DiDi no mencionan costo en ninguna página leída. **No se puede afirmar que sean gratis hoy** |
| **Si el *Technology Integration Agreement* de DiDi aplica a persona física** | Los cuatro documentos que enumera (Acta Constitutiva, RFC, Poder Notarial, ID del representante legal) son de persona moral. El negocio es persona física con actividad empresarial. La doc solo dice que DiDi revisa el perfil y decide "whether you need to sign" [D] |
| **Precio de Otter en México** | La página `es-mx/precios` renderiza el contenedor de planes **vacío**; lo único legible es "Precio inicial por ubicación; algunos servicios pueden conllevar tarifas adicionales basados en el uso" [D] |
| **Moneda de las tarifas de Deliverect México** | La página declara `currencySymbol` `$` sin etiqueta, y su única cifra con moneda explícita es el alta "MXN 3,999". Cambia el costo del proyecto por un factor de ~18 |
| **Lo que cobran los POS mexicanos por esta capacidad** | Los cuatro (Parrot, Soft Restaurant, Otter, Deliverect) son *quote-gated*. Las cifras de 1,800 / 2,100 / 2,800 MXN de Parrot que circulan **no se sostienen**: rastrean a una sola página de marketing de un competidor, sin fuente, que se contradice sola (el texto dice mensual y su tabla rotula "/año"), y el 2,100 no aparece en ninguna fuente — está interpolado. Las páginas de precios de Parrot dan 404 |

### 2.3 Contradicciones entre fuentes, sin resolver

Estas importan porque un spec que escoja la versión equivocada diseña contra un contrato que no existe.

| Contradicción | Las dos versiones |
| --- | --- |
| **Ventana de aceptación de Rappi** | El portal de desarrollador dice **6 minutos** desde `SENT`; el FAQ de comercios de Rappi México dice **4 minutos**. Diseñar contra 4 |
| **Calendario de reintentos de Uber** | La referencia del webhook publica 1 s / 2 s / 4 s; la guía de webhooks publica 10 s / 30 s / 60 s / 120 s. Ambas con tope de **7 entregas totales**. El diseño no debe depender de la diferencia |
| **Cobertura de Deliverect en México** | Una lectura encontró páginas `en-mx` de Rappi y DiDi Food con bandera de México e inyección al POS [D]; otra investigación **no encontró evidencia** de esa cobertura. Verificar antes de citarlo |
| **Cuboh y Rappi** | El sitio de comerciantes de Rappi México nombra a Cuboh como integrador puente; el HTML de `cuboh.com/integrations` tiene **cero** coincidencias de "rappi", "didi" y "mexico" (medido). Alguien de los dos está desactualizado |
| **Enum de `report_type` de Uber** | Una fuente lista 10 valores; solo **tres** aparecen en contenido oficial indexado: `PAYMENT_DETAILS_REPORT`, `ORDER_ERRORS_MENU_ITEM_REPORT` y `FINANCE_SUMMARY_REPORT`. Los otros siete salen de un catálogo de terceros generado para agentes y **no deben tratarse como contrato**. La lista tampoco parece exhaustiva: Uber Eats Manager México ofrece un "Resumen de pagos" sin contraparte en ella |

### 2.4 Lo que no se pudo leer

- **El portal autenticado de DiDi**: el formulario real de Qualifications, la creación de app, el sandbox y el monitoreo. Solo se leyó la documentación que los describe.
- **La referencia de API de Uber directo**: `developer.uber.com` devuelve 404 a `curl` (incluido `/sitemap.xml`) y renderiza por JS. Todo lo de Uber viene de páginas indexadas y de la guía, no del OpenAPI.
- **Estado real de mipOS**: `integrations.mipos.shop` y `sandbox.mipos.shop` resuelven a 5.196.129.59 y agotan el tiempo de conexión en 80 y 443 (medido 2026-09-06); `mipos.mx` sirve una página estacionada de 114 bytes. No se distinguió "apagado" de "filtrado a IPs de socios".
- **Ningún caso público de un restaurante independiente con acceso directo a la API de Uber Eats.** Se buscó expresamente. Ausencia de evidencia, no evidencia de imposibilidad.

---

## 3. Plataforma por plataforma

### 3.1 Uber Eats (30% de comisión medida)

**Acceso**

| Requisito | Nivel | Fuente |
| --- | --- | --- |
| Sandbox self-serve: crear app "Testing" en el Developer Dashboard, scopes automáticos solo para dominios de sandbox. Las tiendas de prueba se piden a Integration Tech Support | [D] | [sandbox](https://developer.uber.com/docs/eats/guides/sandbox) |
| "Sandbox access does NOT guarantee production access — all applications moving to production are reviewed in detail" | [D] | [sandbox](https://developer.uber.com/docs/eats/guides/sandbox) |
| Prerrequisitos antes de desarrollar: cuenta de desarrollador, **NDA y API licensing agreement firmados**, y "Speak with your Uber Eats partner manager" | [D] | [getting-started](https://developer.uber.com/docs/eats/guides/getting-started) |
| Producción en cuatro pasos: ticket para agendar *integration verification* con pruebas end-to-end conjuntas, app de producción aparte con cuenta distinta, ticket con el `client_id` para whitelistear scopes, y avisar la primera tienda con una semana de anticipación | [D] | [going-live](https://developer.uber.com/docs/eats/guides/going-live) |
| El contrato de *POS Provider* obliga a "provide its POS integration services for **all Merchants** that use Provider's services" (§4.3.3), auditoría anual de instalaciones y sistemas (§3.2.3), no sustituir la tableta de Uber como vía de aceptación sin aprobación previa (§4.3.2), y término de 1 año con rescisión a un año de aviso (§6.1) | [D] | [PDF del acuerdo, copia 2022](https://capsource-bucket.s3.us-west-2.amazonaws.com/wp-content/uploads/2022/04/08121506/Uber_Eats_POS_Provider-NDA__API_Agreement.pdf) |

**El hallazgo que decide:** no hay un umbral de volumen que reprobar — hay una **figura jurídica en la que El Gato Bobah no encaja**. Firmar como *Provider* obliga a dar servicio de integración POS a terceros y a aceptar auditorías. Y el §4.3.2 choca de frente con el objetivo (2): prohíbe sustituir la tableta de Uber como vía de aceptación sin permiso previo. **[I]** Un local que se integra a sí mismo no es la contraparte que ese contrato describe.

Uber sí reconoce por escrito la categoría de "restaurant partners with self-built integrations" en su ayuda a comercios [D — [help.uber.com](https://help.uber.com/en/merchants-and-restaurants/article/how-can-i-get-pos-integrated-with-uber-eats?nodeId=e7b75f85-322a-4409-bbf2-f85433e22a01)]. Esa categoría existe; que se conceda en México a un local de 1-2 pedidos al día no está documentado ni desmentido.

**Qué entrega (si se concediera)**

| Pieza | Detalle | Nivel |
| --- | --- | --- |
| Auth | OAuth2. `client_credentials` para operación normal; `authorization_code` solo para descubrimiento de tienda y activación. Token de **30 días** (2,592,000 s). Scopes: `eats.store`, `eats.store.status.write`, `eats.order`, `eats.store.orders.read`, `eats.report`, `eats.pos_provisioning` | [D] |
| Pedidos | Webhook `orders.notification` con `event_id`, `meta.resource_id` y `resource_href`; el detalle se baja con `GET /v2/eats/order/{id}`. Aceptar o rechazar con `POST /accept_pos_order` o `/deny_pos_order` | [D] |
| **Folio** | `id` = Order UUID completo. `display_id` = los **últimos 5 caracteres del UUID** (ej. `BC953`). `external_reference_id` = folio propio que se puede mandar al aceptar | [D] |
| Dinero del pedido | `charges` con `marketplace_fee_due_to_uber`, `delivery_fee`, `tip`, `total_promo_applied`; `promotions[]` con `promo_type` y `promo_funding_splits` (qué fondeador pagó cuánto); items con `base_non_loyalty_unit_price` | [D] |
| Reportes | `POST /v1/eats/report`, asíncrono (devuelve `workflow_id`), CSV, scope `eats.report`. Tope de 50 tiendas por request, 60 req/min, rango máximo de 30 días | [D] |

**Consecuencia para el diseño [I]:** el folio corto que ve el personal es **derivable** del UUID. Guardando el UUID se tiene también el folio humano; guardando solo el código de 5 no se puede reconstruir el UUID. Cualquier columna nueva guarda el identificador completo.

**Qué NO entrega**

- **El dinero final.** Reembolsos y chargebacks "appear only in Reporting — not Orders API", con hasta 72 h para asentar y disputas a 30 días [D — [reporting](https://developer.uber.com/docs/eats/guides/reporting)]. Tomar el webhook como verdad para el corte de caja reporta ingreso que el negocio no tuvo — exactamente lo que prohíbe el principio III.
- **Un enum de reportes confiable.** Ver §2.3.
- **Autoservicio de activación de tienda.** Los tres flujos documentados (pre-integración por Uber, activación por soporte a petición del comercio, activación iniciada por el partner) pasan todos por Uber o por un partner [D].

### 3.2 Rappi (20% de comisión medida — la más barata y la más abierta)

**Acceso**

| Requisito | Nivel | Fuente |
| --- | --- | --- |
| Documentación **pública y completa**, legible sin login, en es/en/pt. El portal es `dev-portal.rappi.com` (`developers.rappi.com` es NXDOMAIN, medido 2026-09-06 contra 8.8.8.8) | [D] | [dev-portal.rappi.com](https://dev-portal.rappi.com/en/api-reference/) |
| Credenciales **no** self-serve: "integrating with our API requires direct contact with our team". Alta como Rappi Aliado vía el punto de contacto comercial; credenciales de desarrollo tras aprobación | [D] | [api-reference](https://dev-portal.rappi.com/en/api-reference/) |
| Incluso el "Self-Onboarding" exige un *Technical Account Manager* que cree la entidad de integración y el `clientId` en Auth0 a mano, y registre el `redirect_uri` | [D] | [self-onboarding](https://dev-portal.rappi.com/en/self-onboarding/) |
| Rappi México reconoce por escrito la vía directa: "Si prefieres encargarte tú mismo de la integración, contamos con una potente API" | [D] | [merchants.rappi.com](https://merchants.rappi.com/es-mx/que-ofrecemos/sistema-pos) |
| Sandbox documentado en `https://api.dev.rappi.com` (resuelve y responde HTTP, medido 2026-09-06), pero sin credenciales no se puede probar nada antes del alta | [D] | doc de Rappi |

**Qué entrega**

| Pieza | Detalle | Nivel |
| --- | --- | --- |
| Auth | OAuth2 client credentials. `POST /restaurants/auth/v1/token/login/integrations` con `{client_id, client_secret}`. El token va en header **`x-authorization: Bearer …`**, no en `Authorization`. Vigencia de **1 semana** | [D] |
| Dominios MX | Nuevo `https://api.rappi.com.mx`; legado `https://services.mxgrability.rappi.com`. Son dos hosts, no uno: usar el legado para un endpoint nuevo devuelve 401 por audience equivocada | [D] |
| Pedidos | `GET orders`, `GET stores/{id}/orders`, `GET orders/status/sent`, `PUT …/take`, `PUT …/take` con tiempo de cocción, `PUT …/reject`, `POST …/ready-for-pickup`. Solo `take` y `reject` son obligatorios | [D] |
| Webhooks | Once eventos, incluido `NEW_ORDER`, que "is going to send the same information that we can get from the getOrders endpoint" — o sea, **el pedido completo, no un puntero** | [D] |
| **Conciliación** | Módulo Financial en `/restaurants/finance/v2/`, con login propio. `GET /stores/{store_id}/orders` devuelve `order_id` **junto con** `payment_id`, y acepta filtros `order_ids:eq`, `payment_id:eq`. Dado un depósito se listan los pedidos que lo formaron | [D] |
| Descuentos | Partidos en `amount_by_rappi` y `amount_by_partner` | [D] |

**Es la única de las tres donde bajar el estado de cuenta no es scraping [I].**

**Advertencias de diseño**

- **`GET orders` es destructivo:** "You can only retrieve new orders once. After you retrieve these orders, the system changes their status from READY to SENT" [D]. Un timeout de red *después* de que Rappi cambió el estado pierde el pedido en silencio. `GET orders/status/sent` es la red de seguridad y hay que barrerla al arrancar.
- **La firma del webhook es OPCIONAL y por omisión no existe:** el campo `secret` "if omitted, no signature will be sent" [D]. El endpoint del POS es público con TLS; sin secret, cualquiera que adivine la URL inyecta pedidos falsos. El spec debe exigirlo, no dejarlo opcional como Rappi.
- **Estándares operativos vinculantes:** un login por semana, 45 s entre requests de pedidos, y "integrations that fail to meet a 98% success rate… can be subject to revoked API access, removal, or disabling of restaurant stores" [D].

**Qué NO entrega**

- **La equivalencia declarada entre el `order_id` de pedidos y el de Financial.** Ninguna de las 19 páginas de guía ni de las 15 de referencia lo declara; "How to Reconcile" solo nombra `payment_id` como llave de agrupación. La evidencia a favor es circunstancial pero consistente. **La diferencia de tipo (string vs integer) NO es evidencia en contra**: Rappi tipa ese mismo id de las dos formas dentro de su propia superficie de pedidos (integer en `ORDER_RT_TRACKING`, en `handoff` y en `bag-drink-confirmation`; string en `getOrders`). Verificación empírica obligatoria antes de escribir la migración.
- **Las claves de comisión de México.** "The billing object is not a fixed schema… the set of keys therefore varies by country". Solo se listan las de Brasil, y la propia doc remite a un contacto humano de settlement [D]. El mapeo no se diseña por adelantado: se descubre leyendo una respuesta real.
- **Claridad de versión:** la página Rests API pone de ejemplo `finance/v1` mientras toda la referencia documenta `finance/v2`. Sin resolver.

### 3.3 DiDi Food (30% de comisión medida — la más opaca comercialmente, la mejor documentada técnicamente)

**Acceso**

| Requisito | Nivel |
| --- | --- |
| La documentación completa es **pública**: el árbol de 60+ nodos responde 200 a una petición anónima con la cabecera `Origin: https://developer.didi-food.com`. Lo cerrado es el portal (crear apps, credenciales, sandbox), no la doc | [D] |
| El proceso arranca con negociación comercial donde DiDi "check if you are a store partner or a POS, **how many stores you have in operation** and how many stores you are able to integrate… **numbers of orders per day**" | [D] |
| Sigue NDA por DocuSign firmado por el dueño o alguien con poder notarial, registro de cuenta y auditoría | [D] |
| "Partners need to sign the NDA and complete the qualification. New stores need to have passed DiDi's audit process and have the commercial and cooperation agreement in signed status" | [D] |
| El RFC sustituye al DUNS en el Business Profile, pero el *Technology Integration Agreement* pide Acta Constitutiva, RFC, Poder Notarial e ID del representante legal — **los cuatro son de persona moral** | [D] |
| Una tienda solo puede estar ligada a **UNA** app de producción | [D] |

**El filtro del primer paso mide literalmente número de tiendas y pedidos por día.** Un local con 1-2 pedidos diarios entra ahí en la peor posición posible. No hay corte publicado ni prohibición de categoría: el restaurante individual sí es categoría reconocida ("store partner") [D].

**Qué entrega (si se concediera)**

| Pieza | Detalle | Nivel |
| --- | --- | --- |
| Host | `https://openapi.didi-food.com`, vivo. Hay **OpenAPI 3.0 descargable sin login** (auth, order, shop, item, image): el cliente Go se puede generar, no adivinar | [D] |
| Auth | `app_id` + `app_secret` → `auth_token` **por tienda** vía `GET /v1/auth/authtoken/get`, que va como parámetro en todos los demás endpoints. Vigencia 30 días, refresh máximo 1 cada 30 s | [D] |
| Pedidos | Webhook `orderNew` con la **estructura completa** (misma que `GET /v1/order/order/detail`): `order_id` (entero de 64 bits, 19 dígitos), `order_index` (folio corto), `status`, `order_items`, `price`, `pay_type`, `delivery_type` | [D] |
| Enlace de tienda | `POST /v1/auth/authorizationpage/getUrl` devuelve una URL donde el manager de la tienda autoriza con un clic. **Este paso sí es autoservicio** una vez que existe la app | [D] |
| Conciliación | `GET /v1/finance/finance/getBillList`: reporte semanal lunes a domingo, 3 meses de historia, con `order_id`, `commission_base_amount`, `commission_rate`, `commission_amount`, `settlement_amount`, `expect_settle_ts`, `payment_method`. Incluye los códigos **40160 "ISR - retained (Persona física México)"** y **40164 "VAT - retained (Persona física México)"** | [D] |
| México | Soportado de primera clase: código MX, tienda de prueba con dirección en la CDMX, app DiDiStore en `didi-food.com/es-MX/store` | [D] |

**Qué NO entrega**

- **`getBillList` es la pieza más cerrada de todas y la propia DiDi la declara no autoritativa:** exige whitelist interna *además* de un "cooperation contract" firmado, y advierte "this is a pilot version for reference only… if you find any difference, please use the data from report downloaded from the DiDiStore APP as the correct one" [D]. **[I]** Conciliar automáticamente contra una fuente que la propia plataforma declara secundaria es construir un número que nadie puede auditar. El reporte de la app es la fuente buena — y ese se baja hoy, sin API.
- **`getBillList` no aparece en el OpenAPI público.**
- **Obligaciones operativas duras:** el webhook tiene timeout de **6 segundos** y debe responder JSON con `errno 0`; el pedido debe confirmarse en menos de **5 minutos** o se cancela solo [D].
- **Firma sin protección de replay:** `MD5(rawBody + app_secret)` contra la cabecera `didi-header-sign`. **No hay timestamp ni nonce dentro de la firma** [D]. La defensa contra reenvío hay que construirla del lado del POS.
- **Precios en la unidad más chica:** 123.45 pesos llega como `12345` [D]. El POS maneja `float64` en pesos con `Round2` ([server/internal/domain/money.go](../server/internal/domain/money.go)): toda esa frontera es fuente de error de redondeo y se aísla y se prueba.
- **Documentación solo en inglés**, aunque el país esté soportado.

---

## 4. La vía middleware

La columna que decide es **"expone el pedido hacia un POS de terceros"**. Sin eso, el middleware resuelve el problema de otro negocio.

| Proveedor | Cubre UE + Rappi + DiDi en MX | Expone el pedido a un POS de terceros | Alta self-serve | Precio público MX | Veredicto |
| --- | --- | --- | --- | --- | --- |
| **Deliverect** | Sí [D], **con una lectura que lo contradice** (§2.3) | **Sí**, pero por programa de partners con certificación | **No** | Tiers en `en-mx/pricing` + "MXN 3,999" de alta [D], moneda de los tiers sin confirmar | Único candidato con precio de lista |
| **Otter (ex Hubster)** | Rappi y DiDi evidenciados **solo en un artículo de ayuda sobre impuestos**; la página `es-mx/integraciones` no los menciona (cero coincidencias en el HTML) | **Sí**, la documentación más completa de las dos | **No**: "Registration is manual. Contact your Account Representative" | No. La página de precios renderiza el bloque de planes **vacío** | Candidato, sin precio |
| **Cuboh** | **No.** Cero coincidencias de "rappi", "didi" y "mexico" en su HTML (medido) — aunque Rappi MX lo nombra como puente (§2.3) | Sí | Sí, mes a mes, $119/$169/$229 USD | Sí, pero de EE.UU. | Descartado por canales |
| **Chowly** | **No.** Plataforma de EE.UU., contrato bajo ley de Illinois con foro en Chicago, precios en USD. Anuncia "150+ delivery apps" sin catálogo público, así que la ausencia de Rappi/DiDi en su marketing no lo prueba por sí sola | Sí | No | Piso contractual de "$35 per month minimum fee per location" [D, T&C act. 13-ene-2026] | Descartado por geografía |
| **GetOrder** | Nombrado por Rappi México como puente; publica páginas de DiDi Food | Contra lista de POS soportados | No | No | Sin investigar a fondo |
| **Ordatic** | — | — | — | — | **Su dominio no resuelve** (sin registros A/AAAA; `www` NXDOMAIN, medido 2026-09-06) |
| **UrbanPiper (ex Ordermark)** | Sin evidencia de Rappi ni DiDi en MX | Sí | No | No | Ordermark dejó de existir como producto (venta a UrbanPiper en 2023, retiro del nombre en ene-2025) [R] |
| **KitchenHub** | No: canales de EE.UU. | "Our API is not publicly open" | No | No | Descartado |
| **Nash** | Categoría equivocada: orquesta mensajería, no agrega marketplaces | — | — | — | Descartado |
| **Vita Mojo** | Reino Unido, grupos de 10 a 100+ sitios | — | — | — | Descartado |
| **mipOS** | **Sí, los tres + iFood/SinDelantal** [D] | **Sí**: `POST /api/oauth/register` con rol POS, sandbox, webhook de pedido nuevo, `external_id` = id en la plataforma y `order_number` | **Sí** | — | **Infraestructura no responde** (timeout en 80 y 443, medido 2026-09-06) |

**Lo que hay que saber de Deliverect antes de contarlo como opción**

| Dato | Nivel |
| --- | --- |
| Su payload al POS trae `channelOrderId` ("the full unique ID from the delivery channel") y `channelOrderDisplayId` ("typically a shortened version"), distintos del `_id` interno. Los tres marcados "Always present" | [D] — verificado en `developers.deliverect.com/page/glossary-pos-orders.md` |
| **`channelOrderId` es único solo junto con `channel`, y solo "for 48h after pickup"**. Como llave de conciliación hay que guardar canal + folio, y no asumir unicidad global ni perpetua | [D] |
| **`channelOrderDisplayId` NO es único** (4-5 alfanuméricos, hasta 12): sirve para mostrar, jamás como llave | [D] |
| Acceso: formulario de partner, respuesta en ~14 días hábiles, cuenta de staging, y "a series of certification calls with the Deliverect team" (promedio dos) antes de credenciales de producción | [D] |
| ToS (los mismos para México, contraparte Deliverect NV, Bélgica): alta **única y fija**, suscripción fija **por cada local**, y tarifa transaccional. Periodos de 30 días, 90 días, 6 o 12 meses con **renovación automática** y aviso de cancelación de 15 a 90 días | [D] |
| El campo `number_of_locations` con opciones 1-2 / 3-5 / … que aparece en el HTML de "become a partner" **no pertenece a ese formulario**: está en una tabla de ruteo de leads ligada a otros 14 formularios, y para México esa tabla ni siquiera segmenta por locales. **No hay evidencia de que acepten a un solo local** | [D, medido] |

**[I] Conclusión de la sección:** la vía middleware no es una compra, es otra solicitud de acceso — el filtro no desaparece, se mueve de la plataforma al integrador, y el solicitante sería un POS con un cliente y ~45 pedidos de plataforma al mes. A los precios reportados (desde ~MXN 999/mes según una entrevista [R], o los tiers de la página de Deliverect [D]) el costo por pedido integrado supera los MXN 20 antes de contar el trabajo de construir y certificar la integración.

---

## 5. Cómo lo hacen los POS mexicanos

| POS | Cómo se conecta | Nivel |
| --- | --- | --- |
| **Soft Restaurant (NationalSoft)** — el más extendido, +40 mil clientes | **Dos caminos, no uno.** Integración **directa** con Uber Eats, Rappi y DiDi Food, en su propia pestaña "Plataformas de Delivery", con manual propio de integración de Uber Eats; y **por separado**, channel managers (Ordatic, Deliverect, FoodBot, Hubster) | [D] — [softrestaurant.com/nuestra-esencia/integraciones](https://softrestaurant.com/nuestra-esencia/integraciones), consultada 2026-09-06 |
| **Parrot** | Vende integración nativa con las tres dentro de su paquete base; su contrato lista "Parrot Delivery Apps (Rappi, DidiFood, UberEats)". Marca como costo adicional estaciones de mesero, marcas adicionales, timbres de facturación y Parrot Pedidos Online. **Sin precio público** (páginas de precios en 404) | [D] para el alcance, ninguno para el precio |
| **Otros (Toteat, Bistrosoft, Fudo)** | No documentan públicamente su proceso de certificación | — |

**Corrección importante respecto a una versión anterior de este análisis:** *no* es cierto que "ni el incumbente más grande va directo". Soft Restaurant sí va directo con exactamente las tres plataformas que le importan a este negocio. Lo que su caso demuestra es lo contrario de lo que parecía: **ambas rutas están mediadas por un acuerdo comercial de integrador**, y ninguna es acceso público. El camino directo existe y funciona — para quien entra al programa de partners.

**Ningún POS mexicano documenta públicamente el proceso de certificación que tuvo que pasar.** Las afirmaciones de "mejor integración de México" y "99.9% de aceptación" son autopublicadas. La única descripción pública de un proceso de certificación la publican las plataformas, no los POS.

---

## 6. La forma técnica que tendría que tener nuestro POS

Esto solo aplica si alguna puerta se abre. Se escribe ahora para que el spec no tenga que redescubrirlo, y porque varias piezas de §7 se justifican con esto.

### 6.1 Los relojes: por qué esto es un servicio 24/7, no una bandeja de pedidos

| Plataforma | Ventana para aceptar | Timeout del webhook | Consecuencia de no responder |
| --- | --- | --- | --- |
| Uber Eats | **11.5 min**, con `POST accept_pos_order` / `deny_pos_order`. A los **90 s** sin respuesta dispara una llamada robotizada al local | 200 con cuerpo vacío | Se cancela |
| Rappi | **4 min** según el FAQ de comercios MX; 6 min según el portal de desarrollador (§2.3). Diseñar contra 4 | No documentado | Pasa a `TIMEOUT` |
| DiDi Food | **5 min** (`/v1/order/order/confirm`) | **6 segundos**, respuesta JSON con `errno 0` | Se cancela |

**[I] Ningún camino que dependa de que un operador esté mirando la tableta cabe en esa ventana en hora pico.** El spec tendría que definir las reglas bajo las que el backend acepta solo, y qué hace cuando no se cumplen.

**Los estándares de disponibilidad no perdonan el volumen bajo.** Uber publica 99.9% de *injection success rate* como línea base, con revocación de acceso y deshabilitación de tiendas para quien no sostenga 99% [D]; Rappi publica el mismo mecanismo a 98% [D]. **[I]** Con ~15 pedidos al mes por plataforma, **un solo pedido perdido deja la métrica de ese mes en 93%**. La VM chica de GCP deja de ser un detalle de infraestructura y pasa a ser un requisito de arquitectura.

### 6.2 El handler del webhook, en orden

1. `io.ReadAll` sobre un `http.MaxBytesReader`. **La firma se valida sobre los bytes crudos, antes de deserializar** — Uber usa HMAC-SHA256 hex minúsculas con el `client_secret`; DiDi usa `MD5(rawBody + app_secret)`; Rappi HMAC-SHA256 solo si se configuró `secret` [D].
2. Firma inválida → **400 y se descarta, nunca 5xx**: un 5xx dispara reintentos de algo que jamás va a validar [D].
3. **Responder 200 NO es aceptar el pedido**: son dos llamadas distintas. El handler acusa recibo en milisegundos y encola; la aceptación es un job con su propio reintento y su propio presupuesto de tiempo, menor al SLA [D].
4. **Idempotencia con dos llaves**: `unique (plataforma, event_id)` sobre una tabla de eventos crudos, y `unique (plataforma, order_id remoto)` sobre `orders`. El reintento se resuelve **por lectura**: si el evento ya está procesado, 200 vacío y se acaba. El patrón exacto ya existe en el repo — índice único **parcial** por `(company_id, client_uuid)` en `orders` y `order_payments`, nullable y con `where`, en [server/migrations/0057_cobro_idempotente.sql](../server/migrations/0057_cobro_idempotente.sql).
5. **Los duplicados son el caso normal, no la excepción**: Uber reenvía hasta **7 entregas totales** del mismo evento, y un 200 que llega tarde cuenta como fallo [D]. No hace falta estar caído: basta con tardar.
6. **Ninguna de las plataformas firma un timestamp ni publica una ventana de tolerancia de reloj.** No hay anti-replay del lado de la plataforma: la defensa **es** la llave de idempotencia [D].

### 6.3 El webhook de Uber es un puntero, no el pedido

Trae `event_id`, `meta.resource_id`, `resource_href`; el detalle se baja con un `GET` aparte [D]. Dos consecuencias que muerden:

- **La verdad es la lectura, no el cuerpo del webhook.** Un reintento tardío devuelve un estado *más nuevo* — el mismo evento puede leerse ya cancelado.
- **Si el `GET` falla no se puede aceptar, con el reloj corriendo.** Ese `GET` necesita su propio reintento con presupuesto menor al SLA.

Rappi y DiDi mandan el pedido completo, así que en esas dos el contrato interno es distinto. **[I]** El conector no puede tener un solo modelo de "llegó un pedido": son dos formas.

### 6.4 Recuperación tras una caída, en tres pasos y en orden

1. **Mientras está caído, la tienda se marca offline** en la plataforma. Uber expone `POST /eats/stores/{id}/status` y su guía pide usarlo "cuando no se pueden cumplir pedidos, por ejemplo por problemas de conectividad" [D].
2. **Al volver NO se esperan los reintentos**: para cuando la API responde, las 7 entregas ya se agotaron. Se barren los pedidos creados en la ventana de caída (`GET` de órdenes creadas por tienda en Uber; `GET orders/status/sent` en Rappi) y se reconcilia contra la tabla de eventos.
3. Recién ahí se vuelve a poner online.

**El barrido no es opcional: es el único camino por el que un pedido perdido reaparece [I].** Sin él, cada deploy y cada reinicio del contenedor es un hueco silencioso de pedidos que nunca se van a saber que existieron.

### 6.5 Estado de la tienda: la doc SÍ dice quién gana

Corrige un supuesto que parecía razonable y es falso.

| Regla | Nivel |
| --- | --- |
| `GET /eats/stores/{id}/status` devuelve `offlineReason` con cuatro valores que identifican al actor: `OUT_OF_MENU_HOURS`, `INVISIBLE`, `PAUSED_BY_UBER`, `PAUSED_BY_RESTAURANT` | [D] |
| El pausado automático (pedidos no aceptados seguidos, dispositivo offline más de 1 h, auto-reanudación a las 6:00 am del día siguiente) lo hace **Uber**, no el dueño: es `PAUSED_BY_UBER` | [D] |
| "Holiday hours override store hours on the specified date(s)" | [D] |
| **Con un incidente detectado por Uber, `SetStoreStatus` responde 400 Bad Request**: "Uber has detected your order fulfillment application is experiencing an outage. Store status cannot be updated at this time" (`INCIDENT_DETECTED_ON_ORDER_MANAGEMENT_APPLICATION`, changelog 2024-05-27) | [D] |

**Dos consecuencias que rompen el diseño ingenuo [I]:**

- Un lazo de convergencia que "guarda el estado deseado local y escribe hasta lograrlo" **martillea el endpoint contra un error permanente** hasta que Uber libere el incidente. El 400 es estado esperado, no error a reintentar.
- `PAUSED_BY_RESTAURANT` cubre por igual la pausa del dueño desde la app y la del propio POS por API: **el espejo no puede reconocer su propia escritura**. La regla "un offline local gana sobre un online remoto" no es implementable como se enunciaba.

### 6.6 Menú y disponibilidad

| Regla | Nivel |
| --- | --- |
| El POS es fuente de verdad del catálogo, pero **no del momento**: Uber manda `store.menu_refresh_request` y el POS sube el **menú completo** por `PUT` (upsert, no parches) | [D] |
| Un producto agotado **no** se resuelve editando el menú: es un canal aparte. DoorDash lo dice explícito y expone `PUT …/items/status`; Uber lo maneja como suspensión del ítem. La disponibilidad tiene que estar en **push y en pull** | [D] |
| El menú de plataforma **no es el del mostrador**: en el POS se vende "Arma tu Crepa" y en la plataforma cada crepa por sabor, a propósito | Ya declarado en [.specify/memory/constitution.md](../.specify/memory/constitution.md), principio VIII |
| **No se verificó** si el modelo de modificadores de las plataformas admite ese mapeo uno-a-varios. Si no lo admite, la sincronización de menú es un proyecto aparte del webhook | Hueco |

### 6.7 Reconciliación: dos canales, dos estados del dinero

El pedido dice **lo que se vendió**; el depósito lo determinan la comisión, los reembolsos, los ajustes y las retenciones, que aparecen en **otro documento y con otra periodicidad** [D]. Uber pide marcar como PROVISIONAL todo lo que venga en tiempo real, bajar el reporte de madrugada y sobrescribir lo provisional con lo asentado.

Esto empata con lo ya medido en el negocio y documentado en [docs/plataformas-digitales.md](plataformas-digitales.md): comisión por pedido, depósito semanal, retenciones certificadas por mes.

**[I] El esquema necesita, desde el día uno, dos estados del dinero: provisional y asentado.** Y lo necesita aunque la captura siga siendo manual (§7).

### 6.8 Los cuatro modos de fallo que se descubren tarde

| # | Fallo | Qué exige |
| --- | --- | --- |
| 1 | **Cancelación después de aceptado** (Uber tiene webhook `orders.cancel`; Rappi mueve a `TIMEOUT` sola) | Un estado local "cancelado por la plataforma" **distinto** de "cancelado por el operador", y registrar la merma: el pedido ya está en cocina |
| 2 | **Cambio de ítems post-aceptación** (Uber expone un `PATCH` sobre el carrito) | Si el POS asume carrito inmutable, el ticket de cocina y lo que se cobra dejan de coincidir **en silencio** |
| 3 | **Duplicado** | Cubierto por la llave de evento, pero **solo si se aplica antes de tocar el pedido** |
| 4 | **Pedido con la caja cerrada** | Es el específico de este repo. El cobro cuelga de la sesión de caja y del folio por turno ([0061](../server/migrations/0061_folio_por_turno.sql), [0062](../server/migrations/0062_fecha_de_venta_del_reloj.sql)). Un pedido de plataforma que entra a las 23:50 o sin sesión abierta **no tiene dónde aterrizar, o aterriza en el turno equivocado y descuadra el corte del día siguiente** |

El #4 no es una decisión que se pueda posponer al conector: se decide en §7.

### 6.9 PII

La PII llega quiera uno o no: nombre del cliente, teléfono **anonimizado** más `phone_code` (los dos hacen falta para que la llamada conecte), dirección con lat/long, y notas que la propia doc admite que pueden incluir alergias [D]. El teléfono anonimizado **caduca con el pedido**: persistirlo "por si acaso" no sirve para nada y sí es dato personal.

**[I]** El repo ya prohíbe `customerName` y notas en logs salvo debug o 5xx (principio V). La tabla de eventos crudos que hace posible replayar una integración rota **es PII persistida**: necesita retención definida y quedar fuera de cualquier export.

---

## 7. Qué se puede hacer HOY, y qué NO cerrar

**Esta es la sección que importa antes de abrir cualquier otro spec.** El negocio va a construir otras cosas primero; lo que sigue son las decisiones que, tomadas mal ahora, cuestan una migración de datos en producción después — el criterio del principio VIII: *¿esto se puede agregar después al mismo costo?*

### 7.1 Lo que se puede hacer hoy sin ninguna API

| Acción | Qué objetivo cierra | Costo | Depende de |
| --- | --- | --- | --- |
| **Campo de folio de plataforma en la captura manual** de una venta de plataforma | **(1) completo** | Una columna y un input | Nadie |
| **Importar el CSV de pagos** que ya se descarga del portal: Payment Details Report en Uber Eats Manager [D], reporte de la app DiDiStore (que la propia DiDi declara **autoritativo por encima de su API** [D]), estado de cuenta del portal de comercios de Rappi (**no verificado** que sea descargable) | **(3) en buena parte** | Un parser y una pantalla de conciliación | Nadie |
| **Registrar la comisión por pedido** al capturar, tomada del documento de pago | Cruza la puerta "cuánto deja cada plataforma" de la constitución | Dos columnas | Nadie |
| **Marcar la venta de plataforma como provisional hasta conciliar** | Cumple el principio III sobre datos que aún no son ingreso final | Una columna de estado | Nadie |

**[I] El detalle que hace que la captura manual sea mejor de lo que parece:** el problema no resuelto de las tres plataformas es si el identificador de la API empata con el del documento de pago (§2.1). **La captura manual esquiva ese problema por construcción**, porque la persona copia el identificador que aparece **en el documento contra el que hay que conciliar**. Regla operativa: se teclea el folio *tal como aparece en el reporte de pago*, sin normalizar, sin recortar y sin interpretarlo.

### 7.2 Decisiones de esquema que cierran la puerta

Lo que va en la columna "cuesta después" es lo que **no se puede recuperar**: un hecho que nunca se registró o una fila que ya se repartió.

| Decisión | Qué la cerraría | Qué dejar abierto | Cuesta después |
| --- | --- | --- | --- |
| **Folio de plataforma en `orders`** | No tenerlo, o tenerlo como entero, o guardar solo el código corto | Columna **texto**, nullable, más `delivery_platform_id` que ya existe. Nunca un entero: Uber es UUID, DiDi es entero de 19 dígitos, Rappi es ambiguo | Irrecuperable: el pasado se queda sin folio para siempre |
| **Guardar el identificador COMPLETO** | Guardar los 5 caracteres que ve el personal | El UUID de Uber (los 5 se derivan de él); el `order_id` de 19 dígitos de DiDi, **no** el `order_index`, que se repite por tienda y por día | Irrecuperable: de los 5 no se reconstruye el UUID |
| **Unicidad del folio** | Un índice único global por folio | Índice único **parcial** por `(company_id, delivery_platform_id, folio) where folio is not null`, copiando [0057](../server/migrations/0057_cobro_idempotente.sql). El folio de Deliverect es único **solo junto con el canal y solo 48 h** [D]; el `channelOrderDisplayId` **no es único** [D] | Barato de agregar, caro de quitar si ya rechazó capturas legítimas |
| **Referencia del depósito** | No guardarla | Columna para el `payment_id` de Rappi / `Payout reference ID` de Uber. Es lo que convierte "conciliar por monto y fecha" (que empata mal) en una unión exacta | Irrecuperable pasados los 3 meses de historia que expone Rappi y los 31 días del reporte de Uber |
| **Comisión como snapshot** | Calcularla con un porcentaje configurado ("20% de Rappi") en lugar de guardar el monto real del documento | Monto y tasa **por pedido**, copiados del documento. Las tasas cambian y las promociones las alteran | Irrecuperable: recalcular el pasado con la tasa de hoy reescribe la historia |
| **Quién financió el descuento** | Registrar solo `discount_total` (que hoy existe y siempre vale cero) | Una columna para la parte que puso la plataforma. Rappi la parte en `amount_by_rappi` / `amount_by_partner`; Uber en `promo_funding_splits` [D] | Irrecuperable, y sin ella **cada promoción se registra como pérdida propia del restaurante** |
| **Dos estados del dinero** | Un solo total que se da por final al capturar | Estado provisional / asentado y fecha de asentamiento. Aplica igual a la captura manual | Barato como columna, caro como corrección de reportes ya emitidos |
| **Pedido de plataforma vs. sesión de caja** | Asumir que toda venta requiere sesión de caja abierta y folio de turno ([0061](../server/migrations/0061_folio_por_turno.sql)) | **Decidirlo por escrito antes del próximo spec de caja**: si un pedido de plataforma se registra contra la fecha de venta sin turno, o si exige sesión abierta. Hoy la captura manual lo esconde porque siempre hay alguien capturando dentro de un turno | Cambia el esquema y el corte. Descubrirlo con el conector encima es peor |
| **Producto de plataforma ≠ producto del POS** | Cualquier cosa que asuma que un producto del catálogo es un producto de la plataforma | [product_platform_prices](../server/migrations/0037_platform_prices.sql) ya está por `(producto, plataforma)`: la llave correcta existe. **No construir** el nombre propio ni el estado activo hoy — pero **no tomar ninguna decisión que asuma 1:1** | Ya declarado como puerta abierta en la constitución |
| **Llave estable del producto hacia afuera** | Usar como identificador algo que el operador pueda editar | Si algún día se sube menú, la llave que se le da a la plataforma no puede cambiar: si cambia, **el histórico de esa plataforma deja de empatar** | Irrecuperable en el histórico de la plataforma |

### 7.3 Lo que NO hay que construir hoy

Aplicando el principio VI, todo esto se agrega después al mismo costo y no debe entrar a ningún spec ahora:

- Tabla de eventos crudos de webhook (no hay webhooks).
- Módulo de auth OAuth contra ninguna plataforma.
- Barrido de recuperación, healthcheck que apague la tienda, sincronización de menú, canal de disponibilidad.
- Cualquier cliente HTTP contra Uber, Rappi, DiDi o un middleware.
- Pantallas de "pedidos entrantes".

---

## 8. Cuándo conviene abrir este spec

**Hoy no.** No por falta de diseño —§6 está resuelto en lo esencial— sino porque **el primer entregable no sería código, sería un acceso que no depende del equipo**, y el proyecto entero fracasa en ese punto sin dejar nada útil.

Orden sugerido, del que no depende de nadie al que depende de todos:

| Paso | Qué es | Qué desbloquea | Depende de |
| --- | --- | --- | --- |
| **A. Folio a mano** | Campo de folio + comisión + estado provisional en la captura de venta de plataforma (§7.1, §7.2) | Objetivo (1). Y deja el esquema listo para que cualquier automatización posterior solo **llene** columnas que ya existen | Nadie. Cabe en un spec chico |
| **B. Importador de CSV de pagos** | Parser del reporte que ya se descarga, más pantalla de conciliación depósito ↔ pedidos | Buena parte del objetivo (3), **sin API, sin NDA y sin SLA** | Abrir un archivo real de cada plataforma para conocer sus columnas de México (§2.1) |
| **C. Preguntar por el canal publicado** | Llenar el formulario de PDV de Uber Eats México, escribir a `globalsupportapi@didiglobal.com` (DiDi) y al portal de Rappi. Cuesta cero y responde en semanas o nunca | Es lo único que puede cambiar el veredicto de §1. **Nada del plan debe depender de que contesten** | Nadie |
| **D. Spec de integración** | Lo de §6, para **una** plataforma | Objetivo (2) | Que C haya devuelto un acceso concedido **por escrito**, incluido el territorio |

**Cuál plataforma primero, si alguna se abre [I]:** Rappi. Es la única con documentación pública completa, la única con API financiera que une `order_id` con `payment_id` de fábrica, la única que reconoce por escrito la vía autogestionada en su sitio mexicano, y la de menor comisión medida (20% contra 30%). Construir para las tres a la vez es alcance mal recortado.

**Lo primero que se prueba en sandbox, antes de escribir la migración:** que el `order_id` del pedido sea el mismo que el de Financial, y que ese identificador aparezca en el documento de pago real que el negocio ya recibe. Si no empata, la feature entrega folios que no concilian y el valor se cae entero.

### Condiciones que reabren esta decisión

- Uber México confirma por escrito elegibilidad y **territorio** (el contrato de 2022 no incluye México ni Latinoamérica).
- Rappi abre credenciales de sandbox.
- Deliverect u Otter confirman que certifican un POS propio de un local, con precio en pesos.
- mipOS vuelve a responder (su modelo —alta self-serve, sandbox, los tres canales y el folio en el payload— es exactamente el que este negocio pediría).
- El volumen de plataforma sube al punto en que un pedido perdido al mes deja de ser el 7% de la métrica de disponibilidad.

---

## Mantenimiento

Este documento es referencia viva y va indexado en [docs/README.md](README.md). Cuando una de las contradicciones de §2.3 se resuelva, o cuando algo de §2 deje de ser hueco, se actualiza aquí y se marca con qué se midió — no en un documento nuevo. Las afirmaciones marcadas **[R]** e **[I]** no se ascienden a **[D]** sin una lectura de fuente primaria citada.
