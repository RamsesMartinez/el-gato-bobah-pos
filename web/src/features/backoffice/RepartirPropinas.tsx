import { useState } from 'react';
import { Box, Button, HStack, Input, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import type { TipPayoutInput, TipsDecision, TipsPending } from '../../api/backoffice';
import { money } from '../../utils/format';
import { repartoParejo, validarAjustado } from './propinas';

// RepartirPropinas: la hoja de «Entregar propina», opción C de las maquetas (decidida el
// 2026-10-09). Se elige a una o varias personas y se reparte parejo o ajustado, siempre en pesos
// enteros. Un movimiento por persona lo arma el servidor.
export function RepartirPropinas({ isOpen, pendiente, currency, guardando, onEntregar, onClose }: {
  isOpen: boolean;
  pendiente: TipsPending | null;
  currency: string;
  guardando: boolean;
  onEntregar: (input: TipPayoutInput) => void;
  onClose: () => void;
}) {
  const [elegidas, setElegidas] = useState<number[]>([]);
  const [ajustado, setAjustado] = useState(false);
  const [montos, setMontos] = useState<Record<number, string>>({});
  const total = Number(pendiente?.total ?? 0);
  const personas = pendiente?.people ?? [];
  const nombre = (id: number) => personas.find((p) => p.id === id)?.name ?? '';

  // Se limpia al salir: la hoja queda montada entre aperturas.
  const salir = () => { setElegidas([]); setAjustado(false); setMontos({}); onClose(); };
  const alternar = (id: number) => {
    setElegidas((xs) => (xs.includes(id) ? xs.filter((x) => x !== id) : [...xs, id]));
    // Quien se agrega ya ajustando arranca vacío: el parejo cambió y no se adivina su parte.
  };

  const parejo = repartoParejo(total, elegidas.length);
  // Ajustar arranca del reparto parejo: lo común es mover un monto, no teclearlos todos.
  const precargar = () => {
    const m: Record<number, string> = {};
    for (const id of elegidas) m[id] = montos[id] ?? (parejo ? String(parejo.cada) : '');
    return m;
  };
  const errorAjustado = ajustado && elegidas.length > 0
    ? validarAjustado(total, elegidas.map((id) => montos[id] ?? '')) : null;

  let etiqueta = 'Elige a quién';
  let listo = false;
  if (elegidas.length > 0 && !ajustado) {
    if (parejo === null) {
      etiqueta = 'No alcanza un peso para cada persona';
    } else {
      listo = true;
      etiqueta = elegidas.length === 1
        ? `Entregar ${money(parejo.cada, currency)} a ${nombre(elegidas[0])}`
        : `Entregar ${money(parejo.cada, currency)} a cada uno (${elegidas.length})`;
    }
  } else if (elegidas.length > 0) {
    listo = errorAjustado === null;
    etiqueta = `Entregar a ${elegidas.length} ${elegidas.length === 1 ? 'persona' : 'personas'}`;
  }

  const entregar = () => {
    onEntregar(ajustado
      ? { mode: 'ajustado', recipients: elegidas.map((id) => ({ userId: id, amount: Number(montos[id]) })) }
      : { mode: 'parejo', recipients: elegidas.map((id) => ({ userId: id })) });
  };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open && !guardando) salir(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="92dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <HStack justify="space-between">
            <Text fontSize="lg" fontWeight="700">Repartir propinas</Text>
            <Text fontSize="lg" fontWeight="700">{money(total, currency)}</Text>
          </HStack>
          <Text fontSize="sm" color="fg.muted">Toca a una o varias personas.</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          <SimpleGrid columns={4} gap={2}>
            {personas.map((p) => {
              const sel = elegidas.includes(p.id);
              return (
                <Button key={p.id} minH="56px" variant={sel ? 'solid' : 'outline'}
                  colorPalette={sel ? 'orange' : 'gray'} aria-pressed={sel} onClick={() => alternar(p.id)}>
                  {p.name}
                </Button>
              );
            })}
          </SimpleGrid>

          <HStack mt={3} gap={2}>
            <Button minH="48px" flex="1" variant={!ajustado ? 'solid' : 'outline'} colorPalette="gray"
              onClick={() => setAjustado(false)}>Parejo</Button>
            <Button minH="48px" flex="1" variant={ajustado ? 'solid' : 'outline'} colorPalette="gray"
              onClick={() => { setMontos(precargar()); setAjustado(true); }}>Ajustar montos</Button>
          </HStack>

          {ajustado && elegidas.length > 0 && (
            <VStack align="stretch" gap={2} mt={3}>
              {elegidas.map((id) => (
                <HStack key={id} justify="space-between">
                  <Text fontSize="md" fontWeight="600">{nombre(id)}</Text>
                  <Input aria-label={`Monto para ${nombre(id)}`} inputMode="numeric" maxW="160px" minH="48px"
                    value={montos[id] ?? ''} placeholder="$"
                    onChange={(e) => setMontos((m) => ({ ...m, [id]: e.target.value }))} />
                </HStack>
              ))}
              {errorAjustado && <Text color="red.600" fontSize="sm">{errorAjustado}</Text>}
            </VStack>
          )}

          {!ajustado && parejo && parejo.sobrante > 0 && (
            <Box mt={3}>
              <Text fontSize="sm" color="fg.muted">
                Quedan {money(parejo.sobrante, currency)} para el siguiente reparto.
              </Text>
            </Box>
          )}
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack w="100%" gap={6}>
            <Button flex="1" minH="52px" variant="outline" colorPalette="gray" disabled={guardando} onClick={salir}>
              Cancelar
            </Button>
            <Button flex="2" minH="52px" colorPalette="orange" disabled={!listo} loading={guardando} onClick={entregar}>
              {etiqueta}
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

// PropinasDelCierre: la pregunta del cierre cuando queda propina. Dos botones grandes en el mismo
// paso del cierre, ninguno elegido de antemano.
export function PropinasDelCierre({ pendiente, currency, decision, onEntregarAhora, onDecidir }: {
  pendiente: TipsPending | null | undefined;
  currency: string;
  decision: TipsDecision | null;
  onEntregarAhora: () => void;
  onDecidir: (d: TipsDecision | null) => void;
}) {
  const total = Number(pendiente?.total ?? 0);
  if (total < 1) return null;
  const seQueda = decision === 'quedan_en_caja';
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
      <Text fontWeight="700">Quedan {money(total, currency)} de propina por entregar</Text>
      <HStack mt={2} gap={2}>
        <Button flex="1" minH="52px" colorPalette="orange" onClick={onEntregarAhora}>Entregar ahora</Button>
        <Button flex="1" minH="52px" variant={seQueda ? 'solid' : 'outline'} colorPalette="gray"
          aria-pressed={seQueda} onClick={() => onDecidir(seQueda ? null : 'quedan_en_caja')}>
          Se queda en caja
        </Button>
      </HStack>
    </Box>
  );
}
