# Quickstart: validar sucursales

## Pruebas automáticas

```bash
cd server && go build ./... && go test ./...
```

Las de integración corren contra Postgres real (las levanta el arnés). Las que importan:

- `branches_test.go`: aislamiento en los tres casos, FK compuesta contra sucursal ajena, dos cajas
  principales abiertas en dos sucursales, existencias separadas, error claro con dos sucursales y
  sin selector.
- `migracion_0076_test.go`: sobre un respaldo con dos empresas, una matriz cada una, cero filas sin
  sucursal, y ventas y corte idénticos antes y después.

## Sobre un respaldo de producción, en local

```bash
make db-restaurar            # con dueños y GRANT
make start                   # la API migra a la 0076 al arrancar
```

Como `gatobobah_app`:

```sql
set role gatobobah_app; set app.company_id = '2';
select branch_number, code, name, is_headquarters from branches;   -- una fila, número 1, matriz
select count(*) from orders where branch_id is null;               -- 0
select current_branch_id();                                        -- el id de la matriz
```

En el POS: vender, cobrar, abrir y cerrar caja. Nada pide sucursal y todo funciona igual.

## En el ambiente de pruebas, con un respaldo de producción (obligatorio antes de darlo por bueno)

1. Prender `pos-vps-dev` (skill `ambiente-dev on`).
2. Restaurar ahí el respaldo más reciente de producción, con dueños y GRANT.
3. Desplegar la rama y dejar que la API migre a la 0076.
4. Repetir las consultas de arriba y recorrer el POS a 1024×600: vender, cobrar, abrir y cerrar
   caja, ver almacén e insumos. Todo pedido de prueba se cobra antes de terminar.
5. Apagar `pos-vps-dev`.
