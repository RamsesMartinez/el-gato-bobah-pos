# Quickstart: validar la 092 en local

Prerrequisitos: worktree `egb-092-…` con `make start` arriba (API `:8092`, web `:3092`), base
restaurada con `make db-restaurar` (respaldo de producción con dos empresas), caja principal abierta.
Pantalla a 1024×600 y a 1280×854, con `hasTouch`.

## 1. Nada cambió para lo existente (SC-001)

- Antes de arrancar la API nueva: contar productos de El Gato Bobah que salen en Vender.
- Arrancar la API (aplica 0118). Vender muestra los mismos productos y la lista de agotados está
  vacía: `GET /api/v1/pos/menu/sold-out` → `{"productIds":[]}`.
- `select count(*) from products where not allow_oversell` → 0.

## 2. Mostrador y Web (US1)

- Menú → crear producto sin abrir «Opciones avanzadas» → sale en Vender; su ficha tiene Mostrador y
  Web encendidas.
- Apagar Mostrador → desaparece de Vender (categoría y búsqueda) y la lista de productos lo sigue
  mostrando. Apagar también Web → la lista lo marca «No se vende en ningún lado».
- `POST /orders` con ese producto y sin plataforma → 422 `MENU_CHANGED`.

## 3. Agotado (US2)

- Un refresco (receta de un insumo por pieza): apagar «Vender sin existencia» y dejar su insumo en 0
  con un conteo o merma.
- En a lo más 60 s, la tarjeta en Vender sale «Agotado» y tocarla no agrega nada.
- Con la pantalla abierta desde antes del agotado, cobrar una cuenta con él → 409 `OUT_OF_STOCK`
  con su nombre; la tarjeta se marca al instante.
- Registrar una compra del insumo → deja de salir «Agotado» en a lo más 60 s.
- Un pedido con plataforma con ese producto entra.
- Dos ventas simultáneas de la última pieza: cubierto por la prueba de integración
  `TestTwoCountersCannotSellTheLastPiece`.

## 4. Venta en web (US3)

- Ficha → «Venta en web»: precio vacío, dice el de mostrador. Capturar 79 → guardar → Vender sigue
  cobrando el de mostrador; `GET /admin/products` trae `webPrice: "79.00"`.
- Borrar el precio → `webPrice: null`. Capturar 0 → mensaje claro, no guarda.

## 5. Configuración (US4)

- Ajustes → Negocio: menú de secciones a la izquierda; cada ajuste de antes aparece en una sola
  sección y funciona igual. Recargar con `?seccion=caja` abre Caja.
- Con un rol sin `attendance.configure`, Personal no aparece.
- «Venta en web» muestra cuántos productos se venden en la web y cuántos con precio propio.

## 6. Pruebas automáticas

```bash
cd server && go build ./... && go test ./... && cd .. && make test-integration
cd web && bun run lint && bun run vitest run && bun run build
make ci-local
```
