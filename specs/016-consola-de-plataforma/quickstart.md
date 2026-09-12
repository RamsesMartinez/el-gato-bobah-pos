# Quickstart: comprobar que la consola está de verdad separada

Cómo verificar que las tres barreras están puestas. Cada paso **falla si la separación no existe**;
ninguno pasa por mirar la pantalla y confiar.

## Antes de empezar

- La API con las dos variables nuevas (`PLATFORM_JWT_SECRET`, `PLATFORM_DATABASE_URL`).
- Un operador de plataforma creado con la bandera del binario.
- Al menos **dos empresas** en la base, y un usuario de negocio en una de ellas.

## Barrera 1 — las firmas no coinciden

1. Entra en la consola y guarda su token.
2. Úsalo contra una ruta del negocio (`GET /api/v1/pos/menu`).

**Se espera**: 401. **Falla si**: contesta cualquier cosa distinta de rechazo — significa que los
dos lados comparten secreto y la separación es un `if`.

3. Entra en el POS con un usuario de negocio y usa su token contra `GET /api/v1/platform/companies`.

**Se espera**: 401.

4. **El que más importa**: comprueba que `PLATFORM_JWT_SECRET` y `JWT_SECRET` son distintos, y que
   la API **no arranca** si los igualas.

**Falla si** arranca: dos secretos iguales son un secreto, y a partir de ahí los pasos 2 y 3 pasan
por casualidad.

## Barrera 2 y 3 — la base dice que no

5. Conectado **como `gatobobah_platform`** (no como owner, que salta todo):

```sql
select count(*) from orders;              -- se espera: 42501, permiso denegado
select count(*) from register_sessions;   -- se espera: 42501
select count(*) from users;               -- se espera: 42501
select count(*) from companies;           -- se espera: el número de empresas
```

**Falla si** alguna de las tres primeras devuelve un número. En local **esto no prueba nada** si la
API corre como owner: hay que hacerlo con el rol de verdad, que es por qué estos casos viven en la
suite de integración y no en un unitario.

6. Al revés, conectado como `gatobobah_app`:

```sql
select count(*) from platform_operators;  -- se espera: 42501
```

**Falla si** devuelve un número: la separación tiene que correr en las dos direcciones.

## Que el operador desactivado pierda el acceso

7. Entra a la consola, desactiva al operador en la base y vuelve a pedir datos con el token que ya
   tenías.

**Se espera**: deja de servir **sin esperar** a que caduque (FR-013). **Falla si** sigue
funcionando hasta que expire: el acceso se retira cuando se decide, no cuando vence.

## Que la tableta no cargue la consola

La barrera de verdad es **automática y corre en cada cambio**: la regla de `eslint.config.js` que
prohíbe importar `consola/` desde fuera de su árbol, en el mismo bloque `FRONTERAS` que ya separa
`features/` de `shared/`. Si alguien cruza los árboles, el commit no pasa.

8. Comprueba que la regla existe y que muerde:

```bash
cd web && bun run lint        # se espera: limpio
```

Y que **muerde de verdad**: agrega temporalmente un `import` de `src/consola/` en un archivo del
POS y vuelve a correrlo. **Falla si** pasa en verde — entonces la regla está mal escrita y la única
barrera contra contaminar la tableta no existe.

9. Además, mide el paquete y compáralo con el «antes»:

```bash
cd web && bun run build && du -sh dist && ls -l dist/assets/*.js
```

**Se espera**: el chunk principal **no crece** respecto de los 1,133.50 kB medidos el 2026-09-11.
Un crecimiento es la señal de contaminación más clara que hay — más que buscar texto, porque el
paquete está minificado y los nombres no sobreviven.

## Que no se filtre dinero por descuido

10. Pide `GET /api/v1/platform/companies` y busca en la respuesta cualquier importe.

**Se espera**: ninguno (FR-008). La consola dice **quiénes** son los clientes, nunca cuánto venden.

## Lo que este quickstart NO cubre

- Que el subdominio correcto apunte al ambiente correcto. Se verifica al desplegar.
- El primer arranque sin ningún operador creado: hay que comprobar a mano que la bandera del binario
  lo crea y que no queda ninguna credencial en el repositorio.
