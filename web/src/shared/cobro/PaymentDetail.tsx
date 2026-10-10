import { useState } from 'react';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuArrowLeft, LuPrinter, LuUndo2 } from 'react-icons/lu';
import type { Currency, OrderLine, PaymentView } from '../../types/pos';
import { money } from '../../utils/format';
import { soloHora } from '../../utils/horaDelNegocio';
import { VOID_REASONS } from './split';

interface Props {
  payment: PaymentView;
  lines: OrderLine[];
  currency: Currency;
  // Zona del negocio, para la hora del pago.
  zona: string;
  canVoid: boolean;
  voiding: boolean;
  onBack: () => void;
  onReprint: () => void;
  onVoid: (reason: string) => void;
}

// cardLike dice si el dinero de ese método no está en el cajón: devolverlo aquí no regresa nada de
// la terminal, y quien opera tiene que hacerlo allá.
function cardLike(method: string): boolean {
  return /tarjeta|terminal|spei|transferencia/i.test(method);
}

// PaymentDetail es un pago abierto desde su ficha: qué cubrió, con qué y cuándo, y las dos acciones
// que lo corrigen. Es una vista de la misma hoja, no otra hoja encima.
export function PaymentDetail({ payment, lines, currency, zona, canVoid, voiding, onBack, onReprint, onVoid }: Props) {
  const [reason, setReason] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const names = new Map(lines.map((l) => [l.id, l.productName]));
  const at = payment.paidAt ? soloHora(payment.paidAt, zona) : '';
  return (
    <VStack align="stretch" gap={3}>
      <HStack gap={2}>
        <Button aria-label="Volver" minH="44px" minW="44px" variant="ghost" colorPalette="gray" onClick={onBack}>
          <LuArrowLeft />
        </Button>
        <VStack align="start" gap={0}>
          <Text fontWeight="800" fontSize="lg" textDecoration={payment.voided ? 'line-through' : undefined}>
            Pago {payment.number} · {money(payment.amount, currency)}
          </Text>
          <Text fontSize="sm" color="fg.muted">
            {payment.methodName}{at ? ` · ${at}` : ''}{payment.split ? ` · Parte ${payment.split.part} de ${payment.split.of}` : ''}
            {payment.voided ? ' · Devuelto' : ''}
          </Text>
        </VStack>
      </HStack>
      {payment.voided && payment.voidReason && (
        <Text fontSize="sm" color="fg.muted">Se devolvió: {payment.voidReason}</Text>
      )}
      {payment.lines.length > 0 ? (
        <Box>
          <Text fontSize="sm" fontWeight="600" mb={1}>
            Cubrió {payment.lines.reduce((n, l) => n + Number(l.qty), 0)} producto{payment.lines.length === 1 && Number(payment.lines[0].qty) === 1 ? '' : 's'}
          </Text>
          <VStack align="stretch" gap={1}>
            {payment.lines.map((l) => (
              <HStack key={l.lineId} justify="space-between">
                <Text>{Number(l.qty) > 1 ? `${Number(l.qty)}× ` : ''}{names.get(l.lineId) ?? 'Producto'}</Text>
                <Text fontWeight="600">{money(l.amount, currency)}</Text>
              </HStack>
            ))}
          </VStack>
        </Box>
      ) : (
        <Text fontSize="sm" color="fg.muted">Este pago no eligió productos.</Text>
      )}
      <Button minH="44px" variant="outline" colorPalette="gray" onClick={onReprint}>
        <LuPrinter /> Reimprimir su ticket
      </Button>
      {/* Devolver va en su propia fila, lejos de la acción frecuente: un dedo que erra no lo toca. */}
      {!payment.voided && !confirming && (
        <Button mt={4} minH="44px" variant="outline" colorPalette="red" disabled={!canVoid}
          onClick={() => setConfirming(true)}>
          <LuUndo2 /> Devolver este pago
        </Button>
      )}
      {!payment.voided && !canVoid && (
        <Text fontSize="sm" color="fg.muted">Pídele a quien encargue la caja que lo devuelva</Text>
      )}
      {confirming && (
        <VStack align="stretch" gap={2}>
          <Text fontSize="sm" fontWeight="600">¿Por qué se devuelve?</Text>
          {VOID_REASONS.map((r) => (
            <Button key={r} minH="48px" justifyContent="flex-start" aria-pressed={reason === r}
              variant={reason === r ? 'solid' : 'outline'} colorPalette={reason === r ? undefined : 'gray'}
              onClick={() => setReason(r)}>
              {r}
            </Button>
          ))}
          <Text fontSize="sm" color="fg.muted">
            {cardLike(payment.methodName)
              ? 'Sus productos vuelven a quedar por cobrar. El reembolso en la terminal se hace aparte.'
              : 'Sus productos vuelven a quedar por cobrar y el dinero sale del cajón.'}
          </Text>
          {/* Pegado al fondo de la vista: con varios productos cubiertos, la confirmación quedaba
              debajo del pliegue. */}
          <HStack gap={2} position="sticky" bottom={0} bg="bg" py={2}>
            <Button minH="44px" variant="ghost" colorPalette="gray" onClick={() => { setConfirming(false); setReason(null); }}>
              Cancelar
            </Button>
            <Box flex="1" />
            <Button minH="44px" colorPalette="red" disabled={reason === null} loading={voiding}
              onClick={() => reason && onVoid(reason)}>
              Devolver {money(payment.amount, currency)}
            </Button>
          </HStack>
        </VStack>
      )}
    </VStack>
  );
}
