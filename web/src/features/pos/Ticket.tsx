import {
  Box, Flex, HStack, VStack, Text, Button, IconButton, Separator, Center, Input, Heading,
} from '@chakra-ui/react';
import { useId, useState, type ReactNode } from 'react';
import {
  LuTrash2, LuStickyNote, LuPanelRightClose, LuStore, LuBike, LuTag, LuEllipsisVertical, LuUser,
  LuLock, LuCheck, LuChevronRight, LuChevronDown, LuCircleX, LuBan, LuPrinter,
} from 'react-icons/lu';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem, MenuSeparator } from '../../components/ui/menu';
import { cobraEnvio } from '../../domain/pedido';
import { descuentoDeLaCuenta, type ModoDeDescuento } from '../../domain/descuento';
import { parseMonto } from '../../domain/numeros';
import type { DraftHeader } from '../../types/pos';
import type { SwipeHandlers } from '../../hooks/useSwipeDownToClose';
import { money } from '../../utils/format';
import type { RenglonNuevo, RenglonPedido, VistaCuenta } from './cuentaEnPantalla';
import { detalleDeModificadores, puedeCancelarLoQueFalta } from './cuentaEnPantalla';

// EL TICKET DE LA CUENTA ABIERTA (spec 030, US3–US6; lienzo V2-1, V2-3, X1).
//
// Pinta lo que dice el servidor, en tres secciones:
//   - «Nuevo · aún no va a cocina»: lo que se está capturando, con −/+. Va PRIMERO porque es lo que
//     el operador está tocando.
//   - «En cocina»: lo enviado, compacto, con la marca de entregado. Quitar sale del ⋮ del renglón
//     (con motivo y lo que pasa con el insumo), no de un bote junto a la marca.
//   - «Pagado»: con candado y sin ⋮. Quitar algo pagado bajaría el total bajo lo cobrado.
// En una cuenta que todavía no se manda solo existe «Nuevo», sin encabezado.
//
// ARRIBA LO QUE DESCRIBE A LA CUENTA, ABAJO LO QUE MUEVE DINERO: el pie solo lleva totales y dos
// botones (caso 30). Cliente, descuento, descartar y cancelar viven en el ⋮ de la cabecera.

interface Props {
  vista: VistaCuenta;
  sinConexion: boolean;
  // El envío del negocio: es el que se pone al pasar una cuenta a domicilio.
  envioPorDefecto: number;
  enviando?: boolean;
  // Por qué no se pudo enviar o cobrar la última vez (sin caja, un producto que ya no se vende).
  // Se queda en el pie: un aviso que se va solo no se lee con el cliente enfrente.
  motivo?: string | null;
  puedeCancelar: boolean;
  onMas: (r: RenglonNuevo) => void;
  onMenos: (r: RenglonNuevo) => void;
  onQuitar: (r: RenglonNuevo) => void;
  onEditLine: (r: RenglonNuevo) => void;
  onCabecera: (h: DraftHeader) => void;
  onQuitarNoDisponibles: () => void;
  onEnviar: () => void;
  onCobrar: () => void;
  onDescartar: () => void;
  onCancelarPedido: () => void;
  // «Cancelar lo que falta» (2026-10-09): entregado pagado a medias cuyo cliente se fue.
  onCancelarResto?: () => void;
  onQuitarDeCocina: (r: RenglonPedido) => void;
  // El papel de la cuenta (spec 012): pre-cuenta si no se ha mandado, o lo enviado más lo nuevo.
  onImprimir: () => void;
  onHide?: () => void;
  swipeHandlers?: SwipeHandlers;
  // La hora a la que se abrió, ya en la zona del negocio.
  hora?: string;
  // El canal con su nombre («Uber Eats»). Sin él se dice el tipo: Mostrador o Domicilio.
  canal?: string;
}

// Lo ya enviado se pliega SOLO cuando hay algo nuevo y más de tres renglones: en el panel de 600 px,
// cinco renglones de cocina empujaban lo nuevo fuera de la vista. Sin nada nuevo se ve completo —es
// lo único que hay que leer—: plegarlo dejaba el ticket en blanco justo después de enviar.
const PLIEGA_CON = 3;

export function Ticket(props: Props) {
  const {
    vista, sinConexion, envioPorDefecto, enviando, motivo, puedeCancelar, onCabecera, onEnviar,
    onCobrar, onDescartar, onCancelarPedido, onCancelarResto, onHide, swipeHandlers,
  } = props;
  const enCaptura = vista.tipo !== 'pedido';
  const llevaEnvio = enCaptura && cobraEnvio(vista);
  const [envioMal, setEnvioMal] = useState(false);
  const [capturando, setCapturando] = useState<null | 'cliente' | 'descuento'>(null);
  const [descuentoMal, setDescuentoMal] = useState(false);

  const hayNuevo = vista.nuevos.length > 0;
  const noSeVende = vista.noDisponibles.length > 0;
  const bloqueo = sinConexion
    ? 'Sin conexión: no se puede mandar a cocina ni cobrar.'
    : vista.guardando ? 'Guardando…'
      : noSeVende ? 'Quita lo que ya no se vende para mandar o cobrar.' : null;
  const capturaMal = (llevaEnvio && envioMal) || descuentoMal;
  const enviarApagado = !hayNuevo || bloqueo !== null || noSeVende || capturaMal;
  const cobrarApagado = vista.falta <= 0 || bloqueo !== null || capturaMal;
  const etiquetaCobrar = hayNuevo ? `Enviar y cobrar ${money(vista.falta)}` : `Cobrar ${money(vista.falta)}`;
  // Piezas, no renglones: cocina recibe dos Cocas aunque vayan en un renglón.
  const piezasNuevas = vista.nuevos.reduce((n, l) => n + l.qty, 0);
  const cambiaTipo = enCaptura && vista.platformId === null;
  const tipo = props.canal ?? (vista.serviceType === 'domicilio' ? 'Domicilio' : 'Mostrador');
  const subtitulo = [
    vista.numero !== null ? `#${vista.numero}` : null, tipo, props.hora || null, vista.customerName || null,
  ].filter(Boolean).join(' · ');

  const conMenu = enCaptura || puedeCancelar || vista.tipo === 'pedido';
  const hayQueImprimir = hayNuevo || vista.enCocina.length + vista.pagados.length > 0;
  // Cada cuenta, y cada vez que aparece o se va lo nuevo, decide de nuevo si se pliega: heredar el
  // estado de la cuenta anterior dejaba cinco renglones de cocina empujando lo nuevo fuera de la vista.
  const claveDeCuenta = vista.pedidoId ?? vista.borradorId ?? 'nueva';

  return (
    <Flex direction="column" h="100%" bg="bg.panel">
      <HStack p={3} pb={2} gap={2}
        style={swipeHandlers ? { touchAction: 'none' } : undefined} {...swipeHandlers}>
        {onHide && (
          <IconButton size="lg" minW="48px" minH="48px" variant="ghost" colorPalette="gray"
            aria-label="Ocultar pedido" onClick={onHide}>
            <LuPanelRightClose />
          </IconButton>
        )}
        {/* El nombre COMPLETO, en hasta dos renglones: «Col…» no dice de quién es la cuenta. El tipo
            (Mostrador o Domicilio) se lee abajo; el botón de texto le quitaba al nombre el ancho. */}
        <Box flex="1" minW={0}>
          <Heading as="h2" fontSize="lg" fontWeight="700" lineHeight="1.2" lineClamp={2} wordBreak="break-word">
            {vista.nombre || (enCaptura ? 'Cuenta nueva' : '')}
          </Heading>
          <Text fontSize="xs" color="fg.muted" truncate>{subtitulo}</Text>
        </Box>
        {/* Un toque y compacto: el nombre necesita el ancho, y el tipo se lee en el subtítulo. Un
            pedido de plataforma ES a domicilio, y uno ya enviado no cambia de tipo. */}
        {cambiaTipo && (
          <IconButton size="sm" minW="44px" minH="44px" variant="outline" colorPalette="gray" flexShrink={0}
            aria-label={vista.serviceType === 'mostrador' ? 'Cambiar a domicilio' : 'Cambiar a mostrador'}
            onClick={() => onCabecera(vista.serviceType === 'mostrador'
              ? { serviceType: 'domicilio', deliveryFee: envioPorDefecto.toFixed(2) }
              : { serviceType: 'mostrador', deliveryFee: '0.00' })}>
            {vista.serviceType === 'mostrador' ? <LuBike /> : <LuStore />}
          </IconButton>
        )}
        {conMenu && (
          <MenuRoot>
            <MenuTrigger asChild>
              <IconButton aria-label="Más opciones de la cuenta" size="sm" minW="44px" minH="44px"
                variant="ghost" colorPalette="gray" flexShrink={0}>
                <LuEllipsisVertical />
              </IconButton>
            </MenuTrigger>
            <MenuContent>
              {enCaptura && (
                <MenuItem value="cliente" minH="48px" onClick={() => setCapturando('cliente')}>
                  <LuUser /> Nombre del cliente
                </MenuItem>
              )}

              {vista.tipo === 'captura' && (
                <MenuItem value="descuento" minH="48px" onClick={() => setCapturando('descuento')}>
                  <LuTag /> Descuento
                </MenuItem>
              )}
              {hayQueImprimir && (
                <MenuItem value="imprimir" minH="48px" onClick={props.onImprimir}>
                  <LuPrinter /> Imprimir cuenta
                </MenuItem>
              )}
              {vista.tipo === 'captura' && <MenuSeparator />}
              {/* Lejos de lo que se toca todo el día, y pregunta en una hoja de la app. */}
              {vista.tipo === 'captura' && (
                <MenuItem value="descartar" minH="48px" mt={3} color="red.fg" onClick={onDescartar}>
                  <LuCircleX /> Descartar cuenta
                </MenuItem>
              )}
              {!enCaptura && puedeCancelar && <MenuSeparator />}
              {!enCaptura && puedeCancelar && (
                <MenuItem value="cancelar" minH="48px" mt={3} color="red.fg" onClick={onCancelarPedido}>
                  <LuBan /> Cancelar pedido
                </MenuItem>
              )}
              {puedeCancelar && onCancelarResto && puedeCancelarLoQueFalta(vista) && <MenuSeparator />}
              {puedeCancelar && onCancelarResto && puedeCancelarLoQueFalta(vista) && (
                <MenuItem value="cancelar-resto" minH="48px" color="red.fg" onClick={onCancelarResto}>
                  <LuBan /> Cancelar lo que falta
                </MenuItem>
              )}
            </MenuContent>
          </MenuRoot>
        )}
      </HStack>

      {capturando === 'cliente' && (
        <CampoCliente inicial={vista.customerName} onListo={(nombre) => {
          onCabecera({ customerName: nombre || null });
          setCapturando(null);
        }} />
      )}
      {capturando === 'descuento' && (
        <CampoDescuento subtotal={vista.subtotalNuevo} onMal={setDescuentoMal} onListo={(d) => {
          onCabecera({ discount: d });
          setDescuentoMal(false);
          setCapturando(null);
        }} />
      )}
      <Separator />

      <VStack align="stretch" gap={0} flex="1" minH={0} overflowY="auto" px={2} py={2}>
        {!hayNuevo && enCaptura && (
          <Center h="100%" px={6}>
            <Text color="fg.subtle" textAlign="center">Toca un producto para agregarlo</Text>
          </Center>
        )}
        {hayNuevo && (enCaptura
          ? <Nuevos {...props} />
          : (
            <Seccion titulo="Nuevo · aún no va a cocina" color="orange.fg" plegable={false} total={vista.nuevos.length}>
              <Nuevos {...props} />
            </Seccion>
          ))}
        {vista.enCocina.length > 0 && (
          <Seccion key={`cocina-${claveDeCuenta}-${hayNuevo}`}
            titulo="En cocina" color="blue.fg" plegable={hayNuevo} total={vista.enCocina.length}>
            {vista.enCocina.map((r) => <EnCocina key={r.id} r={r} onQuitar={() => props.onQuitarDeCocina(r)} />)}
          </Seccion>
        )}
        {vista.pagados.length > 0 && (
          <Seccion key={`pagado-${claveDeCuenta}-${hayNuevo}`}
            titulo="Pagado" color="green.fg" plegable={hayNuevo} total={vista.pagados.length}>
            {vista.pagados.map((r) => <Pagado key={r.id} r={r} />)}
          </Seccion>
        )}
      </VStack>

      <Separator />
      {/* Lo que el servidor ya no vende. Vive FUERA de la caja de totales: adentro mandaba Cobrar a
          un scroll interno. */}
      {noSeVende && (
        <Box colorPalette="orange" borderWidth="1px" borderColor="colorPalette.emphasized"
          bg="colorPalette.subtle" borderRadius="lg" p={2} mx={3} mb={2} flexShrink={0}>
          <Text fontWeight="700" fontSize="sm" color="colorPalette.fg">Ya no están en el menú</Text>
          <Text fontSize="xs" color="fg.muted" mb={2}>{vista.noDisponibles.map((l) => l.name).join(', ')}</Text>
          <Button size="sm" minH="44px" variant="outline" colorPalette="orange" onClick={props.onQuitarNoDisponibles}>
            Quitar del pedido
          </Button>
        </Box>
      )}

      {/* maxH en dvh: sin un alto, `overflowY` no hace scroll —la caja crece— y con el teclado
          abierto Cobrar se va abajo de la pantalla. */}
      <Box p={3} maxH="60dvh" overflowY="auto" flexShrink={0} data-testid="totales">
        {enCaptura ? (
          <Flex justify="space-between" align="center" mb={2}>
            <Text fontSize="lg" fontWeight="600">Total</Text>
            <VStack gap={0} align="end">
              {vista.descuentoNuevo > 0 && (
                <Text fontSize="xs" color="fg.muted">
                  {money(vista.subtotalNuevo)} − {money(vista.descuentoNuevo)} de descuento
                </Text>
              )}
              <Text fontSize="2xl" fontWeight="800">{money(vista.totalNuevo)}</Text>
            </VStack>
          </Flex>
        ) : (
          <VStack align="stretch" gap={0} mb={2}>
            {vista.pagado > 0 && (
              <Flex justify="space-between">
                <Text fontSize="sm" color="fg.muted">Ya pagado</Text>
                <Text fontSize="sm" color="fg.muted">{money(vista.pagado)} de {money(vista.totalPedido)}</Text>
              </Flex>
            )}
            {hayNuevo && (
              <Flex justify="space-between">
                <Text fontSize="sm" color="fg.muted">Nuevo</Text>
                <Text fontSize="sm" color="fg.muted">{money(vista.totalNuevo)}</Text>
              </Flex>
            )}
            <Flex justify="space-between" align="center">
              <Text fontSize="lg" fontWeight="600">Falta</Text>
              <Text fontSize="2xl" fontWeight="800">{money(vista.falta)}</Text>
            </Flex>
          </VStack>
        )}

        {llevaEnvio && (
          <CampoEnvio key={`${vista.borradorId}-${vista.deliveryFee}`} guardado={vista.deliveryFee}
            onMal={setEnvioMal} onGuardar={(v) => onCabecera({ deliveryFee: v })} />
        )}

        {/* APILADOS, cada uno a todo lo ancho: en el panel de ~300 px «Enviar y cobrar $1,234.50» y
            «Enviar 3 a cocina» lado a lado no caben —un botón no parte su texto— y Cobrar se iba a
            un scroll lateral. Cobrar arriba y más alto: es lo que pasa en casi toda venta. */}
        <VStack gap={2} align="stretch" role="group" aria-label="Enviar y cobrar">
          {/* Con algo nuevo el botón LO DICE: cobrar manda primero a cocina (research R-5). */}
          <Button w="100%" size="lg" h="56px" colorPalette="green" fontWeight="800"
            loading={enviando} disabled={cobrarApagado} onClick={onCobrar}>
            {etiquetaCobrar}
          </Button>
          <Button w="100%" size="md" minH="44px" variant="outline" colorPalette="blue"
            disabled={enviarApagado} loading={enviando} onClick={onEnviar}>
            {hayNuevo ? `Enviar ${piezasNuevas} a cocina` : 'Enviar a cocina'}
          </Button>
        </VStack>
        {(bloqueo || motivo) && (
          <Text fontSize="xs" color={motivo ? 'red.fg' : 'fg.muted'} textAlign="right" mt={1} role="status">
            {motivo ?? bloqueo}
          </Text>
        )}
      </Box>
    </Flex>
  );
}

function Seccion({ titulo, color, plegable, total, children }: {
  titulo: string; color: string; plegable: boolean; total: number; children: ReactNode;
}) {
  const id = useId();
  const pliega = plegable && total > PLIEGA_CON;
  const [abierta, setAbierta] = useState(!pliega);
  return (
    <Box as="section" role="region" aria-labelledby={id} mb={2}>
      <Heading as="h3" id={id} fontSize="xs" textTransform="uppercase" color={color} px={2} py={1}>
        {pliega ? (
          <Button variant="ghost" size="xs" minH="44px" px={0} color={color} fontSize="xs"
            textTransform="uppercase" onClick={() => setAbierta((v) => !v)}>
            {titulo} · {total} {abierta ? <LuChevronDown /> : <LuChevronRight />}
          </Button>
        ) : titulo}
      </Heading>
      {/* `!pliega` va primero: si la sección deja de ser plegable estando cerrada (se cobró uno por
          productos y quedaron tres), ya no hay botón con qué abrirla y sus renglones desaparecían. */}
      {(!pliega || abierta) && children}
    </Box>
  );
}

// Un renglón de 44 px en UNA línea: papelera · nombre y precio · −/cantidad/+. En dos líneas medía
// ~80 px y en el panel de 600 px cabían tres. La papelera va al extremo OPUESTO del «+»: quitar un
// renglón por accidente obliga a volver a buscar el producto en el menú.
function Nuevos({ vista, onMas, onMenos, onQuitar, onEditLine }: Props) {
  return (
    <>
      {vista.nuevos.map((l) => (
        <HStack key={l.id} py={1} px={1} gap={1} minH="44px" borderBottomWidth="1px" borderColor="border.muted"
          opacity={l.guardando ? 0.6 : 1}>
          {l.guardando ? (
            <Box minW="44px" />
          ) : (
            <IconButton aria-label="Quitar" size="sm" minW="44px" minH="44px" variant="ghost"
              colorPalette="red" flexShrink={0} onClick={() => onQuitar(l)}><LuTrash2 /></IconButton>
          )}
          <Box flex="1" minW={0} onClick={() => !l.guardando && onEditLine(l)} cursor="pointer">
            <Text fontWeight="600" fontSize="sm" lineClamp={2} textDecoration={l.available ? undefined : 'line-through'}>
              {l.name}
            </Text>
            {l.modifiers.length > 0 && (
              <Text fontSize="xs" color="fg.muted" lineClamp={2}>{detalleDeModificadores(l.modifiers)}</Text>
            )}
            {l.notes && (
              <HStack gap={1} color="orange.500">
                <LuStickyNote size={12} />
                <Text fontSize="xs" truncate>{l.notes}</Text>
              </HStack>
            )}
            <Text fontSize="xs" color="fg.muted">{l.guardando ? 'Guardando…' : money(l.lineTotal)}</Text>
          </Box>
          {!l.guardando && (
            <HStack gap={0} flexShrink={0}>
              <Button size="sm" minW="44px" minH="44px" aria-label="Uno menos" onClick={() => onMenos(l)}>−</Button>
              <Text minW="28px" textAlign="center" fontSize="sm" fontWeight="600">{l.qty}</Text>
              <Button size="sm" minW="44px" minH="44px" aria-label="Uno más" onClick={() => onMas(l)}>+</Button>
            </HStack>
          )}
        </HStack>
      ))}
    </>
  );
}

function EnCocina({ r, onQuitar }: { r: RenglonPedido; onQuitar: () => void }) {
  const entregado = r.delivered >= r.qty;
  return (
    <HStack px={2} minH="44px" gap={2} borderBottomWidth="1px" borderColor="border.muted">
      <Box flex="1" minW={0}>
        <Text fontSize="sm" truncate>{r.qty > 1 ? `${r.name} ×${r.qty}` : r.name}</Text>
        {r.detalle && <Text fontSize="2xs" color="fg.muted" truncate>{r.detalle}</Text>}
      </Box>
      {entregado ? (
        <HStack gap={1} color="green.fg" flexShrink={0}><LuCheck size={14} /><Text fontSize="xs">entregado</Text></HStack>
      ) : r.delivered > 0 ? (
        <Text fontSize="xs" color="fg.muted" flexShrink={0}>{r.delivered} de {r.qty} entregados</Text>
      ) : null}
      <Text fontSize="sm" whiteSpace="nowrap">{money(r.lineTotal)}</Text>
      <MenuRoot>
        <MenuTrigger asChild>
          <IconButton aria-label={`Opciones de ${r.name}`} size="sm" minW="44px" minH="44px"
            variant="ghost" colorPalette="gray" flexShrink={0}>
            <LuEllipsisVertical />
          </IconButton>
        </MenuTrigger>
        <MenuContent>
          <MenuItem value="quitar" minH="48px" color="red.fg" onClick={onQuitar}>
            <LuTrash2 /> Quitar
          </MenuItem>
        </MenuContent>
      </MenuRoot>
    </HStack>
  );
}

function Pagado({ r }: { r: RenglonPedido }) {
  return (
    <HStack px={2} minH="40px" gap={2} borderBottomWidth="1px" borderColor="border.muted" color="fg.muted">
      <Box color="green.fg" aria-label="Pagado" role="img" flexShrink={0}><LuLock size={14} /></Box>
      <Text fontSize="sm" truncate flex="1">{r.qty > 1 ? `${r.name} ×${r.qty}` : r.name}</Text>
      <Text fontSize="sm" whiteSpace="nowrap">{money(r.lineTotal)}</Text>
    </HStack>
  );
}

function CampoEnvio({ guardado, onMal, onGuardar }: {
  guardado: string; onMal: (mal: boolean) => void; onGuardar: (v: string) => void;
}) {
  const [texto, setTexto] = useState(String(Number(guardado)));
  const leido = parseMonto(texto);
  const mal = leido.estado === 'invalido';
  return (
    <HStack gap={2} mb={2}>
      <Text fontSize="sm" color="fg.muted" flexShrink={0}>Envío</Text>
      <Input flex="1" minH="44px" inputMode="decimal" aria-label="Costo de envío"
        value={texto}
        onChange={(e) => {
          setTexto(e.target.value);
          onMal(parseMonto(e.target.value).estado === 'invalido');
        }}
        onBlur={() => {
          if (leido.estado !== 'valido') return;
          const v = leido.valor.toFixed(2);
          if (v !== Number(guardado).toFixed(2)) onGuardar(v);
        }} />
      {mal && <Text fontSize="xs" color="red.fg">Solo números</Text>}
    </HStack>
  );
}

function CampoCliente({ inicial, onListo }: { inicial: string; onListo: (n: string) => void }) {
  const [nombre, setNombre] = useState(inicial);
  return (
    <HStack px={3} pb={2} gap={2} flexShrink={0}>
      <Input flex="1" minW={0} minH="44px" autoFocus aria-label="Nombre del cliente" maxLength={60}
        placeholder="Nombre del cliente" value={nombre} onChange={(e) => setNombre(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter') onListo(nombre.trim()); }} />
      <Button size="sm" minH="44px" px={3} variant="ghost" colorPalette="gray" flexShrink={0}
        onClick={() => onListo(nombre.trim())}>Listo</Button>
    </HStack>
  );
}

function CampoDescuento({ subtotal, onMal, onListo }: {
  subtotal: number;
  onMal: (mal: boolean) => void;
  onListo: (d: DraftHeader['discount']) => void;
}) {
  const [modo, setModo] = useState<ModoDeDescuento>('monto');
  const [texto, setTexto] = useState('');
  const d = descuentoDeLaCuenta(texto, modo, subtotal);
  const mal = d.malEscrito || d.excede;
  const cambiar = (t: string, m: ModoDeDescuento) => {
    setTexto(t);
    setModo(m);
    const x = descuentoDeLaCuenta(t, m, subtotal);
    onMal(x.malEscrito || x.excede);
  };
  const listo = () => {
    const p = d.paraElServidor;
    if (!p) { onListo(null); return; }
    onListo('discountPercent' in p ? { percent: String(p.discountPercent) } : { amount: p.discountAmount.toFixed(2) });
  };
  return (
    <VStack px={3} pb={2} gap={1} flexShrink={0} align="stretch">
    <HStack gap={2}>
      <HStack gap={1} flexShrink={0}>
        {/* Dos botones y no un selector: para dos opciones excluyentes, un toque. */}
        <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en pesos"
          aria-pressed={modo === 'monto'} variant={modo === 'monto' ? 'solid' : 'outline'}
          colorPalette={modo === 'monto' ? undefined : 'gray'} onClick={() => cambiar(texto, 'monto')}>$</Button>
        <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en porcentaje"
          aria-pressed={modo === 'pct'} variant={modo === 'pct' ? 'solid' : 'outline'}
          colorPalette={modo === 'pct' ? undefined : 'gray'} onClick={() => cambiar(texto, 'pct')}>%</Button>
      </HStack>
      <Input flex="1" minW={0} minH="44px" inputMode="decimal" autoFocus aria-label="Descuento"
        placeholder={modo === 'pct' ? '0 %' : money(0)} value={texto}
        onChange={(e) => cambiar(e.target.value, modo)} />
      <Button size="sm" minH="44px" px={3} variant="ghost" colorPalette="gray" flexShrink={0}
        disabled={mal} onClick={listo}>Listo</Button>
    </HStack>
    {/* El aviso en su propia línea: junto al campo lo dejaba en ~10 px en el panel de 300. */}
    {d.malEscrito && <Text fontSize="xs" color="red.fg">Solo números</Text>}
    {d.excede && <Text fontSize="xs" color="red.fg">Máx {modo === 'pct' ? '100 %' : money(subtotal)}</Text>}
    </VStack>
  );
}
