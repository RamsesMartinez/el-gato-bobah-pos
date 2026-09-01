// La zona con la que nace un negocio, del lado de la pantalla.
//
// Es el espejo de `domain.DefaultTimezone` del servidor y tiene que decir lo mismo: si divergen, la
// pantalla y el corte de caja hablan de días distintos. Vive en su propio archivo para que el
// formateo no dependa de nada más y los papeles lo puedan usar sin arrastrar React.
export const DEFAULT_TIMEZONE = 'America/Mexico_City';
