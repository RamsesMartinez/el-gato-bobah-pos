import { useState } from 'react';
import { Box, Button, Flex, SimpleGrid, Text } from '@chakra-ui/react';
import { DrawerBackdrop, DrawerCloseTrigger, DrawerContent, DrawerRoot } from './ui/drawer';
import { formatDay } from '../utils/calendar';

const MONTHS = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];
const pad = (n: number) => String(n).padStart(2, '0');

// ExpiryPickerButton elige una fecha de caducidad en dos pasos cortos: el mes (los próximos 12 desde el
// actual, y otros 12 con ›) y luego el día (spec 098, maqueta «Fecha al recibir», opción C). Las caducidades son de meses o
// años, y el calendario de siempre (DatePickerButton) pide un toque por cada mes que se avanza. Vacío
// es «sin fecha»: así se comporta como hoy. allowPast deja elegir lo que ya caducó (el conteo).
export function ExpiryPickerButton({ value, onChange, label, today, allowPast, placeholder, w, clearLabel, compact }: {
  value: string;
  onChange: (v: string) => void;
  label: string;
  // today es el día del negocio, AAAA-MM-DD.
  today: string;
  allowPast?: boolean;
  placeholder?: string;
  w?: string;
  // clearLabel nombra el botón que deja el campo vacío; sin valor no se ofrece (no haría nada).
  clearLabel?: string;
  // compact cabe en un renglón ya lleno (la hoja de entrega).
  compact?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const thisYear = Number(today.slice(0, 4));
  const thisMonth = Number(today.slice(5, 7)) - 1;
  // Meses contados desde el año 0: una página son 12 desde el mes actual, así cualquier fecha del año
  // que viene cuesta 3 toques aunque cruce de diciembre a enero.
  const now = thisYear * 12 + thisMonth;
  const [page, setPage] = useState(0);
  const [chosen, setChosen] = useState<number | null>(null);
  const start = () => {
    const v = value ? Number(value.slice(0, 4)) * 12 + Number(value.slice(5, 7)) - 1 : now;
    setPage(Math.floor((v - now) / 12));
    setChosen(null);
    setOpen(true);
  };
  const pick = (v: string) => { onChange(v); setOpen(false); };
  const months = Array.from({ length: 12 }, (_, i) => now + page * 12 + i);
  const yOf = (abs: number) => Math.floor(abs / 12);
  const mOf = (abs: number) => abs % 12;
  const year = chosen === null ? 0 : yOf(chosen);
  const month = chosen === null ? null : mOf(chosen);
  const daysIn = (m: number) => new Date(year, m + 1, 0).getDate();
  const pastDay = (d: number) => !allowPast && month !== null && `${year}-${pad(month + 1)}-${pad(d)}` < today;

  return (
    <>
      <Button variant="outline" colorPalette="gray" minH="44px" w={w} justifyContent="flex-start" fontWeight="500"
        px={compact ? 2 : undefined} fontSize={compact ? 'sm' : undefined} aria-label={label} onClick={start}>
        <Text color={value ? 'fg' : 'fg.muted'}>{value ? `Caduca ${compact ? formatDay(value).replace(/ \d{4}$/, '') : formatDay(value)}` : (placeholder ?? 'Caduca…')}</Text>
      </Button>
      <DrawerRoot open={open} placement="bottom" onOpenChange={(e) => { if (!e.open) setOpen(false); }} size="md">
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="90dvh" aria-label={label}>
          <DrawerCloseTrigger />
          <Box maxW="560px" w="100%" mx="auto" px={4} pt={4} pb={6}>
            <Text fontWeight="700" fontSize="lg" mb={2}>{month === null ? '¿Cuándo caduca?' : `¿Qué día de ${MONTHS[month]} ${year}?`}</Text>
            {month === null ? (
              <>
                <Flex justify="space-between" align="center" mb={2}>
                  <Button variant="ghost" minH="44px" minW="44px" aria-label="Meses anteriores"
                    disabled={!allowPast && page <= 0} onClick={() => setPage((p) => p - 1)}>‹</Button>
                  <Text fontWeight="800">{MONTHS[mOf(months[0])]} {yOf(months[0])} – {MONTHS[mOf(months[11])]} {yOf(months[11])}</Text>
                  <Button variant="ghost" minH="44px" minW="44px" aria-label="Meses siguientes"
                    disabled={page >= 9} onClick={() => setPage((p) => p + 1)}>›</Button>
                </Flex>
                <SimpleGrid columns={4} gap={1}>
                  {months.map((abs) => (
                    <Button key={abs} minH="44px" variant={value.startsWith(`${yOf(abs)}-${pad(mOf(abs) + 1)}`) ? 'solid' : 'outline'}
                      aria-label={`${MONTHS[mOf(abs)]} ${yOf(abs)}`} onClick={() => setChosen(abs)}>
                      {MONTHS[mOf(abs)]}{yOf(abs) !== thisYear ? ` ${String(yOf(abs)).slice(2)}` : ''}
                    </Button>
                  ))}
                </SimpleGrid>
              </>
            ) : (
              <SimpleGrid columns={8} gap={1}>
                {Array.from({ length: daysIn(month) }, (_, i) => i + 1).map((d) => (
                  <Button key={d} minH="44px" px={0} variant="ghost" disabled={pastDay(d)}
                    onClick={() => pick(`${year}-${pad(month + 1)}-${pad(d)}`)}>{d}</Button>
                ))}
              </SimpleGrid>
            )}
            <Flex gap={2} mt={3}>
              {month !== null && <Button flex="1" minH="44px" variant="outline" onClick={() => setChosen(null)}>‹ Otro mes</Button>}
              {value && <Button flex="1" minH="44px" variant="outline" onClick={() => pick('')}>{clearLabel ?? 'No trae fecha'}</Button>}
            </Flex>
          </Box>
        </DrawerContent>
      </DrawerRoot>
    </>
  );
}
