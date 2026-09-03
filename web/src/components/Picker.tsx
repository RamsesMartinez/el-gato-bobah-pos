import { useMemo, useState } from 'react';
import { Box, Button, HStack, VStack, Text, Input } from '@chakra-ui/react';
import { LuChevronDown, LuSearch, LuPlus, LuCheck } from 'react-icons/lu';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger } from './ui/drawer';
import { normalize } from '../utils/format';

export interface PickerOption {
  value: string;
  label: string;
  hint?: string; // texto secundario a la derecha (p.ej. grupo, precio)
}

interface Props {
  value: string;
  options: PickerOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  title?: string;
  clearable?: boolean; // ofrece "— Ninguno —" (para campos opcionales)
  clearLabel?: string;
  searchThreshold?: number; // muestra el buscador si hay más de esto (o si hay alta inline)
  disabled?: boolean;
  size?: 'sm' | 'md' | 'lg';
  // Alta inline: si se define, al buscar algo sin coincidencia aparece "+ Crear «query»".
  // Debe devolver la opción ya creada (para seleccionarla al vuelo).
  onCreate?: (name: string) => Promise<PickerOption>;
}

// Picker táctil: reemplaza al <select> nativo (malo en tablet con muchos ítems). Abre un
// bottom-sheet con buscador y filas grandes; opcionalmente permite crear un elemento nuevo
// sin salir del flujo. Ver memoria touch-pickers-no-native-select.
export function Picker({
  value, options, onChange, placeholder = 'Seleccionar', title, clearable,
  clearLabel = '— Ninguno —', searchThreshold = 6, disabled, size = 'md', onCreate,
}: Props) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState('');
  const [creating, setCreating] = useState(false);

  const selected = options.find((o) => o.value === value);
  const showSearch = options.length > searchThreshold || !!onCreate;
  const filtered = useMemo(() => {
    const n = normalize(q);
    return n ? options.filter((o) => normalize(o.label).includes(n)) : options;
  }, [q, options]);
  const exact = q.trim() !== '' && options.some((o) => normalize(o.label) === normalize(q));

  const pick = (v: string) => { onChange(v); setOpen(false); setQ(''); };

  const create = async () => {
    if (!onCreate || !q.trim()) return;
    setCreating(true);
    try {
      const opt = await onCreate(q.trim());
      pick(opt.value);
    } finally {
      setCreating(false);
    }
  };

  return (
    <>
      {/* 44 px SIEMPRE, aunque el `size` sea sm: la receta del tema solo sube el piso en `md`, y
          `size="sm"` dejaba este disparador en 32 px. El tamaño decide la tipografía y el padding;
          el alto mínimo con el que un dedo acierta a la primera no es negociable por tamaño. */}
      <Button variant="outline" colorPalette="gray" w="100%" justifyContent="space-between" minH="44px"
        size={size} fontWeight="500" disabled={disabled} onClick={() => setOpen(true)}>
        <Text flex="1" minW={0} truncate textAlign="start" color={selected ? 'fg' : 'fg.muted'}>
          {selected ? selected.label : placeholder}
        </Text>
        <LuChevronDown />
      </Button>

      <DrawerRoot open={open} placement="bottom" onOpenChange={(e) => { if (!e.open) { setOpen(false); setQ(''); } }} size="md">
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="80dvh">
          <DrawerCloseTrigger />
          <VStack align="stretch" gap={0} h="100%" maxH="80dvh">
            {title && <Text fontWeight="700" fontSize="lg" px={4} pt={4} pb={2}>{title}</Text>}
            {showSearch && (
              <Box px={4} pt={title ? 0 : 4} pb={2}>
                <HStack px={3} borderWidth="1px" borderRadius="lg" bg="bg.subtle">
                  <LuSearch />
                  <Input variant="flushed" border="none" placeholder="Buscar…" value={q}
                    onChange={(e) => setQ(e.target.value)} autoFocus />
                </HStack>
              </Box>
            )}
            <VStack align="stretch" gap={1} overflowY="auto" px={3} pb={4} pt={1} flex="1">
              {clearable && <PickerRow label={clearLabel} muted selected={!value} onClick={() => pick('')} />}
              {filtered.map((o) => (
                <PickerRow key={o.value} label={o.label} hint={o.hint} selected={o.value === value} onClick={() => pick(o.value)} />
              ))}
              {onCreate && q.trim() !== '' && !exact && (
                <Button size="lg" minH="52px" variant="subtle" colorPalette="green" justifyContent="start"
                  loading={creating} onClick={create}>
                  <LuPlus /> Crear «{q.trim()}»
                </Button>
              )}
              {filtered.length === 0 && !onCreate && <Text color="fg.muted" px={2} py={3}>Sin resultados.</Text>}
              {/* Sin esto el alta inline es invisible: el botón «Crear» solo aparece al escribir, así
                  que quien abre la hoja y no ve su proveedor asume que tiene que salirse a crearlo. */}
              {onCreate && q.trim() === '' && (
                <Text color="fg.subtle" fontSize="sm" px={2} py={3}>
                  ¿No está en la lista? Escribe su nombre y podrás crearlo aquí.
                </Text>
              )}
            </VStack>
          </VStack>
        </DrawerContent>
      </DrawerRoot>
    </>
  );
}

function PickerRow({ label, hint, selected, muted, onClick }: {
  label: string; hint?: string; selected?: boolean; muted?: boolean; onClick: () => void;
}) {
  return (
    <Button variant={selected ? 'subtle' : 'ghost'} colorPalette="gray" size="lg" minH="52px"
      justifyContent="space-between" fontWeight="500" onClick={onClick}>
      <HStack gap={2} minW={0}>
        <Text truncate color={muted ? 'fg.muted' : 'fg'}>{label}</Text>
        {hint && <Text fontSize="xs" color="fg.subtle" flexShrink={0}>{hint}</Text>}
      </HStack>
      {selected && <LuCheck />}
    </Button>
  );
}
