import { useCallback, useEffect, useState } from 'react';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuRefreshCw } from 'react-icons/lu';
import { Page } from '../../components/Page';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import {
  diferencias,
  leerMenu,
  listarConexiones,
  TEXTO_DE_FALLO,
  type ClaseDeDiferencia,
  type Comparacion,
  type ConexionDePlataforma,
  type Diferencia,
} from '../../api/plataformas';

/**
 * En qué difiere el menú publicado del catálogo del POS.
 *
 * DOS DECISIONES DE DISPOSICIÓN que salen del tamaño real de la pantalla (~900 × 552 px útiles):
 *
 *  - **Abre solo con lo accionable**: precio y disponibilidad. Los «solo en un lado» viven en otra
 *    pestaña, porque el POS tiene 174 productos activos y la plataforma 65 platillos: la mayoría de
 *    esa diferencia nunca se va a emparejar a propósito, y mezclarla ahoga todos los días lo que sí
 *    hay que corregir.
 *  - **El nombre va apilado, no en columna de ancho fijo.** Con cinco columnas en 900 px, cada
 *    nombre queda en ~200 y se trunca justo por donde se distinguen.
 */

const ACCIONABLES: ClaseDeDiferencia[] = ['precio', 'disponibilidad'];
const DE_UN_LADO: ClaseDeDiferencia[] = ['solo_en_plataforma', 'solo_en_catalogo'];

/** La diferencia, redactada para quien opera. El orden de los dos importes no es cosmético: manda
 *  lo publicado, y la acción normal es actualizar el POS. */
function RenglonDeDiferencia({ d }: { d: Diferencia }) {
  if (d.kind === 'precio') {
    return (
      <Box borderBottomWidth="1px" py={3}>
        <Text fontWeight="semibold">{d.platformName}</Text>
        <Text color="fg.muted" fontSize="sm">
          En la app cuesta <b>${d.platformPrice}</b> y en el sistema está en ${d.catalogPrice} — «
          {d.localName}»
        </Text>
      </Box>
    );
  }
  if (d.kind === 'disponibilidad') {
    return (
      <Box borderBottomWidth="1px" py={3}>
        <Text fontWeight="semibold">{d.platformName}</Text>
        <Text color="fg.muted" fontSize="sm">
          {d.platformAvailable
            ? 'Se vende en la app y está inactivo en el sistema'
            : 'Está agotado en la app y activo en el sistema'}{' '}
          — «{d.localName}»
        </Text>
      </Box>
    );
  }
  if (d.kind === 'solo_en_plataforma') {
    return (
      <Box borderBottomWidth="1px" py={3}>
        <Text fontWeight="semibold">{d.platformName}</Text>
        <Text color="fg.muted" fontSize="sm">
          Está publicado y no tiene pareja en el sistema
        </Text>
      </Box>
    );
  }
  return (
    <Box borderBottomWidth="1px" py={3}>
      <Text fontWeight="semibold">{d.localName}</Text>
      <Text color="fg.muted" fontSize="sm">
        Está en el sistema y no está publicado
      </Text>
    </Box>
  );
}

export function MenuDePlataformaPage() {
  // El formateador único: fechas pintadas aquí saldrían en la zona del navegador, y la tableta del
  // mostrador no tiene por qué estar en la del negocio.
  const horaNegocio = useHoraDelNegocio();
  const [conexiones, setConexiones] = useState<ConexionDePlataforma[]>([]);
  const [activa, setActiva] = useState<number | null>(null);
  const [pestana, setPestana] = useState<'accionable' | 'unLado'>('accionable');
  const [cmp, setCmp] = useState<Comparacion | null>(null);
  const [aviso, setAviso] = useState<string | null>(null);
  const [leyendo, setLeyendo] = useState(false);

  useEffect(() => {
    listarConexiones().then((cs) => {
      setConexiones(cs);
      if (cs.length > 0) setActiva(cs[0].id);
    });
  }, []);

  const conexion = conexiones.find((c) => c.id === activa) ?? null;

  // Todo el estado se fija en la continuación de la promesa, no en el cuerpo del efecto: hacerlo
  // síncrono encadena renders y la regla `react-hooks/set-state-in-effect` lo bloquea. `vivo` evita
  // pintar una respuesta que llegó después de cambiar de pestaña.
  const cargar = useCallback(() => {
    if (activa == null) return () => {};
    let vivo = true;
    diferencias(activa, pestana === 'accionable' ? ACCIONABLES : DE_UN_LADO)
      .then((c) => {
        if (!vivo) return;
        setCmp(c);
        setAviso(null);
      })
      .catch(() => {
        // Sin mensaje propio: el encabezado ya dice el motivo concreto —nunca se ha leído, falló
        // por tal cosa, se está leyendo— y apilar uno genérico encima le resta claridad al bueno.
        if (!vivo) return;
        setCmp(null);
      });
    return () => {
      vivo = false;
    };
  }, [activa, pestana]);

  useEffect(cargar, [cargar]);

  const pedirLectura = async () => {
    if (activa == null) return;
    setLeyendo(true);
    try {
      await leerMenu(activa);
      // Responde de inmediato; la lectura corre aparte. Se vuelve a pedir la comparación en unos
      // segundos en vez de bloquear la pantalla esperándola.
      setAviso('Leyendo el menú de la app…');
      setTimeout(() => {
        listarConexiones().then(setConexiones);
        cargar();
      }, 4000);
    } catch {
      setAviso('No se pudo pedir la lectura.');
    } finally {
      setLeyendo(false);
    }
  };

  if (conexiones.length === 0) {
    return (
      <Page>
        <Text>Todavía no hay ninguna tienda dada de alta.</Text>
      </Page>
    );
  }

  const ultima = conexion?.lastRead;

  return (
    <Page>
      <VStack align="stretch" gap={3}>
        {/* Sin este selector, la segunda sucursal es invisible: `activa` se fijaba a la primera y
            nunca cambiaba. El modelo soporta varias tiendas por plataforma desde el día uno. */}
        {conexiones.length > 1 && (
          <HStack gap={2} wrap="wrap">
            {conexiones.map((c) => (
              <Button
                key={c.id}
                minH="44px"
                variant={c.id === activa ? 'solid' : 'outline'}
                onClick={() => setActiva(c.id)}
              >
                {c.platformName} · {c.label}
              </Button>
            ))}
          </HStack>
        )}

        <HStack justify="space-between" wrap="wrap" gap={2}>
          <VStack align="start" gap={0}>
            <Text fontWeight="bold">
              {conexion?.platformName} · {conexion?.label}
            </Text>
            {/* De cuándo es el dato, SIEMPRE. Sin esto la pantalla empieza a mentir el segundo día. */}
            <Text fontSize="sm" color="fg.muted">
              {!conexion?.credentialsConfigured
                ? 'Esta tienda todavía no está conectada.'
                : !ultima
                  ? 'Nunca se ha leído el menú de esta tienda.'
                  : ultima.status === 'en_curso'
                    ? 'Leyendo el menú…'
                    : ultima.status === 'fallida'
                      ? `${ultima.failureKind ? TEXTO_DE_FALLO[ultima.failureKind] : 'La última lectura falló.'} Último dato: ${horaNegocio.fechaYHora(ultima.startedAt)}`
                      : `Leído el ${horaNegocio.fechaYHora(ultima.startedAt)}`}
            </Text>
          </VStack>
          <Button
            minH="44px"
            onClick={pedirLectura}
            loading={leyendo}
            disabled={!conexion?.credentialsConfigured}
          >
            <LuRefreshCw /> Leer ahora
          </Button>
        </HStack>

        {ultima?.stale && (
          <Badge colorPalette="orange" alignSelf="flex-start">
            Este dato ya está viejo
          </Badge>
        )}

        <HStack gap={2}>
          <Button
            minH="44px"
            variant={pestana === 'accionable' ? 'solid' : 'outline'}
            onClick={() => setPestana('accionable')}
          >
            Por corregir
          </Button>
          <Button
            minH="44px"
            variant={pestana === 'unLado' ? 'solid' : 'outline'}
            onClick={() => setPestana('unLado')}
          >
            Solo en un lado{cmp ? ` (${cmp.unpaired})` : ''}
          </Button>
        </HStack>

        {pestana === 'unLado' && (
          <Text fontSize="sm" color="fg.muted">
            La mayoría de estos no son errores: hay productos que no se venden en la app a propósito,
            y platillos de la app que todavía no tienen pareja aquí.
          </Text>
        )}

        {aviso && <Text color="fg.muted">{aviso}</Text>}

        {cmp && cmp.differences.length === 0 && !aviso && (
          <Text color="fg.muted">
            {pestana === 'accionable'
              ? 'No hay diferencias de precio ni de disponibilidad.'
              : 'Todo lo publicado tiene pareja en el sistema.'}
          </Text>
        )}

        {cmp && (
          <Box overflowY="auto">
            {cmp.differences.map((d) => (
              <RenglonDeDiferencia key={`${d.kind}-${d.externalId ?? d.localId}`} d={d} />
            ))}
          </Box>
        )}
      </VStack>
    </Page>
  );
}
