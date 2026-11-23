import { useState } from 'react';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter } from '../../../components/ui/drawer';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { pressKey } from '../../../components/pressKey';
import { Picker } from '../../../components/Picker';
import { wasteApi, type WasteResult } from '../../../api/waste';
import type { ExpiryLot } from '../../../api/backoffice';
import { formatDay } from '../../../utils/calendar';
import { formatBaseQty } from '../../../shared/units';
import { parseNumero } from '../../../domain/numeros';
import { mensajeDeError } from '../../../api/mensajes';

// WasteLotSheet manda a merma un lote desde Caducidades (spec 098, historia 4): todo el lote por
// omisión, y en lo caducado el motivo «Caducó» ya elegido, así el caso común son dos toques. En un
// lote vigente el motivo se elige: tirar algo bueno no se decide por omisión.
export function WasteLotSheet({ lot, onClose, onDone }: { lot: ExpiryLot; onClose: () => void; onDone: (r: WasteResult) => void }) {
  const reasons = useQuery({ queryKey: ['waste', 'reasons'], queryFn: wasteApi.reasons });
  const expiredReason = (reasons.data ?? []).find((r) => /caduc/i.test(r.name));
  const [reasonId, setReasonId] = useState<string>('');
  const reason = reasonId || (lot.expired && expiredReason ? String(expiredReason.id) : '');
  const [qty, setQty] = useState(String(Number(lot.qty)));
  const [fresh, setFresh] = useState(true);
  const decimals = lot.unitCode !== 'pieza';
  const n = parseNumero(qty);
  const ok = !!reason && n.estado === 'valido' && n.valor > 0 && n.valor <= Number(lot.qty);
  // La primera tecla reemplaza «todo el lote» (como el resto de las casillas con teclado).
  const key = (k: string) => {
    setFresh(false);
    setQty((v) => pressKey(fresh ? '' : v, k, decimals));
  };
  const save = useMutation({
    mutationFn: () => wasteApi.record({ reasonId: Number(reason), ingredientId: lot.ingredientId, qty,
      warehouseId: lot.warehouseId, lotExpiresOn: lot.expiresOn }),
    onSuccess: onDone,
  });
  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="90dvh">
        <DrawerHeader>
          <Text fontWeight="700" fontSize="lg">Mandar a merma · {lot.name}, lote del {formatDay(lot.expiresOn)} ({lot.warehouseName})</Text>
        </DrawerHeader>
        <DrawerBody>
          <HStack align="start" gap={6}>
            <VStack align="stretch" flex="1" gap={3}>
              <Text>¿Cuánto se tira? Hay {formatBaseQty(lot.qty, lot.unitCode)} en este lote.</Text>
              <HStack>
                <Box minH="56px" minW="160px" borderWidth="2px" borderColor="brand.solid" borderRadius="md" px={3}
                  display="flex" alignItems="center" fontSize="2xl" fontWeight="700" aria-label="Cuánto se tira" data-testid="cantidad">{qty}</Box>
                <Text>{lot.unitCode}</Text>
                <Button minH="44px" variant="ghost" onClick={() => { setQty(String(Number(lot.qty))); setFresh(true); }}>Todo</Button>
              </HStack>
              {n.estado === 'valido' && n.valor > Number(lot.qty) && (
                <Text color="fg.error" fontSize="sm">El lote solo tiene {formatBaseQty(lot.qty, lot.unitCode)}.</Text>
              )}
              <Picker value={reason} onChange={setReasonId} title="Motivo" placeholder="Elige el motivo"
                options={(reasons.data ?? []).map((r) => ({ value: String(r.id), label: r.name }))} />
              {save.isError && <Text color="fg.error" fontSize="sm">{mensajeDeError(save.error)}</Text>}
            </VStack>
            <Box w="260px"><NumericKeypad allowDecimal={decimals} onKey={key} /></Box>
          </HStack>
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
          <Button minH="44px" colorPalette="brand" disabled={!ok} loading={save.isPending} onClick={() => save.mutate()}>Mandar a merma</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
