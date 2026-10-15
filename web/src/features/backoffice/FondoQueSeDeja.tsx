import { Badge, Box, Button, HStack, Input, Text } from '@chakra-ui/react';
import { fondoExcedeLoContado, personasDelRetiro, retiroDelCierre } from './fondoQueSeDeja';
import { Picker } from '../../components/Picker';
import { money } from '../../utils/format';

// Con más personas que esto, la fila de botones deja de verse completa a 1024 px y pasa a la hoja
// con buscador (revisión de tableta de la spec 035).
const MAX_EN_FILA = 6;

// FondoQueSeDeja: el fondo que se queda para el siguiente turno y el retiro (spec 035, opción A de
// las maquetas con la pregunta «¿Se retira dinero?», que abre en Sí). El fondo llega con el fijo de
// la caja y cambiarlo aplica solo a este cierre; el retiro se pinta en el renglón del campo para que
// el teclado del sistema no lo tape mientras se escribe.
export function FondoQueSeDeja({ value, onChange, contado = null, fijo = null, retirar, onRetirar, personas, quien, onQuien, quienCierra = null, currency }: {
  value: string; onChange: (v: string) => void; contado?: number | null; fijo?: number | null;
  retirar: boolean; onRetirar: (v: boolean) => void;
  personas: { id: number; name: string }[]; quien: number | null; onQuien: (id: number) => void;
  quienCierra?: number | null; currency: string;
}) {
  const excede = fondoExcedeLoContado(value, contado);
  const alerta = retirar && excede;
  const retiro = retiroDelCierre(contado, value);
  const soloHoy = fijo !== null && value.trim() !== '' && Number(value) !== fijo;
  const fila = personasDelRetiro(personas, quienCierra);
  return (
    // Rojo solo con «Sí»: con «No» se queda lo contado y no hay fondo que rebase nada.
    <Box borderWidth="1px" borderRadius="lg" p={3} borderColor={alerta ? 'red.400' : undefined} data-alerta={alerta || undefined}>
      <HStack justify="space-between" gap={3} wrap="wrap">
        <Text fontWeight="700">¿Se retira dinero del cajón?</Text>
        <HStack gap={2}>
          <Button minH="48px" variant={retirar ? 'solid' : 'outline'} aria-pressed={retirar} onClick={() => onRetirar(true)}>
            Sí, retirar
          </Button>
          <Button minH="48px" variant={!retirar ? 'solid' : 'outline'} aria-pressed={!retirar} onClick={() => onRetirar(false)}>
            No, todo se queda
          </Button>
        </HStack>
      </HStack>

      {!retirar && (
        <Text mt={2} fontSize="sm">
          Se queda de fondo todo lo contado{contado !== null ? `: ${money(contado, currency)}` : ''}. Sin retiro.
          {fijo !== null && <Text as="span" color="fg.muted"> (el fijo es {money(fijo, currency)})</Text>}
        </Text>
      )}

      {retirar && (
        <>
          <HStack mt={3} gap={3} wrap="wrap">
            <Text>Se queda</Text>
            <Input aria-label="Se queda de fondo" inputMode="decimal" maxW="160px" minH="48px" placeholder="$"
              value={value} onChange={(e) => onChange(e.target.value)} />
            {soloHoy && <Badge colorPalette="orange">Solo hoy</Badge>}
            {retiro !== null && (
              <Text fontWeight="700" color="green.600" _dark={{ color: 'green.300' }}>→ Retiro {money(retiro, currency)}</Text>
            )}
          </HStack>
          {fijo !== null && !soloHoy && <Text mt={1} fontSize="sm" color="fg.muted">Fondo fijo de esta caja</Text>}
          {excede && contado !== null && (
            <Text mt={2} fontSize="sm" color="red.600" _dark={{ color: 'red.300' }}>
              Es más de lo contado en el cajón ({money(contado, currency)}).
            </Text>
          )}
          {retiro !== null && retiro > 0 && (
            <Box mt={3}>
              <Text fontSize="sm" color="fg.muted" mb={1}>Se lo lleva</Text>
              {fila.length > MAX_EN_FILA ? (
                <Picker value={quien === null ? '' : String(quien)} onChange={(v) => onQuien(Number(v))}
                  options={fila.map((p) => ({ value: String(p.id), label: p.name }))}
                  placeholder="Elegir persona" title="¿Quién se lleva el retiro?" size="lg" />
              ) : (
                <HStack gap={2} overflowX="auto" pb={1}>
                  {fila.map((p) => (
                    <Button key={p.id} minH="48px" flexShrink={0} borderRadius="full"
                      variant={quien === p.id ? 'solid' : 'outline'} aria-pressed={quien === p.id}
                      onClick={() => onQuien(p.id)}>
                      {p.name}
                    </Button>
                  ))}
                </HStack>
              )}
            </Box>
          )}
        </>
      )}
    </Box>
  );
}
