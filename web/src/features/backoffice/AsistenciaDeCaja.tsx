import { useState, type ReactNode } from 'react';
import { Badge, Box, Button, Center, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import type { CashAttendanceGate } from '../../api/backoffice';
import { ClockPinSheet } from '../../shared/attendance/ClockPinSheet';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { CierreAutorizadoSheet } from './CierreAutorizadoSheet';

// La caja exige la asistencia de quien la abre (spec 046). Lo que se pide lo decide el servidor
// (`GET /cash-sessions/attendance`, con la misma regla que aplica al confirmar); aquí solo se pinta.
//
// Si esa consulta falla (la API anterior durante una subida, la red), no se decide nada con el
// error: se pinta la caja de siempre y manda el 409 del servidor al confirmar.

export interface EstadoDeAsistencia {
  gate?: CashAttendanceGate;
  failed: boolean;
}

// ---- Caja cerrada (maqueta A de la apertura) ----

export function AperturaConAsistencia({ nombreCaja, estado, abriendo, onContar, onActualizar }: {
  nombreCaja: string;
  estado: EstadoDeAsistencia;
  abriendo: boolean;
  onContar: () => void;
  onActualizar: () => void;
}) {
  const { soloHora } = useHoraDelNegocio();
  const [marcando, setMarcando] = useState(false);
  const { gate, failed } = estado;
  const cargando = !gate && !failed;
  const falta = gate !== undefined && gate.applies && !gate.ready;

  return (
    <VStack align="stretch" gap={4} bg="bg.panel" p={6} borderRadius="lg" borderWidth="1px" maxW="420px">
      <Text fontWeight="600">«{nombreCaja}» está cerrada.</Text>
      {falta ? (
        <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
          <Text fontWeight="700">Para abrir la caja, primero marca tu entrada.</Text>
          <Text fontSize="sm" color="fg.muted">
            {gate.stale ? `${gate.person.name}, tu entrada anterior ya venció.` : `${gate.person.name}, ahora no tienes una entrada marcada.`}
          </Text>
        </Box>
      ) : (
        <>
          {gate?.applies && gate.openSince && (
            <Badge alignSelf="flex-start" colorPalette="green" variant="subtle" size="lg">
              Entrada marcada {soloHora(gate.openSince)}
            </Badge>
          )}
          <Text fontSize="sm" color="fg.muted">Cuenta el efectivo con el que arranca el cajón.</Text>
        </>
      )}
      {falta && <Button minH="52px" onClick={() => setMarcando(true)}>Marcar entrada</Button>}
      {/* El conteo NO va aquí. Medido a 1024×600, esta pantalla mide 1,494 px de alto: una
          rejilla de once denominaciones en este punto nace 200 px debajo del fold. */}
      <Button minH="52px" variant={falta ? 'outline' : 'solid'} disabled={falta || cargando}
        onClick={onContar} loading={abriendo}>
        Contar el efectivo
      </Button>
      {gate && (
        <ClockPinSheet open={marcando} person={gate.person} kind="in" onClose={() => setMarcando(false)}
          onDone={() => { setMarcando(false); onActualizar(); }}
          onStateChanged={() => { setMarcando(false); onActualizar(); }} />
      )}
    </VStack>
  );
}

// ---- Caja abierta: el aviso arriba y el cierre que espera (maqueta A del cierre, aviso subido) ----

// AvisoDelCierre va ARRIBA de la caja abierta, debajo del nombre: a 1024×600 se ve sin desplazar
// (decisión del dueño, 2026-10-30, tras la prueba con usuario: en la sección del cierre quedaba en
// y≈590 y quien llegaba a cerrar no veía ni el aviso ni el botón). Resuelto, queda una franja verde.
export function AvisoDelCierre({ estado, autorizacion, onAutorizar, onQuitarAutorizacion, onActualizar }: {
  estado: EstadoDeAsistencia;
  autorizacion: string | null;
  onAutorizar: (reason: string) => void;
  onQuitarAutorizacion: () => void;
  onActualizar: () => void;
}) {
  const hora = useHoraDelNegocio();
  const [hoja, setHoja] = useState<'pin' | 'autorizar' | null>(null);
  const [salida, setSalida] = useState<string | null>(null);
  const { gate } = estado;
  if (!gate || !gate.applies) return null;

  const nombre = gate.person.name;
  if (gate.ready) {
    return salida ? <FranjaResuelta titulo={`Salida de ${nombre} marcada ${hora.soloHora(salida)}`} /> : null;
  }
  if (autorizacion !== null) {
    return (
      <FranjaResuelta titulo={`Se cerrará sin la salida de ${nombre}`}
        detalle={`Motivo: ${autorizacion}. Su entrada quedará «sin salida».`}
        accion={<Button size="sm" minH="44px" variant="ghost" onClick={onQuitarAutorizacion}>Quitar autorización</Button>} />
    );
  }

  const conPin = !gate.stale && gate.person.active;
  const desde = gate.openSince
    ? (hora.diaDelNegocio(gate.openSince) === hora.diaDelNegocio(new Date())
      ? `desde las ${hora.soloHora(gate.openSince)}`
      : `desde el ${hora.soloFecha(gate.openSince)} a las ${hora.soloHora(gate.openSince)}`)
    : '';
  const situacion = !gate.person.active ? `${nombre} ya no está activa: solo se puede cerrar con autorización.`
    : gate.stale ? 'Su entrada es muy antigua: solo se puede cerrar con autorización.'
      : `${nombre} sigue con la entrada abierta ${desde}`;
  return (
    // El naranja va en el fondo y no como `colorPalette` de la caja: heredado, pintaría de naranja
    // también los botones, que son los de siempre.
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={4} bg="orange.subtle">
      <Text fontWeight="700">{nombre} abrió esta caja y no ha marcado su salida</Text>
      <Text fontSize="sm" mt={1}>{situacion}</Text>
      <Text fontSize="sm" mt={1}>Para cerrar la caja, primero hay que resolverlo.</Text>
      {!gate.canAuthorize && !gate.isSelf && (
        <Text fontSize="sm" mt={1}>Si ya se fue, pide a alguien con permiso que cierre la caja.</Text>
      )}
      {/* Separados: «Cerrar sin la salida» deja horas «sin salida», y pegado al camino normal un dedo
          grueso lo toca por error. */}
      <HStack mt={3} gap={6} flexWrap="wrap">
        {conPin && <Button minH="52px" onClick={() => setHoja('pin')}>Marcar salida de {nombre}</Button>}
        {gate.canAuthorize && (
          <Button minH="52px" variant="outline" onClick={() => setHoja('autorizar')}>Cerrar sin la salida de {nombre}</Button>
        )}
      </HStack>
      <ClockPinSheet open={hoja === 'pin'} person={gate.person} kind="out" onClose={() => setHoja(null)}
        onDone={(r) => { setHoja(null); setSalida(r.at); onActualizar(); }}
        onStateChanged={() => { setHoja(null); onActualizar(); }} />
      <CierreAutorizadoSheet isOpen={hoja === 'autorizar'} nombre={nombre} onVolver={() => setHoja(null)}
        onConfirmar={(reason) => { setHoja(null); onAutorizar(reason); }} />
    </Box>
  );
}

// CierreConAsistencia no monta el bloque del cierre (de la tabla al botón «Cerrar caja») mientras el
// aviso de arriba no se resuelva: así no hay nada que contar antes de saber si se puede cerrar
// (FR-005). En su lugar, un renglón que manda al aviso.
export function CierreConAsistencia({ estado, autorizacion, children }: {
  estado: EstadoDeAsistencia;
  autorizacion: string | null;
  children: ReactNode;
}) {
  const { gate, failed } = estado;
  if (!gate && !failed) return <Center py={6}><Spinner /></Center>;
  if (!gate || !gate.applies || gate.ready || autorizacion !== null) return <>{children}</>;
  return (
    <Text color="fg.muted">Para cerrar la caja, primero resuelve el aviso de arriba sobre {gate.person.name}.</Text>
  );
}

function FranjaResuelta({ titulo, detalle, accion }: { titulo: string; detalle?: string; accion?: ReactNode }) {
  return (
    <HStack borderWidth="1px" borderColor="border.success" bg="bg.success" borderRadius="lg" p={3}
      justify="space-between" gap={2} flexWrap="wrap">
      <Box>
        <Text fontWeight="700" color="fg.success">{titulo}</Text>
        {detalle && <Text fontSize="sm" color="fg.muted">{detalle}</Text>}
      </Box>
      {accion}
    </HStack>
  );
}
