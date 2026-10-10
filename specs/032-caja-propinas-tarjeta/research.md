# Research: Caja, propinas y cobro con tarjeta

## R1. Dónde vive la propina entregada
- **Decision**: en `register_cash_movements`, con `kind = 'propina'`, más una tabla hija `tip_payout_sources` (movimiento → pago que la originó, monto).
- **Rationale**: la entrega es salida de efectivo del cajón y ya hay un arqueo que suma movimientos. Ligarla a pagos permite que una devolución posterior se refleje (EB-07/08).
- **Alternatives**: seguir como gasto con categoría «propinas» (es lo que hoy rompe el corte); tabla aparte fuera de movimientos (duplica la lógica del arqueo).

## R2. Ligar a cobros: ¿obligatorio por pago?
- **Decision**: la entrega se reparte FIFO sobre los pagos con propina pendiente del método elegido, calculado en `domain`; quien opera no elige cobros.
- **Rationale**: minimizar toques; el ligue existe para auditoría y devoluciones.
- **Alternatives**: elegir cobros a mano (más toques, error humano).

## R3. Terminal obligatoria sin romper histórico
- **Decision**: `order_payments.card_terminal_id` y `card_kind` nulables; check `payment_method kind <> 'tarjeta' or created_at < '<fecha de migración>' or (card_terminal_id is not null and card_kind is not null)` implementado por trigger (el kind vive en otra tabla).
- **Alternatives**: rellenar el histórico con una terminal ficticia (inventa un hecho).

## R4. Herencia de propina «se queda en caja»
- **Decision**: al cerrar con esa opción se guarda `register_sessions.tips_carried_over`; el pendiente del siguiente turno de la MISMA caja lo suma.
- **Rationale**: puerta «más de una caja»; un saldo por empresa la cerraría.

## R5. Correo diario
- **Decision**: trabajo en el proceso de la API, cada 15 min, que envía el resumen de los días de negocio con todas sus cajas cerradas y sin envío registrado; tabla `daily_summary_sends` con único `(company_id, business_date)` insertado ANTES de enviar con estado y reintento.
- **Alternatives**: enviarlo dentro del cierre (bloquea el cierre si el SMTP falla, EB-39).

## R6. Motivos de apertura
- **Decision**: lista fija en `domain` (`last_count_wrong`, `float_changed`, `unrecorded_withdrawal`, `otro`) + texto obligatorio con `otro`.
- **Alternatives**: catálogo editable (se agrega después al mismo costo).

## R7. Apertura ciega
- **Decision**: el endpoint de apertura no devuelve el cierre anterior; la respuesta a un conteo con diferencia es 422 `opening_reason_required` sin el monto esperado.
- **Rationale**: un conteo ciego que el navegador puede leer no es ciego.
