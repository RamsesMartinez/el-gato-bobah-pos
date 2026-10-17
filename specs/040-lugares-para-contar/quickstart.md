# Quickstart: Lugares para contar

Contra la API local del worktree, con un respaldo restaurado que tenga artículos con lugar.

1. **Paso de los datos**: antes de migrar, contar los distintos `count_place` por empresa (normalizados);
   después, `count_places` tiene ese número de filas por empresa y ningún artículo que tenía lugar
   quedó sin él.
2. **Elegir**: `PUT /stock/ingredients/{id}/place` con un `placeId` → el artículo lo muestra.
3. **Repetido**: `POST /stock/count-places` con « REFRI » cuando existe «Refri» → 409 con su id.
4. **Renombrar**: `PUT /stock/count-places/{id}` → todos sus artículos muestran el nombre nuevo.
5. **Orden**: mover un lugar al primer puesto y abrir un conteo → sus artículos salen primero.
6. **Archivar con artículos**: sin `moveTo` → 409 con cuántos; con `moveTo` → los artículos quedan
   en el destino y el lugar no se lista.
7. **Otra empresa**: asignar un lugar de otra empresa → 404.
8. **Pantalla**: a 1024×600, el lugar del artículo abre una hoja con «Sin lugar», los lugares y
   la opción de crear uno escribiendo; no hay campo de texto libre.
