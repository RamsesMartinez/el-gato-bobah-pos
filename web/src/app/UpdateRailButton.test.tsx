import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Provider } from '../components/ui/provider';
import { useAppUpdate } from '../stores/appUpdate';
import { UpdateRailButton } from './UpdateRailButton';

const renderButton = () => render(<Provider><UpdateRailButton /></Provider>);

// El aviso de versión nueva en el lugar del logo (spec 064, maqueta A).
describe('el botón de versión nueva del riel', () => {
  beforeEach(() => useAppUpdate.setState({ needRefresh: false, apply: () => {} }));

  test('sin versión nueva no pinta nada', () => {
    renderButton();
    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.queryByText('Actualizar')).toBeNull();
  });

  // Un toque y con el dedo: 44 px como todo control del POS (FR-004).
  test('con versión nueva se aplica con un toque en un botón de 44 px', () => {
    const apply = vi.fn();
    useAppUpdate.setState({ needRefresh: true, apply });
    renderButton();
    const boton = screen.getByRole('button', { name: 'Actualizar a la versión nueva' });
    expect(boton).toHaveTextContent('Actualizar');
    expect(parseInt(getComputedStyle(boton).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
    fireEvent.click(boton);
    expect(apply).toHaveBeenCalledTimes(1);
  });
});
