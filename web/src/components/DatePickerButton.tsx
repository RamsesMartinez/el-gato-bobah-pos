import { useState } from 'react';
import { Box, Button, Flex, SimpleGrid, Text } from '@chakra-ui/react';
import { DrawerBackdrop, DrawerCloseTrigger, DrawerContent, DrawerRoot } from './ui/drawer';
import { WEEKDAY_INITIALS, MONTH_NAMES, formatDay, monthGrid } from '../utils/calendar';

// DatePickerButton elige un día con el dedo. Reemplaza al `<input type="date">`: su calendario lo
// pinta el sistema con celdas de ~30 px, que en una tableta de 7" es lo que la constitución prohíbe
// para cualquier control cuyo desplegable pinte el sistema. Cada día mide 44 px y la hoja sale
// desde abajo con el mismo Drawer que el Picker, que convive con un diálogo abierto debajo.
export function DatePickerButton({ value, onChange, label, max, clearable, w }: {
  value: string;
  onChange: (v: string) => void;
  label: string;
  max?: string;
  // clearable ofrece «Quitar fecha» para los campos opcionales; onChange recibe ''.
  clearable?: boolean;
  w?: string;
}) {
  const [open, setOpen] = useState(false);
  const base = value || max || new Date().toISOString().slice(0, 10);
  const [ym, setYm] = useState(() => ({ y: Number(base.slice(0, 4)), m: Number(base.slice(5, 7)) - 1 }));
  const move = (delta: number) => setYm(({ y, m }) => {
    const t = m + delta;
    return { y: y + Math.floor(t / 12), m: ((t % 12) + 12) % 12 };
  });
  const pick = (v: string) => { onChange(v); setOpen(false); };

  return (
    <>
      <Button variant="outline" colorPalette="gray" minH="44px" w={w} justifyContent="flex-start" fontWeight="500"
        aria-label={label} onClick={() => setOpen(true)}>
        <Text color={value ? 'fg' : 'fg.muted'}>{value ? formatDay(value) : label}</Text>
      </Button>
      <DrawerRoot open={open} placement="bottom" onOpenChange={(e) => { if (!e.open) setOpen(false); }} size="md">
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="90dvh" aria-label={label}>
          <DrawerCloseTrigger />
          <Box maxW="420px" w="100%" mx="auto" px={4} pt={4} pb={6}>
            <Text fontWeight="700" fontSize="lg" mb={2}>{label}</Text>
            <Flex justify="space-between" align="center" mb={2}>
              <Button variant="ghost" minH="44px" minW="44px" aria-label="Mes anterior" onClick={() => move(-1)}>‹</Button>
              <Text fontWeight="800" textTransform="capitalize">{MONTH_NAMES[ym.m]} {ym.y}</Text>
              <Button variant="ghost" minH="44px" minW="44px" aria-label="Mes siguiente" onClick={() => move(1)}>›</Button>
            </Flex>
            <SimpleGrid columns={7} gap={1}>
              {WEEKDAY_INITIALS.map((d, i) => <Text key={i} textAlign="center" fontSize="xs" color="fg.muted">{d}</Text>)}
              {monthGrid(ym.y, ym.m).map((d, i) => d === null ? <Box key={`gap${i}`} /> : (
                <Button key={d} minH="44px" px={0} variant={d === value ? 'solid' : 'ghost'}
                  disabled={max !== undefined && d > max} onClick={() => pick(d)}>
                  {Number(d.slice(8))}
                </Button>
              ))}
            </SimpleGrid>
            {clearable && value && (
              <Button mt={3} w="100%" minH="44px" variant="outline" onClick={() => pick('')}>Quitar fecha</Button>
            )}
          </Box>
        </DrawerContent>
      </DrawerRoot>
    </>
  );
}
