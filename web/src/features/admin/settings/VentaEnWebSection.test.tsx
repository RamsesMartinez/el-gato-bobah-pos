import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../../components/ui/provider';
import { VentaEnWebSection } from './VentaEnWebSection';

vi.mock('../../../api/admin', () => ({
  adminApi: { webSalesSummary: () => Promise.resolve({ onWeb: 142, withWebPrice: 3, total: 160 }) },
}));

describe('la sección Venta en web de Ajustes', () => {
  it('dice cuántos se venden en la web, cuántos con precio propio, y lleva a Menú', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<Provider><QueryClientProvider client={qc}><MemoryRouter><VentaEnWebSection /></MemoryRouter></QueryClientProvider></Provider>);
    expect(await screen.findByText('142')).toBeInTheDocument();
    expect(screen.getByText('de 160')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /ir a los productos/i })).toBeInTheDocument();
  });
});
