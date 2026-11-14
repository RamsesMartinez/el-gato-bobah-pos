import { useState } from 'react';
import { Box, Text, Button, VStack, HStack, Table, Input, Badge } from '@chakra-ui/react';
import { LuArrowDownLeft, LuArrowUpRight } from 'react-icons/lu';
import { medirAccion } from '../../api/uso';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashSession, type CashRegister, type CashMovement, type CashExpenseLine } from '../../api/backoffice';
import { Picker } from '../../components/Picker';
import { montoDeCaja } from './montosDeCaja';
import { hhmm } from './piezasDeCaja';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DEFAULT_TIMEZONE } from '../../utils/zonaPorDefecto';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger } from '../../components/ui/dialog';
import { montoTecleado } from '../../domain/numeros';
import { mensajeDeError } from '../../api/mensajes';


// Tipo del movimiento para la columna "Tipo": traspaso (azul) o entrada/salida (verde/rojo).
function movementType(m: CashMovement): { label: string; palette: string } {
  if (m.transferId !== null) return { label: 'Traspaso', palette: 'blue' };
  if (m.isRefund) return { label: 'Devolución', palette: 'orange' };
  if (m.kind === 'propina') return { label: 'Propina', palette: 'purple' };
  if (m.kind === 'reverso') return { label: 'Corrección', palette: 'gray' };
  // El retiro al cierre (spec 035) no es una salida del negocio: se nombra aparte.
  if (m.kind === 'retiro') return { label: 'Retiro', palette: 'teal' };
  return m.kind === 'entrada' ? { label: 'Entrada', palette: 'green' } : { label: 'Salida', palette: 'red' };
}

// Solo una salida capturada a mano y sin corregir; el servidor aplica la misma regla.
function esCorregible(m: CashMovement): boolean {
  return m.kind === 'salida' && !m.reversed && m.transferId === null && m.expenseId === null && !m.isRefund;
}

// ---- Tablas del resumen (compartidas entre caja en vivo, histórico y resumen post-cierre) ----

// Movimientos de efectivo en tabla: Hora · Tipo · Concepto · Usuario · Monto. Excluye las salidas
// de gastos (van en su propia sección) para no contarlas dos veces.
// La zona llega como PROP y no del hook: esto es una tabla de presentación, y que pidiera los
// ajustes por su cuenta la vuelve imposible de pintar sin montar media aplicación alrededor. Quien
// la usa ya tiene la zona a la mano.
export function MovementsTable({ movements, currency, zona = DEFAULT_TIMEZONE, onCorregir }: {
  movements: CashMovement[];
  currency: string;
  zona?: string;
  // Solo en el turno abierto: la salida a mano sin corregir ofrece «Corregir» (spec 032, punto 4).
  onCorregir?: (m: CashMovement) => void;
}) {
  const rows = (movements ?? []).filter((m) => m.expenseId === null);
  if (rows.length === 0) return <Text fontSize="sm" color="fg.muted">Sin movimientos de efectivo.</Text>;
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Hora</Table.ColumnHeader>
          <Table.ColumnHeader>Tipo</Table.ColumnHeader>
          <Table.ColumnHeader>Concepto</Table.ColumnHeader>
          <Table.ColumnHeader>Usuario</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Monto</Table.ColumnHeader>
          {onCorregir && <Table.ColumnHeader />}
        </Table.Row></Table.Header>
        <Table.Body>
          {rows.map((m) => {
            const t = movementType(m);
            return (
              <Table.Row key={m.id}>
                <Table.Cell whiteSpace="nowrap" color="fg.muted">{hhmm(m.createdAt, zona)}</Table.Cell>
                <Table.Cell><Badge colorPalette={t.palette}>{t.label}</Badge></Table.Cell>
                {/* Sin ancho fijo: a 220 px «Devolución: Producto en mal estado» se cortaba con la
                    tabla a medio llenar, y el motivo es justo lo que se vino a leer. */}
                <Table.Cell><Text lineClamp={2}>{m.recipientName ? `${m.concept} · ${m.recipientName}` : m.concept}</Text></Table.Cell>
                <Table.Cell color="fg.muted" whiteSpace="nowrap">{m.userName}</Table.Cell>
                <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap"
                  color={m.kind === 'entrada' || m.kind === 'reverso' ? 'fg.success' : 'fg.error'}
                  textDecoration={m.reversed ? 'line-through' : undefined}>
                  {m.kind === 'entrada' || m.kind === 'reverso' ? '+' : '−'}{montoDeCaja(m.amount, currency)}
                </Table.Cell>
                {onCorregir && (
                  <Table.Cell textAlign="end">
                    {esCorregible(m) && (
                      <Button size="sm" minH="44px" variant="outline" onClick={() => onCorregir(m)}>Corregir</Button>
                    )}
                  </Table.Cell>
                )}
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}


// Gastos del corte en tabla + total. No renderiza nada si no hay gastos (ahorra espacio).
export function ExpensesTable({ expenses, currency }: { expenses: CashExpenseLine[]; currency: string }) {
  if (!expenses?.length) return null;
  const total = expenses.reduce((s, e) => s + (Number(e.amount) || 0), 0);
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Categoría</Table.ColumnHeader>
          <Table.ColumnHeader>Proveedor</Table.ColumnHeader>
          <Table.ColumnHeader>Método</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Monto</Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {expenses.map((e) => (
            <Table.Row key={e.id}>
              <Table.Cell>{e.category}</Table.Cell>
              <Table.Cell color="fg.muted">{e.supplier ?? '—'}</Table.Cell>
              <Table.Cell color="fg.muted">{e.paymentMethod ?? '—'}</Table.Cell>
              <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap">
                {montoDeCaja(e.amount, currency)}
                {/* Salió del cajón con este gasto, pero no es del negocio (spec 050): no suma al total. */}
                {Number(e.drawerExtra) > 0 && (
                  <Text fontSize="xs" fontWeight="400" color="fg.muted">+ {montoDeCaja(e.drawerExtra!, currency)} no es del negocio</Text>
                )}
              </Table.Cell>
            </Table.Row>
          ))}
          <Table.Row fontWeight="700">
            <Table.Cell colSpan={3}>Total gastos</Table.Cell>
            <Table.Cell textAlign="end" whiteSpace="nowrap">{montoDeCaja(total, currency)}</Table.Cell>
          </Table.Row>
        </Table.Body>
      </Table.Root>
    </Box>
  );
}

// ---- Movimientos de efectivo (entrada/salida) de la sesión abierta ----
export function MovementsPanel({ session }: { session: CashSession }) {
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const [kind, setKind] = useState<'entrada' | 'salida'>('salida');
  const [amount, setAmount] = useState('');
  const [concept, setConcept] = useState('');
  const [conceptId, setConceptId] = useState('');
  const [corrigiendo, setCorrigiendo] = useState<CashMovement | null>(null);
  const { data: conceptos } = useQuery({ queryKey: ['cash', 'concepts'], queryFn: () => backofficeApi.cashConcepts() });
  const opciones = (conceptos?.items ?? []).map((c) => ({ value: String(c.id), label: c.name }));
  const crearConcepto = async (name: string) => {
    const c = await backofficeApi.createCashConcept(name);
    qc.invalidateQueries({ queryKey: ['cash', 'concepts'] });
    return { value: String(c.id), label: c.name };
  };

  const mut = useMutation({
    mutationFn: () => {
      const monto = montoTecleado(amount) ?? 0;
      return backofficeApi.cashMovement(session.registerId, kind === 'salida'
        ? { kind: 'salida', amount: monto, conceptId: Number(conceptId) }
        : { kind: 'entrada', amount: monto, concept: concept.trim() });
    },
    onSuccess: () => {
      medirAccion('caja', 'traspaso');
      setAmount(''); setConcept(''); setConceptId(''); qc.invalidateQueries({ queryKey: ['cash'] });
    },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: mensajeDeError(e), type: 'error' }),
  });
  const correctMut = useMutation({
    mutationFn: (p: { id: number; amount: number; conceptId: number }) =>
      backofficeApi.correctCashOut(p.id, { amount: p.amount, conceptId: p.conceptId }),
    onSuccess: () => { setCorrigiendo(null); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo corregir', description: mensajeDeError(e), type: 'error' }),
  });
  const canAdd = (montoTecleado(amount) ?? 0) > 0 && (kind === 'salida' ? conceptId !== '' : concept.trim().length > 0);
  // Go serializa un slice vacío como null; sin esta guarda, `.length`/`.map` revienta el render.
  const movements = session.movements ?? [];

  return (
    <Box>
      <Text fontWeight="700" mb={2}>Movimientos de efectivo</Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={3} mb={3}>
        <HStack gap={2} flexWrap="wrap">
          <Button size="sm" minH="44px" variant={kind === 'entrada' ? 'solid' : 'outline'}
            colorPalette={kind === 'entrada' ? 'green' : 'gray'} onClick={() => setKind('entrada')}>
            <LuArrowDownLeft /> Entrada
          </Button>
          <Button size="sm" minH="44px" variant={kind === 'salida' ? 'solid' : 'outline'}
            colorPalette={kind === 'salida' ? 'red' : 'gray'} onClick={() => setKind('salida')}>
            <LuArrowUpRight /> Salida
          </Button>
          <Input size="sm" minH="44px" w="120px" type="number" inputMode="decimal" placeholder="Monto"
            value={amount} onChange={(e) => setAmount(e.target.value)} />
          {kind === 'salida' ? (
            <Box flex="1" minW="180px">
              {/* De la lista, y si no está se agrega ahí mismo (punto 3). */}
              <Picker value={conceptId} onChange={setConceptId} options={opciones} onCreate={crearConcepto}
                placeholder="Concepto de la salida" title="Concepto de la salida" />
            </Box>
          ) : (
            <Input size="sm" minH="44px" flex="1" minW="140px" placeholder="Concepto (ej. cambio)"
              value={concept} onChange={(e) => setConcept(e.target.value)} />
          )}
          <Button size="sm" minH="44px" disabled={!canAdd} loading={mut.isPending} onClick={() => mut.mutate()}>
            Registrar
          </Button>
        </HStack>
      </Box>
      <MovementsTable movements={movements} currency={session.currency} zona={horaNegocio.zona}
        onCorregir={setCorrigiendo} />
      <CorregirSalida movimiento={corrigiendo} opciones={opciones} onCrear={crearConcepto} guardando={correctMut.isPending}
        onCancelar={() => setCorrigiendo(null)}
        onCorregir={(monto, cid) => { if (corrigiendo) correctMut.mutate({ id: corrigiendo.id, amount: monto, conceptId: cid }); }} />
    </Box>
  );
}

// CorregirSalida: la salida bien capturada que reemplaza a la original. La original no cambia:
// se le crea su reverso y queda a la vista, tachada (punto 4).
function CorregirSalida({ movimiento, opciones, onCrear, guardando, onCancelar, onCorregir }: {
  movimiento: CashMovement | null;
  opciones: { value: string; label: string }[];
  onCrear: (name: string) => Promise<{ value: string; label: string }>;
  guardando: boolean;
  onCancelar: () => void;
  onCorregir: (monto: number, conceptId: number) => void;
}) {
  const [monto, setMonto] = useState('');
  const [cid, setCid] = useState('');
  const salir = () => { setMonto(''); setCid(''); onCancelar(); };
  const valido = (montoTecleado(monto) ?? 0) > 0 && cid !== '';
  return (
    <DialogRoot open={movimiento !== null} onOpenChange={(e) => { if (!e.open && !guardando) salir(); }} placement="center" size="md">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Corregir salida</DialogTitle></DialogHeader>
        <DialogBody>
          <VStack align="stretch" gap={3}>
            <Text fontSize="sm" color="fg.muted">
              {movimiento ? `${movimiento.concept} · ${montoDeCaja(movimiento.amount, 'MXN')}` : ''} queda anulada y se registra esta en su lugar.
            </Text>
            <Input minH="48px" inputMode="decimal" placeholder="Monto correcto"
              value={monto} onChange={(e) => setMonto(e.target.value)} />
            <Picker value={cid} onChange={setCid} options={opciones} onCreate={onCrear}
              placeholder="Concepto correcto" title="Concepto de la salida" />
            <HStack gap={6} pt={2}>
              <Button flex="1" minH="52px" variant="outline" disabled={guardando} onClick={salir}>Volver</Button>
              <Button flex="1" minH="52px" colorPalette="orange" disabled={!valido} loading={guardando}
                onClick={() => onCorregir(montoTecleado(monto) ?? 0, Number(cid))}>Corregir</Button>
            </HStack>
          </VStack>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

// ---- Traspaso de efectivo entre dos cajas abiertas ----
export function TransferDialog({ open, onClose, from, openRegisters, onDone }: {
  open: boolean; onClose: () => void; from: CashRegister; openRegisters: CashRegister[]; onDone: () => void;
}) {
  const [toId, setToId] = useState('');
  const [amount, setAmount] = useState('');
  const [note, setNote] = useState('');
  const destinations = openRegisters.filter((r) => r.id !== from.id);

  const reset = () => { setToId(''); setAmount(''); setNote(''); };
  const mut = useMutation({
    mutationFn: () => backofficeApi.cashTransfer(from.id, Number(toId), montoTecleado(amount) ?? 0, note || undefined),
    onSuccess: () => { reset(); onDone(); },
    onError: (e) => toaster.create({ title: 'No se pudo traspasar', description: String(e), type: 'error' }),
  });
  const canSend = !!toId && (montoTecleado(amount) ?? 0) > 0;

  return (
    <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }} placement="center" size="sm">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Traspaso desde «{from.name}»</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <VStack align="stretch" gap={3}>
            <Text fontSize="sm" color="fg.muted">
              Mueve efectivo a otra caja abierta: sale de «{from.name}» y entra en la caja destino automáticamente.
            </Text>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Caja destino</Text>
              <Picker value={toId} onChange={setToId} placeholder="Elegir caja destino" title="Caja destino"
                options={destinations.map((r) => ({ value: String(r.id), label: r.name }))} />
            </Box>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Monto</Text>
              <Input type="number" inputMode="decimal" placeholder="0" value={amount} onChange={(e) => setAmount(e.target.value)} />
            </Box>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Nota (opcional)</Text>
              <Input placeholder="Motivo del traspaso" value={note} onChange={(e) => setNote(e.target.value)} />
            </Box>
            <Button colorPalette="blue" disabled={!canSend} loading={mut.isPending} onClick={() => mut.mutate()}>
              Confirmar traspaso
            </Button>
          </VStack>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
