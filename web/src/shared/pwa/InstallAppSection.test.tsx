import { expect, test } from 'vitest';
import { render } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import { InstallAppSection } from './InstallAppSection';

// La sección explicaba cómo forzar la instalación por IP con chrome://flags: un internal del
// navegador que quien opera no necesita y que la constitución prohíbe en pantalla.
test('instalar en la tablet no habla de internals del navegador', () => {
  const { container } = render(<Provider><InstallAppSection /></Provider>);
  expect(container.textContent).not.toMatch(/chrome:\/\/|flags|http:\/\//i);
  expect(container.textContent).toContain('Instalar en esta tablet');
});
