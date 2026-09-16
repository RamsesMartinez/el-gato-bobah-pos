# Costear recetas: qué hace la industria, qué se abandona y por qué

**Investigado el 2026-09-14.** Referencia previa a cualquier spec de costeo o de inventario de
insumos. No es un plan: es lo que se sabe y con qué respaldo.

Este documento existe porque la pregunta *«¿cuánto me cuesta de verdad un platillo?»* es la que
decide el precio en plataformas que cobran 30%, y porque el costeo es una de las features que más se
abandona a medio camino. Lo segundo está documentado con nombres.

**Niveles de evidencia**: **[D]** documentado en fuente primaria · **[R]** reportado por terceros ·
**[I]** inferido.

> **Los números del negocio NO van aquí.** Recetas, costos de proveedor y márgenes propios viven en
> la base de producción, no en el repositorio (ver la regla en `AGENTS.md` §1). Lo que se documenta
> aquí son hechos que siguen siendo ciertos en otro restaurante.

---

## 1. La respuesta corta

**Lo indirecto se parte en dos cubos, y solo uno se prorratea por platillo.**

| Cubo | Qué hace la industria | Nivel |
|---|---|---|
| **Consumibles chicos** — sal, aceite, especias, grasa de freidora | Un **factor fijo de 5–15%** sobre el costo crudo, o una cifra plana por plato (`$0.10`). Los operadores lo llaman **«Q factor»** | [R], tres testimonios independientes |
| **Renta, luz, agua, sueldos** | **No se reparten por platillo.** Se manejan con **margen de contribución** para decidir precio y **punto de equilibrio** para lo fijo | [R], por ausencia: ningún caso del corpus los prorratea |

El argumento económico que cierra la discusión, de un operador:

> *«9 de cada 10 veces le agrega unos centavos a la receta… El costo total de averiguarlo fueron
> como $250 por dos personas para todo el catálogo. Así que, por querer ser meticuloso con el costo,
> desperdiciaron $248. ¡NUNCA OLVIDES EL COSTO DE LA MANO DE OBRA!»*

La consecuencia de diseño: **perseguir el costo exacto de la sal cuesta más que la sal.** El costeo
detallado se reserva para lo que pesa —proteínas y los insumos caros— y el resto entra como factor.

---

## 2. Lo que cuesta capturarlo, medido

| Qué | Cuánto | Nivel |
|---|---|---|
| Receta simple | **2.5 h por platillo** — *«unos 40 ítems de menú y tomó probablemente 100 horas al principio»* | [R] |
| Receta con sub-recetas anidadas | **hasta 4 h** — *«tienes que costear el kimchi, luego el gochujang, luego la mayonesa de gochujang, luego la salsa de queso con gochujang»* | [R] |
| Mantenimiento, una vez capturado | *«ahora dedico 5 minutos a la semana a actualizar precios»* | [R] |
| Conteo físico | 2 h semanales (barra), 4–6 h mensuales con captura, hasta **7 h** en un conteo corporativo | [R] |

**La captura no es lo que mata la feature.** De cinco abandonos documentados con nombre de producto
—R365, xtraCHEF, MarginEdge y dos más—, **todos son por mantenimiento**, ninguno por la carga
inicial:

> *«Intentar configurar y mantener recetas es doloroso. Hoy en día solo mantenemos los ítems de
> mayor volumen.»*

Y un local de 20 años que pasó por cuatro sistemas paga **3% de su COGS** en software y un
administrador, y sigue sin cuadrar la comida.

### Lo que rompe el mantenimiento no es el precio: es el SKU

> *«Tienes que comprar pimientos verdes de otro proveedor y entonces necesitas un producto nuevo,
> un grupo nuevo insertado en todas tus recetas, y es interminable.»* — siete a ocho años operando
> el sistema.

**Este repo ya resolvió ese problema**, y conviene saberlo antes de diseñar encima:
[supplier_items](../server/migrations/0030_supplier_items.sql) separa el código del proveedor del
insumo interno y aprende el mapeo por `(proveedor, código|nombre)` con similitud por trigramas. Es
el activo más valioso del inventario actual.

---

## 3. Dónde está el dinero mal puesto: en las opciones, no en los platillos

Los dos únicos errores de costeo **cuantificados** de todo el corpus son de extras, y los dos
estaban invertidos:

| Caso | Se cobraba | Costaba | Food cost |
|---|---|---|---|
| Vinagreta de miel | $0.50 | **$0.65** | **130%** — perdía $0.15 cada vez |
| Tocino extra (2 rebanadas) | $2.00 | $1.48 | **74%** |
| Carne extra (1 pieza) | $3.00 | $1.37 | 46% |

> *«Realistamente esos precios deberían estar invertidos.»*

**Los dos se descubrieron costeando recetas, no contando existencias.** Es el argumento más fuerte
para que el costo viva como snapshot en la línea de venta y no dependa de un módulo de inventario.

**El patrón general, y es el que aplica a cualquier catálogo configurable**: un grupo de
modificadores con **precio plano** sobre opciones de **costo distinto** garantiza que algunas dejen
dinero y otras lo pierdan, sin que nada lo señale. Cuantas más opciones tenga el grupo, mayor la
dispersión.

---

## 4. Qué costear, según quien lo hace

No es «todo» ni es «solo los más vendidos». Un contralor lo acota:

> *«Hazlo solo para tus 5 ítems de mayor valor, semanalmente. Proteínas primero… un hueco semanal
> de 15 libras en res es dinero real: a $4 la libra son más de $3,000 al año en un solo
> ingrediente.»*

Y sobre los conteos, el veredicto de un operador con décadas:

> *«Nunca en mi experiencia he visto un restaurante que haga inventarios significativos con éxito.»*

**Consecuencia para el alcance de cualquier spec**: conteo físico y varianza quedan **fuera** del
mínimo. El mínimo es costear lo que pesa y mantenerlo solo.

---

## 5. Lo que este documento NO cubre

- **Cero fuentes mexicanas.** El corpus es Estados Unidos, con casos sueltos de Reino Unido,
  Australia y República Checa. Nada medido contra precios, proveedores ni salarios de México. **Los
  porcentajes de «Q factor» hay que medirlos aquí antes de adoptarlos.**
- **Ningún caso del tipo «creía que mi food cost era 30% y era 42%».** Esa pregunta casi no se
  contesta en público; el único hilo que la hizo fue rechazado por la comunidad: *«Las varianzas no
  son "pérdida"… Literalmente el número de nadie más importa, solo el tuyo.»*
- **Los roles son autodeclarados** de foros de operadores, verificados contra el historial de cada
  cuenta. Es **[R]** fuerte, no fuente institucional.
- **La literatura académica no usa el término «Q factor».** Existe en la práctica, no en los
  papers — si alguien lo busca en fuentes formales no lo va a encontrar, y eso no lo invalida.

---

## 6. Qué dice esto sobre el orden de construir

1. **Recalcular el costo cuando cambia su insumo** es la precondición de todo. Un costo que no se
   recalcula solo es un costo viejo, y un precio decidido sobre él es un precio viejo.
2. **Costear las opciones de los grupos con precio plano** es lo que más dinero mueve por hora de
   captura: son porciones de un solo insumo, minutos cada una, y es donde la industria documenta
   sus dos únicos errores cuantificados.
3. **El costo indirecto NO se prorratea.** Lo que hace falta es margen de contribución y punto de
   equilibrio, que son dos cálculos, no un módulo.
4. **Conteo físico y varianza, fuera** hasta que lo anterior se mantenga solo.

## Mantenimiento

Referencia viva, indexada en [docs/README.md](README.md). Cuando se midan datos propios —recetas,
costos de proveedor, el «Q factor» real de este negocio— **los números van a la base, no aquí**;
lo que se actualiza aquí es el hecho general que hayan confirmado o desmentido.
