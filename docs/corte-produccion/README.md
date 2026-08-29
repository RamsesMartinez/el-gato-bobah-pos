# Corte a producción — la empresa que queda como principal

Hasta agosto de 2026 el POS convivió con FUDO: se usaba para probar mientras el negocio cobraba en
el otro sistema. Esa etapa dejó **58 ventas de prueba**, 3 cortes de caja y 66 niveles de inventario
en negativo dentro de la única empresa que existía. Al pasar este sistema a principal había que
arrancar en limpio sin perder nada de eso.

**Cómo se resolvió**: no se borró nada. Se abrió una **empresa nueva** que nace vacía y se le copió
el catálogo completo; la vieja se quedó con su histórico y cambió de nombre.

**Aplicado en producción el 2026-08-29.** Así quedó:

| | Empresa |
|---|---|
| **Producción** (la que se usa) | `id 2`, slug `gatobobah`, nombre *El Gato Bobah*. Catálogo copiado, cero ventas. |
| **Pruebas** (histórico) | `id 1`, slug `bobah-pruebas`, nombre *Bobah Pruebas*. Conserva las 58 ventas, los cortes y los gastos de prueba. Sigue activa: se entra con `usuario@bobah-pruebas`. |

> **Una sesión abierta de antes del corte sigue en la empresa de pruebas.** El `company_id` viaja
> dentro del JWT, así que un dispositivo que no cerró sesión sigue cobrando en *Bobah Pruebas* hasta
> que alguien salga y vuelva a entrar. Al aplicar el corte hay que cerrar sesión en todas las
> tablets.

El slug se intercambió a propósito. El slug es la mitad derecha del identificador de login, así que
dejando `gatobobah` en la empresa nueva **los operadores siguen entrando con `admin@gatobobah`** y no
tienen que aprender nada. Las contraseñas y los PIN se copiaron tal cual (mismo hash bcrypt), así que
los cuatro usuarios existen en las dos empresas con las mismas credenciales.

## Los archivos

| Archivo | Qué hace |
|---|---|
| [`00_dry_run.sql`](00_dry_run.sql) | No modifica nada. Enseña de dónde parte el corte. |
| [`01_nueva_empresa.sql`](01_nueva_empresa.sql) | El corte, en una sola transacción, con sus verificaciones al final. |
| [`01_rollback.sql`](01_rollback.sql) | Deshace el corte y devuelve el slug original. Se niega a correr si la empresa nueva ya tiene pedidos. |

Requiere la migración **0036** (`categories_name_scope` con `company_id`) aplicada antes; sin ella el
copiado truena en `categories`.

## Lo que se copia y lo que no

- **Se copia** el catálogo: categorías, productos, grupos y opciones de modificadores, recetas e
  ingredientes, proveedores, canales, plataformas de reparto, categorías de gasto, cajas, usuarios y
  la configuración del ticket (logo, textos e impresión automática).
- **No se copia** nada transaccional: pedidos, pagos, movimientos de stock, cortes de caja, gastos ni
  los contadores de folio diario. La empresa nueva empieza en el ticket #1.
- **No se copian** los niveles de inventario. Los 66 que había estaban todos en negativo (hasta
  −6250) porque nunca se cargó inventario y cada movimiento era una resta de una venta de prueba; el
  nivel correcto al arrancar es que la fila no exista, y el sistema la crea sola en el primer
  movimiento real.
- **`payment_methods` y `units` son globales** (no tienen `company_id`): las dos empresas comparten
  las mismas filas y no hay nada que copiar.

## Lo que se aprendió armándolo

Cuatro cosas rompieron el copiado en el ensayo contra una copia real de producción, y ninguna era
evidente leyendo el esquema:

1. **`channels`, `delivery_platforms` y `payment_methods` tienen el `id` en `smallint`.** Un offset
   global de 1,000,000 revienta con *smallint out of range*. Por eso el offset es por tabla y vale
   `max(id)` de esa tabla.
2. **`payment_methods` no tiene `company_id`.** Es global. Copiarla habría duplicado los métodos de
   pago para todos.
3. **`categories_name_scope` no incluía `company_id`** — el defecto de verdad, ver abajo.
4. **`products.margin_amount` es una columna generada** y no se puede insertar.

### El defecto que salió a la luz

`categories_name_scope` se creó en [`0004_catalog.sql`](../../server/migrations/0004_catalog.sql),
antes de que el sistema fuera multi-tenant, como único sobre `(coalesce(parent_id,0), name)`. Las
migraciones 0022–0024 agregaron `company_id` y RLS a todas las tablas pero no revisaron los índices
únicos anteriores. Para una categoría **raíz** `parent_id` es NULL y el `coalesce` da 0 en todas las
empresas, así que la segunda empresa que quisiera su propia "Bebidas" chocaba contra la primera.

La RLS aislaba las lecturas perfecto, pero el índice hacía **imposible poblar un tenant nuevo**. Lo
arregla [`0036_categories_name_scope_tenant.sql`](../../server/migrations/0036_categories_name_scope_tenant.sql),
con su test en `TestCategoriaRaizPuedeRepetirNombreEntreEmpresas`.

## Antes de correrlo en producción

1. Respaldo completo y **verificado por checksum en los dos lados**, no solo tomado.
2. Ensayarlo restaurando ese respaldo en una base local y corriendo ida y vuelta (corte + rollback).
   Los cuatro fallos de arriba salieron ahí, no en el servidor.
3. La migración 0036 aplicada (entra sola al desplegar: goose corre al arrancar el binario).
