import { useCallback, useEffect, useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuCircleHelp, LuLink2, LuPlus, LuTrash2 } from 'react-icons/lu';
import { useNavigate } from 'react-router';
import { Page } from '../../components/Page';
import { Picker, type PickerOption } from '../../components/Picker';
import {
  borrarConexion,
  crearConexion,
  listarConexiones,
  parejasDeLaConexion,
  type ConexionDePlataforma,
} from '../../api/plataformas';
import { MenuDePlataformaPage } from './MenuDePlataformaPage';

/**
 * Las tiendas conectadas: alta, baja y el acceso a lo demás.
 *
 * Sin esta pantalla la feature no existe — no había forma de dar de alta una conexión, así que la
 * lista de diferencias se quedaba en «todavía no hay ninguna tienda» para siempre.
 */

// Las plataformas del catálogo. Son fijas y su id viene de `delivery_platforms`; se piden al
// servidor junto con las conexiones para no adivinar ids que son por empresa.
const PLATAFORMAS: PickerOption[] = [
  { value: '1', label: 'Didi' },
  { value: '2', label: 'Uber Eats' },
  { value: '3', label: 'Rappi' },
];

function Alta({ onListo }: { onListo: () => void }) {
  const [platformId, setPlatformId] = useState('');
  const [storeId, setStoreId] = useState('');
  const [label, setLabel] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [ayuda, setAyuda] = useState(false);
  const [guardando, setGuardando] = useState(false);

  const guardar = async () => {
    setGuardando(true);
    setError(null);
    try {
      await crearConexion({ platformId: Number(platformId), externalStoreId: storeId.trim(), label: label.trim() });
      setPlatformId('');
      setStoreId('');
      setLabel('');
      onListo();
    } catch {
      setError('No se pudo dar de alta. Revisa que la tienda no esté registrada ya.');
    } finally {
      setGuardando(false);
    }
  };

  return (
    <Box borderWidth="1px" borderRadius="md" p={4}>
      <Text fontWeight="semibold" mb={3}>
        Conectar una tienda
      </Text>
      <VStack align="stretch" gap={3}>
        <Picker
          value={platformId}
          options={PLATAFORMAS}
          onChange={setPlatformId}
          placeholder="¿De qué app?"
          title="Aplicación de reparto"
          size="lg"
        />
        <VStack align="stretch" gap={1}>
          <HStack>
            <Text fontSize="sm" color="fg.muted">
              ID de tienda en la app
            </Text>
            {/* El detalle operativo va detrás de un icono de ayuda, no en la etiqueta. */}
            <Button variant="ghost" size="xs" minH="44px" onClick={() => setAyuda((v) => !v)}>
              <LuCircleHelp />
            </Button>
          </HStack>
          {ayuda && (
            <Text fontSize="sm" color="fg.muted">
              Lo da la aplicación cuando conectas el punto de venta. Cópialo tal cual: no se escribe
              a mano.
            </Text>
          )}
          <Input value={storeId} onChange={(e) => setStoreId(e.target.value)} minH="44px" />
        </VStack>
        <VStack align="stretch" gap={1}>
          <Text fontSize="sm" color="fg.muted">
            Cómo la vas a llamar
          </Text>
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Sucursal Centro"
            minH="44px"
          />
        </VStack>
        {error && <Text color="red.600">{error}</Text>}
        <Button
          minH="44px"
          onClick={guardar}
          loading={guardando}
          disabled={!platformId || !storeId.trim() || !label.trim()}
          alignSelf="flex-start"
        >
          <LuPlus /> Conectar
        </Button>
      </VStack>
    </Box>
  );
}

export function PlataformasPage() {
  const [conexiones, setConexiones] = useState<ConexionDePlataforma[] | null>(null);
  const [porBorrar, setPorBorrar] = useState<{ c: ConexionDePlataforma; parejas: number } | null>(null);
  const navegar = useNavigate();

  const cargar = useCallback(() => {
    listarConexiones()
      .then(setConexiones)
      .catch(() => setConexiones([]));
  }, []);

  useEffect(cargar, [cargar]);

  // El borrado se lleva las lecturas Y el emparejamiento. Se dice CUÁNTAS parejas se pierden antes
  // de confirmar: son decisiones manuales de una sesión completa y no se reconstruyen.
  const preguntar = async (c: ConexionDePlataforma) => {
    const parejas = await parejasDeLaConexion(c.id).catch(() => 0);
    setPorBorrar({ c, parejas });
  };

  const confirmarBorrado = async () => {
    if (!porBorrar) return;
    await borrarConexion(porBorrar.c.id).catch(() => undefined);
    setPorBorrar(null);
    cargar();
  };

  if (conexiones === null) return <Page><Text color="fg.muted">Cargando…</Text></Page>;

  return (
    <Page>
      <VStack align="stretch" gap={4}>
        {conexiones.length > 0 && <MenuDePlataformaPage />}

        <Text fontWeight="bold">Tiendas conectadas</Text>
        {conexiones.length === 0 && (
          <Text color="fg.muted">Todavía no hay ninguna. Conecta la primera abajo.</Text>
        )}
        {conexiones.map((c) => (
          <HStack key={c.id} justify="space-between" borderBottomWidth="1px" py={3} wrap="wrap" gap={2}>
            <VStack align="start" gap={0}>
              <Text fontWeight="semibold">
                {c.platformName} · {c.label}
              </Text>
              <Text fontSize="sm" color="fg.muted">
                {c.credentialsConfigured ? 'Conectada' : 'Falta configurar el acceso'}
              </Text>
            </VStack>
            <HStack gap={2}>
              <Button minH="44px" variant="outline" onClick={() => navegar(`/plataformas/${c.id}/emparejar`)}>
                <LuLink2 /> Emparejar
              </Button>
              <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => preguntar(c)}>
                <LuTrash2 />
              </Button>
            </HStack>
          </HStack>
        ))}

        {porBorrar && (
          <Box borderWidth="1px" borderColor="red.400" borderRadius="md" p={4}>
            <Text fontWeight="semibold">
              ¿Desconectar {porBorrar.c.platformName} · {porBorrar.c.label}?
            </Text>
            <Text color="fg.muted" mt={1}>
              {porBorrar.parejas > 0
                ? `Se pierden ${porBorrar.parejas} platillos ya emparejados y hay que volver a hacerlos uno por uno.`
                : 'No hay platillos emparejados que perder.'}
            </Text>
            <HStack mt={3} gap={2}>
              <Button minH="44px" variant="outline" onClick={() => setPorBorrar(null)}>
                Mejor no
              </Button>
              <Button minH="44px" colorPalette="red" onClick={confirmarBorrado}>
                Desconectar
              </Button>
            </HStack>
          </Box>
        )}

        <Alta onListo={cargar} />
      </VStack>
    </Page>
  );
}
