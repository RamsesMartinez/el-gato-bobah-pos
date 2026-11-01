import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi, expect, test, describe, beforeEach } from 'vitest';
import type { ReactElement } from 'react';
import { Provider } from '../../components/ui/provider';
import type { CashAttendanceGate } from '../../api/backoffice';
import { AperturaConAsistencia, AvisoDelCierre, CierreConAsistencia, type EstadoDeAsistencia } from './AsistenciaDeCaja';

const clock = vi.hoisted(() => vi.fn());
vi.mock('../../api/attendance', () => ({ attendanceApi: { clock: (...a: unknown[]) => clock(...a) } }));
vi.mock('../../hooks/useHoraDelNegocio', async () => {
  const real = await vi.importActual<typeof import('../../utils/horaDelNegocio')>('../../utils/horaDelNegocio');
  const z = 'America/Mexico_City';
  return { useHoraDelNegocio: () => ({
    zona: z, soloHora: (v: string) => real.soloHora(v, z), soloFecha: (v: string) => real.soloFecha(v, z),
    diaDelNegocio: (v: string | Date) => real.diaDelNegocio(v, z), fechaYHora: (v: string) => real.fechaYHora(v, z),
  }) };
});

beforeEach(() => { clock.mockReset(); });

function wrap(ui: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>{ui}</Provider></QueryClientProvider>);
}

const gate = (o: Partial<CashAttendanceGate>): CashAttendanceGate => ({
  stage: 'open', applies: true, ready: false, person: { id: 12, name: 'Ana', active: true },
  stale: false, isSelf: true, canAuthorize: false, ...o,
});

const typePin = (pin: string) => { for (const d of pin) fireEvent.click(screen.getByRole('button', { name: d })); };

describe('la tarjeta de caja cerrada', () => {
  const apertura = (estado: { gate?: CashAttendanceGate; failed: boolean }, onActualizar = vi.fn(), onContar = vi.fn()) =>
    wrap(<AperturaConAsistencia nombreCaja="Mostrador" estado={estado} abriendo={false} onContar={onContar} onActualizar={onActualizar} />);

  test('sin entrada pide marcarla y no deja contar', () => {
    apertura({ gate: gate({}), failed: false });
    expect(screen.getByText('Para abrir la caja, primero marca tu entrada.')).toBeInTheDocument();
    expect(screen.getByText('Ana, ahora no tienes una entrada marcada.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Marcar entrada' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Contar el efectivo' })).toBeDisabled();
  });

  test('con la entrada vencida lo dice', () => {
    apertura({ gate: gate({ stale: true, openSince: '2026-10-29T14:00:00Z' }), failed: false });
    expect(screen.getByText('Ana, tu entrada anterior ya venció.')).toBeInTheDocument();
  });

  test('marcar la entrada desde la tarjeta vuelve a preguntar el estado', async () => {
    clock.mockResolvedValue({ kind: 'in', at: '2026-10-30T15:02:00Z' });
    const onActualizar = vi.fn();
    apertura({ gate: gate({}), failed: false }, onActualizar);
    fireEvent.click(screen.getByRole('button', { name: 'Marcar entrada' }));
    expect(await screen.findByText('Marcar entrada · Ana')).toBeInTheDocument();
    typePin('4821');
    fireEvent.click(screen.getByRole('button', { name: 'Entrada' }));
    await waitFor(() => expect(onActualizar).toHaveBeenCalled());
    expect(clock).toHaveBeenCalledWith(12, '4821', 'in');
  });

  test('con la entrada marcada, el estado y contar habilitado', () => {
    const onContar = vi.fn();
    apertura({ gate: gate({ ready: true, openSince: '2026-10-30T15:02:00Z' }), failed: false }, vi.fn(), onContar);
    expect(screen.getByText('Entrada marcada 09:02 a.m.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Contar el efectivo' }));
    expect(onContar).toHaveBeenCalled();
    expect(screen.queryByText('Para abrir la caja, primero marca tu entrada.')).not.toBeInTheDocument();
  });

  // B24: la consulta falló (API anterior, red): la tarjeta de hoy, y que mande el servidor.
  test('si la consulta falla, la tarjeta de siempre', () => {
    apertura({ failed: true });
    expect(screen.getByText('Cuenta el efectivo con el que arranca el cajón.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Contar el efectivo' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Marcar entrada' })).not.toBeInTheDocument();
  });

  // SC-004: sin la regla, idéntica a la de hoy.
  test('sin asistencia, la tarjeta de hoy sin nada nuevo', () => {
    apertura({ gate: gate({ applies: false, ready: true }), failed: false });
    expect(screen.getByText('Cuenta el efectivo con el que arranca el cajón.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Contar el efectivo' })).toBeEnabled();
    expect(screen.queryByText(/Entrada marcada/)).not.toBeInTheDocument();
    expect(screen.queryByText(/marca tu entrada/)).not.toBeInTheDocument();
  });
});

describe('el cierre', () => {
  const BLOQUE = 'bloque del cierre (tabla, terminales, fondo, notas y «Cerrar caja»)';
  type Extra = Partial<{ autorizacion: string | null; onAutorizar: (r: string) => void; onQuitarAutorizacion: () => void; onActualizar: () => void }>;
  // Como en CashPage: el aviso arriba de la caja abierta y el bloque del cierre más abajo.
  const ui = (estado: EstadoDeAsistencia, p: Extra = {}) => (
    <>
      <AvisoDelCierre estado={estado} autorizacion={p.autorizacion ?? null} onAutorizar={p.onAutorizar ?? vi.fn()}
        onQuitarAutorizacion={p.onQuitarAutorizacion ?? vi.fn()} onActualizar={p.onActualizar ?? vi.fn()} />
      <p>Movimientos de efectivo</p>
      <CierreConAsistencia estado={estado} autorizacion={p.autorizacion ?? null}><p>{BLOQUE}</p></CierreConAsistencia>
    </>
  );
  const cierre = (estado: EstadoDeAsistencia, p: Extra = {}) => wrap(ui(estado, p));
  const dentro = (o: Partial<CashAttendanceGate> = {}) => gate({
    stage: 'close', isSelf: false, openSince: new Date(Date.now() - 2 * 3600_000).toISOString(), ...o,
  });

  test('con quien abrió dentro, el aviso va antes que el resto de la caja y el bloque del cierre no se monta', () => {
    cierre({ gate: dentro({ canAuthorize: true }), failed: false });
    const titulo = screen.getByText('Ana abrió esta caja y no ha marcado su salida');
    // Arriba: antes que los movimientos, que es lo que lo empujaba fuera de los 600 px.
    expect(titulo.compareDocumentPosition(screen.getByText('Movimientos de efectivo')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByText(/Ana sigue con la entrada abierta desde las/)).toBeInTheDocument();
    expect(screen.getByText('Para cerrar la caja, primero hay que resolverlo.')).toBeInTheDocument();
    expect(screen.queryByText(BLOQUE)).not.toBeInTheDocument();
    expect(screen.getByText('Para cerrar la caja, primero resuelve el aviso de arriba sobre Ana.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Marcar salida de Ana' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cerrar sin la salida de Ana' })).toBeInTheDocument();
  });

  test('una entrada de otro día dice el día', () => {
    cierre({ gate: dentro({ openSince: '2026-10-28T15:02:00Z' }), failed: false });
    expect(screen.getByText(/desde el .* a las 09:02 a\.m\./)).toBeInTheDocument();
  });

  test('con la entrada vencida no se ofrece el PIN', () => {
    cierre({ gate: dentro({ stale: true, canAuthorize: true }), failed: false });
    expect(screen.getByText('Su entrada es muy antigua: solo se puede cerrar con autorización.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Marcar salida de Ana' })).not.toBeInTheDocument();
  });

  // B7: dada de baja, checar rechazaría su PIN.
  test('si quien abrió ya no está activa, no se ofrece el PIN', () => {
    cierre({ gate: dentro({ person: { id: 12, name: 'Ana', active: false }, canAuthorize: true }), failed: false });
    expect(screen.getByText('Ana ya no está activa: solo se puede cerrar con autorización.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Marcar salida de Ana' })).not.toBeInTheDocument();
  });

  test('sin el permiso dice a quién pedirlo y no ofrece autorizar', () => {
    cierre({ gate: dentro({ canAuthorize: false }), failed: false });
    expect(screen.getByText('Si ya se fue, pide a alguien con permiso que cierre la caja.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cerrar sin la salida de Ana' })).not.toBeInTheDocument();
  });

  test('tras marcar la salida, el bloque vuelve y arriba queda la franja de la salida', async () => {
    clock.mockResolvedValue({ kind: 'out', at: '2026-10-31T04:04:00Z' });
    const onActualizar = vi.fn();
    const { rerender } = cierre({ gate: dentro(), failed: false }, { onActualizar });
    fireEvent.click(screen.getByRole('button', { name: 'Marcar salida de Ana' }));
    expect(await screen.findByText('Marcar salida · Ana')).toBeInTheDocument();
    typePin('4821');
    fireEvent.click(screen.getByRole('button', { name: 'Salida' }));
    await waitFor(() => expect(onActualizar).toHaveBeenCalled());
    expect(clock).toHaveBeenCalledWith(12, '4821', 'out');
    const qc = new QueryClient();
    rerender(<QueryClientProvider client={qc}><Provider>
      {ui({ gate: dentro({ ready: true, openSince: undefined }), failed: false }, { onActualizar })}
    </Provider></QueryClientProvider>);
    expect(await screen.findByText(BLOQUE)).toBeInTheDocument();
    expect(screen.getByText('Salida de Ana marcada 10:04 p.m.')).toBeInTheDocument();
  });

  test('con la autorización decidida, la franja con el motivo y el bloque', () => {
    const quitar = vi.fn();
    cierre({ gate: dentro({ canAuthorize: true }), failed: false }, { autorizacion: 'Se fue sin checar', onQuitarAutorizacion: quitar });
    expect(screen.getByText('Se cerrará sin la salida de Ana')).toBeInTheDocument();
    expect(screen.getByText(/Motivo: Se fue sin checar/)).toBeInTheDocument();
    expect(screen.getByText(BLOQUE)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Quitar autorización' }));
    expect(quitar).toHaveBeenCalled();
  });

  // B24.
  test('si la consulta falla, el cierre de siempre y sin aviso', () => {
    cierre({ failed: true });
    expect(screen.getByText(BLOQUE)).toBeInTheDocument();
    expect(screen.queryByText(/marcado su salida/)).not.toBeInTheDocument();
  });

  test('mientras se pregunta, el bloque no se monta', () => {
    cierre({ failed: false });
    expect(screen.queryByText(BLOQUE)).not.toBeInTheDocument();
  });

  // SC-004.
  test('sin asistencia, el cierre de hoy sin nada nuevo', () => {
    cierre({ gate: dentro({ applies: false, ready: true }), failed: false });
    expect(screen.getByText(BLOQUE)).toBeInTheDocument();
    expect(screen.queryByText(/marcado su salida|Salida de|aviso de arriba/)).not.toBeInTheDocument();
  });
});
