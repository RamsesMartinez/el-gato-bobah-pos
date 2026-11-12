# Quickstart: caso de aceptación contra local

Prerrequisitos: `make start` del worktree arriba, respaldo de producción restaurado con
`make db-restaurar`, API en `http://localhost:8077`.

1. **Token**: en la base local (copia restaurada, nunca en producción) se le pone un secreto conocido
   a la fila del token de la empresa con `write:inventory` para usar ESE token: `whoami` lista
   `ingredient_update` (con `active` en su esquema) sin regenerarlo.
2. **(a)** Un insumo con existencia distinta de 0: `ingredient_update` `active: false` en vista
   previa → lo primero es la existencia por sucursal y la indicación de merma o ajuste.
3. **(b)** Un insumo que usa una receta activa: la vista previa nombra la receta; aplicar se rechaza.
4. **(c)** Un insumo sin existencia ni recetas: aplicar `active: false`; comprobar que no sale en
   `stock_count_current`, en `articles_search` ni en la vista previa de `purchase_create` por su
   nombre, ni en `stock_levels`; y que sus movimientos, historial de costo y bitácora siguen.
5. **(d)** `articles_search` con `include_inactive: true` lo encuentra con `active: false`;
   `ingredient_update` `active: true` lo reactiva y vuelve a salir.
6. **(e)** Un token sin `write:inventory` y un usuario con rol sin `inventory.configure`: la acción
   se niega y nada cambia.

Nada de esto abre pedidos ni cajas. El resultado medido se anota en `tasks.md`.
