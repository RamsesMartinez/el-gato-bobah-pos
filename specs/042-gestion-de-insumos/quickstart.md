# Quickstart: validar la 042

Prerrequisitos: `make start` en el worktree, `make db-restaurar` con un respaldo de producción y
sesión de administrador.

1. **Crear**: en Almacén › Artículos, toca «Nuevo artículo», escribe «Leche prueba» en líquido con
   2 L de existencia inicial y guarda. Debe aparecer en la lista, con un evento
   `ingredient.created` en la bitácora.
2. **Sin artículos huérfanos**: en Gastos, toca Nuevo gasto, crea «Prueba huérfana» en «Buscar o
   crear» y cancela. La búsqueda en Artículos no debe encontrarlo.
3. **Empaques**: en la ficha de «Leche prueba», toca Agregar empaque. Las sugerencias deben ser solo
   en L, sin ninguna en kg.
4. **Costo**: corrige el costo a 24 por L. El historial debe mostrar la corrección con tu nombre, y
   la gráfica un punto.
5. **Unidad**: cambia «Leche prueba» a piezas con la equivalencia 1 pieza = 1,000 ml. Debe ser
   directo (solo tiene ajustes), la existencia debe quedar en 2 piezas y la bitácora debe guardar
   los ml originales.
6. **Conteo**: abre un conteo y escribe 740 kg en un artículo en gramos sin existencia. Debe pedir
   confirmar mostrando «740 kg» en grande. Sin nada capturado, «Guardar» debe estar apagado.
   Tocar «No hay» debe preguntar «¿No hay nada de …?» y, al confirmar, guardar 0 sin el aviso
   de cifra.
7. **Archivar**: archiva «Leche prueba». Debe dejar de ofrecerse en el gasto, y el filtro
   «Archivados» debe mostrarlo.

Pruebas automáticas: `make api-test`, `make test-integration` y `cd web && bun run vitest run`.
