import { expect, test, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { EmployeesPage } from './EmployeesPage';

vi.mock('../../api/admin', () => ({
  adminApi: { users: () => Promise.resolve({ items: [{ id: 1, name: 'Ana', username: 'ana', role: 'cajero', isActive: true }] }) },
}));

const px = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

// En Empleados los campos del alta medían 40 px y «Gestionar» 36: bajo el mínimo de 44 px con el
// que un dedo acierta a la primera (validación con Playwright a 1024×600).
test('todo campo y botón de Empleados mide al menos 44 px', async () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const { container } = render(<QueryClientProvider client={qc}><Provider><EmployeesPage /></Provider></QueryClientProvider>);
  await screen.findByText('Ana');
  const chicos = [...container.querySelectorAll('input:not([type=checkbox])'), ...screen.getAllByRole('button')]
    .filter((el) => px(el) < 44)
    .map((el) => el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.textContent);
  expect(chicos).toEqual([]);
  expect(px(screen.getByRole('button', { name: 'Gestionar' }))).toBeGreaterThanOrEqual(44);
});
