import { useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { Picker } from '../../components/Picker';
import { DatePickerButton } from '../../components/DatePickerButton';
import { NumericKeypad } from '../../components/NumericKeypad';
import { useKeypadField } from '../../components/useKeypadField';
import { backofficeApi, type Article, type EditLineBody } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { parseMonto, parseNumero } from '../../domain/numeros';
import { formatBaseQty, purchaseUnitsFor, unitLabel } from '../../shared/units';

// Agregar un renglón a un gasto guardado (spec 053). No escribe nada: devuelve el renglón a la
// edición, que lo junta con los demás cambios y pasa por la vista previa. «Ya llegó» lo recibe en la
// misma confirmación con su fecha. Las cifras van con el teclado en pantalla.

export interface DraftLine extends EditLineBody {
  label: string; // cómo se ve en la lista mientras no se guarda
}

type Field = 'qty' | 'amount';

export function AddLineSheet({ expenseDate, canReceive, stockItemLabel, onClose, onAdd }: {
  expenseDate: string;
  canReceive: boolean;
  stockItemLabel: string;
  onClose: () => void;
  onAdd: (l: DraftLine) => void;
}) {
  const hora = useHoraDelNegocio();
  const today = hora.diaDelNegocio(new Date());
  const { data: articles } = useQuery({ queryKey: ['articles'], queryFn: () => backofficeApi.searchArticles('') });
  const { data: presentations } = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations });
  const { data: units } = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units });
  const [kind, setKind] = useState<'stock' | 'other'>('stock');
  const [pick, setPick] = useState('');
  const [description, setDescription] = useState('');
  const [qty, setQty] = useState('1');
  const [how, setHow] = useState<{ presentationId?: number; unitId?: number }>({});
  const [gift, setGift] = useState(false);
  const [amount, setAmount] = useState('');
  const [arrived, setArrived] = useState(false);
  const [arrivedOn, setArrivedOn] = useState(today);
  const { field, setField, apply, ref } = useKeypadField<Field>();

  const list: Article[] = articles?.items ?? [];
  const art = list.find((a) => `${a.itemType}:${a.id}` === pick);
  const packs = art?.itemType === 'ingrediente' ? (presentations ?? []).filter((p) => p.ingredientId === art.id && p.active) : [];
  const buyUnits = art ? purchaseUnitsFor(art.unitKind, units?.items ?? []) : [];
  const n = parseNumero(qty);
  const m = parseMonto(amount);
  const qtyOk = n.estado === 'valido' && n.valor > 0;
  const moneyOk = gift || (m.estado === 'valido' && m.valor > 0);
  const ok = kind === 'stock'
    ? !!art && qtyOk && !!(how.presentationId || how.unitId) && moneyOk
    : description.trim() !== '' && m.estado === 'valido' && m.valor > 0;

  const choose = (v: string) => {
    setPick(v);
    const a = list.find((x) => `${x.itemType}:${x.id}` === v);
    const first = a?.itemType === 'ingrediente' ? (presentations ?? []).find((p) => p.ingredientId === a.id && p.active) : undefined;
    const baseUnit = (units?.items ?? []).find((u) => u.code === a?.unitCode);
    setHow(first ? { presentationId: first.id } : { unitId: baseUnit?.id });
  };
  const key = (k: string) => {
    if (field === 'qty') setQty(apply(qty, k, !how.presentationId && art?.unitCode !== 'pieza'));
    if (field === 'amount') setAmount(apply(amount, k, true));
  };

  const add = () => {
    if (kind === 'other') {
      onAdd({ itemType: '', description: description.trim(), quantity: '1', amount: String(m.estado === 'valido' ? m.valor : 0), label: description.trim() });
      return;
    }
    if (!art) return;
    const packName = packs.find((p) => p.id === how.presentationId)?.name;
    onAdd({
      itemType: art.itemType, ingredientId: art.itemType === 'ingrediente' ? art.id : undefined,
      productId: art.itemType === 'producto' ? art.id : undefined, description: art.name, quantity: qty,
      presentationId: how.presentationId, unitId: how.unitId, amount: gift ? '0' : amount, isGift: gift || undefined,
      arrivedOn: arrived ? arrivedOn : undefined,
      label: `${art.name} · ${qty}${packName ? ` ${packName.toLowerCase()}` : ''}${gift ? ' · regalo' : ''}`,
    });
  };

  const numberBox = (f: Field, value: string, label: string, w: string) => (
    <Input ref={ref(f)} aria-label={label} w={w} minH="44px" inputMode="none" readOnly value={value} textAlign="center" fontWeight="700"
      borderWidth="2px" borderColor={field === f ? 'brand.solid' : undefined}
      onClick={() => setField(f)} onFocus={() => setField(f)} />
  );

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="90dvh">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">Agregar renglón</Text></DrawerHeader>
        <DrawerBody>
          <VStack align="stretch" gap={3}>
            <HStack gap={0} borderWidth="1px" borderRadius="md" alignSelf="start" role="group" aria-label="Qué es">
              <Button minH="44px" aria-pressed={kind === 'stock'} variant={kind === 'stock' ? 'solid' : 'ghost'}
                colorPalette={kind === 'stock' ? 'brand' : 'gray'} onClick={() => setKind('stock')}>
                Un {stockItemLabel} del almacén
              </Button>
              <Button minH="44px" aria-pressed={kind === 'other'} variant={kind === 'other' ? 'solid' : 'ghost'}
                colorPalette={kind === 'other' ? 'brand' : 'gray'} onClick={() => { setKind('other'); setArrived(false); }}>
                Otro (bolsa, envío…)
              </Button>
            </HStack>
            {kind === 'other' ? (
              <HStack gap={2} flexWrap="wrap">
                <Input aria-label="Qué fue" placeholder="Qué fue" minH="44px" flex="1" minW="200px" value={description}
                  onChange={(e) => setDescription(e.target.value)} onFocus={() => setField(null)} />
                <Text>Importe</Text>
                {numberBox('amount', amount, 'Importe', '120px')}
              </HStack>
            ) : (
              <>
                <Picker value={pick} onChange={choose} placeholder={`¿Qué ${stockItemLabel}?`} title={`¿Qué ${stockItemLabel}?`}
                  options={list.map((a) => ({ value: `${a.itemType}:${a.id}`, label: a.name, hint: a.unitCode }))} />
                {art && (
                  <HStack gap={2} flexWrap="wrap">
                    <Text>Cantidad</Text>
                    {numberBox('qty', qty, 'Cantidad', '90px')}
                    {packs.map((p) => (
                      <Button key={p.id} minH="44px" size="sm" aria-pressed={how.presentationId === p.id}
                        variant={how.presentationId === p.id ? 'subtle' : 'outline'} colorPalette={how.presentationId === p.id ? 'brand' : 'gray'}
                        onClick={() => setHow({ presentationId: p.id })}>
                        {p.name} · {formatBaseQty(p.qtyInBase, p.baseUnitCode)}
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
                <HStack gap={3} flexWrap="wrap">
                  <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label="¿Se pagó?">
                    <Button minH="44px" aria-pressed={!gift} variant={!gift ? 'solid' : 'ghost'} colorPalette={!gift ? 'brand' : 'gray'} onClick={() => setGift(false)}>Lo pagué</Button>
                    <Button minH="44px" aria-pressed={gift} variant={gift ? 'solid' : 'ghost'} colorPalette={gift ? 'brand' : 'gray'} onClick={() => { setGift(true); setField(null); }}>Fue regalo</Button>
                  </HStack>
                  {!gift && <><Text>Importe del renglón (total)</Text>{numberBox('amount', amount, 'Importe', '120px')}</>}
                </HStack>
                {canReceive && (
                  <HStack gap={3} flexWrap="wrap">
                    <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label="¿Ya llegó?">
                      <Button minH="44px" aria-pressed={!arrived} variant={!arrived ? 'solid' : 'ghost'} colorPalette={!arrived ? 'brand' : 'gray'} onClick={() => setArrived(false)}>Por recibir</Button>
                      <Button minH="44px" aria-pressed={arrived} variant={arrived ? 'solid' : 'ghost'} colorPalette={arrived ? 'brand' : 'gray'} onClick={() => setArrived(true)}>Ya llegó</Button>
                    </HStack>
                    {arrived && <DatePickerButton value={arrivedOn} onChange={setArrivedOn} max={today} label="Día en que llegó" />}
                    {arrived && arrivedOn < expenseDate && (
                      <Text fontSize="sm" color="fg.error">No puede llegar antes de la fecha del gasto.</Text>
                    )}
                  </HStack>
                )}
              </>
            )}
          </VStack>
        </DrawerBody>
        <DrawerFooter>
          {field ? (
            <HStack w="100%" gap={3} align="end">
              <Box flex="1" maxW="320px">
                <NumericKeypad allowDecimal={field === 'amount' || (!how.presentationId && art?.unitCode !== 'pieza')} onKey={key} />
              </Box>
              <Box flex="1" />
              <Button minH="56px" colorPalette="brand" onClick={() => setField(null)}>Listo</Button>
            </HStack>
          ) : (
            <HStack w="100%" gap={3}>
              <Box flex="1" />
              <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
              <Button minH="44px" colorPalette="brand" disabled={!ok || (arrived && arrivedOn < expenseDate)} onClick={add}>Agregar</Button>
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
