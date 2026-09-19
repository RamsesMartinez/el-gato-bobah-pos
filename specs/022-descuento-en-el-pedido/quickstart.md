# Quickstart: verificar el descuento de punta a punta

## Prerrequisitos

```bash
make start              # Postgres, Redis, API y web
# o, en Windows, la API en contenedor (ver AGENTS.md §7)
```

## 1. Los gates que tienen que estar verdes

```bash
cd server && go build ./... && go test ./...      # = make api-build && make api-test
cd ../web  && bun run lint && bun run vitest run && bun run build
```

## 2. La aritmética, sin base de datos

```bash
cd server && go test ./internal/domain -run Descuento -v
```

Prueba lo que se puede probar puro: el porcentaje resuelto a pesos, el descuento mayor que la venta,
el monto y el porcentaje juntos, el envío que no se descuenta.

## 3. La migración y el recálculo, contra Postgres real

```bash
cd server && go test ./internal/integration -run Descuento -v
```

Cubre lo que un unitario no puede ver: que la migración corra sobre datos de **dos empresas**, que el
rol `gatobobah_app` (no el owner) pueda escribir las columnas nuevas, y que cancelar o agregar
renglones deje el total donde debe.

## 4. A mano, en la pantalla

1. Abrir una cuenta y meter productos hasta ~$385.
2. Tocar `+ Descuento`, elegir `%`, escribir `20`. El total baja a $308 **en los tres lugares** que
   lo pintan: el panel del ticket, la píldora y la barra angosta.
3. Borrar el campo. El total vuelve a $385 — el campo vacío es ausencia, no cero.
4. Escribir `$400`. La pantalla no deja mandar y dice cuál es el máximo.
5. Escribir `$50`, mandar el pedido y **cobrarlo** (el ambiente de pruebas se comparte: un pedido
   abierto suma a "por cobrar" y bloquea el cierre de caja de quien opera).
6. Imprimir el ticket: subtotal, descuento y total, y las tres cifras cierran.
7. Abrir Ventas → el detalle del pedido muestra el descuento y quién lo aplicó.

## 5. Lo que hay que ver en rojo antes de darlo por bueno

Cada test de la tabla *Bordes → dónde queda su test* del plan se escribe **antes** del código y se ve
fallar por la razón correcta. Un test que nunca estuvo en rojo no prueba nada.
