// Lenguaje visual único de la UI. Un solo radio y grosor de borde para cards,
// botones, chips, tabs, barras y nav — evita que el sistema "haga ruido" con
// estilos dispares. Los componentes Chakra por defecto (Button/Input/Menu/Card)
// ya heredan este radio vía semanticTokens en components/ui/provider.tsx; usa
// estas constantes para los elementos basados en Box, que no lo heredan.
export const RADIUS = 'lg' as const; // esquina de todo control/card
export const BORDER_W = '1px' as const; // grosor de borde estándar
export const ACCENT_W = '4px' as const; // franja de acento por color (categoría)

// Los tamaños táctiles POR ENCIMA del piso. El piso de 44 px vive en la receta del botón
// (components/ui/provider.tsx) para que se cumpla sin que nadie se acuerde; estos son para los
// controles que merecen más blanco por lo que cuesta equivocarse en ellos.
//
// Escritos a mano eran once literales entre dos pantallas y tres constantes `TAP` locales que ni
// entre ellas coincidían.
export const TAP_LG = '52px' as const; // controles de dinero: método, billetes, propina
export const TAP_XL = '56px' as const; // la acción única de una pantalla: COBRAR, desbloquear
