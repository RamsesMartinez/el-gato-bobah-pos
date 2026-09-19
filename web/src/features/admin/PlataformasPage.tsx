import { useCallback, useEffect, useRef, useState } from 'react';
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
  tiendasDisponibles,
  type ConexionDePlataforma,
  type TiendaDePlataforma,
} from '../../api/plataformas';
import { MenuDePlataformaPage } from './MenuDePlataformaPage';
import { useMenu } from '../../hooks/useMenu';

/**
 * Las tiendas conectadas: alta, baja y el acceso a lo demás.
 *
 * Sin esta pantalla la feature no existe — no había forma de dar de alta una conexión, así que la
 * lista de diferencias se quedaba en «todavía no hay ninguna tienda» para siempre.
 */

function Alta({ onListo }: { onListo: () => void }) {
  // LA TIENDA SE ELIGE, NO SE TECLEA.
  //
  // El identificador es un UUID —`3806dacf-965d-4b09-91b6-6d40ad5e15db`— y es el único dato del
  // alta que una persona no puede producir de memoria ni deducir. Pedirlo escrito es lo que deja
  // fuera a quien nunca ha usado el sistema: no sabe qué es, ni de dónde sacarlo, ni si lo copió
  // bien. Se piden a la plataforma y se muestran por nombre y ciudad.
  const [tiendas, setTiendas] = useState<TiendaDePlataforma[] | null>(null);
  const [fallaTiendas, setFallaTiendas] = useState(false);
  // LAS PLATAFORMAS SE LEEN DEL MENÚ, no se escriben aquí.
  //
  // `delivery_platforms.id` es POR EMPRESA: en el respaldo de producción, Uber Eats es el 2 para
  // una empresa y el 6 para otra. Una lista fija en el front manda el id de la empresa equivocada;
  // la FK compuesta del esquema lo rechaza —falla seguro, no conecta mal— pero el formulario deja
  // de servir y el mensaje no dice por qué.
  //
  // «Propio» queda fuera: es reparto del propio negocio, sin menú publicado que leer.
  const { data: menu } = useMenu();
  const plataformas: PickerOption[] = (menu?.platforms ?? [])
    .filter((p) => p.name !== 'Propio')
    .map((p) => ({ value: String(p.id), label: p.name }));

  const [platformId, setPlatformId] = useState('');
  const [storeId, setStoreId] = useState('');
  const [label, setLabel] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [manual, setManual] = useState(false);
  const [guardando, setGuardando] = useState(false);

  // Se piden al elegir la app, no al abrir la pantalla: sin app no hay a quién preguntarle. Y se
  // piden desde el toque, no desde un efecto, porque es consecuencia de lo que hizo el operador.
  //
  // `pedidoPara` descarta la respuesta de una app que ya no está elegida: cambiar de app dos veces
  // seguidas puede hacer que la primera respuesta llegue al final y deje en pantalla las tiendas de
  // la otra app, con sus nombres, listas para conectarse a la equivocada.
  const pedidoPara = useRef('');
  const elegirPlataforma = (id: string) => {
    setPlatformId(id);
    setStoreId('');
    setManual(false);
    setTiendas(null);
    setFallaTiendas(false);
    pedidoPara.current = id;
    if (!id) return;
    tiendasDisponibles(Number(id))
      .then((s) => {
        if (pedidoPara.current === id) setTiendas(s);
      })
      .catch(() => {
        if (pedidoPara.current === id) setFallaTiendas(true);
      });
  };

  const yaConectada = !!tiendas?.find((s) => s.externalStoreId === storeId)?.alreadyAdded;

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
          options={plataformas}
          onChange={elegirPlataforma}
          placeholder="¿De qué app?"
          title="Aplicación de reparto"
          size="lg"
        />
        {platformId && (
          <VStack align="stretch" gap={1}>
            <Text fontSize="sm" color="fg.muted">
              ¿Cuál de tus tiendas?
            </Text>
            {tiendas === null && !fallaTiendas && (
              <Text fontSize="sm" color="fg.muted">
                Buscando tus tiendas en la app…
              </Text>
            )}
            {tiendas !== null && tiendas.length > 0 && (
              <Picker
                value={storeId}
                options={tiendas.map((s) => ({
                  value: s.externalStoreId,
                  label: s.city ? `${s.name} — ${s.city}` : s.name,
                  // Marcada, NO escondida: quien la busca y no la encuentra cree que se equivocó
                  // de tienda en vez de entender que ya estaba.
                  hint: s.alreadyAdded ? 'Ya conectada' : undefined,
                }))}
                onChange={setStoreId}
                placeholder="Elige tu tienda…"
                title="Tus tiendas"
                size="lg"
              />
            )}
            {/* Se DEJA elegir y se dice por qué no se puede conectar. Ignorar el toque en silencio
                deja al operador tocando la misma fila sin entender qué pasa. */}
            {yaConectada && <Text color="fg.muted">Esa tienda ya está conectada.</Text>}
            {tiendas !== null && tiendas.length === 0 && (
              <Text fontSize="sm" color="fg.muted">
                Esa app no reporta ninguna tienda para este negocio.
              </Text>
            )}
            {/* La escritura a mano queda como salida, no como el camino: si la lista no carga, el
                alta no puede quedar bloqueada. */}
            {(fallaTiendas || manual) && (
              <>
                <Text fontSize="sm" color="fg.muted">
                  {fallaTiendas
                    ? 'No se pudo traer la lista. Escribe el ID de la tienda:'
                    : 'ID de la tienda:'}
                </Text>
                <Input value={storeId} onChange={(e) => setStoreId(e.target.value)} minH="44px" />
              </>
            )}
            {!fallaTiendas && !manual && (
              <Button variant="ghost" minH="44px" alignSelf="flex-start" onClick={() => setManual(true)}>
                <LuCircleHelp /> No veo mi tienda
              </Button>
            )}
          </VStack>
        )}
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
          disabled={!platformId || !storeId.trim() || !label.trim() || yaConectada}
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
  const [abrirAlta, setAbrirAlta] = useState(false);
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
              <Button
                minH="44px"
                variant="ghost"
                colorPalette="red"
                aria-label={`Desconectar ${c.platformName} ${c.label}`}
                onClick={() => preguntar(c)}
              >
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

        {/* EL FORMULARIO NO OCUPA ALTO CUANDO NO SE VA A USAR. Conectar una tienda se hace una vez
            por sucursal; revisar diferencias, todos los días. Con el formulario siempre desplegado,
            la tarea diaria empuja al botón de alta fuera de la pantalla y la ocasional le cobra
            250 px a la que sí se usa. Con la primera tienda no hay nada más que hacer en esta
            pantalla, así que ahí va abierto. */}
        {conexiones.length === 0 || abrirAlta ? (
          <Alta onListo={() => { setAbrirAlta(false); cargar(); }} />
        ) : (
          <Button minH="44px" variant="outline" alignSelf="flex-start" onClick={() => setAbrirAlta(true)}>
            <LuPlus /> Conectar otra tienda
          </Button>
        )}
      </VStack>
    </Page>
  );
}
