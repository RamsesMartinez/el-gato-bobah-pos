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

// La revisión de tableta: «Cancelar pedido» rojo pegado a «Volver» con 8 px se toca por error.
test('destructiva: la acción roja va aparte de «Volver»', async () => {
  render(
    <Provider>
      <ReasonSheet isOpen destructive title="¿Cancelar el pedido de Siamés?" label="Motivo"
        confirmLabel="Cancelar pedido" onDone={vi.fn()} />
    </Provider>,
  );
  const rojo = await screen.findByRole('button', { name: 'Cancelar pedido' });
  expect(rojo.parentElement).not.toBe(screen.getByRole('button', { name: 'Volver' }).parentElement);
});

// Un motivo frecuente se elige sin teclear (dueño, 2026-10-09: «Se fue sin pagar»), y la hoja puede
// decir qué va a pasar antes de confirmar.
describe('ReasonSheet: atajos y descripción', () => {
  test('el atajo llena el motivo sin teclear, mide 44 px y se puede confirmar', async () => {
    const onDone = vi.fn();
    render(
      <Provider>
        <ReasonSheet isOpen required title="t" label="Motivo" confirmLabel="Confirmar"
          atajos={['Se fue sin pagar']} description="Se dan por perdidos $6" onDone={onDone} />
      </Provider>,
    );
    expect(await screen.findByText('Se dan por perdidos $6')).toBeInTheDocument();
    const atajo = screen.getByRole('button', { name: 'Se fue sin pagar' });
    expect(parseInt(getComputedStyle(atajo).minHeight, 10)).toBeGreaterThanOrEqual(44);
    await userEvent.click(atajo);
    await userEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
    expect(onDone).toHaveBeenCalledWith('Se fue sin pagar');
  });
});
