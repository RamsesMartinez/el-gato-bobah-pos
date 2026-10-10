# Quickstart: validar el emparejamiento

## Automático

```bash
cd server && go build ./... && go test ./...
TEST_DATABASE_URL=… TEST_RESTORED_DATABASE_URL=… go test -tags=integration ./internal/integration/...
cd ../web && bun run lint && bun run vitest run && bun run build
```

## En el ambiente de pruebas, con un respaldo de producción

1. Prender `pos-vps-dev`, respaldar su base y restaurar el respaldo más reciente de producción.
2. Desplegar la rama; la API migra a la 0077. Antes, medir cuántas parejas con
   `local_kind = 'opcion_de_modificador'` trae producción: si hay alguna, la migración aborta.
3. Leer el menú de la tienda (con credenciales de pruebas) y, a 1024×600:
   - ver los tres grupos con su conteo y el buscador;
   - confirmar en lote y uno por uno; deshacer;
   - ligar dos platillos al mismo producto y ver que exige el precio de captura;
   - ligar una opción de Uber con una opción del POS;
   - marcar «solo existe en Uber»;
   - volver a leer y ver el aviso de precios cambiados.
4. En el POS, el precio de Uber de un producto emparejado se ve bloqueado con «Lo pone Uber».
5. Cobrar todo pedido de prueba, regresar el ambiente a como estaba y apagarlo.
