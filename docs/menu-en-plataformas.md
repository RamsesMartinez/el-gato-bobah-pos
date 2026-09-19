# Cuántos platillos publicar en una plataforma, y cómo agruparlos

**Investigado el 2026-09-15.** Referencia previa a cualquier decisión de qué se publica en Uber
Eats, Rappi o DiDi Food. Es una decisión de negocio, no de arquitectura: el modelo de datos soporta
las dos formas (ver principio VIII de la constitución, puerta *«una lista de productos por
plataforma»*).

**Niveles de evidencia**: **[D]** documentado en fuente primaria · **[R]** reportado por terceros ·
**[I]** inferido.

> **Los números del negocio no van aquí.** Lo que se documenta son hechos que siguen siendo ciertos
> en otro restaurante (AGENTS.md §1).

---

## 1. La pregunta y la respuesta corta

La hipótesis a validar era: *publicar el mismo producto partido en varias variantes hace que
aparezca más veces en el buscador de la app, y por lo tanto vende más.*

**No tiene respaldo documental en ninguna de las tres plataformas.** Y la única evidencia
cuantitativa pública apunta en la dirección contraria.

Pero la hipótesis **no es absurda**, y conviene saber por qué:

| | Qué dice la evidencia | Nivel |
|---|---|---|
| **En contra** | DoorDash + Technomic: **90% de los comercios que redujeron su menú de delivery reportan impacto positivo en ventas**. Uber y Rappi recomiendan menús cortos y navegación simple en sus guías oficiales | [D] como publicación — **con el sesgo a la vista**: es un estudio de la propia plataforma, con comercios auto-reportando. No es causal |
| **A favor** | El buscador de Uber Eats es **semántico**: consulta y platillo se embeben en el mismo espacio vectorial a partir de **título y descripción del item**. Un nombre específico («Chamoyada de Mango») empata mejor una búsqueda literal que uno genérico («Arma tu Chamoyada») | [D] el mecanismo · [I] la ventaja concreta — ningún documento dice «nombra tus variantes para aparecer más» |
| **Ni una cosa ni otra** | Qué porcentaje del tráfico llega por buscador y cuánto por navegación de la home | **No encontrado**, ni oficial ni de terceros. Es el dato que decidiría la pregunta, y no es público |

**Veredicto**: partir por **sabor**, con nombre y foto propios, es merchandising defendible y tiene
un mecanismo técnico que lo respalda. Partir por **tier de precio** —«1 ingrediente», «2
ingredientes», «3 ingredientes»— no tiene ningún respaldo encontrado y sí cuesta mantenimiento.

---

## 2. Lo que NO se encontró, y hay que decirlo

- **Cero evidencia de México.** Todo el corpus cuantitativo es de Estados Unidos. De DiDi Food
  México solo hay guías genéricas (fotos, descripciones, combos), nada sobre número de platillos ni
  sobre su buscador.
- **Rappi y DiDi no documentan su motor de descubrimiento.** Lo que se sabe del ranking es de Uber.
- **Nadie ha publicado una medición** —ni un blog, ni un foro, ni un paper— de un restaurante que
  haya convertido un producto configurable en variantes fijas y reportado el resultado.
- Una cifra que circula («12 a 20 platillos, evitar más de 30») se atribuye a Rappi en resúmenes de
  búsqueda, pero **no aparece en la página oficial al abrirla**. No se usa.

Con esos huecos, la decisión se toma con el costo operativo, que sí es medible, y no con la promesa
de ventas, que no lo es.

---

## 3. El costo de partir, que sí se puede medir

- **Un cambio de precio se multiplica por el número de variantes.** Doce variantes de lo mismo son
  hasta doce ediciones en el portal cada vez que cambia el costo del insumo [I].
- **El agotado depende de dónde esté el insumo.** Si se acaba algo común a todas las variantes —el
  hielo, la base— es **un** interruptor con producto configurable y **doce** con variantes
  separadas. Si se acaba el insumo de una sola variante, cuesta igual en los dos esquemas [I].
- **La industria reconoce el dolor**: el «menu drift» entre plataformas es la razón de existir de
  productos enteros (Otter, Deliverect) [D].
- **La comisión no cambia**: se cobra sobre el valor del pedido, no por platillo [D, medido con
  documentos reales en [docs/plataformas-digitales.md](plataformas-digitales.md)].

---

## 4. Los estantes promocionales NO son variantes

Una plataforma permite que **el mismo platillo aparezca en varias categorías** —«Los Favoritos»,
«Por Menos de $100»— sin crear un platillo nuevo. Medido en un menú real: 16 de 65 platillos
aparecían en más de una categoría.

**Eso es lo único que aumenta la superficie de descubrimiento sin costo de mantenimiento**, y se
confunde fácil con «tener más productos». No es lo mismo: un platillo en tres estantes sigue siendo
un renglón que editar.

---

## 5. Cómo decidir cada familia

La regla que sale de todo lo anterior:

| Si las variantes se distinguen por… | Qué conviene | Por qué |
|---|---|---|
| **Sabor**, y cada una tiene nombre propio que alguien buscaría | **Separadas**, con foto propia | Es lo que el buscador semántico puede empatar |
| **Tamaño o cantidad** (170 g / 270 g / 500 g) | **Una**, con grupo de tamaño | Nadie busca «papas 270 g»; y es donde más se multiplica el mantenimiento |
| **Cuántos ingredientes lleva** | **Una**, con grupo de ingredientes | Es un tier de precio, no un platillo. Sin respaldo alguno |
| **Nada más que el precio** | **Una** | Son la misma cosa |
| Ser **productos distintos** que casualmente cuestan lo mismo | **Separadas** | Un latte y un chocolate caliente no son variantes |

El último renglón importa: agrupar «todo lo que cuesta $69» produce un catálogo absurdo. La pregunta
no es el precio, es si el cliente los considera el mismo platillo.

---

## 6. Mantenimiento

Referencia viva, indexada en [docs/README.md](README.md). Se actualiza si alguna plataforma publica
datos de descubrimiento, o cuando este negocio mida su propio antes-y-después de una consolidación
— que sería el primer dato de México sobre la pregunta.
