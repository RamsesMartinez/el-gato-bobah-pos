# Quickstart: comprobar que el mapa mide y que no estorba

Cada paso falla si la feature no cumple. Ninguno pasa por mirar la pantalla y confiar.

## Antes de empezar

- La migración aplicada y la consola de plataforma funcionando (spec 016).
- Al menos **dos empresas** en la base: con una sola, «unificado o por empresa» es el mismo número.

## Que mida

1. Entra al POS, abre tres pantallas distintas y cobra un pedido.
2. Espera diez segundos (el lote se vacía por tiempo) y abre el mapa en la consola.

**Se espera**: las tres pantallas con conteo ≥ 1 y la acción de cobro con 1.
**Falla si** el mapa está vacío: el lote no salió, o el nombre que manda el front no está en la
lista blanca (revisa el contador de descartes en el log del servidor).

## Que NO estorbe — el paso que de verdad importa

3. Con el POS abierto, **apaga la red** (modo avión en la tableta, o bloquea el dominio de la API).
4. Arma una cuenta completa y llega hasta la pantalla de cobro.

**Se espera**: todo funciona igual que hoy y **no aparece ni un aviso** sobre el registro de uso.
**Falla si** algo se traba, parpadea o muestra un error: significa que la medición está en el camino
del operador, y eso es un no-go, no un detalle.

5. Vuelve a encender la red.

**Se espera**: lo que se perdió, se perdió. No hay que ver un pico de eventos atrasados — no se
reintenta a propósito.

## Que no se pueda reconstruir a la persona

6. Con dos usuarios de roles distintos usando el POS, corre contra la base:

```sql
select * from usage_events limit 5;   -- no hay ninguna columna de usuario
select * from usage_daily  limit 5;
```

**Falla si** aparece cualquier columna que apunte a una persona.

7. Deja **un solo** usuario activo de un rol y genera uso con él. Mira las filas del agregado.

**Se espera**: ese uso quedó con `role` **nulo** (sin corte).
**Falla si** quedó con el rol escrito: FR-009 no se está aplicando al escribir, y entonces la
promesa de FR-002 se rompe en la pantalla.

## Que la consola no alcance el grano fino

8. Conectado **como `gatobobah_platform`** (no como owner, que salta todo):

```sql
select count(*) from usage_daily;   -- se espera: un número
select count(*) from usage_events;  -- se espera: 42501, permiso denegado
```

**Falla si** la segunda devuelve un número: la consola está leyendo hechos y no conteos, y el día
que el evento lleve coordenadas estaría leyendo dónde puso el dedo la gente de un cliente.

## Que quepa

9. Siembra un año de uso simulado y mide:

```sql
select pg_size_pretty(pg_total_relation_size('usage_events')),
       pg_size_pretty(pg_total_relation_size('usage_daily'));
```

**Se espera**: por debajo del techo declarado (15 MB por empresa).
**Falla si** lo pasa: el agregado no está agregando —revisa el `nulls not distinct` de la llave— o
el recorte no corrió.

## Que el POS no engordó

10. `cd web && bun run build`

**Se espera**: el chunk principal no crece más de unos pocos KB sobre los **1,133.50 kB** medidos el
2026-09-11. El registrador son unas decenas de líneas; si el paquete salta, algo del mapa se coló al
POS.

## Lo que este quickstart NO cubre

- Que el mapa se lea bien con datos de verdad: eso se ve mirándolo, con un mes de uso encima.
- Que el recorte diario corra en la VM: se comprueba al día siguiente del despliegue, contando filas
  viejas.
