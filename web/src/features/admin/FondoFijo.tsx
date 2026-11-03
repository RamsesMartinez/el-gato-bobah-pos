import { useState } from 'react';
import { Box, Button, Grid, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { DrawerBackdrop, DrawerBody, DrawerContent, DrawerFooter, DrawerHeader, DrawerRoot } from '../../components/ui/drawer';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashRegisterConfig } from '../../api/backoffice';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { money } from '../../utils/format';

const TECLAS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '0', '⌫'];

// FondoFijo: lo que se queda en el cajón al cerrar cada caja (spec 035), opción B de las maquetas.
// Un renglón por caja; tocarlo abre una hoja con teclado propio y el campo en solo lectura, para que
// el teclado del sistema no salga encima del de la hoja. Sin permiso se ve el monto y no se abre nada.
export function FondoFijo() {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const puede = can('cash.configure', user);
  const { data } = useQuery({ queryKey: ['cash', 'registers', 'all'], queryFn: () => backofficeApi.allCashRegisters() });
  const [editando, setEditando] = useState<CashRegisterConfig | null>(null);
  const [valor, setValor] = useState('');
  const guardar = useMutation({
    mutationFn: (p: { id: number; fixedFloat: number | null }) => backofficeApi.setFixedFloat(p.id, p.fixedFloat),
    onSuccess: () => { setEditando(null); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo guardar el fondo', description: mensajeDeError(e), type: 'error' }),
  });
  const abrir = (c: CashRegisterConfig) => { setEditando(c); setValor(c.fixedFloat != null ? String(Number(c.fixedFloat)) : ''); };
  const tecla = (k: string) => setValor((v) => {
    if (k === '⌫') return v.slice(0, -1);
    if (k === '.' && v.includes('.')) return v;
    const n = (v + k).replace(/^0(\d)/, '$1');
    return /^\d{0,7}(\.\d{0,2})?$/.test(n) ? n : v;
  });
  const cajas = (data?.items ?? []).filter((c) => c.isActive);

  return (
    <>
      <VStack align="stretch" gap={0}>
        {cajas.map((c) => {
          const monto = c.fixedFloat != null
            ? <Text fontWeight="700">{money(c.fixedFloat)}</Text>
            : <Text color="fg.muted">Sin configurar</Text>;
          return puede ? (
            <Button key={c.id} variant="ghost" minH="56px" justifyContent="space-between" px={1} onClick={() => abrir(c)}>
              <Text fontWeight="600">{c.name}</Text>
              <HStack gap={2}>{monto}<Text color="fg.muted">›</Text></HStack>
            </Button>
          ) : (
            <HStack key={c.id} minH="56px" justify="space-between" px={1}>
              <Text fontWeight="600">{c.name}</Text>
              {monto}
            </HStack>
          );
        })}
      </VStack>

      <DrawerRoot open={editando !== null} placement="bottom" onOpenChange={(e) => { if (!e.open) setEditando(null); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="l3" maxH="85dvh">
          <DrawerHeader pb={1}><Text fontWeight="700" fontSize="lg">Fondo fijo · {editando?.name}</Text></DrawerHeader>
          <DrawerBody>
            <HStack align="start" gap={6} wrap="wrap">
              <Box>
                <Input aria-label="Fondo fijo" readOnly inputMode="none" value={valor} placeholder="$"
                  minH="56px" maxW="220px" fontSize="2xl" textAlign="end" />
                <Text mt={2} fontSize="sm" color="fg.muted">El cierre lo trae escrito y se puede cambiar solo ese día.</Text>
              </Box>
              <Grid templateColumns="repeat(3, 72px)" gap={2}>
                {TECLAS.map((k) => (
                  <Button key={k} minH="52px" variant="outline" aria-label={k === '⌫' ? 'Borrar' : k} onClick={() => tecla(k)}>{k}</Button>
                ))}
              </Grid>
            </HStack>
          </DrawerBody>
          <DrawerFooter justifyContent="space-between">
            {/* Quitarlo cambia el cierre de mañana: lejos de Guardar y en rojo. */}
            <Button minH="52px" variant="ghost" colorPalette="red" disabled={editando?.fixedFloat == null}
              onClick={() => editando && guardar.mutate({ id: editando.id, fixedFloat: null })}>Quitar fondo fijo</Button>
            <HStack gap={3}>
              <Button minH="52px" variant="outline" onClick={() => setEditando(null)}>Cancelar</Button>
              <Button minH="52px" colorPalette="orange" disabled={valor === '' || valor === '.'} loading={guardar.isPending}
                onClick={() => editando && guardar.mutate({ id: editando.id, fixedFloat: Number(valor) })}>Guardar</Button>
            </HStack>
          </DrawerFooter>
        </DrawerContent>
      </DrawerRoot>
    </>
  );
}
