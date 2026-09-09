import { describe, it, expect, beforeEach } from 'vitest';

import { useTicketStore } from '../../stores/ticket';
import { armarPedido } from '../../domain/pedido';
import { cuentaActivaAhora } from './useMandarPedido';

// EL FOLIO QUE SE TECLEA EN LA HOJA TIENE QUE LLEGAR AL SERVIDOR.
//
// Es el defecto más caro que se encontró en esta feature, y no se dedujo: se reprodujo. La hoja
// escribe el folio en la cuenta y en el MISMO manejador llama a mandar(). React no re-renderiza a
// media función, así que el `cuenta` que el hook cerró en el render anterior sigue teniendo el
// folio VACÍO, y `armarPedido` manda `undefined`.
//
// El pedido nace sin folio y aparece como pendiente, con el operador convencido de que lo capturó.
// Es justo el dato que la migración declara irrecuperable: el reporte de Rappi expone 3 meses y el
// de Uber 31 días.
describe('el folio capturado en la hoja llega al cuerpo del pedido', () => {
  beforeEach(() => {
    useTicketStore.getState().setPlatform(null);
    useTicketStore.getState().setPlatformOrderRef('');
  });

  it('reproduce el escape: el valor del render anterior NO trae el folio', () => {
    // Lo que el hook tenía cerrado antes de que la hoja escribiera.
    const cuentaDelRenderAnterior = useTicketStore.getState().tabs[0];

    useTicketStore.getState().setPlatform(6);
    useTicketStore.getState().setPlatformOrderRef('UBER-1234');

    const conElValorViejo = armarPedido({
      cuenta: cuentaDelRenderAnterior, lineas: [], clientUuid: 'x', deliveryFee: 0,
    });
    expect(conElValorViejo.platformOrderRef,
      'este es el escape que se está arreglando: con el valor del render anterior el folio no viaja')
      .toBeUndefined();
  });

  it('cuentaActivaAhora lee el store al momento, así que el folio SÍ viaja', () => {
    useTicketStore.getState().setPlatform(6);
    useTicketStore.getState().setPlatformOrderRef('UBER-1234');

    const body = armarPedido({
      cuenta: cuentaActivaAhora(), lineas: [], clientUuid: 'x', deliveryFee: 0,
    });
    expect(body.platformOrderRef,
      'el folio que el operador tecleó no llegó al cuerpo del pedido: nace sin folio y aparece ' +
      'como pendiente, con el operador convencido de que lo capturó').toBe('UBER-1234');
  });
});
