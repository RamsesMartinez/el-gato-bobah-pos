import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';

import { Provider } from '../../components/ui/provider';
import { CatalogPage } from './CatalogPage';

function montar(path: string) {
  return render(
    <Provider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/catalogo" element={<CatalogPage />}>
            <Route path="*" element={<div />} />
          </Route>
        </Routes>
      </MemoryRouter>
    </Provider>,
  );
}

// MENÚ: cada pestaña explica para qué es detrás de un icono discreto, no con texto fijo en pantalla.
describe('Menú › ayuda de cada pestaña', () => {
  it('en Recetas explica qué es una receta y por dónde empezar', async () => {
    montar('/catalogo/recetas');
    fireEvent.click(screen.getByRole('button', { name: '¿Para qué sirve Recetas?' }));
    expect(await screen.findByText('Qué sale del almacén cada vez que vendes algo.')).toBeInTheDocument();
  });

  it('la ayuda es la de la pestaña abierta', async () => {
    montar('/catalogo/opciones');
    fireEvent.click(screen.getByRole('button', { name: '¿Para qué sirve Extras?' }));
    expect(await screen.findByText(/Lo que se le agrega o se le quita a un producto/)).toBeInTheDocument();
    expect(screen.queryByText('Qué sale del almacén cada vez que vendes algo.')).not.toBeInTheDocument();
  });

  it('el icono mide lo que un dedo acierta', () => {
    montar('/catalogo/productos');
    expect(screen.getByRole('button', { name: '¿Para qué sirve Productos?' })).toHaveStyle({ minHeight: '44px' });
  });
});
