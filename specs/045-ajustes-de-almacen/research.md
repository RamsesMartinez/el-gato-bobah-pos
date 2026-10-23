# Research: 045

- **Unidades**. Decisión: `formatBaseQty` (`shared/units.ts`) es la única regla. Por qué: ya decide
  kg/L/m y el plural de pieza; Existencias es la única que pinta `on_hand` crudo (`StockPage.tsx:92`).
  Alternativa descartada: formatear en la API (otra regla que mantener en paralelo). Al implementar se
  barre `web/src` buscando `on_hand}` / `unit_code}` pintados a mano.
- **Buscador**. Decisión: coincidencia por inicio de palabra, todas las palabras buscadas. Por qué:
  quita «Tapioca» y conserva «almen». Alternativa descartada: coincidencia solo al inicio del nombre
  (pierde «leche de almendra» al buscar «almendra»).
- **Ir al conteo**. Decisión: navegar por URL (`/almacen?tab=conteo&q=<nombre>`). Por qué: `StockPage` ya resuelve la
  pestaña por `tab` y sobrevive a recargar. Se verifica en implementación el id exacto de la pestaña.
- **Tema**. Causa confirmada: `defaultTheme="light"` en `main.tsx`. next-themes solo guarda al elegir,
  así que quien ya eligió conserva su tema.
- **Barra lateral**. Causa sin confirmar; se reproduce antes de tocar código (B10).
