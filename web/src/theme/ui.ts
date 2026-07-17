// Lenguaje visual único de la UI. Un solo radio y grosor de borde para cards,
// botones, chips, tabs, barras y nav — evita que el sistema "haga ruido" con
// estilos dispares. Los componentes Chakra por defecto (Button/Input/Menu/Card)
// ya heredan este radio vía semanticTokens en components/ui/provider.tsx; usa
// estas constantes para los elementos basados en Box, que no lo heredan.
export const RADIUS = 'lg' as const; // esquina de todo control/card
export const BORDER_W = '1px' as const; // grosor de borde estándar
export const ACCENT_W = '4px' as const; // franja de acento por color (categoría)
