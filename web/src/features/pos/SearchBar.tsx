import { Input, InputGroup, IconButton } from '@chakra-ui/react';
import { LuSearch, LuX } from 'react-icons/lu';

interface Props {
  value: string;
  onChange: (v: string) => void;
}

export function SearchBar({ value, onChange }: Props) {
  return (
    <InputGroup
      startElement={<LuSearch color="fg.subtle" />}
      endElement={
        value ? (
          <IconButton aria-label="Limpiar" size="sm" variant="ghost" onClick={() => onChange('')}>
            <LuX />
          </IconButton>
        ) : undefined
      }
    >
      <Input
        size="lg"
        placeholder="Buscar producto…"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        bg="bg.panel"
        borderRadius="lg"
      />
    </InputGroup>
  );
}
