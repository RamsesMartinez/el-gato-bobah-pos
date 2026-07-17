import {
  Box, Heading, SimpleGrid, Table, Center, Spinner, Text, Stat, HStack,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { money } from '../../utils/format';

export function ReportsPage() {
  const sales = useQuery({ queryKey: ['report', 'sales'], queryFn: () => backofficeApi.reportSales() });
  const margins = useQuery({ queryKey: ['report', 'margins'], queryFn: backofficeApi.reportMargins });

  if (sales.isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const totalRevenue = sales.data?.byDay.reduce((s, d) => s + d.revenue, 0) ?? 0;
  const totalOrders = sales.data?.byDay.reduce((s, d) => s + d.orders, 0) ?? 0;

  return (
    <Box p={6} maxW="960px">
      <Heading size="lg" mb={4}>Reportes (últimos 30 días)</Heading>
      <HStack mb={4}>
        <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px"><Stat.Label>Ventas</Stat.Label><Stat.ValueText>{money(totalRevenue)}</Stat.ValueText></Stat.Root>
        <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px"><Stat.Label>Pedidos</Stat.Label><Stat.ValueText>{totalOrders}</Stat.ValueText></Stat.Root>
      </HStack>

      <SimpleGrid columns={{ base: 1, lg: 2 }} gap={4}>
        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4}>
          <Text fontWeight="700" mb={2}>Por medio de pago</Text>
          <Table.Root size="sm">
            <Table.Header><Table.Row><Table.ColumnHeader>Método</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Pagos</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Total</Table.ColumnHeader></Table.Row></Table.Header>
            <Table.Body>
              {sales.data?.byMethod.map((m) => (
                <Table.Row key={m.method}><Table.Cell>{m.method}</Table.Cell><Table.Cell textAlign="end">{m.payments}</Table.Cell><Table.Cell textAlign="end">{money(m.total)}</Table.Cell></Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        </Box>

        <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} overflowX="auto">
          <Text fontWeight="700" mb={2}>Utilidad por producto</Text>
          <Table.Root size="sm">
            <Table.Header><Table.Row><Table.ColumnHeader>Producto</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Cant.</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Venta</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Margen</Table.ColumnHeader></Table.Row></Table.Header>
            <Table.Body>
              {margins.data?.items.slice(0, 20).map((m) => (
                <Table.Row key={m.product_name}>
                  <Table.Cell>{m.product_name}</Table.Cell>
                  <Table.Cell textAlign="end">{m.qty}</Table.Cell>
                  <Table.Cell textAlign="end">{money(m.revenue)}</Table.Cell>
                  <Table.Cell textAlign="end" color={m.margin < 0 ? 'red.600' : 'green.600'}>{money(m.margin)}</Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        </Box>
      </SimpleGrid>
    </Box>
  );
}
