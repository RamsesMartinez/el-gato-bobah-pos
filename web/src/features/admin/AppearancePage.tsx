import { Box, Heading, Text, SimpleGrid, HStack, VStack, Flex, Button } from '@chakra-ui/react';
import { LuCheck, LuSun, LuMoon, LuMonitor } from 'react-icons/lu';
import { useTheme } from 'next-themes';
import { PALETTES, useUiStore } from '../../stores/ui';
import { RADIUS } from '../../theme/ui';

const THEMES = [
  { id: 'light', label: 'Claro', icon: LuSun },
  { id: 'dark', label: 'Oscuro', icon: LuMoon },
  { id: 'system', label: 'Sistema', icon: LuMonitor },
];

export function AppearancePage() {
  const palette = useUiStore((s) => s.palette);
  const setPalette = useUiStore((s) => s.setPalette);
  const topCount = useUiStore((s) => s.topCount);
  const setTopCount = useUiStore((s) => s.setTopCount);
  const { theme, setTheme } = useTheme();

  return (
    <Box p={6} maxW="900px">
      <Heading size="lg" mb={1}>Interfaz</Heading>
      <Text color="fg.muted" mb={6}>Personaliza el aspecto del sistema. Los cambios se aplican al instante.</Text>

      <Text fontWeight="700" mb={2}>Tema</Text>
      <HStack gap={3} mb={8} flexWrap="wrap">
        {THEMES.map((t) => {
          const Icon = t.icon;
          const active = (theme ?? 'system') === t.id;
          return (
            <Button
              key={t.id}
              colorPalette={active ? palette : 'gray'}
              variant={active ? 'solid' : 'outline'}
              onClick={() => setTheme(t.id)}
            >
              <Icon /> {t.label}
            </Button>
          );
        })}
      </HStack>

      <Text fontWeight="700" mb={2}>Paleta de colores</Text>
      <SimpleGrid columns={{ base: 2, md: 3 }} gap={4}>
        {PALETTES.map((p) => {
          const active = p.id === palette;
          return (
            <Box
              key={p.id}
              as="button"
              textAlign="left"
              colorPalette={p.id}
              onClick={() => setPalette(p.id)}
              borderWidth="2px"
              borderColor={active ? 'colorPalette.500' : 'border'}
              borderRadius={RADIUS}
              overflow="hidden"
              bg="bg.panel"
              transition="border-color .15s, box-shadow .15s"
              _hover={{ boxShadow: 'md' }}
            >
              <Flex h="72px">
                {[300, 500, 700].map((shade) => (
                  <Box key={shade} flex="1" bg={`colorPalette.${shade}`} />
                ))}
              </Flex>
              <HStack p={3} justify="space-between" align="start">
                <VStack align="start" gap={0}>
                  <Text fontWeight="700">{p.label}</Text>
                  <Text fontSize="xs" color="fg.muted">{p.hint}</Text>
                </VStack>
                {active && (
                  <Box color="colorPalette.600"><LuCheck size={20} /></Box>
                )}
              </HStack>
            </Box>
          );
        })}
      </SimpleGrid>

      <Text fontWeight="700" mt={8} mb={1}>Pantalla de venta</Text>
      <Text color="fg.muted" fontSize="sm" mb={3}>
        Cuántos productos «Top» (los más vendidos) se muestran al abrir el POS.
      </Text>
      <HStack gap={3}>
        <Button colorPalette="gray" variant="outline" onClick={() => setTopCount(topCount - 4)}>−</Button>
        <Text minW="48px" textAlign="center" fontWeight="700" fontSize="lg">{topCount}</Text>
        <Button colorPalette="gray" variant="outline" onClick={() => setTopCount(topCount + 4)}>+</Button>
      </HStack>

      <Box mt={8} p={4} borderWidth="1px" borderRadius="lg" bg="bg.panel">
        <Text fontWeight="700" mb={3}>Vista previa</Text>
        <HStack gap={3} flexWrap="wrap">
          <Button colorPalette={palette}>Acción principal</Button>
          <Button colorPalette={palette} variant="outline">Secundaria</Button>
          <Button colorPalette={palette} variant="subtle">Sutil</Button>
        </HStack>
      </Box>
    </Box>
  );
}
