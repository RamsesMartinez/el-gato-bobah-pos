import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router';
import { LuStore, LuWallet } from 'react-icons/lu';
import { Provider } from '../../../components/ui/provider';
import { SettingsLayout, type SeccionDeAjustes } from './SettingsLayout';

// Ajustes del negocio por secciones (spec 092, maqueta elegida: selector fijo, lista por omisión).
const SECCIONES: SeccionDeAjustes[] = [
  { id: 'negocio', label: 'Negocio', icon: LuStore, resumen: 'Nombre y zona', contenido: <p>Contenido de negocio</p> },
  { id: 'caja', label: 'Caja', icon: LuWallet, resumen: 'Métodos de cobro', contenido: <p>Contenido de caja</p> },
];

function Donde() {
  const l = useLocation();
  return <span data-testid="url">{l.pathname + l.search}</span>;
}

function pinta(ruta = '/negocio') {
  render(<Provider><MemoryRouter initialEntries={[ruta]}><SettingsLayout secciones={SECCIONES} /><Donde /></MemoryRouter></Provider>);
}

const selector = () => screen.getByRole('radiogroup', { name: /vista de los ajustes/i });

beforeEach(() => localStorage.clear());

describe('ajustes por secciones', () => {
  it('abre en la lista con la primera sección', () => {
    pinta();
    expect(within(selector()).getByRole('radio', { name: /lista/i })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByText('Contenido de negocio')).toBeInTheDocument();
    expect(screen.queryByText('Contenido de caja')).not.toBeInTheDocument();
  });

  it('la sección va en la URL: un enlace abre en ella y tocar otra la cambia', async () => {
    const u = userEvent.setup();
    pinta('/negocio?seccion=caja');
    expect(screen.getByText('Contenido de caja')).toBeInTheDocument();
    await u.click(screen.getByRole('button', { name: /^negocio$/i }));
    expect(screen.getByTestId('url')).toHaveTextContent('/negocio?seccion=negocio');
  });

  it('los tres botones de vista tienen texto y la vista elegida se recuerda', async () => {
    const u = userEvent.setup();
    pinta();
    for (const n of [/lista/i, /íconos/i, /índice/i]) expect(within(selector()).getByRole('radio', { name: n })).toBeInTheDocument();
    await u.click(within(selector()).getByRole('radio', { name: /índice/i }));
    expect(screen.getByText('Métodos de cobro')).toBeInTheDocument();
    expect(localStorage.getItem('egb:ajustes:vista')).toBe('indice');
  });

  it('el selector no se mueve al cambiar de vista: es el mismo nodo en la barra', async () => {
    const u = userEvent.setup();
    pinta();
    const antes = selector();
    await u.click(within(antes).getByRole('radio', { name: /íconos/i }));
    expect(selector()).toBe(antes);
    await u.click(within(antes).getByRole('radio', { name: /índice/i }));
    expect(selector()).toBe(antes);
  });

  it('en el índice, entrar a una sección y volver', async () => {
    const u = userEvent.setup();
    localStorage.setItem('egb:ajustes:vista', 'indice');
    pinta();
    await u.click(screen.getByRole('button', { name: /caja/i }));
    expect(screen.getByText('Contenido de caja')).toBeInTheDocument();
    await u.click(screen.getByRole('button', { name: /todas las secciones/i }));
    expect(screen.getByText('Nombre y zona')).toBeInTheDocument();
  });

  it('sin almacenamiento del navegador abre en la lista', () => {
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('bloqueado'); });
    pinta();
    expect(within(selector()).getByRole('radio', { name: /lista/i })).toHaveAttribute('aria-checked', 'true');
    get.mockRestore();
  });
});
