# Research: Notificaciones de inventario (084)

Hechos medidos en el código (2026-11-14) y la decisión que sale de cada uno.

## R1. Dónde se detecta el cruce

- **Hecho**: toda existencia pasa por `stock_movements` → `apply_stock_movement()` (0076:298), trigger
  `trg_stock_movement` AFTER INSERT. No hay movimiento tipo `conteo`: el cierre de un conteo inserta
  `ajuste` con `stock_count_id`. Uniones y cambios de unidad insertan `ajuste` con `replacement_id`.
- **Decisión**: un trigger aparte, `trg_stock_movement_notify` AFTER INSERT (los triggers del mismo
  evento corren por nombre: va después de `trg_stock_movement`, con la existencia ya sumada). Llama a
  `evaluate_low_stock(company, branch, item_type, ingredient, product)`, la misma función que llama Go
  al cambiar la configuración, archivar, unir o cerrar un conteo.
- **Por qué no dentro de `apply_stock_movement`**: esa función mantiene la existencia, que nunca puede
  fallar; el aviso sí puede y va en su propio bloque `begin … exception when others` (subtransacción),
  solo en la rama donde hay algo que abrir o cerrar. Un filtro barato antes del bloque (artículo sin
  alerta y sin aviso abierto → salir) deja el costo de una venta normal en una lectura indexada.
- **Alternativas descartadas**: barrido periódico (el issue lo descarta; además tarda y no ve el cruce
  que se corrige solo); detectar en Go en cada camino que inserta movimientos (son 12 caminos, más la
  API anterior durante la subida: uno olvidado vende sin avisar).

## R2. Concurrencia

- **Decisión**: índice único parcial `notifications (company_id, branch_id, item_key, kind) where
  status = 'open' and kind <> 'not_found'` e `insert … on conflict do nothing`. Dos ventas que cruzan el mismo mínimo ya se
  serializan en la fila de `stock_levels`; la segunda ve el aviso de la primera o choca en el índice.
  Prueba con dos conexiones bajo `appRoleStore`.

## R3. Tiempo real

- **Hecho**: `realtime.Broker` es en memoria y lo publica Go; el trigger no lo alcanza. SSE en
  `GET /events`.
- **Decisión**: el trigger y los servicios llaman `pg_notify('notifications', company_id::text)`.
  `NOTIFY` es transaccional: llega solo si la transacción confirma y nunca si se revierte. Un proceso
  de la API escucha con una conexión dedicada (fuera del pool, rol `gatobobah_app`, sin datos de
  empresa) y publica `notifications.changed` al Broker. En la subida azul/verde cada API escucha y
  sirve a sus propios clientes. Respaldo: la campana vuelve a pedir el contador cada 60 s y al
  reconectar (`alReconectar`).

## R4. Correo: ventana, enfriamiento y resumen

- **Hecho**: `DailySummaryService` reclama una fila por `(company_id, business_date)` en
  `daily_summary_sends`, sin columna `kind`.
- **Decisión**: tabla propia `notification_email_sends` con `kind` (`window` | `morning`) y llave
  única `(company_id, kind, period_key)`. Agregar `kind` a `daily_summary_sends` exigía cambiar su
  llave única, que la API anterior usa en `on conflict` durante la subida.
- **Ventana**: bloques fijos de 5 minutos por empresa. Un aviso abierto en el bloque `[t, t+5)` sale en
  el correo que se reclama al cerrar el bloque; como máximo un correo por bloque, hasta 5 min de
  espera. Solo salen avisos todavía abiertos al enviar; los demás quedan `skipped`.
- **Enfriamiento**: un aviso cuyo artículo ya salió en un correo en las últimas 12 h queda
  `email_state = 'skipped'` con motivo `cooldown` (sigue en la campana).
- **Resumen de la mañana**: `period_key = business_date`, hora en `business_settings`
  (`notifications_morning_at`, 08:00), zona `business_settings.timezone`. Reclamo por fila como el
  cierre: un reinicio no duplica.
- **Reloj**: el servicio recibe `now func() time.Time`; las pruebas usan `clockAt`.

## R5. Plantillas MJML

- **Hecho**: `mjml` 5.4.1 es la última estable (2026-09-10); 6.0.0 está en alfa. `mailer.Send` manda
  un solo cuerpo `text/html`.
- **Decisión**: fuente en `emails/*.mjml` y un script `web/scripts/compilar-correos.ts` que corre con
  `bun` (dependencia de desarrollo en `web/`, nunca en el servidor); la salida se versiona en
  `server/internal/mailtpl/templates/*.html.tmpl` y se embebe con `go:embed` + `html/template`. CI
  corre el script y `git diff --exit-code`. Las marcas `{{ }}` de Go viajan dentro de `mj-text`/`mj-raw`.
  Antes de agregarla: `bun audit` sin avisos altos.
- **Pruebas**: golden por plantilla, peso < 102 KB, colores de marca, logo PNG por URL absoluta.

## R6. Idempotencia de «Se acabó»

- **Hecho**: no hay llave HTTP genérica. Hay `orders.client_uuid` y la tabla del MCP ligada al token.
- **Decisión**: `stock_out_reports.client_key uuid`, único por empresa. Repetir la clave devuelve el
  reporte guardado. La herramienta MCP usa su propia idempotencia sobre la misma ruta.

## R7. Permisos y destinatarios

- **Hecho**: `inventory.count/configure/view` solo admin y gerente; `inventory.write_off` los cuatro.
  `users` no tiene correo, solo `recovery_email` opcional.
- **Decisión**: permiso nuevo `inventory.report_out` (id 38) para los cuatro sugeridos. Destinatarios
  por omisión: activos con `inventory.configure` y `recovery_email`. La pantalla de destinatarios no
  edita correos de personas (cambiar el correo de recuperación de otro sería tomar su cuenta): los
  extra se capturan como correos sueltos.
- **Quién ve la campana**: quien tiene `inventory.report_out` ve los avisos de existencia; los de «No
  lo encuentro» solo quien tiene `inventory.configure`.

## R8. Consumo y lista de compra

- **Hecho**: no existe consulta de consumo. Los empaques no guardan costo; `supplier_id` del insumo no
  se llena.
- **Decisión**: consumo diario = −Σ(`venta` + `cancelacion`) de 14 días / 14, en unidad base, por
  sucursal; «medido» si hay movimiento en ≥ 7 días distintos. Empaque: el activo con menor costo por
  unidad base en su última compra (`expense_items` recibido); sin compras, el más chico activo.
  Proveedor: el de la última compra. Cálculo puro en `domain.SuggestPurchase`.

## R9. «Sin conteo reciente»

- **Hecho**: no existe en «Salud del inventario».
- **Decisión**: sin renglón contado en un conteo cerrado en los últimos 30 días, o existencia
  negativa ⇒ `needs_count = true` en el aviso.

## R10. Búsqueda sin acentos ni plural

- **Hecho**: `catalog_name_key` (SQL) y `domain.CatalogNameKey` normalizan mayúsculas, acentos y
  espacios, con prueba de paridad.
- **Decisión**: `domain.SearchKeys` singulariza cada palabra de la búsqueda (quita `es`/`s` final si
  quedan ≥ 3 letras) y la consulta busca por prefijo de palabra en nombre y otros nombres normalizados.

## R11. Roles que insertan movimientos

- **Hecho**: solo `gatobobah_app` inserta en `stock_movements` (grant general de la 0024); el webhook
  solo lee conexiones y plataformas.
- **Decisión**: las tablas nuevas llevan su `grant` explícito a `gatobobah_app`; el trigger corre con
  el rol de quien inserta (`security invoker`), así que RLS lo acota a la empresa de la sesión.

## R12. Orden de candados (revisión de arquitectura)

- El trigger toma `stock_levels` (en `apply_stock_movement`) y después escribe `notifications`. Todo
  camino de Go que escriba movimiento y aviso **inserta primero el movimiento** y después toca
  `notifications`; `evaluate_low_stock` nunca hace `for update` sobre `stock_levels` ni `ingredients`;
  resolver o descartar sin movimiento solo toca `notifications`. Al revés se cierra el ciclo que ya
  costó un cobro cancelado (`LockIngredientStockNowait`). Lo prueba
  `TestConcurrentStockOutReportAndSale` con dos conexiones y `lock_timeout` corto: falla con 40P01.

## R13. La API anterior y el permiso nuevo

- `app/roles.go` filtra con `domain.KnownPermission` al calcular lo que puede una persona: la API
  anterior ignora `inventory.report_out` sin fallar. La pantalla de roles anterior lo listaría con su
  código durante los minutos de la subida; se acepta.
