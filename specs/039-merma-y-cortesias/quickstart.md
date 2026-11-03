# Quickstart: Merma y faltante sin explicar

Contra la API local del worktree (o el ambiente de pruebas tras el merge), con un usuario cajero y
uno gerente.

1. **Registrar**: insumo con 10 → registrar 2 con «Caducó» → quedan 8; `GET /waste/today` lo lista.
2. **En empaques**: insumo con empaque «Caja · 12 L» → registrar 1 caja → resta 12 L.
3. **Producto**: un producto con receta → registrar 1 → restan sus insumos; no aparece en Ventas.
4. **Motivo nuevo**: crear «Se derritió» con descripción → aparece la próxima vez; repetirlo con
   otra mayúscula → ofrece el existente.
5. **Deshacer**: deshacer la del paso 1 → vuelve a 10; deshacerla otra vez → 409.
6. **Tope**: poner tope de $100 → registrar una de $150 → queda por aprobar y no resta; el gerente
   la aprueba → resta; otra se rechaza → no resta.
7. **Cubierta**: dejar una pendiente, contar su insumo y cerrar el conteo → queda «la cubrió el
   conteo»; aprobarla → 409.
8. **Faltante**: dos conteos cerrados con 3 de merma registrada entre ellos y el segundo encontró
   2 de menos → el reporte dice perdido 5, registrado 3, sin explicar 2.
9. **Por motivo**: la semana con «Caducó» y «Consumo del personal» → cada uno con su total, el del
   personal aparte.
10. **Aislamiento**: con la sesión de otra empresa, ninguna de estas rutas ve ni toca lo anterior.
