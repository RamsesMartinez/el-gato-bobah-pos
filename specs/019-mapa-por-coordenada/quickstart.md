# Quickstart: comprobar que mide el dedo y no a la persona

## Que mida

1. En el POS, toca cinco veces la misma zona (por ejemplo el botón de una categoría) y una vez una
   esquina vacía. Espera diez segundos.
2. Abre la rejilla de esa pantalla en la consola.

**Se espera**: la celda de la categoría con 5 y la de la esquina con 1; el resto en cero.
**Falla si** está todo en cero: el lote no salió, o la pantalla no está en la lista instrumentada
(revisa el contador de descartes en el log del servidor).

> **Cierra lo que se abra entre toque y toque.** En este catálogo casi todo producto abre su hoja de
> modificadores, y un toque dentro de una hoja **no se cuenta** — es una capa encima—. Sin cerrarla,
> de cinco toques cuenta uno y parece un defecto. Corriendo esto contra el ambiente de pruebas el
> 2026-09-12 pasó exactamente eso.

**Medido el 2026-09-12 contra el ambiente de pruebas**: cinco toques en la celda 39 y uno en la 11
subieron `{39: +6, 11: +1}` (el sexto de la 39 es el toque del paso 3-bis, que abre la hoja y sí
cuenta); diez arrastres no subieron ninguna; seis toques dentro de la hoja, en la celda 42, no
aparecen; y el único cuerpo enviado llevó las llaves `pantalla`, `celda` y `orientacion` y nada más.

## Que NO cuente los arrastres

3. Desplaza la lista de productos arriba y abajo diez veces, sin tocar ningún botón.

**Se espera**: ninguna celda sube.
**Falla si** suben: el mapa está midiendo el rastro del scroll en vez de las intenciones, y las
zonas «calientes» van a ser por donde la gente arrastra.

## Que no salga nada de la pantalla

4. Con las herramientas del navegador abiertas en la pestaña de red, usa el POS un minuto y revisa
   **todas** las peticiones a `/usage`.

**Se espera**: cuerpos con `pantalla`, `celda` y `orientacion`, nada más. Ni una imagen, ni `x`, ni
`y`, ni texto de la pantalla.
**Falla si** aparece cualquier otra cosa: es la promesa central de US2 y no admite matices.

## Que no estorbe

5. Bloquea el endpoint de medición (o apaga la red) y captura un pedido completo tocando rápido.

**Se espera**: el POS responde igual y no aparece ningún aviso.
**Falla si** algo se traba: la medición está en el camino del dedo, que es lo único que esta feature
no puede hacer.

## Que las capas de encima no se cuenten

3-bis. Abre el Picker de una lista, toca tres opciones, ciérralo. Bloquea la pantalla y teclea el PIN
para volver.

**Se espera**: ninguna celda sube por esos toques.
**Falla si** suben: esos toques se están atribuyendo a la pantalla de abajo, y el teclado del PIN
—que siempre cae en el mismo sitio— va a dejar una zona caliente que alguien va a leer como un
control muy usado.

## Que no se pueda reconstruir a la persona ni el momento

6. Contra la base:

```sql
select * from usage_touches_daily limit 5;
```

**Se espera**: día, pantalla, orientación, celda, rol y conteo. **Ninguna columna de tiempo más fina
que el día**, ninguna de usuario.

Medido el 2026-09-12 sobre el ambiente de pruebas ya desplegado:
`day:date | screen:text | orientation:text | cell:smallint | role:USER-DEFINED | hits:bigint | company_id:bigint`.
**Falla si** hay un `created_at` o un `updated_at`: diría a qué hora estuvo activa esa zona, que es
medio camino de vuelta.

7. Con **un solo** empleado activo de un rol, genera toques con él y mira las filas.

**Se espera**: depende de la plantilla, y esto lo cambió la auditoría —léelo antes de llamarlo
defecto—:
- Si hay **otro** rol también con una sola persona, la fila entra con `role` **nulo**: el balde de
  lo suprimido tapa a dos.
- Si ese es el **único** rol por debajo de dos personas, **no hay fila**. Con 1 admin, 2 gerentes,
  3 cajeros y 2 meseros, `null` sería el dueño, y «sin corte» estaría mintiendo.

**Falla si** aparece el nombre del rol en el primer caso, o si aparece una fila en el segundo.

## Que quepa

8. Siembra un trimestre al tope **con el patrón real de escritura** —muchos `update` sobre las
   mismas filas del día, no un `insert` masivo con el total ya sumado— y mide:

```sql
select pg_size_pretty(pg_total_relation_size('usage_touches_daily'));
```

**Se espera**: por debajo de **20 MB** por empresa. Es un tope estructural —84 celdas × 2
orientaciones × 5 cortes de rol × las pantallas instrumentadas— así que si lo pasa, algo está
creando filas que no debería.

**Un `insert` masivo no sirve como prueba**: cada fila real recibe cientos de `update` a lo largo del
turno y cada uno deja muerta la versión vieja. Eso es lo que hay que medir, y es la razón del
`fillfactor` de la tabla.

## Que el POS no engordó

9. `cd web && bun run build` — el chunk principal no crece más de **5 kB** sobre 1,134.94 kB.

Medido el 2026-09-12: **1,136.43 kB**, o sea **+1.49 kB**, y `web/package.json` y `web/bun.lock` sin
un solo cambio (cero dependencias nuevas).

## Lo que este quickstart NO cubre

- Que la rejilla se entienda mirándola: eso se ve con un mes de toques encima.
- Que la celda corresponda al control que uno cree: la pantalla se desplaza, y el mapa mide el
  vidrio, no el contenido (FR-004).
