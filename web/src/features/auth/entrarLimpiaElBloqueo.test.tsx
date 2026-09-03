import { vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';

const login = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({ posApi: { login } }));
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { LoginPage } from './LoginPage';
import { marcarBloqueada, estabaBloqueada } from './inactividad';

// ENTRAR CON USUARIO Y CONTRASEÑA NO PUEDE VOLVER A PEDIR EL PIN.
//
// La marca de "pantalla bloqueada" vive en sessionStorage para sobrevivir a un F5 —si no, bastaría
// recargar para saltarse el bloqueo— pero sobrevivía también al ciclo de salir y volver a entrar con
// credenciales completas. El operador que olvidó su PIN tocaba "Entrar con usuario y contraseña",
// se autenticaba, y la pantalla lo volvía a bloquear pidiéndole justo el PIN que no recuerda:
// quedaba encerrado con el local abierto, que es el escenario que el botón existe para evitar.
//
// Entrar con usuario y contraseña ES identificarse, y de la forma más fuerte que el sistema tiene.
test('al entrar con usuario y contraseña se levanta el bloqueo de pantalla', async () => {
  marcarBloqueada(window.sessionStorage);
  expect(estabaBloqueada(window.sessionStorage)).toBe(true);

  login.mockResolvedValue({
    accessToken: 'tok',
    user: { id: 1, name: 'Ana', role: 'cajero', companyId: 1, companyName: 'Gato' },
  });

  render(
    <ChakraProvider value={defaultSystem}>
      <MemoryRouter><LoginPage /></MemoryRouter>
    </ChakraProvider>,
  );
  fireEvent.change(screen.getByPlaceholderText('usuario@empresa'), { target: { value: 'ana@gato' } });
  fireEvent.change(screen.getByPlaceholderText('Contraseña'), { target: { value: 'x' } });
  fireEvent.click(screen.getByRole('button', { name: /entrar/i }));

  await waitFor(() => expect(login).toHaveBeenCalled());
  await waitFor(() =>
    expect(estabaBloqueada(window.sessionStorage), 'la pantalla vuelve a pedir el PIN que el operador acaba de decir que no recuerda').toBe(false));
});
