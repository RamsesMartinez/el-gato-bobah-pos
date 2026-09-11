import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import { IngresosEgresosCard, TotalsTable, MovementsTable, ExpensesTable, VentasDelCorte, TablaDelCierre, DiferenciaDelCierre, DesgloseDelConteo, ArqueoDelCorte } from './CashPage';
import type { CashMovement, CashExpenseLine, MethodTotal, CorteBreakdown, CashSessionDetail, CorteSale, ConteosDelTurno, ArqueoDelCajon } from '../../api/backoffice';
import { diferenciasDelCierre } from './cierreDeCaja';
import type { ResultadoDelConteo } from './conteo';

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
    { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '115', declared: '115', difference: '0', autoDeclare: false, requiresEntry: false },
    { methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '50', declared: '50', difference: '0', autoDeclare: true, requiresEntry: false },
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

// UN RECORTE SILENCIOSO SE LEE COMO "ESTO ES TODO".
//
// El detalle de un corte trae hasta 200 ventas. Si el corte cobró más, la pantalla tiene que decir
// cuántas hay en total: sin eso, quien revisa un arqueo concluye que faltan ventas o que sobran, y
// no tiene forma de saber cuál de las dos.
test('el detalle dice cuántas ventas hay cuando muestra solo una parte', () => {
  render(
    <Provider>
      <VentasDelCorte session={corteCon({ salesCount: 340, salesShown: 2 })} />
    </Provider>,
  );
  expect(screen.getByText(/340 ventas/)).toBeInTheDocument();
  expect(screen.getByText(/se muestran las 2 más recientes/)).toBeInTheDocument();
});

// El total del corte NO incluye canceladas, reembolsadas ni propinas, y la pantalla lo dice. Una
// cifra agregada que no declara qué incluye invita a sumarla con otra y a reportar dinero que el
// negocio no tuvo.
test('el total de las ventas del corte declara qué deja fuera', () => {
  render(
    <Provider>
      <VentasDelCorte session={corteCon({ salesCount: 2, salesShown: 2, salesTotal: '200.00' })} />
    </Provider>,
  );
  expect(screen.getByText(/sin canceladas, reembolsadas ni propinas/i)).toBeInTheDocument();
});

// Un corte sin ventas lo dice con una frase. Una tabla con encabezados y cero renglones parece un
// error de carga, y manda a quien revisa a recargar en vez de a seguir.
test('un corte sin ventas lo dice en vez de pintar una tabla vacía', () => {
  render(
    <Provider>
      <VentasDelCorte session={corteCon({ salesCount: 0, salesShown: 0, sales: [] })} />
    </Provider>,
  );
  expect(screen.getByText(/no cobró ninguna venta/i)).toBeInTheDocument();
  expect(screen.queryByRole('table')).not.toBeInTheDocument();
});

function corteCon(over: Partial<CashSessionDetail>): CashSessionDetail {
  const ventas: CorteSale[] = [
    { id: 2, dailyNumber: 12, folioName: 'Chartreux', openedAt: '2026-09-04T19:10:00Z', status: 'entregada', serviceType: 'mostrador', total: '100.00', refund: '0.00' },
    { id: 1, dailyNumber: 11, folioName: null, openedAt: '2026-09-04T18:40:00Z', status: 'cancelada', serviceType: 'mostrador', total: '80.00', refund: '0.00' },
  ];
  return {
    id: 1, registerName: 'Caja principal', status: 'cerrada', openingCash: '0', currency: 'MXN',
    openedAt: '2026-09-04T16:00:00Z', closedAt: null, openedByName: 'Ana', closedByName: null,
    notes: null, totals: [], movements: [], expenses: [],
    breakdown: { ingresos: [], ingresosTotal: '0', egresos: [], egresosTotal: '0', plataformas: [] },
    sales: ventas, salesCount: 2, salesShown: 2, salesTotal: '100.00',
    // Los cortes anteriores a la 0066 no tienen desglose, y son la mayoría de los que existen.
    counts: null,
    drawer: null,
    ...over,
  };
}

// "EL CAMPO NO VINO" NO ES "VINO EN CERO".
//
// El front de producción sale ~7 minutos antes que el backend. En esa ventana el detalle del corte
// llega sin `sales`, y tratarlo como una lista vacía hacía que un corte que sí cobró jurara que no
// cobró nada. Una pantalla que miente sobre dinero es peor que una pantalla incompleta.
test('sin los campos del backend la sección no se dibuja, en vez de decir que no hubo ventas', () => {
  const viejo = corteCon({});
  delete (viejo as Partial<CashSessionDetail>).sales;
  delete (viejo as Partial<CashSessionDetail>).salesCount;
  const { container } = render(<Provider><VentasDelCorte session={viejo} /></Provider>);
  expect(screen.queryByText(/no cobró ninguna venta/i)).not.toBeInTheDocument();
  expect(container.querySelector('table')).toBeNull();
});


// LA DIFERENCIA SE VE ANTES DE TOCAR «CERRAR CAJA» (FR-005), Y EL CAJÓN ES UNO SOLO (spec 015).
//
// Antes de la 003 la única tabla con columna de diferencia se pintaba en el diálogo POSTERIOR al
// cierre: el operador firmaba y se enteraba después. Y hasta la 015 esa diferencia se repartía por
// método, así que un turno real quedó con «Efectivo» en $0.00 y «Didi efectivo» en −$64.80 — dos
// diferencias del mismo montón de billetes.
describe('el cierre en vivo', () => {
  const delCierre: MethodTotal[] = [
    { methodId: 1, name: 'Efectivo', kind: 'efectivo', expected: '200', declared: '0', difference: '0', autoDeclare: false, requiresEntry: false },
    { methodId: 8, name: 'Didi efectivo', kind: 'plataforma', expected: '140', declared: '0', difference: '0', autoDeclare: false, requiresEntry: false },
    { methodId: 2, name: 'Tarjeta', kind: 'tarjeta', expected: '80', declared: '0', difference: '0', autoDeclare: true, requiresEntry: false },
  ];
  // Los dos de efectivo caen en el mismo cajón: $340 esperados.
  const cajon: ArqueoDelCajon = {
    expected: '340', counted: null, difference: null, methodIds: [1, 8], requiresCount: true,
  };

  function pintaCierre(conteo: ResultadoDelConteo | null, elCajon: ArqueoDelCajon | null = cajon) {
    const diferencias = diferenciasDelCierre(delCierre, {});
    return wrap(
      <>
        <TablaDelCierre totals={delCierre} currency="MXN" declared={{}}
          onDeclared={() => {}} cajon={elCajon} conteo={conteo} onContar={() => {}}
          diferencias={diferencias} />
        <DiferenciaDelCierre diferencias={diferencias} cajon={elCajon} conteo={conteo} currency="MXN" />
      </>,
    );
  }

  test('con piezas que suman menos de lo esperado, el faltante ya está en pantalla', () => {
    // El cajón debía tener $340 y se contaron $290.
    pintaCierre({ counts: [{ denominationId: 6, pieces: 5 }], total: 290 });
    expect(screen.getByLabelText('Diferencia del cajón')).toHaveTextContent('-$50');
    expect(screen.getByLabelText('Diferencia del arqueo')).toHaveTextContent('-$50');
    expect(screen.getByText('Faltante')).toBeInTheDocument();
  });

  test('el faltante es UNO: ningún método de cajón reporta el suyo', () => {
    pintaCierre({ counts: [], total: 290 });
    for (const nombre of ['Efectivo', 'Didi efectivo']) {
      expect(screen.getByLabelText(`Diferencia de ${nombre}`)).toHaveTextContent('—');
    }
  });

  test('los métodos del cajón dicen que van al cajón, y no reusan «Automático»', () => {
    pintaCierre(null);
    // Dos renglones informativos, uno por cada método de efectivo.
    expect(screen.getAllByText('Va al cajón')).toHaveLength(2);
    // «Automático» es otra cosa: lo resuelve el servidor, no se cuenta físicamente.
    expect(screen.getAllByText('Automático')).toHaveLength(1);
    expect(screen.queryByLabelText('Declarado de Efectivo')).not.toBeInTheDocument();
  });

  test('un conteo exacto dice que el arqueo cuadra', () => {
    pintaCierre({ counts: [], total: 340 });
    expect(screen.getByText('El arqueo cuadra')).toBeInTheDocument();
  });

  test('sin conteo no se muestra una diferencia inventada', () => {
    // Un cajón sin contar valdría cero y reportaría -$340 de faltante: es el defecto del corte de
    // $1,662, que mandó a buscar dinero que estaba en el cajón.
    pintaCierre(null);
    expect(screen.getByLabelText('Diferencia del cajón')).toHaveTextContent('—');
    expect(screen.queryByLabelText('Diferencia del arqueo')).not.toBeInTheDocument();
  });

  test('con el arqueo ciego no existe la columna «Esperado»', () => {
    // No es solo que no haya cifras: la columna entera sobra, y a 1024×600 el ancho que libera se
    // lo devuelve a lo que el operador vino a leer.
    const ciego: ArqueoDelCajon = { ...cajon, expected: null };
    const sinEsperado = delCierre.map((m) => ({ ...m, expected: null }));
    wrap(
      <TablaDelCierre totals={sinEsperado} currency="MXN" declared={{}} onDeclared={() => {}}
        cajon={ciego} conteo={null} onContar={() => {}}
        diferencias={diferenciasDelCierre(sinEsperado, {})} />,
    );
    expect(screen.queryByRole('columnheader', { name: 'Esperado' })).not.toBeInTheDocument();
    // Y el resto de la tabla sigue en pie: se puede contar.
    expect(screen.getByText('Contar efectivo')).toBeInTheDocument();
  });

  test('con el arqueo ciego no se ve el esperado ni la diferencia', () => {
    // El esperado llega en null: lo que la pantalla no debe mostrar no se le manda.
    const ciego: ArqueoDelCajon = { ...cajon, expected: null };
    pintaCierre({ counts: [], total: 290 }, ciego);
    expect(screen.getByLabelText('Diferencia del cajón')).toHaveTextContent('—');
    expect(screen.queryByLabelText('Diferencia del arqueo')).not.toBeInTheDocument();
    // Y sí se ve lo que el operador contó.
    expect(screen.getByRole('button', { name: '$290' })).toBeInTheDocument();
  });

  test('el cajón se cuenta desde un botón, no se teclea', () => {
    pintaCierre(null);
    expect(screen.getByText('Contar efectivo')).toBeInTheDocument();
    expect(screen.getByText('Cajón')).toBeInTheDocument();
  });

  test('con el conteo hecho, el botón muestra la cifra contada', () => {
    pintaCierre({ counts: [], total: 340 });
    expect(screen.getByRole('button', { name: '$340' })).toBeInTheDocument();
  });

  test('una caja sin efectivo no pinta renglón de cajón', () => {
    pintaCierre(null, null);
    expect(screen.queryByText('Cajón')).not.toBeInTheDocument();
    expect(screen.queryByText('Contar efectivo')).not.toBeInTheDocument();
  });
});

// EL DESGLOSE, QUE ES LA RAZÓN DE GUARDARLO (US3).
describe('el desglose de lo contado', () => {
  const conPiezas: ConteosDelTurno = {
    apertura: null,
    cierre: {
      total: '290', manualReason: null,
      lines: [
        { value: '100.00', isCoin: false, pieces: 2, subtotal: '200.00' },
        { value: '20.00', isCoin: false, pieces: 4, subtotal: '80.00' },
        { value: '10.00', isCoin: true, pieces: 1, subtotal: '10.00' },
      ],
    },
  };

  test('un corte contado muestra piezas y subtotal por denominación', async () => {
    wrap(<DesgloseDelConteo counts={conPiezas} currency="MXN" />);
    await userEvent.click(screen.getByText('Efectivo contado'));
    expect(screen.getByText('Al cerrar')).toBeInTheDocument();
    expect(screen.getByText('$100')).toBeInTheDocument();
    expect(screen.getByText('$80')).toBeInTheDocument();
  });

  test('la moneda de menos de un peso se nombra en centavos', async () => {
    const cincuenta: ConteosDelTurno = {
      apertura: { total: '3.5', manualReason: null, lines: [{ value: '0.50', isCoin: true, pieces: 7, subtotal: '3.50' }] },
      cierre: null,
    };
    wrap(<DesgloseDelConteo counts={cincuenta} currency="MXN" />);
    await userEvent.click(screen.getByText('Efectivo contado'));
    expect(screen.getByText('50¢')).toBeInTheDocument();
  });

  test('un arqueo capturado a mano muestra el motivo en vez de piezas', async () => {
    const aMano: ConteosDelTurno = {
      apertura: null,
      cierre: { total: '340', manualReason: 'había un billete que no está en la lista', lines: [] },
    };
    wrap(<DesgloseDelConteo counts={aMano} currency="MXN" />);
    await userEvent.click(screen.getByText('Efectivo contado'));
    expect(screen.getByText(/había un billete que no está en la lista/)).toBeInTheDocument();
    expect(screen.queryByText('Denominación')).not.toBeInTheDocument();
  });

  // El caso de TODOS los cortes que ya existen: no se pinta nada, y menos un aviso que hable del
  // sistema. Que el corte no tenga desglose no es algo que el operador pueda accionar.
  test('un corte anterior a la funcionalidad no pinta nada', () => {
    wrap(<DesgloseDelConteo counts={null} currency="MXN" />);
    // Ni la sección, ni un aviso de que no hay desglose: es una historia del sistema que quien
    // audita no puede accionar.
    expect(screen.queryByText('Efectivo contado')).not.toBeInTheDocument();
  });

  test('un corte con los dos momentos en null tampoco', () => {
    wrap(<DesgloseDelConteo counts={{ apertura: null, cierre: null }} currency="MXN" />);
    expect(screen.queryByText('Efectivo contado')).not.toBeInTheDocument();
  });
});


// EL CORTE CERRADO TIENE QUE SEGUIR MOSTRANDO SU FALTANTE (T016).
//
// Desde la spec 015 los métodos que comparten el cajón guardan su declarado igual a su esperado, así
// que la tabla por método reporta CERO en todos. Si el arqueo del cajón no se pinta, un corte con
// faltante se lee cuadrado justo en la pantalla donde alguien audita.
describe('el arqueo en el corte cerrado', () => {
  test('un corte con faltante lo muestra, aunque la tabla por método esté en ceros', () => {
    wrap(<ArqueoDelCorte drawer={{
      expected: '340', counted: '290', difference: '-50', methodIds: [1, 8], requiresCount: false,
    }} currency="MXN" />);
    expect(screen.getByLabelText('Esperado del cajón')).toHaveTextContent('$340');
    expect(screen.getByLabelText('Contado del cajón')).toHaveTextContent('$290');
    expect(screen.getByLabelText('Diferencia del arqueo del corte')).toHaveTextContent('-$50');
  });

  test('un corte anterior a la feature no pinta arqueo ni inventa un cero', () => {
    wrap(<ArqueoDelCorte drawer={null} currency="MXN" />);
    expect(screen.queryByText('Arqueo del cajón')).not.toBeInTheDocument();
  });

  test('un turno abierto todavía no tiene qué mostrar', () => {
    wrap(<ArqueoDelCorte drawer={{
      expected: '340', counted: null, difference: null, methodIds: [1], requiresCount: true,
    }} currency="MXN" />);
    expect(screen.queryByText('Arqueo del cajón')).not.toBeInTheDocument();
  });
});
