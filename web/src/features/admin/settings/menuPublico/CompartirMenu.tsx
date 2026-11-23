import { useEffect, useState } from 'react';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuCheck, LuCopy, LuDownload, LuExternalLink, LuPrinter, LuShare2 } from 'react-icons/lu';
import type { PublicMenuSettings } from '../../../../api/admin';
import { descargarPng, descargarSvg, imprimirTarjeta, qrSvg } from './qr';

const sinEsquema = (u: string) => u.replace(/^https?:\/\//, '');

// CompartirMenu es la tarjeta de arriba de Venta en web (maqueta B): lo que se hace todos los días
// —copiar el enlace, compartirlo, descargar el QR— a un toque. El QR codifica la dirección del código
// estable, no la del nombre: un QR impreso sigue sirviendo aunque el nombre cambie.
export function CompartirMenu({ menu }: { menu: PublicMenuSettings }) {
  const [svg, setSvg] = useState('');
  const [copiado, setCopiado] = useState(false);
  useEffect(() => {
    let vivo = true;
    qrSvg(menu.qrUrl).then((s) => { if (vivo) setSvg(s); }).catch(() => {});
    return () => { vivo = false; };
  }, [menu.qrUrl]);

  const copiar = async () => {
    try {
      await navigator.clipboard.writeText(menu.url);
      setCopiado(true);
      setTimeout(() => setCopiado(false), 2500);
    } catch {
      // Sin portapapeles (navegador viejo) el enlace queda a la vista para copiarlo a mano.
    }
  };
  const compartir = async () => {
    if (navigator.share) {
      try {
        await navigator.share({ title: menu.businessName, text: `Menú de ${menu.businessName}`, url: menu.url });
        return;
      } catch {
        return; // Cancelar el menú de compartir no es un error.
      }
    }
    await copiar();
  };
  const archivo = `qr-${menu.name}`;

  return (
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
      <HStack gap={5} align="start">
        <Box w="144px" h="144px" flexShrink={0} borderWidth="1px" borderColor="border" borderRadius="lg" overflow="hidden" bg="white"
          aria-label="Código QR del menú" role="img" css={{ '& svg': { width: '100%', height: '100%' } }}
          dangerouslySetInnerHTML={{ __html: svg }} />
        <VStack align="stretch" gap={3} flex="1" minW={0}>
          <Text fontWeight="700">Comparte tu menú</Text>
          <Text fontSize="lg" fontWeight="700" wordBreak="break-all">{sinEsquema(menu.url)}</Text>
          <HStack gap={2} flexWrap="wrap">
            <Button minH="44px" onClick={copiar}>{copiado ? <LuCheck /> : <LuCopy />} {copiado ? 'Copiado' : 'Copiar enlace'}</Button>
            <Button minH="44px" variant="outline" onClick={compartir}><LuShare2 /> Compartir</Button>
            <Button minH="44px" variant="outline" asChild><a href={menu.url} target="_blank" rel="noreferrer"><LuExternalLink /> Abrir</a></Button>
          </HStack>
          <HStack gap={2} flexWrap="wrap">
            <Button minH="44px" variant="outline" disabled={!svg} onClick={() => descargarPng(svg, `${archivo}.png`)}><LuDownload /> PNG</Button>
            <Button minH="44px" variant="outline" disabled={!svg} onClick={() => descargarSvg(svg, `${archivo}.svg`)}><LuDownload /> SVG</Button>
            <Button minH="44px" variant="outline" disabled={!svg} onClick={() => imprimirTarjeta(svg, menu.businessName, sinEsquema(menu.url))}><LuPrinter /> Imprimir tarjeta</Button>
          </HStack>
        </VStack>
      </HStack>
    </Box>
  );
}
