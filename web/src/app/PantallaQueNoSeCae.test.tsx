import { describe, it, expect, vi, afterEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { Provider } from '../components/ui/provider';
import { PantallaQueNoSeCae } from './PantallaQueNoSeCae';
import { useAppUpdate } from '../stores/appUpdate';

// UN DEFECTO EN UNA PANTALLA NO PUEDE DEJAR AL OPERADOR SIN NADA QUE TOCAR.
//
// Sin barrera, un throw durante el render vacía el `#root`: pantalla blanca, sin mensaje y sin el
// aviso de versión nueva —que lo pinta el riel, dentro del árbol que acaba de morir—. La tableta se queda con el service worker viejo sirviendo la versión rota y la única
// salida, cerrar la app y volver a abrirla, es la que nadie adivina. Ya pasó el 8 de septiembre de
// 2026 con el folio de plataforma.
function Truena(): React.ReactElement {
  throw new Error('detalle interno que el operador no puede accionar');
}

// React escribe el error en consola aunque la barrera lo atrape; silenciarlo evita que el ruido
// haga ver el test como fallido.
const silencio = vi.spyOn(console, 'error').mockImplementation(() => {});
afterEach(() => {
  silencio.mockClear();
  useAppUpdate.setState({ needRefresh: false, apply: () => {} });
});

describe('la barrera de pantalla', () => {
  it('deja pasar lo que no truena', () => {
    render(<Provider><PantallaQueNoSeCae><p>catálogo</p></PantallaQueNoSeCae></Provider>);
    expect(screen.getByText('catálogo')).toBeInTheDocument();
  });

  it('pinta una salida en vez de dejar la pantalla vacía', () => {
    const { container } = render(
      <Provider><PantallaQueNoSeCae><Truena /></PantallaQueNoSeCae></Provider>);
    expect(container.textContent?.trim().length,
      'la barrera no pintó nada: el operador ve exactamente la misma pantalla en blanco')
      .toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /Reiniciar la aplicación/i })).toBeInTheDocument();
  });

  // La constitución: en pantalla va lo que el operador necesita para decidir, no el internal que
  // solo entiende quien leyó el código. El mensaje del error va a la consola, no a la tableta.
  it('no le enseña el error interno a quien opera', () => {
    render(<Provider><PantallaQueNoSeCae><Truena /></PantallaQueNoSeCae></Provider>);
    expect(screen.queryByText(/detalle interno/)).toBeNull();
  });

  // El botón tiene que alcanzarse con el dedo a la primera: por debajo de 44 px el operador toca
  // dos veces y la segunda cae en otra cosa.
  it('el botón declara el piso táctil', () => {
    render(<Provider><PantallaQueNoSeCae><Truena /></PantallaQueNoSeCae></Provider>);
    const b = screen.getByRole('button', { name: /Reiniciar la aplicación/i });
    expect(b.getAttribute('data-alto-minimo'),
      'jsdom no resuelve las clases de Chakra, así que el piso se declara en el marcado y se mide de verdad en e2e')
      .toBe('56');
  });

  // El toast de versión nueva vivía fuera de la barrera y era la salida cuando una versión rota
  // tumbaba el render. Con el aviso en el riel (spec 064) muere junto con la pantalla, y «Reiniciar»
  // vuelve a cargar la versión rota: el service worker viejo sigue atendiendo mientras el nuevo
  // espera. La barrera ofrece la versión nueva cuando la hay.
  it('si hay versión nueva, ofrece pasar a ella', () => {
    const apply = vi.fn();
    useAppUpdate.setState({ needRefresh: true, apply });
    render(<Provider><PantallaQueNoSeCae><Truena /></PantallaQueNoSeCae></Provider>);
    fireEvent.click(screen.getByRole('button', { name: 'Actualizar a la versión nueva' }));
    expect(apply).toHaveBeenCalledTimes(1);
  });

  it('sin versión nueva solo ofrece reiniciar', () => {
    render(<Provider><PantallaQueNoSeCae><Truena /></PantallaQueNoSeCae></Provider>);
    expect(screen.queryByRole('button', { name: 'Actualizar a la versión nueva' })).toBeNull();
  });
});

