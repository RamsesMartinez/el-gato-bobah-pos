# Research: venta por plataformas digitales

Lo que había que averiguar antes de diseñar, y qué se encontró **en el código y en la base real**,
no en el diseño original.

## 1. ¿Cuánto de esto ya existe?

**Decisión**: no se crean plataformas, ni métodos de pago, ni el campo de la orden. Solo faltan los
precios.

**Hallazgo**: más de la mitad del andamiaje existe desde `0002_lookups.sql`:

| Pieza | Estado real |
|---|---|
| `delivery_platforms` | Didi, Uber Eats, Rappi, **Propio**, por empresa (4 filas cada una). `id smallint` |
| Métodos de pago | Didi (4), Uber Eats (5), Rappi (6), con `kind='plataforma'` y `affects_cash_drawer=false`. **Globales**, no por empresa |
| `orders.delivery_platform_id` | `smallint` nullable, con FK |
| El corte | `ExpectedByMethodSince` agrupa por **todo método activo**, así que las tres plataformas ya salen en su renglón |

**Consecuencia para el alcance**: la **US3 del spec (que el corte separe cada plataforma) ya
funciona** sin escribir código. Queda como tarea de *verificación*, no de construcción — un test que
la fije para que no se rompa, y nada más.

**Alternativa descartada**: crear una tabla de "listas de precios" genérica desacoplada de
`delivery_platforms`. Sería una abstracción especulativa (principio VI): hoy la lista de precios ES
la plataforma, uno a uno, y no hay un segundo consumidor.

## 2. ¿Dónde vive el cálculo del precio?

**Decisión**: una función pura en `domain` decide el precio efectivo; `app` carga los datos y la
llama; el servidor recalcula siempre.

**Rationale**: principio I y IV — la regla ("manual si existe, si no base × (1+margen), redondeado a
2dp") es lógica pura y se prueba sin base de datos. `BuildOrder` ya recibe un mapa de productos
priceados: el precio efectivo entra por ahí, sin cambiarle la forma a la función que ya funciona.

**Alternativa descartada**: resolver el precio en SQL con un `coalesce(pp.price, p.price * 1.35)`.
Es más corto y deja la regla de dinero repartida entre una query y el redondeo de Go, donde nadie la
puede probar sin Postgres y donde el redondeo de `numeric` y el de `Round2` pueden no coincidir.

## 3. ¿Margen como porcentaje o como precio calculado y guardado?

**Decisión**: porcentaje en `delivery_platforms`, precio calculado al vuelo. Solo se guardan las
excepciones.

**Rationale**: 502 productos × 3 plataformas = 1,506 filas que habría que mantener sincronizadas
cada vez que cambia un precio base. Con el margen al vuelo, subir un precio base actualiza las tres
plataformas solo, y la tabla de excepciones tiene las decenas de filas que de verdad se corrigieron.

**Alternativa descartada**: materializar las 1,506 filas al activar la feature. Se vuelven obsoletas
en el primer cambio de precio base y nadie se entera.

## 4. `Propio` no lleva margen

**Decisión**: el margen por default es 35% para Didi, Uber Eats y Rappi, y **0% para Propio**.

**Rationale**: "Propio" es reparto del negocio con su propio repartidor: no hay comisión de
plataforma que absorber, así que el precio es el de mostrador. Sembrarlo en 35% le subiría el precio
al cliente que llama por teléfono.

## 5. El costo de envío en un pedido de plataforma

**Decisión**: `delivery_fee` va en 0 y la pantalla no lo ofrece cuando la venta es de plataforma.

**Rationale**: el reparto lo cobra y lo hace la plataforma. El POS hoy pre-llena el envío con el
ajuste del negocio ($20) para `service_type='domicilio'`, y un pedido de plataforma **es**
`domicilio` por el check `orders_check`. Sin esto, cada pedido de Uber saldría con $20 de más.

## 6. Modificadores

**Decisión**: misma regla que los productos — margen de la plataforma sobre `price_delta`, con
excepción manual por opción.

**Hallazgo**: `modifier_options.price_delta` es `numeric(10,2)` y **puede ser 0** (opciones sin
costo, que son la mayoría de las 546). Aplicar 35% a 0 da 0, así que las opciones gratis siguen
gratis sin ningún caso especial. El check de la tabla nueva permite 0 pero no negativos.

## 7. Cambiar de plataforma con el ticket ya armado

**Decisión**: cambiar de lista **re-precia las líneas ya agregadas**, y el servidor recalcula todo
al cobrar contra la plataforma que venga en el comando.

**Rationale**: es el riesgo que la checklist de la spec marcó. El carrito del POS (`useTicketStore`)
guarda `unitPrice` por línea para pintar el total; si al cambiar de plataforma no se re-precia, la
pantalla muestra un total y el servidor cobra otro. Como el servidor es autoritativo (principio III),
el que manda es el servidor — así que la pantalla tiene que coincidir o el operador entrega un
ticket con un total que no es el cobrado.

**Alternativa descartada**: bloquear el cambio de plataforma con el ticket armado. Contradice el
requisito de agilidad: el operador se equivoca de plataforma y tendría que rehacer el pedido
completo (la vara de UX del producto es *nunca obligar a deshacer para rehacer*).

## 8. ¿Quién puede escribir un precio de plataforma?

**Decisión**: cualquier rol que pueda vender (cajero incluido), y se audita `updated_by`.

**Rationale**: lo decidió el dueño buscando agilidad — el pedido ya llegó y hay que imprimirlo. El
riesgo es un precio mal capturado que persiste; se acota con validación de cotas en el servidor
(`ValidMoney`) y dejando rastro de quién lo cambió, no con un permiso que obliga a despertar a un
gerente con el pedido detenido.

**Nota de seguridad**: es un endpoint de escritura nuevo accesible a cajero. Pasa por
`security-auditor` antes de mergear (principio V).
