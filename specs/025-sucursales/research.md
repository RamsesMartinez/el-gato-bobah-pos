# Investigación: cómo modelan las sucursales otros sistemas

Hecha el 2026-10-01 para decidir qué es de la empresa y qué de la sucursal. VERIFICADO lleva
fuente oficial; INFERIDO no.

| Sistema | Empresa / sucursal | Catálogo | Matriz | Numeración |
|---|---|---|---|---|
| SAP ERP | Company code → Plant (llave de 4 caracteres) → Storage location. VERIFICADO ([help.sap](https://help.sap.com/docs/SAP_S4HANA_ON-PREMISE/b7eb2f9e70ab4c88abbff8b34a409b26/d268875433c5c02ae10000000a44538d.html)) | Maestro del mandante, extendido por plant. INFERIDO | No es entidad. INFERIDO | Rangos de documento. INFERIDO |
| SAP Business One | Una base con varias sucursales; activarlas es irreversible. Usuarios, almacenes y artículos se asignan a 1..N sucursales. VERIFICADO ([guía](https://help.sap.com/doc/048e30b80b7d4af68a3f28ba6c96446b/10.0/en-US/How_to_Work_with_Multiple_Branches_in_SAP_Business_One.pdf)) | Compartido, filtrado por almacén. VERIFICADO | Al activar se crea una «main branch» que se queda con lo existente. VERIFICADO | Series por sucursal. VERIFICADO |
| NetSuite | Subsidiaria = entidad legal; location = tienda o almacén con caja; su subsidiaria no cambia después. VERIFICADO ([doc](https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_4846759449.html)) | Restringible por subsidiaria o location. VERIFICADO | «Parent company». VERIFICADO | INFERIDO |
| Toast | Grupos → restaurante. VERIFICADO ([enterprise](https://doc.toasttab.com/doc/platformguide/sharingMenusAndOtherInformationAmongRestaurants.html)) | Menús y empleados compartidos, con sobrescritura por location. VERIFICADO | El grupo. VERIFICADO | No verificado |
| Square | Merchant → Location, con dirección, zona horaria, datos fiscales y texto del recibo propios. VERIFICADO ([Location](https://developer.squareup.com/reference/square/objects/Location)) | Catálogo único con presencia y precio por location. VERIFICADO ([overrides](https://developer.squareup.com/reference/square/objects/ItemVariationLocationOverrides)) | «Main location» = la primera. VERIFICADO | Factura única por location. VERIFICADO |
| Lightspeed K | Business → locations. VERIFICADO ([doc](https://k-series-support.lightspeedhq.com/hc/en-us/articles/23973504428059-Navigating-business-locations)) | Artículo local, compartido con precio por location, o global. VERIFICADO | La primera es plantilla de las nuevas. VERIFICADO | No verificado |
| Odoo 17+ | Empresa → sucursal; productos compartidos, precio, inventario, dirección y logo por sucursal. VERIFICADO ([doc](https://www.odoo.com/documentation/17.0/applications/general/companies.html)) | Maestro + lista de precios por sucursal. VERIFICADO | La empresa padre. VERIFICADO | INFERIDO |

**Uber**: cada tienda es un lugar con su dirección (`GET /stores/{store_id}`). VERIFICADO
([Uber](https://developer.uber.com/docs/eats/references/api/v1/get-eats-stores)).

**México, CFDI 4.0** ([Anexo 20](http://omawww.sat.gob.mx/tramitesyservicios/Paginas/documentos/Anexo_20_Guia_de_llenado_CFDI.pdf)):

- Un comprobante emitido en una sucursal lleva **el código postal de esa sucursal** como lugar de
  expedición. VERIFICADO.
- La fecha va en la hora local del lugar de expedición: la zona horaria es de la sucursal. VERIFICADO.
- Serie y folio son de control interno y formato libre; una serie por sucursal es costumbre, no
  obligación. INFERIDO.
- RFC y régimen son de la empresa. VERIFICADO.

## Conclusión que pasó al spec

- **De la sucursal**: lo que ocurre en un lugar (cajas, pedidos, inventario, tiendas de plataforma)
  y los datos del lugar (dirección, código postal, zona horaria).
- **De la empresa**: lo fiscal y lo compartido. El catálogo es maestro con excepciones por sucursal
  después.
- **Matriz**: una marca, no otra entidad.
- **Identificación**: número consecutivo por empresa más un código corto inmutable.

## Notas del arquitecto de base de datos para el plan

- **Llave foránea compuesta.** `(company_id, branch_id)` → `branches(company_id, id)`, el mismo
  patrón que 0040, 0061, 0071 y 0073. Una FK simple dejaría ligar algo a una sucursal de otra
  empresa, porque las FK saltan RLS.
- **El número no es una secuencia global.** Una secuencia global delataría cuántas sucursales tienen
  los demás clientes. Se calcula por empresa con `for update` dentro de la transacción del alta.
- **Turnos.** `register_sessions`, conteos, movimientos y transferencias heredan la sucursal por la
  caja: no llevan columna propia.
- **`orders.branch_id` se escribe al crear el pedido**, no se deriva. Un pedido de plataforma no
  tiene turno y su sucursal sale de la tienda.
- **`folio_counters` ya es por turno (0061)**, así que numera por sucursal sin cambios.
  `order_counters` está muerto: no se toca.
- **Caja principal.** `cash_registers_one_primary` pasa de `(company_id)` a `(company_id, branch_id)`.
- **`GetOpenPrimarySession` tiene cuatro llamadores**: `orders.go` (2), `pedidos_de_plataforma.go`
  y `backoffice.go`. Recibe la sucursal de un único helper que exige exactamente una sucursal activa.
  Sin `branch_id` en el JWT por ahora.
- **Riesgo de la migración**: el relleno de `orders.branch_id` reescribe la tabla más grande. Medir
  el tamaño y, si hace falta, partirlo en lotes.
- **Bajada (Down).** Falla con `raise exception` si ya existe una segunda caja principal en la
  empresa.
