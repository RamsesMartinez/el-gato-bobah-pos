import { useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack, Wrap } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashConcept } from '../../api/backoffice';
import { Picker } from '../../components/Picker';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';

const TAP = '44px';

function Bloque({ titulo, children }: { titulo: string; children: React.ReactNode }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" p={4} bg="bg.panel">
      <Text fontWeight="700" mb={3}>{titulo}</Text>
      {children}
    </Box>
  );
}

// AjustesDeCaja: lo que se configura de caja y tarjeta (spec 032): conceptos de salida, terminales
// por sucursal, cómo se arquea la tarjeta y a quién le llega el resumen diario.
export function AjustesDeCaja() {
  const qc = useQueryClient();
  const avisar = (titulo: string) => (e: unknown) => toaster.create({ title: titulo, description: mensajeDeError(e), type: 'error' });
  const recargar = (k: string) => () => qc.invalidateQueries({ queryKey: [k] });

  // ---- Conceptos ----
  const { data: conceptos } = useQuery({ queryKey: ['cash', 'concepts'], queryFn: () => backofficeApi.cashConcepts() });
  const [juntando, setJuntando] = useState<CashConcept | null>(null);
  const [destino, setDestino] = useState('');
  const [renombrando, setRenombrando] = useState<{ id: number; name: string } | null>(null);
  // Archivar pide confirmación: el concepto deja de ofrecerse en la caja (revisión de tableta).
  const [archivando, setArchivando] = useState<CashConcept | null>(null);
  const archivar = useMutation({
    mutationFn: (id: number) => backofficeApi.updateCashConcept(id, { archived: true }),
    onSuccess: recargar('cash'), onError: avisar('No se pudo archivar'),
  });
  const renombrar = useMutation({
    mutationFn: (c: { id: number; name: string; original: CashConcept }) =>
      backofficeApi.updateCashConcept(c.id, { name: c.name, categoryId: c.original.categoryId, supplierId: c.original.supplierId }),
    onSuccess: () => { setRenombrando(null); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: avisar('No se pudo renombrar'),
  });
  const juntar = useMutation({
    mutationFn: (p: { from: number; into: number }) => backofficeApi.mergeCashConcept(p.from, p.into),
    onSuccess: () => { setJuntando(null); setDestino(''); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: avisar('No se pudieron juntar'),
  });

  // ---- Terminales y modo ----
  const { data: terminales } = useQuery({ queryKey: ['card-terminals'], queryFn: () => backofficeApi.cardTerminals() });
  const { data: modos } = useQuery({ queryKey: ['card-count-modes'], queryFn: () => backofficeApi.cardCountModes() });
  const [nuevas, setNuevas] = useState<Record<number, string>>({});
  const crearTerminal = useMutation({
    mutationFn: (p: { branchId: number; name: string }) => backofficeApi.createCardTerminal(p.branchId, p.name),
    onSuccess: (_r, p) => { setNuevas((n) => ({ ...n, [p.branchId]: '' })); qc.invalidateQueries({ queryKey: ['card-terminals'] }); },
    onError: avisar('No se pudo agregar la terminal'),
  });
  const archivarTerminal = useMutation({
    mutationFn: (id: number) => backofficeApi.updateCardTerminal(id, { archived: true }),
    onSuccess: recargar('card-terminals'), onError: avisar('No se pudo archivar'),
  });
  const cambiarModo = useMutation({
    mutationFn: (p: { branchId: number; mode: 'auto' | 'per_terminal' }) => backofficeApi.setCardCountMode(p.branchId, p.mode),
    onSuccess: recargar('card-count-modes'), onError: avisar('No se pudo cambiar'),
  });

  // ---- Correos ----
  const { data: correosGuardados } = useQuery({ queryKey: ['summary-emails'], queryFn: () => backofficeApi.summaryEmails() });
  const [correos, setCorreos] = useState<string[] | null>(null);
  const [nuevoCorreo, setNuevoCorreo] = useState('');
  const lista = correos ?? correosGuardados?.emails ?? [];
  const guardarCorreos = useMutation({
    mutationFn: () => backofficeApi.setSummaryEmails(lista),
    onSuccess: () => { setCorreos(null); qc.invalidateQueries({ queryKey: ['summary-emails'] }); toaster.create({ title: 'Correos guardados', type: 'success' }); },
    onError: avisar('No se pudieron guardar los correos'),
  });

  const sucursales = modos?.items ?? [];
  const opcionesDestino = (conceptos?.items ?? []).filter((c) => c.id !== juntando?.id).map((c) => ({ value: String(c.id), label: c.name }));

  return (
    <VStack align="stretch" gap={4}>
      <Bloque titulo="Conceptos de salida de caja">
        <VStack align="stretch" gap={2}>
          {(conceptos?.items ?? []).map((c) => (
            <HStack key={c.id} data-concepto justify="space-between" flexWrap="wrap" gap={2}>
              {renombrando?.id === c.id ? (
                <HStack flex="1">
                  <Input minH={TAP} aria-label={`Nuevo nombre de ${c.name}`} value={renombrando.name}
                    onChange={(e) => setRenombrando({ id: c.id, name: e.target.value })} />
                  <Button minH={TAP} onClick={() => renombrar.mutate({ id: c.id, name: renombrando.name, original: c })}>Guardar</Button>
                </HStack>
              ) : (
                <Text flex="1">{c.name}</Text>
              )}
              <HStack gap={2}>
                <Button minH={TAP} variant="outline" onClick={() => setRenombrando({ id: c.id, name: c.name })}>Renombrar</Button>
                <Button minH={TAP} variant="outline" onClick={() => setJuntando(c)}>Juntar con…</Button>
                {/* Archivar aparte: no se confunde con juntar, que mueve salidas. */}
                <Box w={4} />
                <Button minH={TAP} variant="ghost" colorPalette="red" onClick={() => setArchivando(c)}>Archivar</Button>
              </HStack>
            </HStack>
          ))}
        </VStack>
        {juntando && (
          <Box mt={3} borderTopWidth="1px" pt={3}>
            <Text fontSize="sm" mb={2}>Las salidas de «{juntando.name}» pasan a:</Text>
            <Picker value={destino} onChange={setDestino} options={opcionesDestino} placeholder="Elegir concepto" title="Juntar con" />
            <HStack mt={2} gap={6}>
              <Button flex="1" minH="52px" variant="outline" onClick={() => { setJuntando(null); setDestino(''); }}>Volver</Button>
              <Button flex="1" minH="52px" colorPalette="orange" disabled={destino === ''} loading={juntar.isPending}
                onClick={() => juntar.mutate({ from: juntando.id, into: Number(destino) })}>Juntar</Button>
            </HStack>
          </Box>
        )}
      </Bloque>

      <ConfirmSheet isOpen={archivando !== null} destructive title={`¿Archivar «${archivando?.name ?? ''}»?`}
        description="Deja de ofrecerse al registrar salidas. Las salidas que ya lo usan no cambian."
        confirmLabel="Archivar" onCancel={() => setArchivando(null)}
        onConfirm={() => { if (archivando) archivar.mutate(archivando.id); setArchivando(null); }} />

      <Bloque titulo="Terminales de tarjeta">
        <VStack align="stretch" gap={4}>
          {sucursales.map((b) => (
            <Box key={b.branchId}>
              <HStack justify="space-between" flexWrap="wrap" gap={2} mb={2}>
                <Text fontWeight="600">{b.name}</Text>
                <HStack gap={0} role="group" aria-label={`Arqueo de tarjeta en ${b.name}`}>
                  {(['auto', 'per_terminal'] as const).map((m) => (
                    <Button key={m} minH={TAP} borderRadius={0} variant={b.mode === m ? 'solid' : 'outline'}
                      aria-pressed={b.mode === m} onClick={() => cambiarModo.mutate({ branchId: b.branchId, mode: m })}>
                      {m === 'auto' ? 'Automático' : 'Por terminal'}
                    </Button>
                  ))}
                </HStack>
              </HStack>
              <Text fontSize="sm" color="fg.muted" mb={2}>
                {b.mode === 'per_terminal'
                  ? 'Al cerrar caja se escribe el total del corte de cada terminal.'
                  : 'Al cerrar caja la tarjeta se da por cuadrada.'}
              </Text>
              <VStack align="stretch" gap={2}>
                {(terminales?.items ?? []).filter((t) => t.branchId === b.branchId && !t.archived).map((t) => (
                  <HStack key={t.id} justify="space-between">
                    <Text>{t.name}</Text>
                    <Button minH={TAP} variant="ghost" colorPalette="red" onClick={() => archivarTerminal.mutate(t.id)}>Archivar</Button>
                  </HStack>
                ))}
                <HStack>
                  <Input minH={TAP} aria-label={`Nueva terminal en ${b.name}`} placeholder="Nombre (p. ej. Getnet)" maxLength={40}
                    value={nuevas[b.branchId] ?? ''} onChange={(e) => setNuevas((n) => ({ ...n, [b.branchId]: e.target.value }))} />
                  <Button minH={TAP} disabled={(nuevas[b.branchId] ?? '').trim() === ''}
                    onClick={() => crearTerminal.mutate({ branchId: b.branchId, name: (nuevas[b.branchId] ?? '').trim() })}>
                    Agregar terminal
                  </Button>
                </HStack>
              </VStack>
            </Box>
          ))}
        </VStack>
      </Bloque>

      <Bloque titulo="Resumen diario del cierre por correo">
        <Wrap gap={2} mb={2}>
          {lista.map((c) => (
            <Button key={c} minH={TAP} variant="outline" aria-label={`Quitar ${c}`}
              onClick={() => setCorreos(lista.filter((x) => x !== c))}>{c} ×</Button>
          ))}
          {lista.length === 0 && <Text fontSize="sm" color="fg.muted">Sin correos: no se manda el resumen.</Text>}
        </Wrap>
        <HStack>
          <Input minH={TAP} type="email" aria-label="Correo nuevo" placeholder="correo@ejemplo.com"
            value={nuevoCorreo} onChange={(e) => setNuevoCorreo(e.target.value)} />
          <Button minH={TAP} disabled={nuevoCorreo.trim() === '' || lista.length >= 10}
            onClick={() => { setCorreos([...lista, nuevoCorreo.trim()]); setNuevoCorreo(''); }}>Agregar correo</Button>
        </HStack>
        <Button mt={3} minH="52px" colorPalette="orange" disabled={correos === null} loading={guardarCorreos.isPending}
          onClick={() => guardarCorreos.mutate()}>Guardar correos</Button>
      </Bloque>
    </VStack>
  );
}
