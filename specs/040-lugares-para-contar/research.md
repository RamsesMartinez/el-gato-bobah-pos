# Research: Lugares para contar

## R1. Tabla propia y no un enum ni un JSON en la empresa

- **Decision**: tabla `count_places` por empresa, con liga compuesta desde `ingredients`.
- **Rationale**: renombrar un lugar tiene que cambiar todos sus artículos de un golpe, y el nombre
  tiene que ser único sin distinguir mayúsculas. Las dos cosas las da una fila con id.
- **Alternatives**: seguir con el texto y ofrecer los distintos en un Picker. Lo descartó el dueño:
  no se puede renombrar ni ordenar.

## R2. Normalización del nombre

- **Decision**: la misma que motivos de merma e insumos: `lower`, `btrim`, quitar acentos
  (`translate`) y juntar espacios, en un índice único parcial (solo los no archivados).
- **Rationale**: una sola regla de nombres en todo el almacén.

## R3. Orden

- **Decision**: columna `position` entera. En pantalla se reordena manteniendo presionada una
  agarradera de 52 px (500 ms) y arrastrando (elegido por el dueño el 2026-10-16, opción C de la
  maqueta). El servidor recibe la posición destino y renumera los lugares activos de la empresa en
  una transacción que los bloquea todos. Un lugar nuevo va al final.
- **Alternatives**: subir y bajar con botones siempre a la vista (A) o en un modo «Ordenar» (B).
  Descartadas por el dueño: arrastrar se entiende mejor. La espera de 500 ms y el arrastre solo
  desde la agarradera evitan mover un lugar al desplazar la lista.

## R4. Paso de los datos viejos

- **Decision**: por empresa, agrupar `count_place` por nombre normalizado; el nombre que queda es el
  que más artículos usan (empate: el primero alfabético); `position` alfabética. Luego se llena
  `count_place_id` y se quita la columna de texto.
- **Rationale**: ningún artículo pierde su lugar y desaparecen los duplicados de captura.

## R5. Archivar con artículos

- **Decision**: la petición de archivar lleva el destino (`moveTo`: id o nulo). Si el lugar tiene
  artículos y no viene destino, 409 con cuántos tiene, y la pantalla pregunta.
- **Rationale**: ningún artículo queda ligado a un lugar que ya no se ofrece.
