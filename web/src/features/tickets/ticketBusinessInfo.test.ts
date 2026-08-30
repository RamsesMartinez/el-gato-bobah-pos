import { describe, it, expect } from 'vitest';
import { toTicketBusinessInfo } from './ticketBusinessInfo';
import type { BusinessSettings } from '../../api/pos';

const settings: BusinessSettings = {
  deliveryFee: '20',
  businessName: 'El Gato Bobah',
  address: 'Av. Siempre Viva 742',
  phone: '55 1234 5678',
  headerNote: 'Wi-Fi: gatobobah',
  footerNote: '¡Vuelve pronto!',
  autoPrintOnClose: false,
  timezone: 'America/Mexico_City',
  hasLogo: false,
  logoUpdatedAt: null,
};

describe('toTicketBusinessInfo', () => {
  it('cae al logo del Gato Bobah cuando el negocio no subió uno', () => {
    const info = toTicketBusinessInfo(settings);
    // El default viaja empaquetado con el front: un negocio recién dado de alta imprime un ticket
    // con logo desde el primer pedido, sin configurar nada.
    expect(info.logoDataUri).toMatch(/^data:image\//);
  });

  it('usa el logo subido cuando existe', () => {
    const subido = 'data:image/png;base64,QUFB';
    expect(toTicketBusinessInfo({ ...settings, hasLogo: true }, subido).logoDataUri).toBe(subido);
  });

  it('ignora un logo subido vacío y regresa al default', () => {
    // Una conversión fallida del binario no debe imprimir <img src=""> en el ticket.
    expect(toTicketBusinessInfo({ ...settings, hasLogo: true }, '').logoDataUri).toMatch(/^data:image\//);
  });

  it('pasa la identidad del negocio tal cual, sin inventar defaults', () => {
    const info = toTicketBusinessInfo(settings);
    expect(info).toMatchObject({
      businessName: 'El Gato Bobah',
      address: 'Av. Siempre Viva 742',
      phone: '55 1234 5678',
      footerNote: '¡Vuelve pronto!',
    });
  });
});
