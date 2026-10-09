import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { accionPropia } from '../../stores/accionesPropias';
import { ApiError } from '../../api/client';
import { mensajeDeError } from '../../api/mensajes';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { usePosStore } from '../../stores/pos';
import type { DraftView, OrderView } from '../../types/pos';
import { uuid } from '../../utils/uuid';
import { descartarCuenta } from './descartarCuenta';
import { reportarResultado } from './useSinConexion';

// MANDAR A COCINA (spec 030, US3; research R-4).
//
// El servidor convierte la cuenta en pedido —o le agrega lo nuevo a uno que ya existe— en una sola
// transacción, idempotente por la cuenta: un reintento tras un corte de red devuelve el mismo pedido
// y `printLineIds` vacío, así que la comanda no sale dos veces.
//
// La cuenta enviada SE QUEDA abierta en esta tableta, ahora como pedido (caso 5): mandar a cocina
// borraba la cuenta de la pantalla y nada recordaba que faltaba cobrar.

interface Opciones {
  onComanda: (order: OrderView, printLineIds: number[]) => void;
}

export function useEnviarCuenta({ onComanda }: Opciones) {
  const qc = useQueryClient();
  const [motivo, setMotivo] = useState<string | null>(null);

  // Lo «Nuevo» de un pedido que se cerró mientras se capturaba no puede entrar a ese pedido (D-9):
  // se ofrece llevarlo a una cuenta nueva sin volver a capturarlo.
  const empezarNueva = async (vieja: DraftView) => {
    const id = uuid();
    const nueva = await posApi.createDraft({
      id, orderId: null,
      lines: (vieja.lines ?? []).map((l) => ({
        opId: uuid(), productId: l.productId, qty: l.qty, notes: l.notes ?? '',
        modifiers: (l.modifiers ?? []).map((m) => ({ optionId: m.optionId, qty: m.qty })),
      })),
    });
    qc.setQueryData(['pos', 'draft', nueva.id], nueva);
    usePosStore.getState().seleccionar({ kind: 'draft', id: nueva.id });
    // Si otra tableta le agregó algo entretanto no se descarta: eso se queda a la vista en la vieja.
    await descartarCuenta(qc, vieja.id, vieja.version).catch(() => undefined);
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };

  const m = useMutation({
    mutationFn: (draftId: string) => accionPropia(() => posApi.sendDraft(draftId)),
    onSuccess: (r, draftId) => {
      reportarResultado(null);
      setMotivo(null);
      qc.setQueryData(['orders', r.order.id], r.order);
      qc.removeQueries({ queryKey: ['pos', 'draft', draftId] });
      usePosStore.getState().seleccionar({ kind: 'order', id: r.order.id });
      qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['pos', 'folio-names'] });
      qc.invalidateQueries({ queryKey: ['modifier-defaults'] });
      const ids = r.printLineIds ?? [];
      if (ids.length > 0) onComanda(r.order, ids);
    },
    onError: (e: unknown, draftId) => {
      reportarResultado(e);
      const texto = e instanceof ApiError && e.code === 'NO_OPEN_REGISTER' ? 'No hay caja abierta' : mensajeDeError(e);
      setMotivo(texto);
      if (e instanceof ApiError && e.code === 'ORDER_CLOSED') {
        const vieja = qc.getQueryData<DraftView>(['pos', 'draft', draftId]);
        toaster.create({
          title: texto, type: 'info', duration: 10_000,
          action: vieja ? { label: 'Empezar cuenta nueva con estos productos', onClick: () => empezarNueva(vieja) } : undefined,
        });
        return;
      }
      const detalle = e instanceof ApiError ? e.details : undefined;
      toaster.create({
        title: detalle?.productName ? `No se pudo: ${detalle.productName}` : texto,
        type: 'error',
      });
      qc.invalidateQueries({ queryKey: ['pos', 'draft', draftId] });
    },
  });

  return {
    enviar: async (draftId: string): Promise<OrderView | null> => {
      try {
        return (await m.mutateAsync(draftId)).order;
      } catch {
        return null;
      }
    },
    enviando: m.isPending,
    motivo,
    limpiarMotivo: () => setMotivo(null),
  };
}
