// Las direcciones de Caja (#290, FR-017; contracts/rutas-web.md). La pestaña se deduce de la ruta y
// no se guarda en memoria: así una recarga vuelve a la misma vista.
export type PestanaDeCaja = 'operar' | 'historico' | 'gestion';

export interface SeccionDeCaja {
  pestana: PestanaDeCaja;
  cajaId: number | null;
  corteId: number | null;
}

const entero = (s: string | undefined) => (s !== undefined && /^\d+$/.test(s) ? Number(s) : null);

// seccionDeCaja lee la ruta. Sin permiso de configurar, /caja/administrar cae en Cajas: la barrera de
// verdad es el servidor, pero la pantalla no ofrece lo que no se puede usar.
export function seccionDeCaja(pathname: string, puedeAdministrar: boolean): SeccionDeCaja {
  const [primero, segundo] = pathname.replace(/^\/caja\/?/, '').split('/');
  if (primero === 'historico') return { pestana: 'historico', cajaId: null, corteId: entero(segundo) };
  if (primero === 'administrar' && puedeAdministrar) return { pestana: 'gestion', cajaId: null, corteId: null };
  return { pestana: 'operar', cajaId: entero(primero), corteId: null };
}

export function rutaDeCaja(pestana: PestanaDeCaja): string {
  return pestana === 'historico' ? '/caja/historico' : pestana === 'gestion' ? '/caja/administrar' : '/caja';
}
