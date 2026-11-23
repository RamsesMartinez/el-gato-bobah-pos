import { useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { pressKey } from '../../../components/pressKey';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { toaster } from '../../../components/ui/toaster';
import { backofficeApi } from '../../../api/backoffice';
import { formatBaseQty } from '../../../shared/units';
import { toBase, typedUnit } from './transferUnits';
import { round2 } from '../../../domain/numeros';

// BringSheet es «Traer de Bodega» desde Existencias (spec 094): un negativo en el almacén de venta con
// mercancía en otro almacén se repone con un traspaso de una sola línea. La cantidad propuesta es lo
// que falta para llegar a cero, sin pasar de lo que hay en el origen.
export function BringSheet({ item, from, to, onClose }: {
  item: { itemType: string; id: number; name: string; unit: string; onHand: string };
  from: { warehouseId: number; name: string; onHand: string };
  to: { id: number; name: string };
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const unit = typedUnit(item.unit);
  const missing = Math.min(Math.max(0, -Number(item.onHand)), Math.max(0, Number(from.onHand)));
  const [qty, setQty] = useState(missing > 0 ? String(round2(missing / unit.factor)) : '');
  // Una clave por hoja abierta: un reintento por red no duplica el traspaso.
  const [clientKey] = useState(() => crypto.randomUUID());
  const [error, setError] = useState('');
  // La primera tecla reemplaza la cantidad propuesta, como en las demás casillas con teclado.
  const fresh = useRef(true);
  const key = (k: string) => {
    const first = fresh.current;
    fresh.current = false;
    setQty((v) => (first ? (k === 'back' ? '' : pressKey('', k, unit.decimals)) : pressKey(v, k, unit.decimals)));
  };
  const base = toBase(qty, item.unit);
  const send = useMutation({
    mutationFn: () => backofficeApi.createTransfer({
      clientKey, fromWarehouseId: from.warehouseId, toWarehouseId: to.id,
      lines: [{ itemType: item.itemType, ...(item.itemType === 'producto' ? { productId: item.id } : { ingredientId: item.id }), qty: String(base) }],
    }),
    onSuccess: () => {
      toaster.create({ title: `Listo: ${item.name} de ${from.name} a ${to.name}`, type: 'success' });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void qc.invalidateQueries({ queryKey: ['warehouses'] });
      onClose();
    },
    onError: () => setError('No se pudo traspasar. Vuelve a intentar.'),
  });

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader><Text fontWeight="700">Traer {item.name} de {from.name} a {to.name}</Text></DrawerHeader>
        <DrawerBody>
          <HStack gap={6} align="start" flexWrap="wrap">
            <VStack align="stretch" flex="1" minW="220px" gap={2}>
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
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button minH="52px" disabled={base <= 0} loading={send.isPending} onClick={() => send.mutate()}>
            Traspasar {qty || '0'} {unit.label}
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
