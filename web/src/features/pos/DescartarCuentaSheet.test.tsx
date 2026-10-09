import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import { DescartarCuentaSheet } from './DescartarCuentaSheet';
import { hayQuePreguntar } from './estadosDeCuenta';

describe('descartar una cuenta que no se ha mandado (US5, D-7)', () => {
  // Una cuenta vacía no tiene nada que perder: preguntar sería un toque de más para nada.
  test('vacía se descarta sin preguntar; con productos pregunta', () => {
    expect(hayQuePreguntar(0)).toBe(false);
    expect(hayQuePreguntar(1)).toBe(true);
  });

  function montar() {
    const onSeguir = vi.fn();
    const onDescartar = vi.fn();
    render(
      <Provider>
        <DescartarCuentaSheet isOpen nombre="Levkoy" productos={2} total={74}
          onSeguir={onSeguir} onDescartar={onDescartar} />
      </Provider>,
    );
    return { onSeguir, onDescartar };
  }

  test('dice de quién es, cuánto se pierde y que no se mandó a cocina', async () => {
    montar();
    expect(await screen.findByText('¿Descartar la cuenta de Levkoy?')).toBeInTheDocument();
    expect(screen.getByText(/Se pierden 2 productos \(\$74\); no se ha mandado a cocina\./)).toBeInTheDocument();
  });

  test('«Seguir capturando» es la principal y no descarta', async () => {
    const { onSeguir, onDescartar } = montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Seguir capturando' }));
    expect(onSeguir).toHaveBeenCalled();
    expect(onDescartar).not.toHaveBeenCalled();
  });

  test('«Descartar» es roja, va aparte y descarta', async () => {
    const { onDescartar } = montar();
    const b = await screen.findByRole('button', { name: 'Descartar' });
    expect(b.getAttribute('data-destructive')).toBe('true');
    expect(b.parentElement).not.toBe(screen.getByRole('button', { name: 'Seguir capturando' }).parentElement);
    fireEvent.click(b);
    expect(onDescartar).toHaveBeenCalled();
  });

  test('un solo producto se dice en singular', async () => {
    render(
      <Provider>
        <DescartarCuentaSheet isOpen nombre="Persa" productos={1} total={29} onSeguir={vi.fn()} onDescartar={vi.fn()} />
      </Provider>,
    );
    expect(await screen.findByText(/Se pierde 1 producto \(\$29\)/)).toBeInTheDocument();
  });
});
