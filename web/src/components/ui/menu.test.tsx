import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { Provider } from './provider';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem, MenuRadioItemGroup, MenuRadioItem } from './menu';

// LOS RENGLONES DEL MENÚ MIDEN 44 PX. La receta de Chakra los dejaba en ~32 px, y en la tableta los
// filtros del catálogo —nueve renglones seguidos— se tocaban uno por otro. Lo encontró la revisión
// de tableta de la spec 028.
describe('menú táctil', () => {
  it('cada renglón tocable mide al menos 44 px', async () => {
    render(
      <Provider>
        <MenuRoot open>
          <MenuTrigger>Filtros</MenuTrigger>
          <MenuContent>
            <MenuItem value="uno">Uno</MenuItem>
            <MenuRadioItemGroup value="a">
              <MenuRadioItem value="a">Opción A</MenuRadioItem>
            </MenuRadioItemGroup>
          </MenuContent>
        </MenuRoot>
      </Provider>,
    );
    for (const name of ['Uno', 'Opción A']) {
      const item = (await screen.findByText(name)).closest('[role^="menuitem"]') as HTMLElement;
      expect(getComputedStyle(item).minHeight, name).toBe('44px');
    }
  });
});
