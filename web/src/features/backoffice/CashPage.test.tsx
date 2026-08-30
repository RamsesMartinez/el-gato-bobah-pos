import { render, screen } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import { IngresosEgresosCard, TotalsTable, MovementsTable, ExpensesTable } from './CashPage';
import type { CashMovement, CashExpenseLine, MethodTotal, CorteBreakdown } from '../../api/backoffice';

// Render con el Provider de Chakra (los componentes usan su sistema de temas).
function wrap(ui: React.ReactElement) {
  return render(<Provider>{ui}</Provider>);
}

const mov = (o: Partial<CashMovement>): CashMovement => ({
  id: 1, kind: 'entrada', amount: '0', concept: '', createdAt: new Date().toISOString(),
  userName: 'Ana', transferId: null, expenseId: null, ...o,
});

test('IngresosEgresosCard renderiza la jerarquía con Ventas y Propinas', () => {
  const breakdown: CorteBreakdown = {
    ingresos: [{ method: 'Efectivo', total: '115', items: [{ concept: 'Ventas', amount: '100' }, { concept: 'Propinas', amount: '15' }] }],
    ingresosTotal: '115',
    egresos: [{ concept: 'Gastos', amount: '20' }],
    egresosTotal: '20',
    plataformas: [],
  };
  wrap(<IngresosEgresosCard openingCash="100" breakdown={breakdown} currency="MXN" />);
  for (const label of ['Monto inicial', 'Ingresos', 'Efectivo', 'Ventas', 'Propinas', 'Egresos', 'Gastos']) {
    expect(screen.getByText(label)).toBeInTheDocument();
  }
});

test('IngresosEgresosCard no revienta si el breakdown viene incompleto (shape-drift)', () => {
  // @ts-expect-error: simula un backend viejo sin breakdown (guarda defensiva).
  wrap(<IngresosEgresosCard openingCash="0" breakdown={undefined} currency="MXN" />);
  expect(screen.getByText('Sin ingresos')).toBeInTheDocument();
  expect(screen.getByText('Sin egresos')).toBeInTheDocument();
});

test('MovementsTable excluye la salida de gasto y etiqueta los tipos', () => {
  const moves = [
    mov({ id: 1, kind: 'entrada', amount: '50', concept: 'Fondo extra' }),
    mov({ id: 2, kind: 'salida', amount: '20', concept: 'Gasto: Servilletas', expenseId: 9 }), // gasto → excluido
    mov({ id: 3, kind: 'salida', amount: '30', concept: 'Traspaso a Caja fuerte', transferId: 7 }),
    mov({ id: 4, kind: 'salida', amount: '25', concept: 'Retiro parcial' }), // salida manual
  ];
  wrap(<MovementsTable movements={moves} currency="MXN" />);
  expect(screen.getByText('Fondo extra')).toBeInTheDocument();
  expect(screen.getByText('Traspaso a Caja fuerte')).toBeInTheDocument();
  expect(screen.getByText('Retiro parcial')).toBeInTheDocument();
  expect(screen.queryByText('Gasto: Servilletas')).not.toBeInTheDocument(); // el gasto no va aquí
  // Las 3 etiquetas de tipo (movementType) se ejercitan vía el render:
  expect(screen.getByText('Entrada')).toBeInTheDocument();
  expect(screen.getByText('Salida')).toBeInTheDocument();
  expect(screen.getByText('Traspaso')).toBeInTheDocument();
});

test('TotalsTable con withTotalRow agrega la fila Total', () => {
  const totals: MethodTotal[] = [
    { methodId: 1, name: 'Efectivo', expected: '115', declared: '115', difference: '0', autoDeclare: false },
    { methodId: 2, name: 'Tarjeta', expected: '50', declared: '50', difference: '0', autoDeclare: true },
  ];
  wrap(<TotalsTable totals={totals} currency="MXN" withTotalRow />);
  expect(screen.getByText('Efectivo')).toBeInTheDocument();
  expect(screen.getByText('Total')).toBeInTheDocument();
});

test('ExpensesTable muestra filas y total (o nada si vacío)', () => {
  // Cada fila es un PAGO, no un gasto: el importe es el del pago (un gasto liquidado con dos
  // medios aparece en dos renglones, posiblemente en cortes distintos).
  const exps: CashExpenseLine[] = [
    { id: 1, expenseId: 9, category: 'Insumos', supplier: 'Prov A', paymentMethod: 'Efectivo', amount: '46', currency: 'MXN', status: 'pagada' },
  ];
  const { rerender } = wrap(<ExpensesTable expenses={exps} currency="MXN" />);
  expect(screen.getByText('Insumos')).toBeInTheDocument();
  expect(screen.getByText('Total gastos')).toBeInTheDocument();
  // Vacío → no renderiza nada (ahorra espacio).
  rerender(<Provider><ExpensesTable expenses={[]} currency="MXN" /></Provider>);
  expect(screen.queryByText('Total gastos')).not.toBeInTheDocument();
});

// El subtotal por plataforma suma sus DOS métodos, y ese total no está en ningún renglón de arriba:
// es el número que se concilia contra el depósito que la plataforma manda después.
test('IngresosEgresosCard muestra el subtotal por plataforma', () => {
  const breakdown: CorteBreakdown = {
    ingresos: [
      { method: 'Uber Eats en línea', total: '270', items: [{ concept: 'Ventas', amount: '270' }] },
      { method: 'Uber Eats efectivo', total: '135', items: [{ concept: 'Ventas', amount: '135' }] },
    ],
    ingresosTotal: '405',
    egresos: [],
    egresosTotal: '0',
    plataformas: [{ platform: 'Uber Eats', total: '405' }],
  };
  wrap(<IngresosEgresosCard openingCash="0" breakdown={breakdown} currency="MXN" />);
  expect(screen.getByText('Por plataforma')).toBeInTheDocument();
  expect(screen.getByText('Uber Eats')).toBeInTheDocument();
});

// Un turno sin ventas de plataforma no muestra la sección: un encabezado vacío en el corte es una
// pregunta más que el operador se hace mientras busca un descuadre.
test('sin ventas de plataforma la sección no aparece', () => {
  const breakdown: CorteBreakdown = {
    ingresos: [{ method: 'Efectivo', total: '100', items: [{ concept: 'Ventas', amount: '100' }] }],
    ingresosTotal: '100',
    egresos: [],
    egresosTotal: '0',
    plataformas: [],
  };
  wrap(<IngresosEgresosCard openingCash="0" breakdown={breakdown} currency="MXN" />);
  expect(screen.queryByText('Por plataforma')).not.toBeInTheDocument();
});
