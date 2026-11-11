# Quickstart: caso de aceptación contra local

Prerrequisitos: `make start` del worktree arriba, respaldo restaurado con `make db-restaurar`
(el de producción del día), API en `http://localhost:8076`.

1. **Token**: en la base local (copia restaurada, nunca en producción) se le pone un secreto conocido
   a la fila de «Token Gestion Claude» para usar ESE token y no uno nuevo:
   `whoami` debe listar `ingredient_create`, `ingredient_update` y `presentations_list`.
2. **(a)** `ingredient_create` en vista previa con un nombre que ya existe escrito de otra forma
   (p. ej. «Jamon pierna»): lo primero de la respuesta es «quizá ya existe: Jamón de pierna (749)».
3. **(b)** `ingredient_create` de un insumo nuevo, aplicado; `presentation_create` aplicado sobre él;
   `purchase_create` en vista previa con ese insumo y ese empaque: la compra lo acepta.
4. **(c)** `ingredient_update` de 749: `name: "Jamón"`, `alt_names: ["Jamón de pierna", "Jamón de
   pavo"]`; vista previa con antes y después y parecidos; aplicar; `articles_search` lo encuentra por
   los tres nombres.
5. **(d)** `presentations_list` con `ingredient_id: 749`: solo los empaques de 749.

Los pedidos y compras de este caso son vistas previas o altas de catálogo: no queda nada abierto que
cerrar. El resultado medido se anota en `tasks.md`.
