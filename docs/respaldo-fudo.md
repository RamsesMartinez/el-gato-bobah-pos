# El respaldo de FUDO — qué trae y cómo se lee

**Analizado en septiembre de 2026.** FUDO fue el sistema del negocio hasta el **28 de agosto de
2026**; el POS propio arrancó el **29 de agosto**. El respaldo es la única copia de todo lo anterior.

Este documento existe para que nadie vuelva a pelearse con estos archivos. Traen dos años y medio de
ventas reales, y la mitad del trabajo de leerlos es descubrir que los nombres mienten.

> **El respaldo NO está en el repo.** Vive en `C:\Users\ramys\OneDrive\Documentos\Backup Fudo` en la
> máquina del dueño. Es su única copia — un respaldo que solo existe en una carpeta sincronizada no
> es un respaldo.
>
> No confundir con [`references/`](../references/), que son exports de FUDO usados por el importador
> de catálogo (`cmd/fudo-import`) y sí están versionados.

---

## 1. Los nombres de archivo no dicen el año

Se dedujo comparando cada `.xls` suelto contra el `.zip` gemelo, byte por byte:

| Archivo | Año | Tamaño |
|---|---|---|
| `ventas.xls` | **2024** | 5,040,640 |
| `ventas 3.xls` | **2025** | 6,750,720 |
| `ventas 2.xls` | **2026** (2 ene – 30 ago) | 3,460,608 |

Los `.zip` (`ventas_2024.zip`, etc.) contienen exactamente esos mismos archivos. Ignóralos.

Además: `productos.xls`, `ingredientes.xls`, `gastos-*.xlsx`, `movimientos-de-caja_{2024,2025,2026}_*.xlsx`,
`propinas-{2024,2025,2026}_*.xlsx`.

## 2. Cómo se leen

Los de ventas, productos e ingredientes son **`.xls` binarios legacy (OLE2)**. `openpyxl` no los
abre y **`xlrd` 2.x tampoco** — hay que fijar la 1.2.0:

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "C:/Users/ramys/OneDrive/Documentos/Backup Fudo:/f:ro" \
  python:3.11-slim sh -c "pip install --quiet 'xlrd==1.2.0' pandas openpyxl; python -c \"
import xlrd
b = xlrd.open_workbook('/f/ventas 2.xls')
print(b.sheet_names())
s = b.sheet_by_index(0)
print(s.nrows, s.ncols, s.row_values(3))
\""
```

Los `.xlsx` (gastos, movimientos, propinas) son modernos: `openpyxl` con `data_only=True`.

Dos trampas de formato:

- **Los encabezados están en la fila 4**, no en la 1. Las filas 1-2 traen el rango del export.
- La columna `Fecha` es **texto** `'YYYY-MM-DD'`, pero `Creación` y `Cerrada` son **seriales de
  Excel**. Convierte con `xlrd.xldate_as_tuple(v, b.datemode)` y **verifica una contra un dato
  conocido** antes de confiar en el resto: una fecha mal convertida arruina cualquier análisis por
  mes.

## 3. Estructura de un archivo de ventas

Nueve hojas, con el mismo formato en los tres años (verificado columna por columna — un importador
sirve para los tres):

| Hoja | Grano | Ojo con |
|---|---|---|
| `Ventas` | Un **pedido**. 18 columnas | — |
| `Adiciones` | Un **renglón** de producto | **`Precio` YA es el importe del renglón, no el unitario.** Multiplicar por `Cantidad` produce descuadres falsos |
| `Adiciones de Modificadores` | Un modificador aplicado a un renglón | Su `Precio` suma al total del pedido |
| `Pagos` | Un **movimiento de cobro**. Un pago mixto genera dos filas | — |
| `Descuentos` | Un descuento por pedido | — |
| `Costos de Envío` | Un envío por pedido | **Todos en $0.00 en 2026** |
| `Propinas` | Una propina por pedido | Incluye canceladas sin distinguirlas en el total |
| `Más Info` | Resumen llave/valor del propio FUDO | Sirve para validar tu suma contra la suya |
| `Productos` | Catálogo con acumulado del periodo | — |

**Composición del total, verificada en 1,820 de 1,820 ventas cerradas de 2026:**

```
Total = Σ(Precio de Adiciones) + Σ(Precio de Modificadores) − Σ(Descuentos no cancelados)
```

Y `Σ(Pagos no cancelados) = Total` en las 1,820. **La propina queda fuera del total y fuera de los
pagos**: sumarla infla el ingreso.

## 4. La regla de canal — no hay una columna, hay dos

Esto es lo más fácil de hacer mal:

```
Uber  = Origen == 'Uber Eats'
Rappi = Origen == 'Rappi'
DiDi  = Medio de Pago CONTIENE 'Online Didi'      <- DiDi NUNCA llena Origen
resto = Mostrador
```

Clasificar Uber o Rappi por medio de pago manda a Uber una venta de **mostrador** pagada con
"Efectivo Uber Eats" y rompe el cuadre. La regla de arriba reproduce exacto el resumen del propio
FUDO.

`Id. Origen` trae el folio de la plataforma y está poblado en los tres canales, DiDi incluido: UUID
en Uber, numérico largo en DiDi, de 10 dígitos en Rappi. Es el ancla para conciliar contra los
reportes de plataforma.

## 5. El hueco de agosto de 2026

| | Pedidos | Venta |
|---|---|---|
| Mostrador en FUDO (1–28 ago) | 126 | $30,396 |
| **Plataformas en FUDO** | **19** | **$5,113** |
| Mostrador en el POS (29–31 ago) | 20 | $3,949 |
| **Total real de agosto** | **165** | **$39,458** |

**Traslape cero, verificado**: el último pedido de mostrador de FUDO es del 28 de agosto y el primero
del POS es del 29. Los dos sistemas no se pisan.

Las plataformas de agosto —DiDi 11/$3,321, Uber 4/$1,332, Rappi 4/$460— **no están en el POS**, y
este respaldo es su único registro. Uber **cuadra al peso y periodo de pago por periodo de pago**
contra su propio reporte de ganancias, lo que valida la regla de canal de §4.

Dos huecos que este respaldo **no** cierra: el archivo se corta el 30 de agosto a las 19:50, así que
**el 31 de agosto no existe en ningún sistema**; y dos pedidos "Eliminada" con folio de DiDi quedaron
sin renglones y en $0.

## 6. Lo que el histórico habilita

| Dato | Para qué |
|---|---|
| **664 días con venta en 2024-2025**, sin hueco de más de 3 días | Comparativo año contra año en el dashboard desde el día uno |
| **Hora de creación en el 100% de los pedidos** | Mapa de calor real: pico a las 20h, franja 18-21h = 60-69% de los pedidos |
| Día de la semana | Viernes a domingo hacen 11-13 pedidos/día contra 5-7 el resto |
| Serie de participación de plataformas | 9.3% (2024) → 23.2% (2025) → 12.5% (2026) |
| 2026 ene-ago | $398,256.75 en 1,829 pedidos, ticket promedio $219 |
| **El 100% de los nombres de producto vendidos existe en el catálogo actual** | Cero tabla de equivalencias para importar |

## 7. Trampas para un importador

1. **`Precio` de Adiciones ya es el importe del renglón.** Ver §3.
2. **Las propinas incluyen canceladas** en el total: sumar `Valor` crudo infla 18% (2024) y 27% (2025).
3. **La clasificación de canal** de §4.
4. **`Producto Genérico` y `Opcional Genérico`** ensucian cualquier reporte por producto —
   `Opcional Genérico` fue el "más vendido" de 2025 con 1,182 unidades. Necesitan bandera.
5. **En los pedidos de Rappi el nombre real del producto vive en la columna `Comentario`**, no en
   `Producto`. Un importador que no la lea pierde el detalle entero.
6. **139 pedidos cerrados en $0** (2025) inflan el conteo 3.9% y bajan el ticket promedio.

## 8. Caja, gastos y propinas — leer con cuidado

- **La mitad de los movimientos de caja son traspasos internos** entre las cajas "Principal" y
  "Bolsa": 210 pares detectados. Sumar los egresos como salidas de dinero **duplica $242,157**.
- **No hay cortes de caja en el respaldo.** Los movimientos son cada entrada y salida individual;
  no hay esperado, contado ni diferencia. No se puede contar "cuántos cortes hubo".
- **FUDO nunca registró las comisiones de plataforma como gasto, ni el depósito neto como ingreso.**
  Cero coincidencias buscando `uber|rappi|didi|comisi|plataforma` en 1,129 gastos y 871 movimientos.
  Las plataformas nunca estuvieron en la contabilidad operativa — ni por lo que vendían ni por lo que
  costaban. Es la razón de fondo de que la pérdida no se notara.
- **Las propinas se clasificaron como gasto** bajo "Pago colaboradores", con categoría inconsistente
  (32 de 36 filas). Y **no cuadran**: $2,833.10 cobradas contra $1,420.60 pagadas, $1,412.50 sin
  salida registrada.
- El 74% de las propinas está atribuido al usuario genérico "Mostrador", no a una persona.
- **Huecos de captura que no son meses sin operación**: gastos sin 2024-09 ni 2024-12; movimientos
  sin nada entre agosto de 2024 y febrero de 2025. Una serie mensual sobre ellos reporta cero donde
  faltó captura.
- **Los gastos de agosto NO se duplicaron**: el POS solo tiene 2 gastos y ambos son de septiembre.

## 9. Sobre importarlo

**Recomendación: sí, los tres años, y a una tabla histórica propia — nunca a `orders`.**

El riesgo que parecía mayor **no existe**: un arqueo cerrado no se puede mover desde `orders`, porque
`register_session_totals` guarda un snapshot al cerrar y no recalcula. Eso ya está bien resuelto.

Lo que sí impide meterlos en `orders` son tres cosas duras del esquema:

- `orders.daily_number` es único por día y lo administra la bolsa de folios: importar inventaría
  folios que nunca se imprimieron y que chocan con los del POS en cualquier día compartido.
- `orders.opened_by` es `not null references users(id)`: habría que inventar un usuario "FUDO" que
  después aparece en los reportes por cajero.
- `order_lines.product_id` es `not null references products(id)`, y los renglones "Producto
  Genérico" **no tienen producto**.

Además hay ocho consultas que filtran solo por `business_date` y `status` — `SalesByDay`,
`SalesByMethod`, `RefundsByDay`, `ProductMargins`, `ListSales`, `CountSales`,
`SalesTotalsByStatus`, `SalesTotalsByMethod` — donde el dinero importado entraría sin distinguirse de
una venta real.

El volumen es trivial: ~7,900 pedidos y ~23,500 renglones entre los tres años.
