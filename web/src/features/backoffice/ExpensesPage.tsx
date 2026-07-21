import { useState } from 'react';
import {
  Box, Heading, Button, HStack, Table, Input, Center, Spinner,
} from '@chakra-ui/react';
import { NativeSelectRoot, NativeSelectField } from '../../components/ui/native-select';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';

export function ExpensesPage() {
  const qc = useQueryClient();
  const { data: cats } = useQuery({ queryKey: ['expense-cats'], queryFn: backofficeApi.expenseCategories });
  const { data, isLoading } = useQuery({ queryKey: ['expenses'], queryFn: backofficeApi.expenses });
  const [categoryId, setCategoryId] = useState('');
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');

  const create = useMutation({
    mutationFn: () => backofficeApi.createExpense({ categoryId: Number(categoryId), amount: parseFloat(amount) || 0, description }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['expenses'] });
      setAmount(''); setDescription('');
    },
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="820px">
      <Heading size="lg" mb={4}>Gastos</Heading>
      <HStack bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px" mb={4} align="end" flexWrap="wrap">
        <NativeSelectRoot maxW="220px">
          <NativeSelectField placeholder="Categoría" value={categoryId} onChange={(e) => setCategoryId(e.target.value)}>
            {(cats?.items ?? []).map((c) => <option key={c.id} value={c.id}>{c.name} ({c.financial_group})</option>)}
          </NativeSelectField>
        </NativeSelectRoot>
        <Input placeholder="Monto" type="number" value={amount} onChange={(e) => setAmount(e.target.value)} maxW="140px" />
        <Input placeholder="Descripción" value={description} onChange={(e) => setDescription(e.target.value)} flex="1" minW="180px" />
        <Button disabled={!categoryId || !amount} loading={create.isPending} onClick={() => create.mutate()}>
          Agregar
        </Button>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row><Table.ColumnHeader>Fecha</Table.ColumnHeader><Table.ColumnHeader>Categoría</Table.ColumnHeader><Table.ColumnHeader>Proveedor</Table.ColumnHeader><Table.ColumnHeader>Descripción</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Monto</Table.ColumnHeader></Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((e) => (
              <Table.Row key={e.id}>
                <Table.Cell>{e.expense_date?.slice(0, 10)}</Table.Cell>
                <Table.Cell>{e.category}</Table.Cell>
                <Table.Cell>{e.supplier ?? '—'}</Table.Cell>
                <Table.Cell>{e.description ?? '—'}</Table.Cell>
                <Table.Cell textAlign="end">{money(e.amount, e.currency)}</Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
    </Page>
  );
}
