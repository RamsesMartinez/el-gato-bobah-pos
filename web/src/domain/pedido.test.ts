import { describe, expect, it } from 'vitest';
import { cobraEnvio } from './pedido';

// EL DEFECTO: la pantalla ofrecía cobrar un envío que el servidor no cobra, y el cobro rebotaba
// dejando el pedido creado y sin cobrar.
describe('cobraEnvio', () => {
  it('un domicilio propio sí cobra envío', () => {
    expect(cobraEnvio({ serviceType: 'domicilio', platformId: null })).toBe(true);
  });

  it('mostrador no cobra envío', () => {
    expect(cobraEnvio({ serviceType: 'mostrador', platformId: null })).toBe(false);
  });

  it('con plataforma NO cobra envío, aunque la cuenta diga domicilio', () => {
    // Es el caso caro: el reparto lo cobra la plataforma, no el negocio.
    expect(cobraEnvio({ serviceType: 'domicilio', platformId: 3 })).toBe(false);
  });
});
