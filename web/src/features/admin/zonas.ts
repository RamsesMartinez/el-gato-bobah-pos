// Zonas horarias que ofrece la pantalla de Negocio. Es una lista corta a propósito: el sistema se
// vende en México y ofrecer las ~400 zonas IANA convierte un ajuste de una vez en una búsqueda.
// El servidor acepta cualquier nombre IANA válido, así que un negocio fuera de esta lista se
// configura igual — lo que esta lista optimiza es el caso de todos los días.
export const ZONAS_MEXICO = [
  { value: 'America/Mexico_City', label: 'Centro (Ciudad de México, Guadalajara, Monterrey)' },
  { value: 'America/Cancun', label: 'Sureste (Cancún, Quintana Roo)' },
  { value: 'America/Mazatlan', label: 'Pacífico (Mazatlán, Sinaloa, Nayarit)' },
  { value: 'America/Chihuahua', label: 'Chihuahua' },
  { value: 'America/Tijuana', label: 'Noroeste (Tijuana, Baja California)' },
  { value: 'America/Hermosillo', label: 'Sonora (Hermosillo)' },
] as const;

// etiquetaDeZona da el nombre legible de una zona, o la zona misma si no está en la lista corta.
// Un negocio configurado con una zona de fuera no debe ver un campo vacío.
export function etiquetaDeZona(value: string): string {
  return ZONAS_MEXICO.find((z) => z.value === value)?.label ?? value;
}
