import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger, DrawerHeader, DrawerTitle, DrawerBody,
} from '../components/ui/drawer';
import { normalize } from '../utils/format';

export interface SearchOption {
  value: string;
  label: string;
  hint?: string;
}

// SearchSheet es una hoja con buscador para elegir de una lista larga (cientos de insumos o
// productos). Como el Picker, pero el renglón «crear» lo pone quien la usa: crear un insumo pide
// además en qué se mide, y eso no cabe en un solo toque.
export function SearchSheet({ open, title, options, onPick, onClose, error, renderCreate, emptyText, restoreFocus = true, onExitComplete }: {
  open: boolean;
  title: string;
  options: SearchOption[];
  onPick: (value: string) => void;
  onClose: () => void;
  error?: string;
  // Se muestra cuando lo buscado no coincide exacto con ninguna opción.
  renderCreate?: (query: string) => ReactNode;
  emptyText?: string;
  // false cuando quien la abre pone el foco en otro lado al cerrarse (la cantidad del insumo elegido).
  restoreFocus?: boolean;
  onExitComplete?: () => void;
}) {
  const [q, setQ] = useState('');
  // Monta cerrada y abre en el render siguiente: una hoja que nace abierta no se monta con
  // Chakra 3.37 (AGENTS.md §3).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(open); if (!open) setQ(''); }, [open]);

  const n = normalize(q.trim());
  const shown = useMemo(() => (n ? options.filter((o) => normalize(o.label).includes(n)) : options), [n, options]);
  const exact = !!n && options.some((o) => normalize(o.label) === n);

  return (
    <DrawerRoot open={visible} placement="bottom" size="md" restoreFocus={restoreFocus} onExitComplete={onExitComplete} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="85dvh">
        <DrawerCloseTrigger />
        <DrawerHeader pb={2}><DrawerTitle>{title}</DrawerTitle></DrawerHeader>
        <DrawerBody pt={0}>
          <VStack align="stretch" gap={2}>
            <Input aria-label="Buscar" placeholder="Buscar…" value={q} onChange={(e) => setQ(e.target.value)} minH="44px" />
            {error && (
              <Box role="alert" px={3} py={2} borderRadius="md" bg="red.50" borderWidth="1px" borderColor="red.200" color="red.800" fontSize="sm">{error}</Box>
            )}
            <Box>
              {shown.map((o) => (
                <Button key={o.value} variant="ghost" w="100%" minH="48px" justifyContent="space-between" fontWeight="normal"
                  borderBottomWidth="1px" borderColor="border.muted" borderRadius={0} onClick={() => onPick(o.value)}>
                  <Text truncate>{o.label}</Text>
                  {o.hint && <Text fontSize="sm" color="fg.muted">{o.hint}</Text>}
                </Button>
              ))}
            </Box>
            {!!n && !exact && renderCreate?.(q.trim())}
            {shown.length === 0 && !renderCreate && (
              <HStack justify="center" py={6}><Text color="fg.muted">{emptyText ?? `Nada coincide con «${q.trim()}».`}</Text></HStack>
            )}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
