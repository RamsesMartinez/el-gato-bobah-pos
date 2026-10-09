import { expect, test } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { Provider } from './provider';
import { Toaster, toaster } from './toaster';

// Los avisos tapaban «Cobrar» y «Enviar» varios segundos (validación como usuario nuevo): abajo a la
// derecha es justo el pie del ticket. Arriba al centro caen sobre el catálogo, lejos del dinero.
test('los avisos salen arriba al centro, lejos del pie del ticket', () => {
  expect(toaster.attrs.placement).toBe('top');
});

// Arriba tapan la fila de cuentas y los canales: el aviso no se queda con esos toques (revisión de
// tableta). Solo su botón de acción los recibe.
test('el aviso deja pasar los toques, salvo su botón', async () => {
  render(<Provider><Toaster /></Provider>);
  act(() => { toaster.create({ title: 'No se guardó Taro', action: { label: 'Reintentar', onClick: () => {} } }); });
  const titulo = await screen.findByText('No se guardó Taro');
  const raiz = titulo.closest('[data-part="root"]') as HTMLElement;
  expect(getComputedStyle(raiz).pointerEvents).toBe('none');
  expect(getComputedStyle(screen.getByRole('button', { name: 'Reintentar' })).pointerEvents).toBe('auto');
});
