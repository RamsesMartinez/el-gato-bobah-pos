import { render } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MedidorDeUso } from './MedidorDeUso';
import { pantallaDe } from './rutas-medidas';

vi.mock('../api/uso', () => ({
  medirPantalla: vi.fn(),
  esRecargaDeLaPagina: vi.fn(() => false),
}));

const { medirPantalla, esRecargaDeLaPagina } = await import('../api/uso');

afterEach(() => {
  vi.clearAllMocks();
});

function montarEn(ruta: string) {
  return render(
    <MemoryRouter initialEntries={[ruta]}>
      <MedidorDeUso />
      <Routes>
        <Route path="*" element={<div />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('el medidor de aperturas', () => {
  it('cuenta la pantalla en la que se entra', () => {
    montarEn('/caja');
    expect(medirPantalla).toHaveBeenCalledWith('caja');
  });

  // El caso que el antirebote de N segundos hacía mal por los dos lados: se le escapaba la recarga
  // lenta y se comía las entradas rápidas legítimas. Preguntarle al navegador no tiene ninguno de
  // los dos problemas.
  it('NO cuenta la primera vista si la página se recargó', () => {
    vi.mocked(esRecargaDeLaPagina).mockReturnValue(true);
    montarEn('/pos');
    expect(medirPantalla).not.toHaveBeenCalled();
  });

  it('no mide las pantallas que no se pueden medir', () => {
    // Sin sesión no hay empresa ni rol, así que el servidor no puede atribuir el evento: inventarle
    // un nombre solo llenaría el log de descartes.
    montarEn('/login');
    expect(medirPantalla).not.toHaveBeenCalled();
  });
});

describe('el mapa de rutas', () => {
  it('traduce las rutas del POS a nombres de pantalla', () => {
    expect(pantallaDe('/pos')).toBe('pos');
    expect(pantallaDe('/catalogo/opciones')).toBe('catalogo');
    expect(pantallaDe('/almacen')).toBe('inventario');
  });

  it('deja fuera lo que no se mide', () => {
    for (const ruta of ['/login', '/recuperar', '/reset']) {
      expect(pantallaDe(ruta)).toBeNull();
    }
  });
});
