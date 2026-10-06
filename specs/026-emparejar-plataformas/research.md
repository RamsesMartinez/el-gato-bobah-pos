# Research: emparejar (026)

## Medición previa (T001, 2026-10-03)

Sobre el respaldo anonimizado del 2026-10-02 (dos empresas):

- Parejas en `platform_item_links`: 0, así que ninguna de tipo opción guardada como producto. La
  guarda de la 0077 se queda igual: protege a cualquier otro ambiente.
- Precios por plataforma capturados a mano: 14 de producto y 15 de opción. Todos quedan con
  `source = manual`.
- Ningún script de `docs/reorg/` escribe en las tablas de precios por plataforma.
- 0077 está libre en todas las ramas vivas (la más alta fuera de esta línea es 0072).

Repetir el conteo de parejas sobre el respaldo de producción antes del ensayo (T034).

## Ensayo en el ambiente de pruebas (T034, 2026-10-03)

Sobre el respaldo de producción de esa noche, con la API de la rama:

- Migró de la 72 a la 77 sin errores. Producción no tenía parejas ni tiendas conectadas, así que la
  guarda de parejas de opción no se activó.
- Tienda simulada con nombres reales del catálogo: 6 propuestas confirmadas en un lote; la segunda
  pareja al mismo producto pidió el precio de captura; «solo en Uber» se guardó; el aviso antes de
  borrar la tienda contó parejas y decisiones; el genérico no salió ni en el catálogo ni al vender.
- A 1024×600 y en vertical: sin scroll horizontal, la página cabe en el alto.

Defectos que encontró y quedaron corregidos con su test:

1. Los conteos de los grupos sumaban platillos y opciones mientras la lista mostraba un solo nivel:
   «Sin pareja 1» sobre una lista vacía.
2. Opciones repetidas del catálogo («Ranch Cremoso» en dos grupos) salían como candidatos idénticos;
   ahora cada candidato dice su grupo o su categoría.

No ensayado: la copia de precios desde una lectura real y aceptar un pedido real, porque el
ambiente de pruebas no tiene credenciales de Uber (las del respaldo son de producción y no se
descifran en otro ambiente). Los dos caminos tienen sus pruebas de integración.
