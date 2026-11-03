import { render, screen, fireEvent } from '@testing-library/react';
import { vi, expect, test } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { CierreAutorizadoSheet } from './CierreAutorizadoSheet';

function renderSheet(onConfirmar = vi.fn(), onVolver = vi.fn()) {
  render(<Provider><CierreAutorizadoSheet isOpen nombre="Ana" onConfirmar={onConfirmar} onVolver={onVolver} /></Provider>);
  return { onConfirmar, onVolver };
}

const seguir = () => screen.getByRole('button', { name: 'Seguir con el cierre' });

test('tres motivos en botones de 52 px y lo que queda registrado', () => {
  renderSheet();
  for (const m of ['Se fue sin checar', 'Salió por una emergencia', 'Otro']) {
    expect(screen.getByRole('button', { name: m })).toHaveStyle({ minHeight: '52px' });
  }
  expect(screen.getByText(/La entrada de Ana quedará «sin salida»/)).toBeInTheDocument();
  expect(screen.getByText('Detalle (opcional)')).toBeInTheDocument();
  expect(seguir()).toBeDisabled();
});

test('el motivo viaja con su detalle', () => {
  const { onConfirmar } = renderSheet();
  fireEvent.click(screen.getByRole('button', { name: 'Se fue sin checar' }));
  expect(seguir()).toBeEnabled();
  fireEvent.change(screen.getByRole('textbox'), { target: { value: '  avisó por teléfono ' } });
  fireEvent.click(seguir());
  expect(onConfirmar).toHaveBeenCalledWith('Se fue sin checar: avisó por teléfono');
});

test('sin detalle viaja solo el motivo', () => {
  const { onConfirmar } = renderSheet();
  fireEvent.click(screen.getByRole('button', { name: 'Salió por una emergencia' }));
  fireEvent.click(seguir());
  expect(onConfirmar).toHaveBeenCalledWith('Salió por una emergencia');
});

test('con «Otro» pregunta qué pasó y espera a que se escriba', () => {
  const { onConfirmar } = renderSheet();
  fireEvent.click(screen.getByRole('button', { name: 'Otro' }));
  expect(screen.getByText('¿Qué pasó?')).toBeInTheDocument();
  expect(seguir()).toBeDisabled();
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'Se sintió mal' } });
  fireEvent.click(seguir());
  expect(onConfirmar).toHaveBeenCalledWith('Otro: Se sintió mal');
});

test('volver no decide nada', () => {
  const { onConfirmar, onVolver } = renderSheet();
  fireEvent.click(screen.getByRole('button', { name: 'Se fue sin checar' }));
  fireEvent.click(screen.getByRole('button', { name: 'Volver' }));
  expect(onVolver).toHaveBeenCalled();
  expect(onConfirmar).not.toHaveBeenCalled();
});
