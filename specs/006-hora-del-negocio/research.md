# Fase 0 — Investigación

Todo lo de aquí está medido contra el código de `develop` o contra la base de producción el
2026-09-01. Lo que no se pudo medir se dice.

## Hallazgo 1 — El histórico del negocio real está LIMPIO. No hay que arreglar datos

El dueño autorizó tocar históricos y reconstruir desde respaldos. **No hace falta**, y eso se midió
antes de usar el permiso:

| Empresa | Pedidos cuya fecha de negocio coincide con la de su turno |
| --- | --- |
| `gatobobah` (real) | **21 de 21** |
| `bobah-pruebas` | 19 de 21 |

Los dos que no coinciden son los pedidos 61 y 62 de la **cuenta de pruebas**, del 2026-08-29 a las
20:50 y 20:51 hora local, con fecha de negocio 2026-08-30 — que es la fecha **UTC** de ese instante.
Su turno decía 2026-08-29. Lo mismo con la sesión 3 de esa empresa, abierta el 2026-07-22 a las 21:26
local y fechada 2026-07-23.

**Ninguna cifra del negocio real está mal.** Reconstruir desde el respaldo movería dinero que hoy
está bien puesto.

## Hallazgo 2 — La causa de esos dos: la fecha de negocio cae a UTC cuando no hay zona

[`BackofficeService.businessDate`](../../server/internal/app/backoffice.go) hace:

```go
tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
if err != nil {
    return domain.BusinessDate(s.now(), time.UTC)   // ← el hueco
}
```

Su comentario justifica el fallback: *"UTC en vez de fallar: abrir caja no se detiene por un ajuste
mal escrito"*. La intención es correcta —no tumbar la apertura de caja— pero el valor elegido no: el
producto tiene un default (`domain.DefaultTimezone`, `America/Mexico_City`) y caer a UTC en vez de a
él corre la fecha seis horas **sin avisar**, y con ella el arqueo en el que cae el dinero.

Es el mismo patrón que el principio V llama por su nombre: un parámetro que cae a un default en
silencio y devuelve una pantalla que se ve correcta reportando un número que nadie pidió.

**Esto sí se arregla**, y es lo único del backend que esta feature toca aparte de las dos consultas.

## Hallazgo 3 — Doce formateos, cero con zona

```text
web/src/app/SystemInfo.tsx
web/src/features/backoffice/CashPage.tsx      (4 sitios)
web/src/features/backoffice/StockPage.tsx
web/src/features/sales/SaleDetailDialog.tsx
web/src/features/sales/SalesPage.tsx
web/src/utils/format.ts
web/src/utils/printKitchen.ts                 ← papel
web/src/utils/printReceipt.ts                 ← papel
```

Ninguno pasa `timeZone`. Los dos últimos son los que salen impresos: el ticket del cliente y la
comanda de cocina.

**Consecuencia**: cada pantalla dice la hora del navegador de esa tableta. Con dos Surface no hay
garantía de que coincidan, y el ticket que se lleva el cliente lleva la hora de la máquina, no la del
local.

## Hallazgo 4 — La zona ya viaja al front; nadie la consume

`GET /business-settings` ya devuelve `timezone` ([`BusinessSettings`](../../web/src/api/pos.ts) lo
tiene tipado) y la pantalla de Negocio ya la guarda con `updateTimezone`. Además
[`ticketBusinessInfo.ts`](../../web/src/features/tickets/ticketBusinessInfo.ts) ya trae los ajustes
para armar el encabezado del ticket, con su `useQuery` y su función pura `toTicketBusinessInfo`.

**Consecuencia**: no hay que traer nada nuevo del servidor. El dato ya está en la pantalla; lo que
falta es un solo lugar que lo aplique y del que todos cuelguen.

## Hallazgo 5 — "Entregados hoy" tiene el mismo defecto que tenía la barra

[`OrdersService.DeliveredToday`](../../server/internal/app/orders.go) pasa `s.now()` —el reloj del
servidor, en UTC— a una consulta que filtra `business_date = $1`. En México la medianoche UTC cae a
las 18:00 locales: la lista se vacía a media hora pico.

Es exactamente el defecto que la feature 005 corrigió en la barra de pedidos en curso atándola a la
fecha del turno abierto. Esa corrección **se retira** en esta feature: el dueño decidió que los
pedidos activos se ven siempre, sin filtro de fecha.

## Hallazgo 6 — Producción: el rezago que la US2 viene a limpiar

| Empresa | Pedidos abiertos |
| --- | --- |
| `bobah-pruebas` | 11, desde el 2026-07-22 |
| `gatobobah` | 0 (se cerraron el 2026-09-01) |

Sin filtro de fecha, los 11 de la cuenta de pruebas vuelven a la pantalla de golpe. Es lo que el
dueño quiere —fuerza a cerrarlos— pero es el escenario real que el diseño de la barra tiene que
aguantar, y es más de lo que se midió como máximo diario (6).

## Hallazgo 7 — El horario de verano de México ya no existe, pero la zona sí puede tenerlo

México eliminó el horario de verano en 2022 para casi todo el país; `America/Mexico_City` ya no
cambia. Pero **`America/Tijuana` sí sigue cambiando** (está alineada con la costa oeste de Estados
Unidos), y está en la lista corta de zonas que el producto ofrece.

**Consecuencia**: el corte "a la medianoche" no se puede calcular sumando 24 horas al anterior. Dos
veces al año esa distancia es de 23 o 25 horas para un negocio en Tijuana, y un corte que suma horas
fijas se desfasa justo ese día. Se calcula la medianoche **de esa fecha en esa zona**.

## Hallazgo 8 — Dos empresas en la misma base

`bobah-pruebas` (id 1) y `gatobobah` (id 2), cada una con sus ajustes y su zona. Cualquier consulta
nueva se mide por empresa, y cualquier test de migración corre con las dos.

## Lo que NO se investigó, y por qué

- **Si conviene mover el corte de vista al servidor.** El spec pide que la lista refleje el corte sin
  recargar, lo que se puede resolver de los dos lados. El plan lo decide; la investigación no midió
  el costo de cada uno porque ninguno es caro a esta escala.
- **Cuánto tarda el navegador en formatear con `timeZone`.** No se midió: son ~20 fechas por pantalla
  como mucho, y la API de internacionalización del navegador está pensada exactamente para esto.
