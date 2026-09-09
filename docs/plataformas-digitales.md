# Plataformas digitales — lo que cobran, lo que retienen y lo que no se sabe

**Medido en septiembre de 2026** contra documentos fiscales y reportes reales de Uber Eats, Rappi y
DiDi Food, más el respaldo de FUDO ([respaldo-fudo.md](respaldo-fudo.md)). Lo que está aquí **no se
vuelve a derivar**: se cita.

Este documento existe porque las cifras se midieron una vez, costaron dos días de trabajo y viven en
documentos que no están en el repo (son fiscales y no se commitean). Un spec, un agente o una
persona que necesite saber cuánto cobra una plataforma lee esto, no vuelve a abrir los PDFs.

> **Los documentos fuente NO están en el repo.** Viven en `D:\investigaciones` en la máquina del
> dueño, organizados por plataforma. Traen RFC, domicilio y datos bancarios: no se commitean.

---

## 1. Las comisiones, medidas

| Plataforma | Comisión | Base | Certeza |
|---|---|---|---|
| **Uber Eats** | **30.00% exacto** | Venta **con IVA**, por pedido | **Medida** — 4 pedidos de agosto 2026 y su CFDI mensual, dos vías independientes |
| **DiDi Food** | **30.00% exacto** | Venta **con IVA**, por pedido | **Medida** — 14 pedidos en 6 semanas y 5 facturas semanales |
| **Rappi** | **20.00% exacto** | Venta **con IVA**, por pedido | **Medida** — 7 de 8 pedidos del reporte de pagos (jul-sep 2026) |

**El IVA de la comisión (16%) se cobra aparte** y es acreditable. El costo total es entonces
`1.16 × c` del precio de plataforma: **34.80% en Uber y DiDi, 23.20% en Rappi**.

### Rappi cobra 10 puntos menos, y eso invierte el veredicto

Con la misma configuración de **35% de markup en las tres**, el resultado no se parece:

| | Comisión | Markup necesario | Con 35% recibe | |
|---|---|---|---|---|
| Uber Eats | 30% | 53.37% | $88.02 de cada $100 | **pierde** |
| DiDi Food | 30% | 53.37% | $88.02 de cada $100 | **pierde** |
| **Rappi** | **20%** | **30.21%** | **$103.68 de cada $100** | **gana** |

Es la justificación de fondo de tener un precio por plataforma y no un margen único: **el mismo 35%
está 18 puntos corto en dos plataformas y casi 5 puntos largo en la tercera.**

El caso raro que se midió: un pedido del 23 de agosto pagó **13% de comisión** en vez de 20%, y es el
único donde Rappi absorbió el descuento completo. Con un solo caso no se puede derivar la regla; se
anota para no confundirlo con un error.

> **Cómo se cerró.** Los CFDI de Rappi no alcanzan: traen el importe de la comisión pero su `Base` es
> la comisión misma, no la venta, y su complemento fiscal trae `ComisionDelServicio` con
> `Base = 0.01` y `Porcentaje = 1` **idénticos en dos meses con ventas que difieren 47%** — es
> relleno, no dato. Lo que sí sirve es el **reporte de pagos del portal de aliados**
> (`Rappi_Report_<desde>_<hasta>.xlsx`, hoja **`Detalle`**): la columna *Uso y alquiler de plataforma
> Rappi* dividida entre *Venta y descuentos para llegar a la venta neta* da la comisión por pedido.
> Las hojas `Resumen` e `Indice` vienen en cero; toda la información está en `Detalle`.

### No se puede negociar

Decisión del dueño, septiembre de 2026: **el negocio no tiene volumen para negociar comisiones ni es
tienda destacada en ninguna plataforma.** DiDi está en 30% —el tope de su banda— porque no hay
reparto propio; el plan de 8.5% de "DiDi Tu Negocio" exige repartidor propio y no aplica.

Consecuencia para cualquier spec: **la comisión es un dato de entrada, no una variable sobre la que
se pueda actuar.** La única palanca es el precio.

---

## 2. La fórmula del precio de equilibrio

Para que el negocio reciba **lo mismo** que vendiendo en mostrador, dado un precio de mostrador `P`
y una comisión `c` cobrada sobre el precio con IVA:

```
U = P / (1 - 1.16 · c)          markup = (1.16 · c) / (1 - 1.16 · c)
```

| Comisión | Markup necesario |
|---|---|
| 15% | 21.1% |
| 20% | 30.2% |
| 25% | 40.8% |
| **30%** | **53.37%** |
| 35% | 68.4% |

### El error que hay que no cometer

**Subir 30% NO compensa una comisión del 30%.** La comisión se cobra sobre el precio ya subido, que
es mayor. No se suma: **se divide**.

Y el `1.16` aplica **aunque el IVA de la comisión sea acreditable**. La razón no es el IVA en sí: es
que la comisión se calcula sobre el precio **con IVA incluido**, y ese IVA no es ingreso del negocio.
Un cálculo que use `c / (1 - c)` da 42.86% en vez de 53.37% y deja 10 puntos sobre la mesa.

### Estado al momento de medir

`price_markup_pct` valía **35% en las tres plataformas**, con **cinco** precios manuales de
excepción: cuatro en DiDi entre 53% y 109% (bien puestos) y **uno en Uber a −0.7%** — un producto que
se vendía en la plataforma **más barato que en el mostrador**, con Uber llevándose además el 30%.

El resultado por plataforma con ese 35% está en la tabla de §1: pierde 12% en Uber y en DiDi, y gana
3.7% en Rappi.

> **Lo que NO se puede afirmar**: cuánto dinero perdió el negocio en pesos en un mes dado. Se intentó
> y la verificación adversarial lo tumbó: exige conocer el precio de mostrador de cada producto
> vendido en plataforma en esa fecha, y los documentos de plataforma no lo traen. Lo defendible es la
> **tasa**, no el monto. Ver §6.

---

## 3. Cómo llega cada peso, y con qué grano

Tres periodicidades distintas conviven y **el esquema no puede forzarlas a coincidir**:

| Concepto | Grano | Documento |
|---|---|---|
| Comisión + su IVA | **Por pedido** | Reporte de ganancias (Uber), libro semanal (DiDi) |
| Depósito | **Semanal** | Resumen de pagos |
| Retención de ISR e IVA | Se calcula por pedido, se liquida por semana, **se certifica por MES** | CFDI de retenciones |

- **Retención de IVA = 50% del IVA de las ventas.** Medido en Uber, DiDi y Rappi.
- **Retención de ISR = 2.5% de la base sin IVA.** Es la tasa vigente desde 2026 para el régimen de
  plataformas; antes era 1%.
- Uber **parte su 30% en dos CFDI semanales**: "Servicios de Intermediación digital" (~2%) y
  "Servicios de inteligencia comercial y soporte administrativo" (~98%). Sumados dan la Tasa de
  servicio exacta. Los dos son CFDI de ingreso con IVA acreditable.
- Rappi **solo tiene grano mensual**. No existe ningún documento suyo con detalle por pedido ni por
  semana.

### La conclusión de esquema, y su porqué

**Las retenciones necesitan dos tablas, no una:**

- **Por pedido** (opcional, se puebla solo donde la plataforma la expone: Uber y DiDi).
- **Certificada por mes** (obligatoria, una por plataforma-mes, con el total del CFDI, su UUID y sus
  bases).

Y la regla que decide: **el total que se declara y se acredita sale SIEMPRE de la tabla mensual,
nunca de sumar pedidos.** Porque no coinciden — DiDi difiere $0.14 y Uber $2.77 al mes. Poco, que es
lo peor: nadie lo audita. **La diferencia se guarda como campo, no se corrige**, porque es real y se
repite cada mes.

Colgar la retención solo del pedido falla en Rappi por falta de dato, y falla en Uber y DiDi por
centavos que no cuadran contra el comprobante fiscal.

---

## 4. Inconsistencias encontradas en los documentos

Están aquí para que nadie las vuelva a descubrir ni las tome por un error propio.

### Cuestan dinero o cambian una decisión

| Qué | Consecuencia |
|---|---|
| **Rappi: FUDO registra $460 de agosto y su CFDI implica $354.99** | $105 sin explicar |
| **Los nombres de archivo de Rappi mienten**: cada PDF de retención corresponde al XML del *otro* mes | Cruzar por nombre asigna el mes equivocado y mueve la base $280.61 |
| **Uber, julio 2026: su CFDI mensual reporta $134.70 de comisión que ningún CFDI semanal ampara** | Sin soporte deducible, ni sus $21.55 de IVA |
| **Uber cobró IVA de 15% en tres artículos y 16% en cuatro**, el mismo mes | Su CFDI declara $5.57 más de IVA trasladado del que cobró. Cada mes, en silencio |
| **DiDi, julio 2026: el CFDI implica $2,756 de ventas y los libros solo muestran $742** | Falta la mayor parte del mes en grano de pedido |

### Encabezados que mienten

**El reporte a nivel artículo de Uber rotula la comisión "Tasa de servicio (con IVA)" y el de nivel
pedido rotula la MISMA cifra "sin IVA".** La correcta es **sin IVA**: 76.20 es el 30% de 254.00 y su
IVA se reporta aparte. Quien tome el archivo de artículos como buena fuente **subestima el costo en
4.8 puntos**.

### Archivos que se repiten (riesgo de doble conteo)

- **Los nueve archivos de DiDi contienen 14 pedidos únicos, no 20.** Los "Recibo DiDi" repiten
  semanas que ya están en los "Bill Report". Cargar ambos duplica $935 de venta.
- **La factura semanal de DiDi y el renglón `Commission` de su libro son el mismo cobro.** Igual el
  `Platform VAT Deduction` del libro y el "IVA 16% de la sucursal" de la factura.
- **El CFDI mensual de retenciones de Uber NO es un comprobante de ingreso.** Su `MontoTotOperacion`
  son las **ventas del negocio**, no un cobro de Uber. Meterlo en la misma columna que los CFDI
  semanales mezcla ingreso con gasto.
- **El depósito ya viene neto.** Registrarlo como ingreso *y además* deducir la comisión cuenta el
  costo dos veces.
- Las cuatro piezas de Rappi son **dos** CFDI, no cuatro: cada uno con su PDF y su XML.

### Diferencias de centavos, y lo que significan juntas

Uber declara $91.86 de IVA retenido donde los depósitos descontaron $89.09. DiDi retiene $8.19 donde
la base impresa en el mismo renglón por 2.5% da $8.23. Su CFDI mensual dice $71.58 de ISR donde la
suma de pedidos da $71.44. Un pedido de Uber retiene $17.10 donde el 50% del IVA da $17.09, sin regla
de redondeo que lo explique.

Ninguna importa sola. Juntas sostienen la decisión de esquema de §3: **la suma de pedidos nunca
reproduce el CFDI mensual.**

---

## 4-bis. Promociones — quién las paga y sobre qué se cobra la comisión

Medido en un **2x1 real de Uber Eats** (pedido del 5 de septiembre de 2026, dos sodas de $110):

| Concepto | Monto |
|---|---|
| Ventas (con IVA) — precio de lista de las dos | $220.00 |
| **Promociones en artículos** | **−$110.00** |
| **Tasa de servicio (con IVA)** | **−$38.28** |
| **Tarifa de canje de la oferta (con IVA)** | **−$9.99** |
| IVA retenido | −$7.59 |
| ISR retenido | −$2.37 |
| **Ganancias netas** | **$51.77** |

Tres reglas que salen de ahí, y las tres cambian el cálculo del precio sugerido:

1. **El restaurante absorbe el 100% de la promoción en Uber.** Los $110 salen íntegros de su lado.

   **En Rappi NO hay regla: depende de la campaña.** Su reporte trae una columna *Descuentos
   asumidos por Rappi* con $283.50 en el periodo, pero eso NO significa que Rappi sistemáticamente
   absorba. Según el asesor de la cuenta, **a veces** Rappi corre campañas propias en las que asume
   un porcentaje del descuento; no siempre, y el porcentaje varía. Sobre 8 pedidos aparece en 3, con
   montos que van de absorber una parte a absorber el total.

   **Consecuencia de diseño, y es la importante: quién paga una promoción es un dato DEL PEDIDO, no
   un ajuste de la plataforma.** Un campo de configuración del tipo "Rappi absorbe X%" mentiría el
   día que no haya campaña. Se lee del pedido o no se sabe.
2. **Las retenciones van sobre el NETO** (después del descuento), y eso sí es inequívoco: `7.59` es
   la mitad del IVA contenido en $110, y `2.37` es el 2.5% de su base. Sobre $220 no cuadran.

   **La base de la COMISIÓN de Uber, en cambio, sigue sin resolverse.** El dato observado
   (`38.28 / 1.16 = 33.00`) es compatible con dos lecturas que este pedido no distingue, porque el
   descuento fue exactamente la mitad: `30% de $110` y `15% de $220` dan el mismo $33. Y la
   documentación de la propia columna de Uber dice textualmente que **«%fee se aplica al subtotal
   antes de los descuentos»**, lo que apunta a la segunda — o sea, a una tasa reducida durante la
   campaña, no a una base reducida.

   **Lo cierra un pedido con promoción que NO sea de 50%.** Hasta entonces, el cálculo del precio
   sugerido debe asumir la lectura cara —comisión completa sobre el precio de lista— porque
   equivocarse de ese lado cuesta margen y del otro cuesta dinero.

2-bis. **DiDi sí está resuelto, y es la lectura cara.** Su comisión es el 30% de
   `Original Item Price`, que su propia hoja de definiciones llama *"Precio total del producto **sin
   promoción**"*. Medido en 13 pedidos. En DiDi una promoción cuesta el descuento **más** la comisión
   sobre el precio completo.
3. **Hay un cargo que solo existe cuando hay promoción**: la *Tarifa de canje de la oferta*. En los
   datos de agosto venía en cero porque no hubo promociones, así que un modelo construido solo con
   agosto no la contempla.

**Lo que cuesta de verdad un 2x1**: vendiendo una sola soda a $110 sin promoción, las ganancias netas
habrían sido $61.76. Con el 2x1 son $51.77 y se entregaron **dos** sodas. O sea, la promoción cuesta
**la tarifa de canje ($9.99) más el costo del producto regalado** — y ese segundo término hoy no se
conoce, porque no hay costeo por receta. Es el puente entre esta feature y la de ingredientes.

### La trampa del rótulo "con IVA"

**El mismo rótulo significa cosas distintas según dónde se lea.** En el detalle del pedido de Uber
Eats Manager, "Tasa de servicio (con IVA) −38.28" **sí** incluye el IVA (33 × 1.16). En el CSV a
nivel artículo, la columna rotulada igual **no** lo incluye. Verifica siempre dividiendo entre la
venta: si da 30% exacto, es sin IVA; si da 34.8%, es con IVA.

### Los reportes van con retraso

Un pedido de hoy **no aparece todavía** en el reporte de ganancias ni en el estado de cuenta: entra
cuando cierra el periodo de pago. Un mes no está completo hasta que se liquida el periodo siguiente,
y eso explica parte de los huecos entre meses. **No leas un mes en curso como si estuviera cerrado.**

## 5. Restricciones que no son técnicas

- **Uber Eats México exige paridad de precios con la tienda física, "salvo pacto en contrario"**
  (Términos y Condiciones para Comercios, sección 3.1). Y en su modalidad de reventa, si el precio al
  usuario excede el de tienda, **la diferencia se la queda Uber**. Subir precios en Uber es
  negociable, no automático.
- **Rappi y DiDi no muestran esa restricción.** DiDi dice explícitamente que el restaurante fija el
  precio que se muestra.
- El negocio es **persona física con actividad empresarial en el régimen de plataformas digitales**,
  y **acredita el IVA de las comisiones** (dato del dueño, septiembre de 2026).

---

## 6. Lo que NO se sabe, y qué documento lo cerraría

| Pregunta abierta | Qué la cierra |
|---|---|
| Si el 35% está realmente aplicado en las tres apps, artículo por artículo | El export del menú con precio y tasa de impuesto de cada plataforma |
| Por qué tres artículos de Uber salen con IVA de 15% | El mismo export: trae la tasa configurada por artículo |
| Cómo se calcula la *Tarifa de canje de la oferta* de Uber ($9.99 sobre un descuento de $110) | Dos o tres promociones más, para ver si es fija, porcentual o escalonada |
| Por qué un pedido de Rappi pagó 13% en vez de 20% | Más pedidos con descuento asumido por Rappi |
| El soporte deducible de las comisiones de DiDi | Sus CFDI (XML) semanales; hoy solo hay el recibo comercial en PDF |
| Julio 2026 de Uber | Sus CFDI semanales y su reporte de ganancias de ese mes |
| Si DiDi cobra su comisión antes o después del descuento, como Uber | Un pedido de DiDi con promoción activa |

**Cuánto perdió el negocio en pesos** no está en esta tabla a propósito: no es una pregunta que un
documento de plataforma pueda contestar. Exige el precio de mostrador de cada producto en la fecha de
la venta, y eso vive en el catálogo, no en la plataforma.

---

## 7. Cómo se midió

Dos workflows de agentes en paralelo sobre 47 documentos fiscales y 18 archivos de FUDO, con
**verificación adversarial de cada cifra de dinero**: tres agentes por afirmación, con lentes
distintas (aritmética, fuente, alcance) y con el encargo de **refutar, no de confirmar**.

Valió la pena: **de las primeras 8 afirmaciones en pesos, 7 fueron objetadas**. La que más importa,
por lo que enseña: se calculó un "faltante de $118 en agosto" dividiendo las ventas de Uber entre
1.35, o sea **asumiendo** que los precios en la app llevaban el markup configurado. El reporte a
nivel artículo probó que no lo llevaban.

**Lección para el próximo que mida esto: una tasa se puede afirmar; un monto en pesos exige conocer
el precio de mostrador de cada producto, y ese dato no está en los documentos de plataforma.**
