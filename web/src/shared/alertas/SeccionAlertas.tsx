import { useState } from 'react';
import { Box, Button, Checkbox, HStack, Input, Stack, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter } from '../../components/ui/drawer';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { ApiError } from '../../api/client';
import { medirAccion } from '../../api/uso';
import { lowStockApi, type AlertItemType, type LowStockConfig, type LowStockEffect, type LowStockInput } from '../../api/notifications';
import { formatBaseQty } from '../units';
import { QtyWithUnit } from '../qty';
import { baseQty, bigUnitOf, type Kind } from '../articleUnits';
import { resumenDeAlertas } from './resumenDeAlertas';

// La sección «Alertas» de la ficha (spec 084, maqueta M1, opción B): un renglón con el estado en
// palabras y «Configurar ›», que abre su hoja con la casilla, el mínimo, los eventos, la franja y los
// días de cobertura. Guardar abre siempre la confirmación con lo que pasará al instante (decisión del
// dueño). Al abrir una ficha que nunca preguntó, se pregunta si se quieren alertas.

interface Props {
  itemType: AlertItemType;
  id: number;
  name: string;
  kind: Kind;
  canConfigure: boolean;
  // onSaved recibe la versión nueva del artículo: guardar las alertas la cambia, y la ficha la necesita
  // para que su autoguardado no choque contra sí misma.
  onSaved?: (updatedAt: string) => void;
}

interface Forma {
  enabled: boolean;
  onMin: boolean;
  onOut: boolean;
  banner: boolean;
  min: { qty: string; big: boolean };
  dias: string;
}

const DIAS_POR_OMISION = 14;

function formaDe(c: LowStockConfig, kind: Kind): Forma {
  const big = bigUnitOf(kind);
  const n = c.minStock == null ? null : Number(c.minStock);
  const enGrande = !!big && n != null && n >= big.factor;
  return {
    enabled: c.enabled, onMin: c.onMin, onOut: c.onOut, banner: c.banner,
    min: { qty: n == null || n === 0 ? '' : String(enGrande ? n / big!.factor : n), big: n == null || n === 0 ? !!big : enGrande },
    dias: c.coverageDays == null ? '' : String(c.coverageDays),
  };
}

// cuerpo arma lo que se manda. Mínimo vacío es «sin mínimo» (null), nunca un cero escrito a medias; y
// sin mínimo, «llegue al mínimo» viaja apagada aunque la casilla haya quedado marcada.
function cuerpo(f: Forma, kind: Kind, updatedAt?: string): LowStockInput {
  const min = baseQty(f.min.qty, kind, f.min.big);
  return {
    enabled: f.enabled, onMin: f.onMin && min != null && Number(min) > 0, onOut: f.onOut, banner: f.banner, minStock: min ?? null,
    coverageDays: f.dias.trim() === '' ? null : Number(f.dias), ...(updatedAt ? { expectedUpdatedAt: updatedAt } : {}),
  };
}

// diasInvalidos: vacío es «los de omisión»; si se escribe, de 1 a 90 (lo mismo que acepta el servidor).
const diasInvalidos = (d: string) => d.trim() !== '' && !(Number(d) >= 1 && Number(d) <= 90);

const KIND_WORDS: Record<string, string> = { stock_low: 'al llegar al mínimo', stock_out: 'de que se acabó' };

function Casilla({ checked, onChange, disabled, children }: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean; children: React.ReactNode }) {
  return (
    <Checkbox.Root minH="44px" w="100%" checked={checked} disabled={disabled} onCheckedChange={(e) => onChange(e.checked === true)}>
      <Checkbox.HiddenInput /><Checkbox.Control /><Checkbox.Label>{children}</Checkbox.Label>
    </Checkbox.Root>
  );
}

export function SeccionAlertas({ itemType, id, name, kind, canConfigure, onSaved }: Props) {
  const qc = useQueryClient();
  const cfg = useQuery({ queryKey: ['alertas', itemType, id], queryFn: () => lowStockApi.get(itemType, id) });
  const [hoja, setHoja] = useState(false);
  const [forma, setForma] = useState<Forma | null>(null);
  const [efecto, setEfecto] = useState<LowStockEffect | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [preguntaCerrada, setPreguntaCerrada] = useState(false);

  const fallo = (e: unknown) => {
    // Otra tableta (o «Alertas por lote») guardó mientras tanto: se vuelve a leer en vez de pisarla.
    if (e instanceof ApiError && e.code === 'STALE') {
      setError('Alguien más cambió estas alertas mientras las veías. Se volvieron a leer: revisa y guarda otra vez.');
      void qc.invalidateQueries({ queryKey: ['alertas', itemType, id] }).then(() => {
        const nueva = qc.getQueryData<LowStockConfig>(['alertas', itemType, id]);
        if (nueva) setForma(formaDe(nueva, kind));
      });
      return;
    }
    setError(e instanceof ApiError ? e.message : 'No se pudo; vuelve a intentar.');
  };
  const yaPregunto = useMutation({
    mutationFn: () => lowStockApi.asked(id),
    onSettled: () => void qc.invalidateQueries({ queryKey: ['alertas', itemType, id] }),
  });
  const verEfecto = useMutation({
    mutationFn: (f: Forma) => lowStockApi.preview(itemType, id, cuerpo(f, kind, cfg.data?.updatedAt)),
    onSuccess: (r) => { setError(null); setEfecto(r); },
    onError: fallo,
  });
  const guardar = useMutation({
    mutationFn: (f: Forma) => lowStockApi.save(itemType, id, cuerpo(f, kind, cfg.data?.updatedAt)),
    onSuccess: (r) => {
      if (r.updatedAt) onSaved?.(r.updatedAt);
      medirAccion('inventario', 'alertas');
      setEfecto(null);
      setHoja(false);
      void qc.invalidateQueries({ queryKey: ['alertas'] });
      void qc.invalidateQueries({ queryKey: ['avisos'] });
      if (itemType === 'ingrediente') void qc.invalidateQueries({ queryKey: ['article', id] });
    },
    onError: (e) => { setEfecto(null); fallo(e); },
  });

  const c = cfg.data;
  if (!c) return null;
  const abrir = (encender?: boolean) => {
    const f = formaDe(c, kind);
    setForma(encender ? { ...f, enabled: true } : f);
    setError(null);
    setHoja(true);
  };
  const preguntar = canConfigure && itemType === 'ingrediente' && !c.asked && !preguntaCerrada && !hoja;

  return (
    <>
      <HStack justify="space-between" minH="44px" data-alertas>
        <Box>
          <Text>Alertas: <b>{resumenDeAlertas(c)}</b></Text>
          {/* El mínimo de la sucursal gana sobre el del artículo: se dice, o el renglón promete otro. */}
          {c.enabled && c.branchMinStock != null && (
            <Text fontSize="sm" color="fg.muted">En esta sucursal avisa al llegar a {formatBaseQty(c.branchMinStock, c.unitCode)}</Text>
          )}
        </Box>
        {canConfigure && <Button minH="44px" variant="outline" onClick={() => abrir()}>Configurar ›</Button>}
      </HStack>

      {forma && (
        <DrawerRoot open={hoja} placement="bottom" onOpenChange={(e) => { if (!e.open) setHoja(false); }}>
          <DrawerBackdrop />
          <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
            <DrawerHeader><Text fontSize="lg" fontWeight="700">Alertas de {name}</Text></DrawerHeader>
            <DrawerBody flex="1" minH={0} overflowY="auto">
              <Stack gap={1} maxW="560px">
                <Casilla checked={forma.enabled} onChange={(enabled) => setForma({ ...forma, enabled })}>
                  <Text fontWeight="700">Enviar alerta de stock bajo</Text>
                </Casilla>
                {forma.enabled && (
                  <Stack gap={2} pl={7}>
                    <QtyWithUnit label="Mínimo" optional kind={kind} value={forma.min} keepAmount onChange={(min) => setForma({ ...forma, min })} />
                    <Text fontSize="sm" color="fg.muted">Avisar cuando:</Text>
                    <Casilla checked={forma.onMin} disabled={forma.min.qty.trim() === '' || Number(forma.min.qty) === 0}
                      onChange={(onMin) => setForma({ ...forma, onMin })}>
                      llegue al mínimo{forma.min.qty.trim() === '' ? ' (captura un mínimo)' : ''}
                    </Casilla>
                    <Casilla checked={forma.onOut} onChange={(onOut) => setForma({ ...forma, onOut })}>se acabe</Casilla>
                    <Casilla checked={forma.banner} onChange={(banner) => setForma({ ...forma, banner })}>Mostrar franja en pantalla cuando se acabe</Casilla>
                    <HStack gap={2} flexWrap="wrap">
                      <Text>Comprar para</Text>
                      <Input aria-label="Días de cobertura" w="80px" minH="48px" inputMode="numeric" placeholder={String(DIAS_POR_OMISION)}
                        value={forma.dias} onChange={(e) => setForma({ ...forma, dias: e.target.value.replace(/\D/g, '').slice(0, 2) })} />
                      <Text>días (lista de compra)</Text>
                    </HStack>
                  </Stack>
                )}
                {error && <Text role="alert" color="fg.error">{error}</Text>}
              </Stack>
            </DrawerBody>
            <DrawerFooter>
              <Button minH="52px" variant="ghost" onClick={() => setHoja(false)}>Cancelar</Button>
              <Button minH="52px" colorPalette="brand" loading={verEfecto.isPending} onClick={() => {
                if (forma.enabled && diasInvalidos(forma.dias)) { setError('Los días de cobertura van de 1 a 90 días.'); return; }
                verEfecto.mutate(forma);
              }}>Guardar</Button>
            </DrawerFooter>
          </DrawerContent>
        </DrawerRoot>
      )}

      {forma && efecto && (
        <ConfirmSheet isOpen title="Guardar las alertas" confirmLabel="Guardar" confirmPalette="brand" loading={guardar.isPending}
          onConfirm={() => guardar.mutate(forma)} onCancel={() => setEfecto(null)}>
          <VStack align="stretch" gap={2}>
            <Text>Se actualizará la configuración de alertas de <b>{name}</b>: {resumenDeAlertas({ ...c, ...cuerpo(forma, kind), minStock: efecto.min ?? cuerpo(forma, kind).minStock }).toLowerCase()}.</Text>
            {efecto.effect.opens.length > 0 && (
              <Box bg="orange.subtle" borderRadius="md" p={3} fontWeight="600">
                Hoy quedan {formatBaseQty(efecto.onHand, c.unitCode)}, así que se enviará un aviso {efecto.effect.opens.map((k) => KIND_WORDS[k]).join(' y ')} al guardar.
                {efecto.needsCount ? ' La existencia está en negativo: conviene contarlo.' : ''}
              </Box>
            )}
            {efecto.effect.closes.length > 0 && <Text>Se cerrará el aviso abierto de este artículo.</Text>}
            {efecto.effect.opens.length === 0 && efecto.effect.closes.length === 0 && (
              <Text color="fg.muted">Hoy quedan {formatBaseQty(efecto.onHand, c.unitCode)}: no se envía nada al guardar.</Text>
            )}
          </VStack>
        </ConfirmSheet>
      )}

      <ConfirmSheet isOpen={preguntar} title={`¿Quieres recibir alertas de stock bajo de ${name}?`}
        confirmLabel="Sí, configurar" cancelLabel="No, por ahora" confirmPalette="brand"
        description="Te avisamos por la campana y por correo cuando quede poco o se acabe. Lo puedes cambiar después en «Alertas» de esta ficha."
        onConfirm={() => {
          // La hoja se abre en la vuelta siguiente: si nace abierta en la misma actualización en que se
          // cierra la pregunta, Chakra no la monta (lo mismo que CobrarSheet, AGENTS.md §3). Y la
          // pregunta queda hecha también al decir que sí: si no, volvía en cada ficha que se abría.
          setPreguntaCerrada(true);
          yaPregunto.mutate();
          setTimeout(() => abrir(true), 0);
        }}
        onCancel={() => { setPreguntaCerrada(true); yaPregunto.mutate(); }} />
    </>
  );
}
