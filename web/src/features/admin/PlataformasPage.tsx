import { useCallback, useEffect, useRef, useState } from 'react';
import { Box, Button, HStack, IconButton, Input, Text, VStack } from '@chakra-ui/react';
import { LuCircleHelp, LuKeyRound, LuLink2, LuPlug, LuPlus, LuTrash2 } from 'react-icons/lu';
import { useNavigate } from 'react-router';
import { Page } from '../../components/Page';
import { Picker, type PickerOption } from '../../components/Picker';
import {
  borrarConexion,
  crearConexion,
  getCredentialsState,
  estadoDeLlave,
  saveCredentials,
  guardarLlave,
  listarConexiones,
  parejasDeLaConexion,
  retirarLlaveAnterior,
  tiendasDisponibles,
  type ConexionDePlataforma,
  type CredentialsState,
  type EstadoDeLlave,
  type TiendaDePlataforma,
} from '../../api/plataformas';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogFooter } from '../../components/ui/dialog';
import { MenuDePlataformaPage } from './MenuDePlataformaPage';
import { useMenu } from '../../hooks/useMenu';
import { ApiError } from '../../api/client';
import { useSessionStore } from '../../stores/session';

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
  // Sin acceso a la app no hay a quién pedirle la lista. Ofrecer «escribe el id a mano» en ese caso
  // manda a buscar un UUID que nadie sabe de dónde sacar, cuando lo que falta es conectar la app.
  const [noAccess, setNoAccess] = useState(false);
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
    setNoAccess(false);
    pedidoPara.current = id;
    if (!id) return;
    tiendasDisponibles(Number(id))
      .then((s) => {
        if (pedidoPara.current === id) setTiendas(s);
      })
      .catch((e: unknown) => {
        if (pedidoPara.current !== id) return;
        const codigo = e instanceof ApiError ? e.code : '';
        if (codigo === 'PLATFORM_NOT_CONFIGURED' || codigo === 'PLATFORM_CREDENTIALS_UNREADABLE') setNoAccess(true);
        else setFallaTiendas(true);
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
          placeholder="¿De qué plataforma?"
          title="Plataforma de reparto"
          size="lg"
        />
        {platformId && (
          <VStack align="stretch" gap={1}>
            <Text fontSize="sm" color="fg.muted">
              ¿Cuál de tus tiendas?
            </Text>
            {tiendas === null && !fallaTiendas && !noAccess && (
              <Text fontSize="sm" color="fg.muted">
                Buscando tus tiendas en la app…
              </Text>
            )}
            {noAccess && (
              <Text color="orange.600">
                Primero conecta con {plataformas.find((p) => p.value === platformId)?.label} en «Conexión con las plataformas».
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
            {!fallaTiendas && !manual && !noAccess && (
              <Button variant="ghost" minH="44px" alignSelf="flex-start" onClick={() => setManual(true)}>
                <LuCircleHelp /> No veo mi tienda
              </Button>
            )}
          </VStack>
        )}
        {/* Sin conexión con la plataforma el alta no se puede terminar: el nombre y el botón esperan,
            y no le quitan a la tableta el alto que el formulario de arriba necesita. */}
        {!noAccess && (
        <>
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
        </>
        )}
      </VStack>
    </Box>
  );
}

/**
 * El acceso a la app que el negocio registró en la plataforma: su Client ID y su Client Secret.
 *
 * Va por APP y antes que cualquier tienda: sin él no se puede ni pedir la lista de tiendas. Se
 * comprueba con la plataforma al guardar, y si no lo acepta no se guarda nada — un dedazo guardado
 * se descubriría con un pedido real esperando.
 *
 * EL SECRETO NO VUELVE A LA PANTALLA: el servidor solo dice cuál app (el Client ID no es secreto) y
 * si se puede usar. Con él se aceptan pedidos a nombre del negocio.
 */
function PlatformConnection({ platformId, platformName }: { platformId: number; platformName: string }) {
  const [estado, setEstado] = useState<CredentialsState | null>(null);
  const [editando, setEditando] = useState(false);
  const [clientId, setClientId] = useState('');
  const [secreto, setSecreto] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [guardando, setGuardando] = useState(false);
  const [ayuda, setAyuda] = useState(false);
  // El servidor solo deja escribir al administrador; el botón no se le ofrece a quien lo vería fallar.
  const canEdit = useSessionStore((s) => s.user?.role) === 'admin';

  const cargar = useCallback(() => {
    getCredentialsState(platformId).then(setEstado).catch(() => setEstado(null));
  }, [platformId]);
  useEffect(cargar, [cargar]);

  // Con el nombre en inglés al lado: es lo que se lee en el tablero de la plataforma.
  const app = estado?.environment === 'sandbox' ? 'Pruebas (Testing)' : 'Producción (Production)';

  const cerrar = () => {
    setClientId('');
    setSecreto('');
    setEditando(false);
    setError(null);
  };

  const guardar = async () => {
    setGuardando(true);
    setError(null);
    try {
      await saveCredentials(platformId, { clientId, clientSecret: secreto });
      cerrar();
      cargar();
    } catch (e) {
      // Lo tecleado SE QUEDA: corregir un campo no debe obligar a volver a copiar los dos.
      setError(rejectionMessage(e, platformName, app));
    } finally {
      setGuardando(false);
    }
  };

  if (!estado?.available) return null;

  const missing = !estado.configured || estado.needsRecapture;
  const summary = !estado.configured
    ? 'Falta conectar'
    : estado.needsRecapture
      ? 'Las credenciales guardadas no sirven en este sistema · vuelve a capturarlas'
      : `Conectada · Client ID termina en ${(estado.clientId ?? '').slice(-4)}`;
  const action = !estado.configured ? `Conectar con ${platformName}` : estado.needsRecapture ? 'Volver a capturar' : 'Cambiar credenciales';

  return (
    <Box borderBottomWidth="1px" py={3}>
      <HStack justify="space-between" wrap="wrap" gap={2}>
        <HStack gap={1}>
          <Text fontWeight="semibold">{platformName}</Text>
          <Text color={missing ? 'orange.600' : 'fg.muted'}>· {summary}</Text>
          <IconButton aria-label={`Dónde encontrar las credenciales de ${platformName}`} size="sm" minH="44px" minW="44px" variant="ghost"
            onClick={() => setAyuda(true)}>
            <LuCircleHelp />
          </IconButton>
        </HStack>
        {!editando && canEdit && (
          <Button minH="44px" variant={missing ? 'solid' : 'outline'} onClick={() => setEditando(true)}>
            <LuPlug /> {action}
          </Button>
        )}
        {missing && !canEdit && (
          <Text fontSize="sm" color="fg.muted">Pídele a un administrador que lo conecte.</Text>
        )}
      </HStack>

      {editando && (
        <VStack align="stretch" gap={2} mt={2}>
          {/* Etiqueta a la vista y no solo placeholder: al pegar, el placeholder desaparece y ya no
              se sabe cuál campo es cuál. Los nombres van en inglés porque así los muestra el tablero. */}
          <Text fontSize="sm" color="fg.muted">Client ID</Text>
          <Input
            autoComplete="off"
            spellCheck={false}
            aria-label={`Client ID de ${platformName}`}
            placeholder="Client ID"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
            minH="44px"
          />
          <Text fontSize="sm" color="fg.muted">Client Secret</Text>
          <Input
            type="password"
            autoComplete="off"
            aria-label={`Client Secret de ${platformName}`}
            placeholder="Client Secret"
            value={secreto}
            onChange={(e) => setSecreto(e.target.value)}
            minH="44px"
          />
          {error && <Text color="red.600">{error}</Text>}
          <HStack gap={2}>
            <Button minH="44px" variant="outline" onClick={cerrar}>
              Cancelar
            </Button>
            <Button minH="44px" onClick={guardar} loading={guardando} loadingText={`Comprobando con ${platformName}…`}
              disabled={!clientId.trim() || !secreto.trim()}>
              Comprobar y guardar
            </Button>
          </HStack>
        </VStack>
      )}

      <DialogRoot open={ayuda} onOpenChange={(e) => setAyuda(e.open)}>
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Dónde encontrar tus credenciales</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <VStack align="stretch" gap={2}>
              <Text color="fg.muted">
                Necesitas una app de desarrollador de {platformName} registrada para tu negocio. Si no la
                tienes, pídesela a quien te instaló el sistema.
              </Text>
              <Text>1. Entra a developer.uber.com con la cuenta del negocio.</Text>
              <Text>2. Abre tu app de la suite Eats Marketplace de tipo {app}.</Text>
              <Text>3. Revisa que tenga activados los permisos eats.store y eats.order.</Text>
              <Text>4. Copia el Client ID y pégalo en su campo.</Text>
              <Text>5. Copia el Client Secret. Solo se muestra al generarlo: si no lo guardaste, genera uno nuevo.</Text>
            </VStack>
          </DialogBody>
          <DialogFooter>
            <Button minH="44px" onClick={() => setAyuda(false)}>
              Entendido
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </Box>
  );
}

/** Qué corregir, en palabras de quien opera. Cada rechazo de la plataforma pide algo distinto. */
function rejectionMessage(e: unknown, platform: string, appKind: string): string {
  switch (e instanceof ApiError ? e.code : '') {
    case 'PLATFORM_CREDENTIALS_REJECTED':
      return `${platform} no reconoce ese Client ID y Client Secret. Revisa que estén completos y que sean de la app de ${appKind}.`;
    case 'PLATFORM_CREDENTIALS_MISSING_SCOPES':
      return `La app de ${platform} no tiene permiso para leer el menú y recibir pedidos. Pide ese acceso en el tablero de ${platform}.`;
    case 'PLATFORM_UNAVAILABLE':
      return `${platform} no respondió. No se guardó nada; intenta de nuevo en unos minutos.`;
    case 'KEY_SERVICE_UNAVAILABLE':
      return 'El servicio de seguridad no respondió. No se guardó nada; intenta de nuevo en unos minutos.';
    case 'VALIDATION':
      return 'Revisa que pegaste el Client ID y el Client Secret completos, cada uno en su campo.';
    case 'TOO_MANY_REQUESTS':
      return 'Demasiados intentos seguidos. Espera un minuto y vuelve a intentar.';
    default:
      return 'No se pudo guardar. Intenta de nuevo en un momento.';
  }
}

/**
 * La llave con la que se comprueba que un pedido lo mandó de verdad la app (spec 021).
 *
 * Una fila por APP, no por tienda: la llave es de la aplicación registrada en la plataforma y todas
 * las sucursales la comparten. Sin ella no entra ningún pedido, y nada más lo dice — la tienda sale
 * «Conectada» porque para leer el menú sí lo está.
 *
 * LA LLAVE NO VUELVE A LA PANTALLA. El servidor solo dice si hay; el campo se vacía al guardar y
 * nace vacío al cambiarla. Con esa llave se meten pedidos a la cocina, y lo que se relee desde la
 * tableta se filtra por una foto.
 */
function LlaveDePedidos({ platformId, platformName }: { platformId: number; platformName: string }) {
  const [estado, setEstado] = useState<EstadoDeLlave | null>(null);
  const [editando, setEditando] = useState(false);
  const [llave, setLlave] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [guardando, setGuardando] = useState(false);
  const [ayuda, setAyuda] = useState(false);
  // Cambiarla y retirar la anterior es solo del administrador; el servidor responde 403 al resto.
  const canEdit = useSessionStore((s) => s.user?.role) === 'admin';

  const cargar = useCallback(() => {
    estadoDeLlave(platformId).then(setEstado).catch(() => setEstado(null));
  }, [platformId]);
  useEffect(cargar, [cargar]);

  const cerrar = () => {
    setLlave('');
    setEditando(false);
    setError(null);
  };

  const guardar = async () => {
    setGuardando(true);
    setError(null);
    try {
      await guardarLlave(platformId, llave);
      cerrar();
      cargar();
    } catch {
      setError('No se pudo guardar. Vuelve a copiar la llave completa del tablero de la app.');
    } finally {
      setGuardando(false);
    }
  };

  const retirar = async () => {
    await retirarLlaveAnterior(platformId).catch(() => undefined);
    cargar();
  };

  if (estado === null) return null;

  return (
    <Box borderBottomWidth="1px" py={3}>
      <HStack justify="space-between" wrap="wrap" gap={2}>
        <HStack gap={1}>
          <Text fontWeight="semibold">Pedidos de {platformName}</Text>
          <Text color={estado.configured && !estado.needsRecapture ? 'fg.muted' : 'orange.600'}>
            ·{' '}
            {!estado.configured
              ? 'Falta la llave para recibir pedidos'
              : estado.needsRecapture
                ? 'Hay que volver a poner la llave'
                : 'Recibe pedidos'}
          </Text>
          <IconButton aria-label="Dónde encontrar la llave" size="sm" minH="44px" minW="44px" variant="ghost"
            onClick={() => setAyuda(true)}>
            <LuCircleHelp />
          </IconButton>
        </HStack>
        {!editando && canEdit && (
          <Button minH="44px" variant="outline" onClick={() => setEditando(true)}>
            <LuKeyRound /> {estado.configured ? 'Cambiar la llave' : 'Poner la llave'}
          </Button>
        )}
      </HStack>

      {!canEdit && (!estado.configured || estado.needsRecapture) && (
        <Text fontSize="sm" color="fg.muted" mt={1}>Pídele a un administrador que la ponga.</Text>
      )}

      {estado.rotating && !editando && canEdit && (
        <HStack justify="space-between" wrap="wrap" gap={2} mt={2}>
          <Text fontSize="sm" color="fg.muted">
            La llave anterior sigue sirviendo. Retírala cuando la app ya use la nueva.
          </Text>
          <Button minH="44px" variant="ghost" onClick={retirar}>
            Retirar la anterior
          </Button>
        </HStack>
      )}

      {editando && (
        <VStack align="stretch" gap={2} mt={2}>
          <Input
            type="password"
            autoComplete="off"
            aria-label={`Llave de firma de ${platformName}`}
            placeholder="Pega aquí la llave"
            value={llave}
            onChange={(e) => setLlave(e.target.value)}
            minH="44px"
          />
          {estado.configured && (
            <Text fontSize="sm" color="fg.muted">
              La llave actual sigue sirviendo hasta que retires la anterior.
            </Text>
          )}
          {error && <Text color="red.600">{error}</Text>}
          <HStack gap={2}>
            <Button minH="44px" variant="outline" onClick={cerrar}>
              Cancelar
            </Button>
            <Button minH="44px" onClick={guardar} loading={guardando} disabled={!llave.trim()}>
              Guardar la llave
            </Button>
          </HStack>
        </VStack>
      )}

      <DialogRoot open={ayuda} onOpenChange={(e) => setAyuda(e.open)}>
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Dónde encontrar la llave</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <VStack align="stretch" gap={2}>
              <Text>1. Entra al tablero de desarrolladores de {platformName} con la cuenta del negocio.</Text>
              <Text>2. Abre tu aplicación y ve a la sección de webhooks.</Text>
              <Text>3. Copia la llave de firma (Signing Key) y pégala aquí.</Text>
            </VStack>
          </DialogBody>
          <DialogFooter>
            <Button minH="44px" onClick={() => setAyuda(false)}>
              Entendido
            </Button>
          </DialogFooter>
        </DialogContent>
      </DialogRoot>
    </Box>
  );
}

/**
 * Una fila por app, no por tienda: las sucursales de una misma app comparten el acceso y la llave.
 * La llave de pedidos aparece solo con una tienda conectada: antes no hay pedidos que verificar.
 */
function PlatformConnections({ apps, conexiones }: { apps: { id: number; name: string }[]; conexiones: ConexionDePlataforma[] }) {
  const withStore = new Set(conexiones.map((c) => c.platformId));
  return (
    <VStack align="stretch" gap={0}>
      <Text fontWeight="bold">Conexión con las plataformas</Text>
      {apps.map((a) => (
        <Box key={a.id}>
          <PlatformConnection platformId={a.id} platformName={a.name} />
          {withStore.has(a.id) && <LlaveDePedidos platformId={a.id} platformName={a.name} />}
        </Box>
      ))}
    </VStack>
  );
}

export function PlataformasPage() {
  const [conexiones, setConexiones] = useState<ConexionDePlataforma[] | null>(null);
  const [porBorrar, setPorBorrar] = useState<{ c: ConexionDePlataforma; parejas: number } | null>(null);
  const [abrirAlta, setAbrirAlta] = useState(false);
  const navegar = useNavigate();
  // Las apps se leen del menú de la empresa (los ids son por empresa), sin «Propio», que no tiene app.
  const { data: menu } = useMenu();
  const apps = (menu?.platforms ?? []).filter((p) => p.name !== 'Propio');

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
  // Falta la conexión si no hay ninguna tienda (primer uso) o si alguna tienda no la tiene.
  const missingConnection = conexiones.length === 0 || conexiones.some((c) => !c.credentialsConfigured);

  return (
    <Page>
      <VStack align="stretch" gap={4}>
        {/* PRIMERO LA CONEXIÓN, DESPUÉS LA TIENDA: sin ella no hay lista de tiendas que elegir ni
            menú que comparar. Mientras falte, va arriba de todo; ya hecha, baja: se configura una
            vez y la tarea diaria es la comparación de menú. */}
        {missingConnection && <PlatformConnections apps={apps} conexiones={conexiones} />}

        {conexiones.length > 0 && <MenuDePlataformaPage />}

        <Text fontWeight="bold">Tus tiendas</Text>
        {conexiones.length === 0 && (
          <Text color="fg.muted">Todavía no hay ninguna. Cuando conectes con la plataforma, da de alta la primera aquí abajo.</Text>
        )}
        {conexiones.map((c) => (
          <HStack key={c.id} justify="space-between" borderBottomWidth="1px" py={3} wrap="wrap" gap={2}>
            <VStack align="start" gap={0}>
              <Text fontWeight="semibold">
                {c.platformName} · {c.label}
              </Text>
              <Text fontSize="sm" color="fg.muted">
                {c.credentialsConfigured ? 'Conectada' : `Falta conectar con ${c.platformName}`}
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

        {!missingConnection && <PlatformConnections apps={apps} conexiones={conexiones} />}

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
