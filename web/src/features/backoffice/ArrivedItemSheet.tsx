import { useState, type ReactNode } from 'react';
import { presentationLabel } from '../../shared/presentationLabel';
import { Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { Picker } from '../../components/Picker';
import { backofficeApi, type Article } from '../../api/backoffice';
import { receiptError } from './receiptError';
import { parseMonto, parseNumero } from '../../domain/numeros';
import { formatBaseQty, purchaseUnitsFor, unitLabel } from '../../shared/units';

const num = (s: string | null | undefined) => { const n = parseNumero(s ?? ''); return n.estado === 'valido' ? n.valor : NaN; };

// ReceiptSheet es la hoja inferior de las acciones de recibir (spec 038).
export function ReceiptSheet({ title, children, footer, onClose }: { title: string; children: ReactNode; footer: ReactNode; onClose: () => void }) {
  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">{title}</Text></DrawerHeader>
        <DrawerBody>{children}</DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
          {footer}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}


// ArrivedItemSheet es «Llegó algo más»: lo que no venía en la compra y ya llegó. Pagado sube el
// total por su importe; un regalo va en 0 y no cambia lo pagado ni el costo.
export function ArrivedItemSheet({ expenseId, supplier, onClose, onDone }: {
  expenseId: number; supplier: string | null; onClose: () => void; onDone: (m: string) => void;
}) {
  const { data: articles } = useQuery({ queryKey: ['articles'], queryFn: () => backofficeApi.searchArticles('') });
  const { data: presentations } = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations });
  const { data: units } = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units });
  const [pick, setPick] = useState('');
  const [value, setValue] = useState('');
  const [how, setHow] = useState<{ presentationId?: number; unitId?: number }>({});
  const [gift, setGift] = useState(false);
  const [amount, setAmount] = useState('');
  const list: Article[] = articles?.items ?? [];
  const art = list.find((a) => `${a.itemType}:${a.id}` === pick);
  const packs = art?.itemType === 'ingrediente' ? (presentations ?? []).filter((p) => p.ingredientId === art.id && p.active) : [];
  const buyUnits = art ? purchaseUnitsFor(art.unitKind, units?.items ?? []) : [];
  const n = num(value);
  const money = parseMonto(amount);
  const ok = !!art && Number.isFinite(n) && n > 0 && (how.presentationId || how.unitId)
    && (gift || (money.estado === 'valido' && money.valor > 0));

  const choose = (v: string) => {
    setPick(v);
    const a = list.find((x) => `${x.itemType}:${x.id}` === v);
    const first = a?.itemType === 'ingrediente' ? (presentations ?? []).find((p) => p.ingredientId === a.id && p.active) : undefined;
    const baseUnit = (units?.items ?? []).find((u) => u.code === a?.unitCode);
    setHow(first ? { presentationId: first.id } : { unitId: baseUnit?.id });
  };

  const save = useMutation({
    mutationFn: () => backofficeApi.addArrivedItem(expenseId, {
      itemType: art!.itemType, ingredientId: art!.itemType === 'ingrediente' ? art!.id : undefined,
      productId: art!.itemType === 'producto' ? art!.id : undefined, description: art!.name, quantity: value,
      presentationId: how.presentationId, unitId: how.unitId, amount: gift ? '0' : amount, isGift: gift,
    }),
    onSuccess: () => onDone(gift
      ? `Listo. ${art!.name}: regalo, no cambia lo pagado.`
      : `Listo. ${art!.name} agregado; el total pagado subió.`),
  });

  return (
    <ReceiptSheet title={`Llegó algo más de ${supplier ?? 'esta compra'}`} onClose={onClose}
      footer={<Button minH="44px" colorPalette="brand" disabled={!ok} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>}>
      <VStack align="stretch" gap={3}>
        <Picker value={pick} onChange={choose} placeholder="¿Qué llegó?" title="¿Qué llegó?"
          options={list.map((a) => ({ value: `${a.itemType}:${a.id}`, label: a.name, hint: a.unitCode }))} />
        {art && (
          <HStack gap={2} flexWrap="wrap">
            <Text>Cantidad</Text>
            <Input aria-label="Cantidad" w="90px" minH="44px" inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} />
            {packs.map((p) => (
              <Button key={p.id} minH="44px" size="sm" aria-pressed={how.presentationId === p.id}
                variant={how.presentationId === p.id ? 'subtle' : 'outline'} colorPalette={how.presentationId === p.id ? 'brand' : 'gray'}
                onClick={() => setHow({ presentationId: p.id })}>
                {presentationLabel(p)} · {formatBaseQty(p.qtyInBase, p.baseUnitCode)}
              </Button>
            ))}
            {buyUnits.map((u) => (
              <Button key={u.id} minH="44px" size="sm" aria-pressed={how.unitId === u.id}
                variant={how.unitId === u.id ? 'subtle' : 'outline'} colorPalette={how.unitId === u.id ? 'brand' : 'gray'}
                onClick={() => setHow({ unitId: u.id })}>
                {unitLabel(u)}
              </Button>
            ))}
          </HStack>
        )}
        <HStack gap={0} borderWidth="1px" borderRadius="md" alignSelf="start" role="group" aria-label="¿Se pagó?">
          <Button minH="44px" aria-pressed={!gift} variant={!gift ? 'solid' : 'ghost'} colorPalette={!gift ? 'brand' : 'gray'} onClick={() => setGift(false)}>Lo pagué</Button>
          <Button minH="44px" aria-pressed={gift} variant={gift ? 'solid' : 'ghost'} colorPalette={gift ? 'brand' : 'gray'} onClick={() => setGift(true)}>Fue regalo</Button>
        </HStack>
        {gift
          ? <Text p={3} borderRadius="lg" bg="orange.subtle" fontSize="sm">Suma a lo guardado. No cambia lo pagado ni el costo del artículo.</Text>
          : (
            <HStack gap={2}>
              <Text>¿Cuánto pagaste?</Text>
              <Input aria-label="Importe" w="120px" minH="44px" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
              <Text fontSize="sm" color="fg.muted">Sube el total pagado de esta compra.</Text>
            </HStack>
          )}
        {save.isError && <Text color="fg.error" fontSize="sm">{receiptError(save.error)}</Text>}
      </VStack>
    </ReceiptSheet>
  );
}
