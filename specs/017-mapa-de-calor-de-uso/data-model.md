# Data model: el uso del sistema

Dos tablas. Una guarda lo que pasó, la otra lo que la pantalla lee.

## Una sola tabla, y por qué se quitó la otra

El plan tenía dos: el conteo y una de grano fino —un renglón por toque, con `detail jsonb` vacío
para las coordenadas del futuro—. **La auditoría adversarial la tumbó por dos caminos
independientes**, los dos del tipo que no se ve mirando el código:

1. **El reloj deshacía el anonimato.** Un renglón por toque con marca de microsegundos se cruza con
   `register_sessions.closed_by` o con `orders.opened_by`: el evento «sin corte» de las
   22:03:11.412 es de quien cerró el turno a las 22:03:11.6. Y como los eventos de un turno son un
   flujo contiguo de la misma tableta, esa sesión etiqueta **todo** lo que cayó en medio. El
   identificador no era el rol: era el instante. Suprimir el rol no protegía de nada.
2. **El volumen no tenía techo real.** Con el limitador intacto, una cuenta puede mandar 2.16
   millones de eventos al día: medido, **298 MB diarios y 4 GB en los 14 días de retención**, 167×
   el techo declarado. Un bucle en una tableta llenaba el disco del VPS y Postgres se detenía — o
   sea, el mostrador dejaba de cobrar por una feature de analítica.

Con solo el conteo las dos desaparecen **por construcción**: no hay instante que cruzar, y el número
de FILAS lo acota la lista blanca (pantallas × acciones × roles) sin importar cuántos eventos
lleguen. Lo que crece es un contador, no la tabla.

**Y la puerta de FR-013 sigue abierta al mismo costo.** El día que se midan coordenadas nace una
tabla para ellas; crear una tabla no migra nada, que es literalmente lo que el requisito pide. Lo
que se pierde es poder recontar distinto los últimos catorce días — y eso no vale una tabla que hoy
nadie lee y que solo el recorte toca.

## `usage_daily` — lo único que lee la consola

| Columna | Tipo | Por qué |
|---|---|---|
| `day` | `date not null` | **El día del negocio**, calculado en Go con la zona de la empresa. Con el servidor en UTC la medianoche cae a las 18:00 en México y la tarde-noche —la franja de más movimiento— se contaría mañana: el mismo defecto que 0038 ya corrigió para la venta |
| `screen` | `text not null` | |
| `action` | `text` | Nulo = fue una apertura de pantalla |
| `role` | `user_role` | Nulo = sin corte |
| `hits` | `bigint not null default 0` | El conteo |
| `company_id` | `bigint not null default …` **`references companies(id) on delete cascade`** | Igual que arriba, escrita literal |

```sql
constraint usage_daily_llave
  unique nulls not distinct (company_id, day, screen, action, role)
```

**`nulls not distinct` no es un detalle**: sin eso, cada evento con `action` nula (toda apertura de
pantalla) o con `role` nulo (todo evento suprimido por k-anonimato) crearía una fila nueva en vez de
sumar, porque en SQL `null <> null`. El agregado dejaría de agregar en silencio y la tabla crecería
como la de eventos. Es de Postgres 15+ y la base corre 16.

**Retención: 13 meses.** Trece y no doce para poder comparar un mes contra el mismo mes del año
pasado, que es la primera comparación que pide un negocio de comida.

## El techo, medido contra el borde y no contra el caso feliz (FR-011)

**Techo declarado: menos de 25 MB por empresa.** Medido: **10.3 MB** con trece meses de agregado y
el churn de un día *al tope de lo que el limitador permite* —no al del uso honesto—, que es la vara
que la constitución pide: el test se escribe contra el borde.

Lo que hace que el número se sostenga es que **las filas están acotadas**: la lista blanca fija
cuántas combinaciones de (pantalla, acción, rol) pueden existir, así que dos millones de eventos en
un día solo suben contadores. Es la diferencia entre un techo que depende de que el limitador
aguante y uno que no depende de nada.

Dos cosas que el cálculo a ojo no vio y la medición sí:

- El índice único de cinco columnas pesa **103 bytes por fila**, no los ~40 que se habían supuesto.
- Cada `update` deja muerta la versión vieja: 135 filas con 2,000 incrementos de a uno pasan de
  64 kB a **232 kB** antes de que autovacuum llegue, y con decenas de miles de filas vivas
  autovacuum tarda días en disparar. Por eso la ingesta **pre-agrega dentro del lote**: un solo
  `update … set hits = hits + n` por combinación, hasta 50× menos escrituras físicas.

Para comparar: la base **completa** de producción —meses de operación, pedidos, productos,
usuarios— pesa hoy **18 MB**, y la VM tiene 18 GB libres.

## Permisos: quién escribe, quién lee, quién no alcanza

| Rol | `usage_daily` |
|---|---|
| `gatobobah_app` (el POS) | `select`, `insert`, `update` |
| `gatobobah_platform` (la consola) | `select` |
| `gatobobah` (dueño) | todo — es quien recorta |

Tres cosas que hay que decir en voz alta:

1. **El `update` del rol de la app es para el `upsert`**, no para editar historia: `insert … on
   conflict do update set hits = usage_daily.hits + n`, con la `n` que trae pre-agregado el lote.
   Postgres exige `select` sobre la columna que se lee en el `set`, y por eso el `select` también
   está.
2. **La consola lee conteos, que es todo lo que hay.** El día que nazca una tabla de coordenadas,
   alcanzarla será una decisión que alguien escriba en una migración, no algo que herede.
3. **El grant no es opcional**: el de [0024](../../server/migrations/0024_tenant_rls.sql) fue
   puntual (`on all tables`, sin default privileges), así que una tabla nueva no hereda nada. Sin
   estas líneas la migración pasa, los tests pasan, `make start` pasa —dev sirve como owner— y en
   producción el primer evento devuelve 42501.

## RLS: las dos políticas, y por qué son dos

```sql
-- Cada empresa ve lo suyo. Igual que las otras ~30 tablas.
create policy tenant_isolation on usage_daily using (…) with check (…);

-- Y la consola ve TODO el agregado, acotada a select y a su rol.
create policy plataforma_lee_todo_el_uso on usage_daily
  for select to gatobobah_platform using (true);
```

La segunda es la gemela de `plataforma_lee_todas_las_empresas` de la 0068, y por la misma razón
medida: **`select` a secas no alcanza**, porque `tenant_isolation` también le aplica al rol de
plataforma y la consola vería el uso de **una empresa de dos** —o de ninguna, porque su conexión no
fija `app.company_id`—. Sin esta política, FR-008 («unificado o por empresa») es imposible.

**Nunca `BYPASSRLS`**, y el arranque del binario ya lo rechaza (`store.AssertPlatformGrants`).

## La lista blanca NO es una tabla

Las pantallas y las acciones que vale la pena contar viven en `domain`, en Go, no en un catálogo en
la base.

**Por qué**: una tabla de catálogo tendría que sembrarse por migración, mantenerse sincronizada con
las rutas del front y, el día que alguien agregue una pantalla, fallaría **en producción** con una
FK en vez de fallar al compilar. La lista en el código se revisa en el diff, viaja con el binario
que la valida y no tiene estado que se desincronice.

Lo que sí queda en la base es la consecuencia: solo entran valores de esa lista, así que el número
de combinaciones distintas está acotado por construcción y el agregado no se puede inflar.

## El recorte, y con qué conexión corre

Dentro del binario que ya está corriendo —la VM tiene 300 MB de RAM libres y agregar un cron es otra
pieza que se puede olvidar.

**Corre al ARRANCAR y luego cada 24 horas**, y lo primero no es un detalle: un `time.Ticker` de 24 h
se reinicia con el proceso, y este repo redespliega en **cada merge**. Si el binario no vive un día
entero seguido —el modo normal mientras hay desarrollo activo— el recorte **no dispara nunca**, en
silencio, y FR-010 queda siendo una promesa que nadie cumple. Lo encontró la revisión de
arquitectura; el ticker solo, que era lo obvio, era justo lo que no funcionaba aquí.

La goroutine cuelga del `context` que ya se cancela en el apagado, para que tenga condición de
término (principio II).

Corre con una conexión **de dueño abierta para eso y cerrada al terminar**, no con la de servicio:
el rol de la app está bajo RLS y solo borraría las filas de *su* empresa, así que el recorte se
quedaría a medias en una instalación multi-empresa — y sin que nada fallara. Es el mismo patrón que
`main.go` ya usa con el pool de bootstrap: se abre, se usa, se cierra.

## Lo que este modelo no resuelve, y hay que saberlo

- **El día es el del negocio**, con su zona, igual que la venta. Lo que NO se resuelve es una
  empresa que cambia de zona horaria: lo escrito conserva el día que se calculó entonces.
- **Una empresa que cambia de plantilla** cambia qué cortes se suprimen, y las filas viejas
  conservan la decisión que se tomó al escribirlas. Es a propósito: la alternativa es reescribir el
  pasado.
