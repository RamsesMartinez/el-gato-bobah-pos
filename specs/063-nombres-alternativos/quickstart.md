# Quickstart: Nombres alternativos (063)

Contra el `make start` del worktree (API :8063, web :3063), base restaurada, 1024×600 con toque.

1. Productos › «Ice Coffee» › Editar › Otros nombres: agregar «Café frío» y «Café helado». Guardar.
2. Vender: buscar «cafe fri» → «Ice Coffee» con «café frío» debajo. Venderlo; ticket y comanda
   dicen «Ice Coffee».
3. Productos: buscar «café helado» → «Ice Coffee» con el alternativo señalado.
4. Otro producto › Otros nombres: «cafe frio» → aviso «ya es otro nombre de «Ice Coffee»».
5. Almacén › Artículos › «Leche entera» › Otros nombres: «Leche Lala». Buscar «lala» en Artículos,
   en Conteo (lista), en un renglón de un gasto nuevo y en «Agregar insumo» de una receta.
6. En un gasto, crear artículo «leche lala» → se usa «Leche entera».
7. Intentar un undécimo alternativo → la pantalla no lo ofrece; el servidor responde 400 a 11.
8. Cobrar y cerrar los pedidos de prueba.
