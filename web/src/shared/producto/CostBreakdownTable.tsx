import { Box, Table, Text } from '@chakra-ui/react';
import type { CostLine } from '../../api/admin';
import { ORIGEN } from '../merma/desglose';
import { moneyExact } from '../../utils/format';
import { formatBaseQty } from '../units';

function real(l: CostLine): string {
  return (l.realWaste ?? [])
    .filter((r) => r.state === 'ratio')
    .map((r) => `${r.warehouseName} ${r.ratio} %`)
    .join(' · ');
}

// CostBreakdownTable es el desglose del costo de un producto (spec 096, maqueta B): cinco columnas
// con la primera y el encabezado fijos, para que a 1024×600 se desplace la tabla y no la pantalla.
// La merma real va debajo del %, en pequeño: una columna por almacén no cabría.
export function CostBreakdownTable({ lines }: { lines: CostLine[] }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" overflowX="auto" bg="bg.panel">
      <Table.Root size="sm" stickyHeader>
        <Table.Header>
          <Table.Row>
            <Table.ColumnHeader position="sticky" left={0} bg="bg.panel" zIndex={1}>Artículo</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Lleva</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Cuesta</Table.ColumnHeader>
            <Table.ColumnHeader>Merma</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Suma</Table.ColumnHeader>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {lines.map((l) => {
            const r = real(l);
            return (
              <Table.Row key={l.ingredientId} aria-label={l.name}>
                <Table.Cell position="sticky" left={0} bg="bg.panel" maxW="220px"><Text truncate>{l.name}</Text></Table.Cell>
                <Table.Cell textAlign="end" whiteSpace="nowrap">{formatBaseQty(l.qtyBase, l.baseUnit)}</Table.Cell>
                <Table.Cell textAlign="end">{moneyExact(l.base)}</Table.Cell>
                <Table.Cell>
                  <Text fontWeight="700">{l.pct} %</Text>
                  <Text fontSize="xs" color="fg.muted">{ORIGEN[l.pctSource]}{r ? ` · real ${r}` : ''}</Text>
                </Table.Cell>
                <Table.Cell textAlign="end" fontWeight="600">{moneyExact(l.waste)}</Table.Cell>
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}
