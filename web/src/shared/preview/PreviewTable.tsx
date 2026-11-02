import type { ReactNode } from 'react';
import { Table, Text } from '@chakra-ui/react';

// Piezas de una vista previa «antes / después» (spec 053, maqueta «Vista previa» opción B; la usan
// también la edición de gastos y la unión de insumos, spec 056). La cifra la calcula el servidor con
// la misma transacción que confirma, revertida; aquí solo se pinta.

// PreviewTable es la tabla de cuatro columnas con el encabezado fijo.
export function PreviewTable({ children }: { children: ReactNode }) {
  return (
    <Table.Root size="sm" stickyHeader>
      <Table.Header>
        <Table.Row>
          <Table.ColumnHeader>Qué</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Antes</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Después</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Cambio</Table.ColumnHeader>
        </Table.Row>
      </Table.Header>
      <Table.Body>{children}</Table.Body>
    </Table.Root>
  );
}

// PreviewGroup es el renglón que titula un grupo de la tabla.
export function PreviewGroup({ label }: { label: string }) {
  return (
    <Table.Row bg="bg.muted">
      <Table.Cell colSpan={4} fontSize="xs" fontWeight="700" textTransform="uppercase" letterSpacing="wide" color="fg.muted">{label}</Table.Cell>
    </Table.Row>
  );
}

// StaleNotice: la confirmación encontró algo distinto de lo que se vio; lo de abajo ya es lo de ahora.
export function StaleNotice() {
  return (
    <Text p={3} mb={3} borderRadius="lg" bg="orange.subtle" fontSize="sm" role="status">
      Algo cambió mientras revisabas. Esto es lo que pasaría ahora.
    </Text>
  );
}
