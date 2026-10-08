import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from './ui/provider';
import { ConfirmSheet } from './ConfirmSheet';

function montar(props: Partial<Parameters<typeof ConfirmSheet>[0]> = {}) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  render(
    <Provider>
      <ConfirmSheet isOpen title="¿Descartar la cuenta de Levkoy?"
        description="Se pierden 2 productos ($74); no se ha mandado a cocina."
        confirmLabel="Descartar" cancelLabel="Seguir capturando" destructive
        onConfirm={onConfirm} onCancel={onCancel} {...props} />
    </Provider>,
  );
  return { onConfirm, onCancel };
}

function alto(el: HTMLElement) {
  return parseInt(getComputedStyle(el).minHeight || '0', 10);
}

describe('ConfirmSheet: la confirmación es una hoja de la app, no un diálogo del sistema', () => {
  test('pinta la pregunta y lo que pasa', async () => {
    montar();
    expect(await screen.findByText('¿Descartar la cuenta de Levkoy?')).toBeInTheDocument();
    expect(screen.getByText(/Se pierden 2 productos/)).toBeInTheDocument();
  });

  test('los dos botones miden al menos 44 px', async () => {
    montar();
    expect(alto(await screen.findByRole('button', { name: 'Descartar' }))).toBeGreaterThanOrEqual(44);
    expect(alto(screen.getByRole('button', { name: 'Seguir capturando' }))).toBeGreaterThanOrEqual(44);
  });

  // La acción destructiva no comparte contenedor con la segura: un toque que se resbala sobre la
  // segura no puede caer en la que borra.
  test('la destructiva está separada de la segura y es roja', async () => {
    montar();
    const borrar = await screen.findByRole('button', { name: 'Descartar' });
    const seguir = screen.getByRole('button', { name: 'Seguir capturando' });
    expect(borrar.parentElement).not.toBe(seguir.parentElement);
    expect(borrar.getAttribute('data-destructive')).toBe('true');
  });

  test('confirmar llama solo a onConfirm', async () => {
    const { onConfirm, onCancel } = montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Descartar' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).not.toHaveBeenCalled();
  });

  test('la acción segura cancela', async () => {
    const { onConfirm, onCancel } = montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Seguir capturando' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  // Escape (y tocar el fondo, que Chakra resuelve igual por onOpenChange) es cancelar, nunca
  // confirmar: salir de la hoja sin leerla no puede borrar nada.
  test('Escape cancela', async () => {
    const { onConfirm, onCancel } = montar();
    await screen.findByRole('dialog');
    await new Promise((r) => setTimeout(r, 50));
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(onCancel).toHaveBeenCalled());
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
