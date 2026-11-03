import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';

import { useSessionStore } from '../../../stores/session';
import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { ArticleSheet } from './ArticleSheet';

const ficha = {
  id: 7, name: 'Hielo en bolsa', isActive: true, baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', currentCost: '0.006',
  minStock: '10000', isPrep: false, categoryId: null, category: null, countPlaceId: null, countPlace: null, onHand: '27000',
  presentations: [{ id: 3, name: 'Bolsa', qtyInBase: '5000' }], costSource: 'compra', updatedAt: '2026-10-19T10:00:00.123456Z',
};
const conAlts = { ...ficha, altNames: ['Hielo Polar'] };
const compra = (id: number, cost: string) => ({ kind: 'purchase', at: `2026-10-${String(id).padStart(2, '0')}T10:00:00Z`, id, unitCost: cost, presentation: 'Bolsa', packQty: '5000', packCost: String(Number(cost) * 5000), supplier: 'Proveedor A', user: null });
const back = vi.hoisted(() => ({
  units: vi.fn(() => Promise.resolve({ items: [
    { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' }, { id: 2, code: 'kg', name: 'Kilo', kind: 'masa', toBase: '1000' },
    { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
  ] })),
  countPlaces: vi.fn(() => Promise.resolve([] as unknown[])),
  createCountPlace: vi.fn(),
  createArticle: vi.fn(),
  similarArticles: vi.fn(() => Promise.resolve({ items: [] as unknown[] })),
  reactivateArticle: vi.fn(() => Promise.resolve()),
  article: vi.fn(),
  updateArticle: vi.fn(() => Promise.resolve({ updatedAt: 'v2' })),
  articleCategories: vi.fn(() => Promise.resolve([])),
  presentations: vi.fn(() => Promise.resolve([{ id: 3, ingredientId: 7, name: 'Bolsa', qtyInBase: '5000', baseUnitCode: 'g', active: true, approximate: false, purchaseUnitId: null }])),
  articleCostHistory: vi.fn(),
  articleAudit: vi.fn(),
  correctArticleCost: vi.fn(() => Promise.resolve()),
  presentationSuggestions: vi.fn(() => Promise.resolve([])),
  articleArchiveCheck: vi.fn(),
  archiveArticle: vi.fn(() => Promise.resolve()),
  setDensity: vi.fn(),
  setArticleAltNames: vi.fn((_id: number, alts: string[], _v?: string) => Promise.resolve({ altNames: alts, updatedAt: 'v-alt' })),
  similarArticleAltNames: vi.fn(() => Promise.resolve([])),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const pos = vi.hoisted(() => ({ businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) }));
vi.mock('../../../api/pos', () => ({ posApi: pos }));

const volver = () => screen.getByRole('button', { name: 'Volver a Artículos' });

function montar(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>{ui}</Provider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.view', 'inventory.configure', 'inventory.receive'] } as never });
  back.article.mockResolvedValue(ficha);
  back.articleCostHistory.mockResolvedValue({ items: [compra(18, '0.006'), compra(10, '0.0055')], next: 'cursor-1', trendPct: '9.1' });
  back.articleAudit.mockResolvedValue({ items: [], next: null });
});

// Spec 063 (maqueta «Editor «Otros nombres»», A; revisión de tableta M2 y A1): en la ficha del
// artículo los otros nombres pasan por el mismo autoguardado que el resto, con su versión.
describe('otros nombres en la ficha del artículo', () => {
  beforeEach(() => back.article.mockResolvedValue(conAlts));

  it('agregar guarda por su ruta y el siguiente guardado usa la versión que devolvió', async () => {
    const u = userEvent.setup();
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByText('Hielo Polar')).toBeInTheDocument();
    await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'Cubos de hielo{Enter}');
    await waitFor(() => expect(back.setArticleAltNames).toHaveBeenCalledWith(7, ['Hielo Polar', 'Cubos de hielo'], ficha.updatedAt));
    const nombre = screen.getByDisplayValue('Hielo en bolsa');
    await u.clear(nombre);
    await u.type(nombre, 'Hielo en cubo{Enter}');
    await waitFor(() => expect(back.updateArticle).toHaveBeenCalledWith(7, expect.objectContaining({ expectedUpdatedAt: 'v-alt', name: 'Hielo en cubo' })));
  });

  it('un choque regresa la lista guardada y lo dice bajo el campo', async () => {
    const u = userEvent.setup();
    back.setArticleAltNames.mockRejectedValueOnce(new ApiError(409, 'ALT_NAME_TAKEN', '«Hielo frapé» ya es el nombre de «Hielo frapé»', 'x', { name: 'Hielo frapé', existingId: 9 }));
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    await u.type(await screen.findByRole('textbox', { name: /otro nombre/i }), 'Hielo frapé{Enter}');
    expect(await screen.findByRole('alert')).toHaveTextContent('ya es el nombre de «Hielo frapé»');
    expect(screen.queryByRole('button', { name: 'Quitar «Hielo frapé»' })).not.toBeInTheDocument();
  });

  it('lo escrito sin «Agregar» se guarda al volver; si choca, la ficha no se cierra', async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    montar(<ArticleSheet id={7} onClose={onClose} />);
    await u.type(await screen.findByRole('textbox', { name: /otro nombre/i }), 'Hielo de garrafón');
    await u.click(volver());
    await waitFor(() => expect(back.setArticleAltNames).toHaveBeenCalledWith(7, ['Hielo Polar', 'Hielo de garrafón'], ficha.updatedAt));
    await waitFor(() => expect(onClose).toHaveBeenCalled());

    onClose.mockClear();
    back.setArticleAltNames.mockRejectedValueOnce(new ApiError(409, 'ALT_NAME_TAKEN', '«Hielo X» ya es otro nombre de «Otro»', 'x', { name: 'Hielo X' }));
    await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'Hielo X');
    await u.click(volver());
    expect(await screen.findByRole('alert')).toHaveTextContent('ya es otro nombre de «Otro»');
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('textbox', { name: /otro nombre/i })).toHaveValue('Hielo X');
  });

  it('quien no configura el almacén no puede editarlos', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.view'] } as never });
    montar(<ArticleSheet id={7} onClose={vi.fn()} />);
    expect(await screen.findByRole('button', { name: 'Quitar «Hielo Polar»' })).toBeDisabled();
  });

  it('ir al conteo también guarda lo escrito sin «Agregar»', async () => {
    const u = userEvent.setup();
    const onCount = vi.fn();
    // «Revisar con un conteo» aparece con existencia negativa y a quien cuenta.
    back.article.mockResolvedValue({ ...conAlts, onHand: '-500' });
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions: ['inventory.view', 'inventory.configure', 'inventory.count'] } as never });
    montar(<ArticleSheet id={7} onClose={vi.fn()} onCount={onCount} />);
    await u.type(await screen.findByRole('textbox', { name: /otro nombre/i }), 'Hielo en cubos');
    await u.click(screen.getByRole('button', { name: /conteo/i }));
    await waitFor(() => expect(back.setArticleAltNames).toHaveBeenCalledWith(7, ['Hielo Polar', 'Hielo en cubos'], ficha.updatedAt));
    await waitFor(() => expect(onCount).toHaveBeenCalled());
  });
});
