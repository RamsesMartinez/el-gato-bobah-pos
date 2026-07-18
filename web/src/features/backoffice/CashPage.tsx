import { useState } from 'react';
import {
  Box, Heading, Text, Button, VStack, HStack, Table,
  Input, Center, Spinner, Stat,
} from '@chakra-ui/react';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';

export function CashPage() {
  const qc = useQueryClient();
  const { data: session, isLoading } = useQuery({ queryKey: ['cash', 'current'], queryFn: backofficeApi.cashCurrent });
  const [opening, setOpening] = useState('');
  const [declared, setDeclared] = useState<Record<string, string>>({});

  const openMut = useMutation({
    mutationFn: () => backofficeApi.cashOpen(parseFloat(opening) || 0),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cash'] }),
  });
  const closeMut = useMutation({
    mutationFn: () => {
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => (d[k] = parseFloat(v) || 0));
      return backofficeApi.cashClose(d);
    },
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: ['cash'] });
      toaster.create({ title: `Caja cerrada`, type: 'success' });
      void s;
    },
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="720px">
      <Heading size="lg" mb={4}>Corte de caja</Heading>

      {!session ? (
        <VStack align="stretch" gap={4} bg="bg.panel" p={6} borderRadius="lg" borderWidth="1px">
          <Text>No hay caja abierta.</Text>
          <HStack>
            <Input placeholder="Fondo inicial" type="number" value={opening} onChange={(e) => setOpening(e.target.value)} />
            <Button onClick={() => openMut.mutate()} loading={openMut.isPending}>Abrir caja</Button>
          </HStack>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <HStack>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Fondo inicial</Stat.Label>
              <Stat.ValueText>{money(session.openingCash)}</Stat.ValueText>
            </Stat.Root>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Abierta desde</Stat.Label>
              <Stat.ValueText fontSize="md">{new Date(session.openedAt).toLocaleString('es-MX')}</Stat.ValueText>
            </Stat.Root>
          </HStack>

          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="hidden">
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Método</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader><Table.ColumnHeader>Declarado</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {session.totals.map((t) => (
                  <Table.Row key={t.methodId}>
                    <Table.Cell>{t.name}</Table.Cell>
                    <Table.Cell textAlign="end">{money(t.expected)}</Table.Cell>
                    <Table.Cell>
                      <Input size="sm" w="120px" type="number" placeholder="0"
                        value={declared[t.methodId] ?? ''} onChange={(e) => setDeclared({ ...declared, [t.methodId]: e.target.value })} />
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </Box>
          <Button colorPalette="red" size="lg" onClick={() => closeMut.mutate()} loading={closeMut.isPending}>
            Cerrar caja
          </Button>
        </VStack>
      )}
    </Page>
  );
}
