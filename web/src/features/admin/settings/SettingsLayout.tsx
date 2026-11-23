import { useState, type ReactNode } from 'react';
import type { IconType } from 'react-icons';
import { useSearchParams } from 'react-router';
import { Box, Button, Flex, Heading, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import { LuChevronLeft, LuChevronRight, LuLayoutGrid, LuList, LuPanelLeft } from 'react-icons/lu';
import { Page } from '../../../components/Page';

export interface SeccionDeAjustes {
  id: string;
  label: string;
  icon: IconType;
  // Lo que hay dentro, para el índice (vista C).
  resumen: string;
  contenido: ReactNode;
}

export type VistaDeAjustes = 'lista' | 'iconos' | 'indice';

const VISTAS: { id: VistaDeAjustes; label: string; icon: IconType }[] = [
  { id: 'lista', label: 'Lista', icon: LuList },
  { id: 'iconos', label: 'Íconos', icon: LuPanelLeft },
  { id: 'indice', label: 'Índice', icon: LuLayoutGrid },
];

// La vista la recuerda cada tableta, como la pantalla Interfaz. El almacenamiento del navegador
// puede no estar (ventana privada): entonces abre siempre en la lista.
const LLAVE_VISTA = 'egb:ajustes:vista';
function vistaGuardada(): VistaDeAjustes {
  try {
    const v = localStorage.getItem(LLAVE_VISTA);
    return v === 'iconos' || v === 'indice' ? v : 'lista';
  } catch {
    return 'lista';
  }
}
function guardarVista(v: VistaDeAjustes) {
  try {
    localStorage.setItem(LLAVE_VISTA, v);
  } catch {
    // Sin almacenamiento se usa la vista elegida mientras la pantalla siga abierta.
  }
}

// SettingsLayout reparte los ajustes del negocio en secciones (spec 092, maqueta «Ajustes del
// negocio»): una barra de título fija con el selector de vista —siempre en el mismo lugar— y la
// sección abierta en la URL (`?seccion=`), para que sobreviva a una recarga y se pueda enlazar.
export function SettingsLayout({ secciones }: { secciones: SeccionDeAjustes[] }) {
  const [vista, setVista] = useState<VistaDeAjustes>(vistaGuardada);
  const [params, setParams] = useSearchParams();
  const pedida = params.get('seccion');
  const abierta = secciones.find((s) => s.id === pedida) ?? null;
  const abrir = (id: string | null) => {
    const next = new URLSearchParams(params);
    if (id) next.set('seccion', id);
    else next.delete('seccion');
    setParams(next, { replace: true });
  };
  const elegirVista = (v: VistaDeAjustes) => {
    setVista(v);
    guardarVista(v);
  };

  return (
    <Flex direction="column" h="100%" minH={0}>
      <HStack justify="space-between" px={6} py={2} minH="60px" borderBottomWidth="1px" borderColor="border" flexShrink={0}>
        <Heading size="lg">Ajustes del negocio</Heading>
        <HStack gap={1} role="radiogroup" aria-label="Vista de los ajustes">
          {VISTAS.map((v) => {
            const I = v.icon;
            const on = v.id === vista;
            return (
              <Button key={v.id} role="radio" aria-checked={on} minH="44px" size="sm" gap={2}
                variant={on ? 'subtle' : 'ghost'} colorPalette={on ? undefined : 'gray'} onClick={() => elegirVista(v.id)}>
                <I /> {v.label}
              </Button>
            );
          })}
        </HStack>
      </HStack>
      <Box flex="1" minH={0} overflowY={vista === 'indice' ? 'auto' : undefined}>
        {vista === 'indice'
          ? <Indice secciones={secciones} abierta={abierta} abrir={abrir} />
          : <Lateral secciones={secciones} abierta={abierta ?? secciones[0]} abrir={abrir} compacto={vista === 'iconos'} />}
      </Box>
    </Flex>
  );
}

function Lateral({ secciones, abierta, abrir, compacto }: {
  secciones: SeccionDeAjustes[]; abierta: SeccionDeAjustes | undefined; abrir: (id: string) => void; compacto: boolean;
}) {
  return (
    <Flex h="100%" minH={0}>
      <VStack as="nav" aria-label="Secciones de ajustes" align="stretch" gap={1} flexShrink={0}
        w={compacto ? '104px' : '224px'} borderRightWidth="1px" borderColor="border" p={2} pt={4} overflowY="auto">
        {secciones.map((s) => {
          const I = s.icon;
          const on = s.id === abierta?.id;
          return compacto ? (
            <Button key={s.id} aria-current={on ? 'page' : undefined} variant={on ? 'subtle' : 'ghost'} colorPalette={on ? undefined : 'gray'}
              h="auto" minH="64px" py={2} flexDirection="column" gap={1} whiteSpace="normal" onClick={() => abrir(s.id)}>
              <I size={20} />
              <Text fontSize="xs" lineHeight="1.15" textAlign="center">{s.label}</Text>
            </Button>
          ) : (
            <Button key={s.id} aria-current={on ? 'page' : undefined} variant={on ? 'subtle' : 'ghost'} colorPalette={on ? undefined : 'gray'}
              minH="48px" justifyContent="start" gap={3} onClick={() => abrir(s.id)}>
              <I size={18} />
              <Text>{s.label}</Text>
            </Button>
          );
        })}
      </VStack>
      <Box flex="1" minW={0} overflowY="auto">
        {abierta && (
          <Page maxW="640px">
            <Heading size="md" mb={4}>{abierta.label}</Heading>
            {abierta.contenido}
          </Page>
        )}
      </Box>
    </Flex>
  );
}

function Indice({ secciones, abierta, abrir }: {
  secciones: SeccionDeAjustes[]; abierta: SeccionDeAjustes | null; abrir: (id: string | null) => void;
}) {
  if (abierta) {
    return (
      <Page maxW="640px">
        <Button variant="ghost" colorPalette="gray" minH="44px" px={2} mb={2} onClick={() => abrir(null)}>
          <LuChevronLeft /> Todas las secciones
        </Button>
        <Heading size="md" mb={4}>{abierta.label}</Heading>
        {abierta.contenido}
      </Page>
    );
  }
  return (
    <Page maxW="900px">
      <SimpleGrid columns={{ base: 1, md: 2 }} gap={3}>
        {secciones.map((s) => {
          const I = s.icon;
          return (
            <Button key={s.id} variant="outline" colorPalette="gray" h="auto" minH="88px" p={4}
              justifyContent="space-between" whiteSpace="normal" textAlign="left" onClick={() => abrir(s.id)}>
              <HStack gap={3} align="start">
                <Box color="brand.solid" pt={1}><I size={22} /></Box>
                <Box>
                  <Text fontWeight="700" color="fg">{s.label}</Text>
                  <Text fontSize="sm" color="fg.muted" fontWeight="normal">{s.resumen}</Text>
                </Box>
              </HStack>
              <LuChevronRight />
            </Button>
          );
        })}
      </SimpleGrid>
    </Page>
  );
}
