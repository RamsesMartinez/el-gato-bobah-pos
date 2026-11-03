import { HStack, IconButton, Text } from '@chakra-ui/react';
import { LuX } from 'react-icons/lu';
import { RADIUS } from '../theme/ui';

// AltNameChip es un nombre alternativo en el editor (spec 063). La «×» mide 44×44: un toque errado
// en una fila de chips borra otro nombre (revisión de tableta, M1).
export function AltNameChip({ name, onRemove, disabled }: { name: string; onRemove: () => void; disabled?: boolean }) {
  return (
    <HStack gap={0} pl={3} minH="44px" bg="bg.subtle" borderWidth="1px" borderColor="border" borderRadius={RADIUS} flexShrink={0}>
      <Text fontSize="sm" whiteSpace="nowrap">{name}</Text>
      <IconButton aria-label={`Quitar «${name}»`} minW="44px" minH="44px" variant="ghost" colorPalette="gray" size="sm"
        disabled={disabled} onClick={onRemove}>
        <LuX />
      </IconButton>
    </HStack>
  );
}
