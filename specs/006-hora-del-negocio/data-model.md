# Fase 1 — Modelo de datos

Una columna nueva. Cero tablas, cero migraciones de datos, cero cambios al histórico.

## Lo que NO cambia, y por qué

| Qué | Por qué se deja igual |
| --- | --- |
| `business_settings.timezone` | Ya existe, ya se valida contra IANA, ya nace en `America/Mexico_City`. Lo único que falta es que la pantalla la use |
| `orders.business_date` | Es el día al que pertenece la venta. Esta feature cambia **qué se muestra**, no en qué día cae el dinero |
| `register_sessions.business_date` | Idem |
| Los datos históricos | Se midió: el negocio real tiene 21 de 21 pedidos con la fecha correcta. El permiso para tocar históricos existe y no hace falta usarlo |

## Columna nueva: el momento de corte de la vista

```text
business_settings
  …
  corte_de_vista  text not null default 'medianoche'
                  check (corte_de_vista in ('medianoche', 'turno', 'cierre_de_caja'))
```

**Qué decide**: hasta cuándo se ve un pedido ya entregado en la pantalla de pedidos. Nada más. No
toca el día de la venta ni el arqueo.

**Por qué texto con check y no un enum de Postgres**: agregar un valor a un enum es una migración; a
un check también, pero sin el `alter type ... add value` que no se puede revertir dentro de una
transacción. Tres valores fijos que probablemente no crezcan.

**Por qué el default es `medianoche`**: es lo que un operador espera sin que nadie se lo explique, y
es el único de los tres que no depende de que alguien se acuerde de cerrar la caja.

### Los tres modos, y qué cuesta cada uno

| Modo | Cuándo se vacía | Costo real |
| --- | --- | --- |
| `medianoche` | A las 00:00 de la zona del negocio | Ninguno: es el default y el camino que se ejercita a diario |
| `turno` | Al abrir el siguiente turno | Ya se sabe la fecha del turno abierto; es una comparación más |
| `cierre_de_caja` | Al cerrar la caja | Idem |

**Honestidad sobre el alcance**: en un negocio que abre a las 16:00 y cierra a las 22:00, los tres
caen en momentos distintos pero el efecto que el operador percibe es casi siempre el mismo. Se
construyen porque el producto se vende a negocios con otros horarios; hoy este local solo usa el
primero. Los otros dos son ~15 líneas de consulta y su test, y esa es toda su deuda.

## El defecto del backend que sí se corrige

`BackofficeService.businessDate` cae a **UTC** cuando no puede leer la zona. Pasa a caer al
**default del producto**, que es lo que el resto del sistema ya usa.

No es cosmético: con el fallback en UTC, un turno abierto después de las 18:00 locales queda fechado
al día siguiente y su dinero cae en el arqueo equivocado. Ya pasó dos veces en la cuenta de pruebas.

**No lleva migración de datos**: el negocio real no tiene ningún pedido afectado.

## Reglas de negocio, y dónde viven

| Regla | Dónde | Por qué ahí |
| --- | --- | --- |
| Qué día es "hoy" en una zona | `domain` — ya existe como `BusinessDate` | Es lógica pura y ya está probada |
| Hasta cuándo se ve un entregado según el modo | `domain`, función pura sobre (modo, ahora, zona, turno) | Se prueba sin base de datos, incluido el borde del horario de verano |
| Qué pedidos están en curso | Consulta, sin fecha | Es un conjunto de filas |
| Cómo se pinta una hora | Frontera de presentación, un solo lugar | Doce sitios sueltos es cómo se desincronizó la primera vez |
