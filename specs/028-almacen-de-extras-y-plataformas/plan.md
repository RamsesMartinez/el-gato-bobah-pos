# Implementation Plan: El almacén descuenta lo que de verdad se vendió

**Branch**: `028-almacen-de-extras-y-plataformas` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)

## Summary

Una sola función pura en `domain` expande lo vendido a los movimientos de almacén:

- un producto por su receta, o por sus existencias propias;
- cada extra por su receta o por el producto al que está ligado;
- un paquete por sus componentes;
- un insumo preparado por los insumos que lo componen.

La usan los tres caminos que venden: crear pedido, agregar a un pedido y aceptar un pedido de
plataforma. Cada movimiento guarda de dónde salió (extra o componente).

La composición se captura en el catálogo. Cada producto, extra e insumo lleva su estado: estimada
o confirmada. Una carga desde FUDO llena lo que falte, marcado como estimado, y eso **sí descuenta**
(decisión del dueño, 2026-10-07).

De paso se corrige un defecto existente: cancelar un pedido completo reponía también lo que ya se
había repuesto al cancelar un renglón, o lo que ya se había preparado.

Migración 0078. Las promociones de plataforma (historia 6) no se construyen en esta entrega:
dependen de un pedido real que confirme el formato.

## Technical Context

**Language/Version**: Go 1.27 (server); React 19 + Chakra v3 (web)

**Storage**: PostgreSQL con RLS por empresa y sucursal en los movimientos (0076)

**Testing**: unitarios en `domain`, integración bajo `appRoleStore` con `inTheThreeCases`, migración
sobre respaldo real con dos empresas, vitest para la pantalla

**Constraints**:
- Mismo número de toques al vender y aceptar.
- Tableta de 1024×600.
- Los archivos de FUDO no entran al repositorio.

**Scale/Scope**:
- 1 migración.
- 1 función de expansión.
- 3 caminos de venta y 2 de reversa.
- 1 comando de carga.
- 1 pantalla de composición en el catálogo.

## Constitution Check

| Principio | Cómo se cumple |
|---|---|
| I. Capas | `domain.ExpandSale` (pura) decide qué se descuenta. `app` carga el grafo de composición y escribe dentro de la transacción de cada venta. Las consultas van en `queries/stock.sql` y `queries/composition.sql`. Handlers finos para la composición |
| II. Errores | `ErrCompositionCycle`, `ErrPackageInPackage` y `ErrUnitKindMismatch` en `domain`, mapeados en `httpapi.Error` |
| III. Dinero | No toca dinero. Las cantidades del almacén se redondean con `Round4` al guardar, como hoy, y una cantidad positiva nunca redondea a cero |
| IV. Pruebas | Bordes listados abajo, cada uno con su test antes del código. El defecto de la doble reposición se reproduce primero. Las tablas nuevas, en los tres casos |
| V. Seguridad | La composición la edita solo admin y gerente. Las FK compuestas impiden ligar el insumo o el producto de otra empresa |
| VI. YAGNI | Sin promociones (no hay formato confirmado), sin tamaños ni variantes que heredan (es la siguiente especificación), sin recalcular ventas pasadas |
| VII. Idioma | Identificadores nuevos en inglés |
| VIII. Puertas | Cruza a medias «Costear con recetas e inventario de insumos»: el snapshot del renglón ya existía, y ahora el movimiento guarda su origen. Agregar el origen después costaría adivinar de qué extra salió cada movimiento viejo, así que va ya |

### Casos de borde, antes del código

1. **Cantidades anidadas**: un extra ×2 en un renglón ×3 descuenta 6 veces. Un paquete ×2 con un
   componente ×2 descuenta 4.
2. **Unidad**: la receta en ml y el insumo en litros. La conversión usa `to_base`, y una cantidad
   positiva que redondea a 0.0000 se guarda como el mínimo (0.0001), no se pierde.
3. **Insumo preparado**: se descuenta por su rendimiento (`yield_qty`). El esquema ya exige
   rendimiento mayor que cero (0003); la guarda de `ExpandSale` es solo defensiva para datos viejos.
4. **Ciclos**: un paquete que se contiene a sí mismo, un insumo preparado circular o un extra ligado
   a un producto cuyo extra lo liga de vuelta se rechazan al capturar. Al vender, la expansión corta
   por profundidad, para que un dato viejo no cuelgue una venta.
5. **Paquete dentro de paquete**: se rechaza al capturar.
6. **Producto con existencias propias**: descuenta el producto, no su receta (no puede tener ambas).
7. **Extra sin composición**: no descuenta y la venta no falla.
8. **Pedido de plataforma**: el renglón genérico no descuenta. Una opción emparejada descuenta su
   composición aunque no haya `order_line_modifiers` (las opciones de la plataforma viajan en la
   nota).
9. **Doble reposición**: cancelar un renglón y luego el pedido repone una sola vez. Cancelar el pedido
   completo sigue reponiendo lo ya enviado a cocina, como antes: cambiarlo es decisión del dueño y
   no se tomó aquí (tasks T002).
10. **Carga de FUDO**: no pisa lo capturado. Un nombre de FUDO que empata con dos del POS no se
    adivina: se reporta. Un insumo que no existe en el POS se reporta, no se crea.
11. **Sucursal**: el movimiento cae en la sucursal del pedido (0076), también los de extras y
    componentes.

## Diseño

1. **Estado de la composición**: `composition_status` (`estimated` | `confirmed` | nulo) con
   `composition_confirmed_by` y `composition_confirmed_at`, en `products`, `modifier_options` e
   `ingredients`. Va en la fila dueña de la composición, porque la composición es la receta o el
   producto ligado de esa fila.
2. **Origen del movimiento**: `stock_movements.modifier_option_id` (el extra que lo originó) y
   `component_of_product_id` (el paquete del que salió el componente), ambos nulables con FK
   compuesta.
3. **Paquetes**: la misma semántica que el costeo (`domain.CostGraph.productCost`, que suma el
   producto *default* de cada hueco × `max(min_select, 1)`), sobre la misma consulta
   (`ListComboSlotDefaultsForCosting`). Si divergieran, el costo de un paquete y lo que descuenta
   dirían cosas distintas. Se apoya en el modelo de combos que ya existe (`products.type = 'combo'`, `combo_slots`). Un
   paquete fijo es un combo cuyos huecos tienen un solo producto, con `min_select = max_select` igual
   a la cantidad. Los combos con elección ya se modelan como extras ligados a productos, y eso lo
   cubre el punto 1 del resumen.
4. **Historial de componentes**: `order_line_components` (renglón, producto, cantidad). Es una copia
   al vender, para que editar un paquete no reescriba cuántas crepas salieron el mes pasado.
5. **Expansión**: `domain.ExpandSale(graph, sale)` devuelve `[]StockDelta` con su origen. El grafo
   se carga una vez por transacción con consultas por empresa: recetas con `to_base`, producto ligado
   de cada extra, huecos fijos de cada paquete y rendimiento de cada insumo preparado.
6. **Pedidos de plataforma**: al registrar el pedido entrante, se guarda el `modifier_option_id` de
   cada renglón hijo emparejado (columna nueva en `platform_incoming_order_lines`). Al aceptar se
   expande igual que en el mostrador, sin el genérico.
7. **Reversas**: `RestockCancelledOrder` repone por renglón lo **neto**: venta menos cancelación ya
   hecha, y solo de los renglones que `domain.ReponeInventario` permite. Es un solo `insert … select`
   agregado. `CancelarConDevolucion` toma `GetOrderForUpdate`, como `CancelarRenglon`, para que una
   cancelación de renglón concurrente no se cuele entre leer y reponer; hoy eso solo se evita por
   accidente. `RestockCancelledLine` ya lee por renglón y recoge solo los movimientos nuevos.
8. **Carga de FUDO**: `cmd/fudo-import -compositions -company <slug>`. Cada consulta de empate filtra
   `company_id` **explícito**: corre como owner, y RLS no aplica. La importación actual tiene ese
   defecto (`main.go:504` lee recetas de todas las empresas) y se corrige en la misma pasada. Lee `recetas.csv`,
   `mod_productos.csv`, `subingredientes.csv` y `subproductos.csv` y no borra nada. Escribe solo
   donde falta composición, marcada `estimated`, y termina con un reporte de lo que no pudo empatar.
   Corre como owner (bootstrap), como el import de hoy.
9. **Pantalla**: «Qué lleva ›» es un botón en el diálogo de producto y en el de extra. Abre su
   propia hoja inferior, con el mismo patrón que `Picker`, para no meter un diálogo dentro de otro.
   La hoja tiene:
   - renglones de insumo con cantidad (`Input type=number`) y unidad (Picker), y un «quitar» de 44 px
     separado de «agregar»;
   - o «Es el producto…» (Picker);
   - «Confirmar» cuando la composición está estimada.

   El filtro «Composición: sin capturar / estimada» va dentro del menú de filtros que ya tiene
   `ProductsAdminPage`, para no sumar otra fila de chips.
10. **Diálogos que no caben** (defecto que ya existe hoy, lo encontró la revisión de tableta):
    `ProductEditDialog` y `OptionFormDialog` no fijan `scrollBehavior="inside"`. Su contenido pasa de
    600 px y «Guardar» queda fuera de la pantalla. Se corrige con `scrollBehavior="inside"` y
    `placement="center"`, como en `CashPage`, y un test que cuenta que el cuerpo es el que hace
    scroll.

## Project Structure

```text
server/
├── migrations/0078_stock_composition.sql
├── queries/composition.sql (nueva), stock.sql, orders.sql, pedidos_de_plataforma.sql
├── internal/domain/composition.go (+ _test)      # ExpandSale, validación de ciclos y unidades
├── internal/app/composition.go                   # cargar grafo, capturar y confirmar
├── internal/app/orders.go, devolucion.go, pedidos_de_plataforma.go
├── internal/httpapi/handlers_composition.go, router.go, respond.go
├── cmd/fudo-import/compositions.go
└── internal/integration/stock_composition_test.go, stock_composition_migration_test.go
web/src/
├── features/admin/CompositionEditor.tsx (+ test), shared/ProductEditDialog.tsx, OptionFormDialog.tsx
└── api/admin.ts
```
