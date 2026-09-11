import { vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const groups = vi.hoisted(() => vi.fn());
vi.mock('../../api/admin', async (original) => {
  const real = await original<typeof import('../../api/admin')>();
  return { ...real, adminApi: { ...real.adminApi, groups, groupOptions: vi.fn() } };
});
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { ModifierOptionsPage } from './ModifierOptionsPage';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

function grupo(over: Partial<Record<string, unknown>> = {}) {
  return {
    id: 1, name: 'Salsas', isActive: true, defaultMin: 0, defaultMax: 3,
    optionCount: 0, optionPreview: '', productCount: 0, overrideCount: 0, ...over,
  };
}

function pagina(items: unknown[]) {
  return { items, total: items.length, counts: { act: items.length, inact: 0 } };
}

afterEach(() => { vi.clearAllMocks(); });

// EL GRUPO SIN OPCIONES ACTIVAS, VISTO DESDE LA PANTALLA.
//
// Es el dato que tumbó `/catalogo/opciones` en producción con un 500: `string_agg` sobre un
// conjunto vacío devuelve NULL y sqlc lo tipaba como `string`. Arreglado en el servidor con
// `coalesce(..., '')`, la pantalla recibe cadena vacía — y lo que se prueba aquí es que sepa
// pintarla, porque un renglón de vista previa en blanco se lee como una tarjeta rota.
test('un grupo sin opciones activas no pinta renglón de vista previa', async () => {
  groups.mockResolvedValue(pagina([grupo({ name: 'Salsas sin opciones' })]));
  const { container } = pinta(<ModifierOptionsPage />);

  expect(await screen.findByText('Salsas sin opciones')).toBeInTheDocument();
  expect(screen.getByText(/0 opciones/)).toBeInTheDocument();
  // Sin "y N más" colgando de una lista que no existe.
  expect(screen.queryByText(/y .* más/)).toBeNull();

  // Y SIN EL RENGLÓN VACÍO. Afirmarlo por lo que NO aparece no basta: con la guardia quitada, la
  // tarjeta pinta un párrafo en blanco que ninguna búsqueda por texto encuentra y que en pantalla
  // se ve como un hueco. Se cuenta contra la misma tarjeta CON vista previa, que es la única
  // comparación que distingue "no hay renglón" de "hay uno vacío".
  const parrafos = [...container.querySelectorAll('p')].filter((e) => e.children.length === 0);
  expect(parrafos.some((e) => (e.textContent ?? '') === ''),
    'la tarjeta pinta un renglón de vista previa vacío').toBe(false);
});

// LA VISTA PREVIA SE TOPA A 4 Y EL RESTO SE RESUME, y esa resta es donde vive el off-by-one: el
// servidor manda cuatro nombres y el conteo COMPLETO, así que la tarjeta tiene que restar los que
// ya mostró. Con cinco opciones dice "y 1 más", no "y 5 más" ni "y 0 más".
test('con más de cuatro opciones resume el resto sin contar dos veces las que ya mostró', async () => {
  groups.mockResolvedValue(pagina([grupo({
    optionCount: 6, optionPreview: 'BBQ · Búfalo · Chipotle · Ajo',
  })]));
  pinta(<ModifierOptionsPage />);

  expect(await screen.findByText(/BBQ · Búfalo · Chipotle · Ajo/)).toBeInTheDocument();
  expect(screen.getByText(/y 2 más/)).toBeInTheDocument();
});

// Y con exactamente cuatro no sobra nada que resumir: "y 0 más" sería un renglón que miente.
test('con exactamente cuatro opciones no dice que haya más', async () => {
  groups.mockResolvedValue(pagina([grupo({
    optionCount: 4, optionPreview: 'BBQ · Búfalo · Chipotle · Ajo',
  })]));
  pinta(<ModifierOptionsPage />);

  expect(await screen.findByText(/BBQ · Búfalo · Chipotle · Ajo/)).toBeInTheDocument();
  expect(screen.queryByText(/más/)).toBeNull();
});
