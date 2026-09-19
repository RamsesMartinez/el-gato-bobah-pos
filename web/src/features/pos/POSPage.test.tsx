import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { POSPage } from './POSPage';
import { useTicketStore } from '../../stores/ticket';

// El estado de caja lo decide el BACKEND (/cash-status contesta la misma regla que el cobro), así
// que aquí solo se prueba que la pantalla lo obedezca: sin turno abierto, el catálogo y el ticket
// no se muestran. Antes esto era un aviso naranja que dejaba seguir, y el operador armaba el
// pedido completo para toparse con el rechazo al cobrar, con el cliente enfrente.
const cashStatus = vi.hoisted(() => ({
  current: { open: true } as { open: boolean; deOtroDia?: boolean; openedAt?: string },
}));
vi.mock('../../api/pos', () => ({
  posApi: {
    cashStatus: () => Promise.resolve(cashStatus.current),
    menu: () => Promise.resolve({ categories: [], products: [] }),
    popular: () => Promise.resolve({ items: [] }),
    modifierDefaults: () => Promise.resolve({}),
  },
}));
vi.mock('../../hooks/useMenu', () => ({
  useMenu: () => ({ data: { categories: [], products: [] }, isLoading: false, error: null }),
}));
vi.mock('../../hooks/usePopular', () => ({ usePopular: () => ({ data: [] }) }));
// EL ANCHO SE MOCKEA PORQUE JSDOM NO HACE LAYOUT: `el.clientWidth` es 0 y `wide` sería SIEMPRE
// false, así que la rama ancha del POS —donde vive la píldora flotante— no se monta nunca y
// cualquier test que crea estar mirándola está mirando la barra angosta. Ese hueco dejó pasar una
// píldora que pintaba el total SIN descuento mientras su propio botón cobraba con él.
const anchoDelPos = vi.hoisted(() => ({ width: 500 }));
// Y el ALTO: el panel del pedido arranca colapsado bajo `(max-height: 720px)`, que es el caso de la
// tableta de 600 px. Con el panel abierto la píldora tampoco se monta, así que sin esto el test
// ancho seguiría sin verla.
function tabletaBaja(baja: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: baja && query.includes('max-height'),
    media: query,
    onchange: null,
    addEventListener: () => {}, removeEventListener: () => {},
    addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}
vi.mock('../../hooks/useContainerWidth', () => ({
  useContainerWidth: () => ({ ref: { current: null }, width: anchoDelPos.width }),
}));
vi.mock('../../hooks/useModifierDefaults', () => ({ useModifierDefaults: () => ({ data: {} }) }));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <MemoryRouter>
          <POSPage />
        </MemoryRouter>
      </Provider>
    </QueryClientProvider>,
  );
}

test('sin caja abierta no se muestra la pantalla de venta', async () => {
  cashStatus.current = { open: false };
  montar();
  expect(await screen.findByText(/no hay caja abierta/i)).toBeInTheDocument();
  // Y lo que importa: el botón de cobrar no está por ningún lado.
  expect(screen.queryByRole('button', { name: /cobrar/i })).not.toBeInTheDocument();
});

test('con caja abierta la pantalla de venta se muestra', async () => {
  cashStatus.current = { open: true };
  montar();
  // El bloqueo desaparece; el catálogo vacío del mock no estorba para verificarlo.
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/no hay caja abierta/i)).not.toBeInTheDocument();
});

// EL AVISO DE TURNO VIEJO NO PUEDE APAGAR LA CAJA.
//
// Nace de un defecto real: un turno se quedó abierto cinco días y nada lo decía. Pero el remedio no
// puede ser peor que el mal — un negocio en operación prefiere una fecha corrida a una caja parada,
// así que el aviso informa y ofrece la acción, nunca bloquea la pantalla de venta.
test('el aviso de turno viejo se ve y NO bloquea la pantalla de venta', async () => {
  cashStatus.current = { open: true, deOtroDia: true, openedAt: '2026-08-31T18:29:00Z' };
  montar();
  expect(await screen.findByText(/la caja lleva abierta desde/i)).toBeInTheDocument();
  // Lo que de verdad importa: la pantalla de venta sigue ahí.
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/no hay caja abierta/i)).not.toBeInTheDocument();
});

// Un turno abierto HOY no molesta a nadie. Sin esto, el aviso se volvería ruido permanente y quien
// opera aprendería a ignorarlo — que es como se pierde un aviso que sí importaba.
test('un turno abierto hoy no muestra el aviso', async () => {
  cashStatus.current = { open: true, deOtroDia: false };
  montar();
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/la caja lleva abierta desde/i)).not.toBeInTheDocument();
});

// El backend puede no traer el campo todavía: el front se despliega ~7 minutos antes. Durante esos
// minutos la pantalla se queda SIN aviso, nunca rota ni con un aviso inventado.
test('sin el campo del backend no se inventa un aviso', async () => {
  cashStatus.current = { open: true };
  montar();
  expect(await screen.findByPlaceholderText(/buscar/i)).toBeInTheDocument();
  expect(screen.queryByText(/la caja lleva abierta desde/i)).not.toBeInTheDocument();
});

// LAS TRES SUPERFICIES QUE PINTAN EL TOTAL TIENEN QUE DECIR LO MISMO.
//
// Se corre en los DOS anchos: a 500 px (tableta angosta) manda la barra de abajo; a 1024 px —el
// presupuesto real del POS— manda la píldora flotante sobre el catálogo, que es la que el operador
// ve cuando el panel arranca colapsado.
//
// A 1024×600 el panel del pedido arranca COLAPSADO, así que la píldora flotante y la barra angosta
// son las que el operador ve todo el día — y son las que antes no sabían del envío: con un envío
// mal escrito cobraban el default del negocio mientras el panel pintaba otra cifra. El descuento
// entra por el mismo lugar, así que se prueba que ninguna superficie lo ignore.
test.each([500, 1024])('el descuento baja el total en todas las superficies que lo pintan (ancho %i)', async (ancho) => {
  anchoDelPos.width = ancho;
  tabletaBaja(true);
  cashStatus.current = { open: true };
  useTicketStore.setState(useTicketStore.getInitialState(), true);
  useTicketStore.getState().addLine({ productId: 1, name: 'Crepa', unitPrice: 95, qty: 1, modifiers: [] });
  useTicketStore.getState().setDescuento('20');
  montar();
  await screen.findByPlaceholderText(/buscar/i);

  // "N art · $X" es la firma de la píldora y de la barra angosta.
  const superficies = screen.getAllByText(/art ·/);
  expect(superficies.length, 'ninguna superficie pinta el total: el test dejó de mirar lo que mira el operador')
    .toBeGreaterThan(0);
  for (const s of superficies) {
    expect(s.textContent, 'esta superficie cobra el total SIN descuento: el operador ve una cifra y se cobra otra')
      .toContain('$75');
  }
});
