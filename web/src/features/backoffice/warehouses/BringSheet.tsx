import { useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { Picker } from '../../../components/Picker';
import { pressKey } from '../../../components/pressKey';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { toaster } from '../../../components/ui/toaster';
import { ApiError } from '../../../api/client';
import { backofficeApi } from '../../../api/backoffice';
import { formatBaseQty } from '../../../shared/units';
import { KEY_REUSED_MESSAGE, richestSource, toBase, typedUnit } from './transferUnits';

type Source = { warehouseId: number; name: string; onHand: string };

// BringSheet es «Traer de…» desde Existencias (spec 094): un negativo en el almacén de venta con
// mercancía en otro almacén se repone con un traspaso de una sola línea. La cantidad propuesta es lo
// que falta para llegar a cero, sin pasar de lo que hay en el origen. Con varios almacenes que tienen
// el artículo, propone el que más tiene y deja cambiarlo.
export function BringSheet({ item, sources, to, onClose }: {
  item: { itemType: string; id: number; name: string; unit: string; onHand: string };
  sources: Source[];
  to: { id: number; name: string };
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const richest = richestSource(sources);
  const [fromId, setFromId] = useState(richest.warehouseId);
  const from = sources.find((s) => s.warehouseId === fromId) ?? richest;
  // La unidad sigue a como se ve la existencia del origen, como en Traspaso: con 105 g se teclea en g.
  const unit = typedUnit(item.unit, from.onHand);
  const proposal = (src: Source) => {
    const srcUnit = typedUnit(item.unit, src.onHand);
    const missing = Math.min(Math.max(0, -Number(item.onHand)), Math.max(0, Number(src.onHand)));
    if (missing <= 0) return '';
    // Cuatro decimales, como la columna: con dos, faltar 4 g se proponía como 0 kg.
    const v = missing / srcUnit.factor;
    return String(srcUnit.decimals ? Number(v.toFixed(4)) : Math.ceil(v));
  };
  const [qty, setQty] = useState(() => proposal(from));
  // Una clave por traspaso: un reintento por red no lo duplica. Se renueva si cambia lo que se manda.
  const [clientKey, setClientKey] = useState(() => crypto.randomUUID());
  const [error, setError] = useState('');
  const [short, setShort] = useState<string | null>(null);
  // La primera tecla reemplaza la cantidad propuesta, como en las demás casillas con teclado.
  const fresh = useRef(true);
  const key = (k: string) => {
    const first = fresh.current;
    fresh.current = false;
    setQty((v) => (first ? (k === 'back' ? '' : pressKey('', k, unit.decimals)) : pressKey(v, k, unit.decimals)));
  };
  const changeFrom = (id: number) => {
    const src = sources.find((s) => s.warehouseId === id);
    if (!src) return;
    setFromId(id);
    setQty(proposal(src));
    fresh.current = true;
    setClientKey(crypto.randomUUID());
  };
  const base = toBase(qty, item.unit, from.onHand);
  const send = useMutation({
    mutationFn: (confirmNegative: boolean) => backofficeApi.createTransfer({
      clientKey, fromWarehouseId: from.warehouseId, toWarehouseId: to.id, confirmNegative: confirmNegative || undefined,
      lines: [{ itemType: item.itemType, ...(item.itemType === 'producto' ? { productId: item.id } : { ingredientId: item.id }), qty: String(base) }],
    }),
    onSuccess: () => {
      toaster.create({ title: `Listo: ${item.name} de ${from.name} a ${to.name}`, type: 'success' });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void qc.invalidateQueries({ queryKey: ['warehouses'] });
      onClose();
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'NEEDS_CONFIRM') {
        setShort(formatBaseQty(e.details?.lines?.[0]?.available ?? '0', item.unit));
        return;
      }
      setShort(null);
      if (e instanceof ApiError && e.code === 'TRANSFER_KEY_REUSED') {
        setClientKey(crypto.randomUUID());
        setError(KEY_REUSED_MESSAGE);
        return;
      }
      setError('No se pudo traspasar. Vuelve a intentar.');
    },
  });

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader><Text fontWeight="700">Traer {item.name} de {from.name} a {to.name}</Text></DrawerHeader>
        <DrawerBody>
          {short !== null ? (
            <VStack align="stretch" gap={1}>
              <Text fontWeight="700">{from.name} se quedaría en negativo</Text>
              <Text>{item.name}: hay {short} en {from.name}</Text>
              <Text fontSize="sm" color="fg.muted" mt={2}>Si de verdad hay más, el conteo lo corregirá después.</Text>
            </VStack>
          ) : (
            <HStack gap={6} align="start" flexWrap="wrap">
              <VStack align="stretch" flex="1" minW="220px" gap={2}>
                {sources.length > 1 && (
                  <>
                    <Box role="group" aria-label="De">
                      <Picker title="¿De dónde sale?" value={String(fromId)} onChange={(v) => changeFrom(Number(v))}
                        options={sources.map((s) => ({ value: String(s.warehouseId), label: `${s.name} · hay ${formatBaseQty(s.onHand, item.unit)}` }))} />
                    </Box>
                    {fromId === richest.warehouseId && <Text fontSize="sm" color="fg.muted">{richest.name} es el que más tiene.</Text>}
                  </>
                )}
                <Text color="fg.muted">En {from.name} hay {formatBaseQty(from.onHand, item.unit)}. En {to.name}: {formatBaseQty(item.onHand, item.unit)}.</Text>
                <Box borderWidth="2px" borderColor="colorPalette.solid" borderRadius="md" px={4} py={2} fontSize="2xl" fontWeight="700"
                  aria-label="Cuánto traer">
                  {qty || '0'} {unit.label}
                </Box>
                {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
              </VStack>
              <Box w="260px">
                <NumericKeypad allowDecimal={unit.decimals} onKey={key} />
              </Box>
            </HStack>
          )}
        </DrawerBody>
        <DrawerFooter>
          {short !== null ? (
            <>
              <Button minH="52px" variant="outline" onClick={() => setShort(null)}>Revisar cantidad</Button>
              <Button minH="52px" ml="auto" colorPalette="orange" loading={send.isPending} onClick={() => send.mutate(true)}>
                Traspasar de todos modos
              </Button>
            </>
          ) : (
            <>
              <Button minH="44px" variant="ghost" onClick={onClose}>Cancelar</Button>
              <Button minH="52px" colorPalette="brand" disabled={base <= 0} loading={send.isPending}
                onClick={() => { setError(''); send.mutate(false); }}>
                Traspasar {qty || '0'} {unit.label}
              </Button>
            </>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
