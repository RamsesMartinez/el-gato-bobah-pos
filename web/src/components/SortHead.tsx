import { HStack, Table, Text } from '@chakra-ui/react';
import { LuArrowDown, LuArrowUp } from 'react-icons/lu';

// Cabecera de columna ordenable: clic alterna asc/desc y muestra la flecha en la columna activa.
// La celda entera es el área tappable (tabletas de 7"), no solo el texto.
export function SortHead<K extends string>({ label, col, sort, dir, onSort, numeric, align }: {
  label: string;
  col: K;
  sort: K;
  dir: 'asc' | 'desc';
  onSort: (col: K, numeric?: boolean) => void;
  numeric?: boolean;
  align?: 'end' | 'center';
}) {
  const active = sort === col;
  const justify = align === 'end' ? 'end' : align === 'center' ? 'center' : 'start';
  return (
    <Table.ColumnHeader textAlign={align} cursor="pointer" userSelect="none" onClick={() => onSort(col, numeric)}
      title={`Ordenar por ${label.toLowerCase()}`}>
      <HStack gap={1} justify={justify}>
        <Text as="span">{label}</Text>
        {active && (dir === 'asc' ? <LuArrowUp size={12} /> : <LuArrowDown size={12} />)}
      </HStack>
    </Table.ColumnHeader>
  );
}
