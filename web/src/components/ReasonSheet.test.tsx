import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from './ui/provider';
import { ReasonSheet } from './ReasonSheet';

function montar(required = false) {
  const onDone = vi.fn();
  render(
    <Provider>
      <ReasonSheet isOpen title="Cancelar gasto" label="Motivo" confirmLabel="Cancelar gasto"
        required={required} onDone={onDone} />
    </Provider>,
  );
  return { onDone };
}

describe('ReasonSheet: el motivo se pide en una hoja de la app', () => {
  test('confirma con el texto escrito, recortado', async () => {
    const { onDone } = montar();
    fireEvent.change(await screen.findByLabelText('Motivo'), { target: { value: '  duplicado  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar gasto' }));
    expect(onDone).toHaveBeenCalledWith('duplicado');
  });

  // Opcional: vacío es una respuesta válida, distinta de arrepentirse.
  test('sin texto y no obligatorio, confirma con cadena vacía', async () => {
    const { onDone } = montar(false);
    fireEvent.click(await screen.findByRole('button', { name: 'Cancelar gasto' }));
    expect(onDone).toHaveBeenCalledWith('');
  });

  test('obligatorio: no deja confirmar vacío ni con puros espacios', async () => {
    montar(true);
    const ok = await screen.findByRole('button', { name: 'Cancelar gasto' });
    expect(ok).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Motivo'), { target: { value: '   ' } });
    expect(ok).toBeDisabled();
  });

  test('volver devuelve null, no una cadena vacía', async () => {
    const { onDone } = montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Volver' }));
    expect(onDone).toHaveBeenCalledWith(null);
  });

  test('Escape devuelve null', async () => {
    const { onDone } = montar();
    await screen.findByRole('dialog');
    await new Promise((r) => setTimeout(r, 50));
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(onDone).toHaveBeenCalledWith(null));
  });

  test('campo y botones de al menos 44 px', async () => {
    montar();
    for (const el of [
      await screen.findByLabelText('Motivo'),
      screen.getByRole('button', { name: 'Cancelar gasto' }),
      screen.getByRole('button', { name: 'Volver' }),
    ]) {
      expect(parseInt(getComputedStyle(el).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
    }
  });
});
