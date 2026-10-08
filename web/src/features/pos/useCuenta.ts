import { useEffect, useMemo, useRef } from 'react';
import { create } from 'zustand';
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { mensajeDeError } from '../../api/mensajes';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { usePosStore, type Seleccion } from '../../stores/pos';
import type {
  CreateDraftBody, DraftHeader, DraftLineInput, DraftView, OrderView, TicketModifier,
} from '../../types/pos';
import { uuid } from '../../utils/uuid';
import { armarVista, type Pendiente, type RenglonNuevo, type VistaCuenta } from './cuentaEnPantalla';
import { cambioEnOtraTableta, type FotoDeCuenta } from './cambioEnOtraTableta';
import { nombreLibre } from './folio';
import { useCuentasVivas } from './useCuentasVivas';
import { esFalloDeRed, reportarResultado, useSinConexion } from './useSinConexion';

// LA CUENTA ABIERTA, CONTRA EL SERVIDOR (spec 030, research R-2 y R-13).
//
// Cada toque es una petición: agregar crea la cuenta si todavía no existe, y si existe le suma un
// renglón. Lo que el servidor no ha confirmado se ve como «guardando» y apaga Enviar y Cobrar; si
// falla sale de la cuenta y un aviso ofrece reintentar CON LA MISMA LLAVE (`opId`), así el
// reintento de algo que sí entró no lo duplica.
//
// Las escrituras de una misma cuenta van EN FILA: el segundo toque no puede llegar al servidor antes
// de que la cuenta exista, y un cambio de cabecera tiene que leer la versión que dejó el anterior.

export interface ProductoAAgregar {
  productId: number;
  name: string;
  // Solo para pintar el renglón mientras se guarda; el precio lo pone el servidor.
  unitPrice: number;
  qty: number;
  modifiers: TicketModifier[];
  notes?: string;
}

interface CapturaState {
  pendientes: Pendiente[];
  // Cuentas que esta tableta está creando. Mientras estén aquí, el servidor todavía no las tiene y
  // pedirlas respondería «no existe».
  naciendo: Record<string, 'creando' | 'fallo'>;
  // El «Nuevo» de cada pedido que esta tableta empezó, o el que adoptó de otra tableta.
  nuevoDePedido: Record<number, string>;
  enVuelo: number;
}

const inicial: CapturaState = { pendientes: [], naciendo: {}, nuevoDePedido: {}, enVuelo: 0 };
const useCaptura = create<CapturaState>()(() => inicial);

// Lo que no hace falta pintar: el cuerpo con el que nace cada cuenta, la fila de cada una y los ids
// que el servidor cambió por otros.
const bases = new Map<string, Omit<CreateDraftBody, 'lines'>>();
const colas = new Map<string, Promise<unknown>>();
const adoptados = new Map<string, string>();

// reiniciarCaptura olvida todo lo que esta tableta tenía en vuelo. Lo usan el cambio de empresa y
// los tests.
export function reiniciarCaptura(): void {
  useCaptura.setState(inicial, true);
  bases.clear();
  colas.clear();
  adoptados.clear();
  ultimaPropia = 0;
}

const real = (id: string) => adoptados.get(id) ?? id;

// Cuándo escribió esta tableta por última vez: lo que el servidor devuelva en los siguientes
// segundos es eco de lo propio, no un cambio de otra tableta.
let ultimaPropia = 0;
const VENTANA_PROPIA_MS = 3000;
const marcarPropia = () => { ultimaPropia = Date.now(); };
const recienEscribio = () => Date.now() - ultimaPropia < VENTANA_PROPIA_MS;

function fotoDeBorrador(d: DraftView): FotoDeCuenta {
  return {
    nombre: d.folioName ?? '', estado: d.status, falta: Number(d.total),
    renglones: Object.fromEntries((d.lines ?? []).map((l) => [l.id, Number(l.qty)])),
  };
}

function fotoDePedido(o: OrderView): FotoDeCuenta {
  return {
    nombre: o.folioName, estado: o.status, falta: Number(o.outstanding),
    renglones: Object.fromEntries((o.lines ?? []).filter((l) => !l.cancelled && l.id !== undefined)
      .map((l) => [String(l.id), Number(l.quantity)])),
  };
}

function encolar<T>(cuenta: string, fn: () => Promise<T>): Promise<T> {
  const antes = colas.get(cuenta) ?? Promise.resolve();
  const sigue = antes.catch(() => undefined).then(fn);
  colas.set(cuenta, sigue);
  return sigue;
}

function cuantoEnVuelo(delta: number) {
  useCaptura.setState((s) => ({ enVuelo: Math.max(0, s.enVuelo + delta) }));
}

const claveDraft = (id: string) => ['pos', 'draft', id] as const;

function guardarVista(qc: QueryClient, v: DraftView) {
  qc.setQueryData(claveDraft(v.id), v);
  qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
}

function avisarCambio() {
  toaster.create({
    title: 'La cuenta cambió en otra tableta',
    description: 'Ya se muestra como quedó.',
    type: 'info',
  });
}

export interface Cuenta {
  seleccion: Seleccion | null;
  vista: VistaCuenta;
  sinConexion: boolean;
  agregar: (p: ProductoAAgregar) => void;
  mas: (r: RenglonNuevo) => void;
  menos: (r: RenglonNuevo) => void;
  quitar: (r: RenglonNuevo) => void;
  cambiar: (r: RenglonNuevo, modifiers: TicketModifier[], notes: string) => void;
  cabecera: (h: DraftHeader) => void;
  // Espera a que todo lo que la cuenta tiene en fila termine. Lo usa «Enviar»: lo que se mande a
  // cocina tiene que incluir el último toque.
  esperar: () => Promise<void>;
}

interface Opciones {
  // El envío del negocio, para la cuenta que nace ya en domicilio.
  envioPorDefecto?: number;
  // Esta tableta está cobrando: lo que cambie el pedido mientras tanto es suyo, no de otra.
  cobrando?: boolean;
}

export function useCuenta({ envioPorDefecto = 0, cobrando = false }: Opciones = {}): Cuenta {
  const qc = useQueryClient();
  const seleccion = usePosStore((s) => s.selected);
  const nueva = usePosStore((s) => s.nueva);
  const captura = useCaptura();
  const sinConexion = useSinConexion();
  const { data: vivas } = useCuentasVivas();
  const { data: folios } = useQuery({
    queryKey: ['pos', 'folio-names'], queryFn: posApi.folioNames, staleTime: 60_000,
  });

  const pedidoId = seleccion?.kind === 'order' ? seleccion.id : null;
  const delPedido = pedidoId === null ? undefined
    : (captura.nuevoDePedido[pedidoId]
      ?? vivas?.items?.find((a) => a.orderId === pedidoId)?.pendingDraftId
      ?? undefined);
  const cuentaLocal = seleccion?.kind === 'draft' ? seleccion.id : delPedido;
  const borradorId = cuentaLocal ? real(cuentaLocal) : undefined;
  const confirmada = borradorId !== undefined && cuentaLocal !== undefined && !(cuentaLocal in captura.naciendo);

  const draftQ = useQuery({
    queryKey: claveDraft(borradorId ?? ''),
    queryFn: () => posApi.getDraft(borradorId as string),
    enabled: confirmada,
    retry: false,
  });
  const orderQ = useQuery({
    queryKey: ['orders', pedidoId],
    queryFn: () => posApi.order(pedidoId as number),
    enabled: pedidoId !== null,
    retry: false,
  });

  // Lo que el servidor no encuentra se olvida sin aviso: otra empresa en esta tableta, o una cuenta
  // que se descartó mientras la tableta dormía.
  const olvidar = usePosStore((s) => s.olvidar);
  useEffect(() => {
    const e = seleccion?.kind === 'draft' ? draftQ.error : orderQ.error;
    if (seleccion && e instanceof ApiError && e.status === 404) olvidar(seleccion);
  }, [seleccion, draftQ.error, orderQ.error, olvidar]);

  // LO QUE CAMBIÓ EN OTRA TABLETA (FR-017). Se compara la foto anterior de ESTA cuenta con la que
  // acaba de llegar; lo propio —escrituras de los últimos segundos, un cobro en curso— no avisa.
  const foto = seleccion?.kind === 'draft'
    ? (draftQ.data ? fotoDeBorrador(draftQ.data) : undefined)
    : (orderQ.data ? fotoDePedido(orderQ.data) : undefined);
  const claveSel = seleccion ? `${seleccion.kind}:${seleccion.id}` : '';
  const anterior = useRef<{ clave: string; foto?: FotoDeCuenta }>({ clave: '' });
  const enVueloAhora = captura.enVuelo;
  useEffect(() => {
    const prev = anterior.current;
    anterior.current = { clave: claveSel, foto };
    if (prev.clave !== claveSel || !foto) return;
    const propia = enVueloAhora > 0 || cobrando || recienEscribio();
    const aviso = cambioEnOtraTableta(prev.foto, foto, propia);
    if (aviso) toaster.create({ title: aviso, description: 'La cuenta se actualizó.', type: 'info' });
    const d = draftQ.data;
    if (seleccion?.kind === 'draft' && d && d.id === real(seleccion.id)) {
      if (d.status === 'enviada' && d.orderId !== null) usePosStore.getState().seleccionar({ kind: 'order', id: d.orderId });
      if (d.status === 'descartada') usePosStore.getState().cuentaNueva();
    }
    // Solo cuando llega una foto nueva de la cuenta abierta.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [foto?.estado, foto?.falta, JSON.stringify(foto?.renglones), claveSel]);

  // Una cuenta que ya se mandó o se descartó en otra tableta deja de ser «la que se captura».
  const draftVivo = draftQ.data && draftQ.data.status === 'capturando' ? draftQ.data : undefined;

  const pendientes = useMemo(
    () => captura.pendientes.filter((p) => p.cuenta === cuentaLocal),
    [captura.pendientes, cuentaLocal],
  );
  const vista = armarVista({
    draft: draftVivo,
    order: pedidoId !== null ? (orderQ.data as OrderView | undefined) : undefined,
    pendientes,
    cabeceraNueva: nueva,
    guardando: captura.enVuelo > 0 || (cuentaLocal !== undefined && captura.naciendo[cuentaLocal] === 'creando'),
  });

  const recargar = async (id: string): Promise<DraftView | undefined> => {
    try {
      const v = await qc.fetchQuery({ queryKey: claveDraft(id), queryFn: () => posApi.getDraft(id), staleTime: 0 });
      qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
      return v;
    } catch {
      return undefined;
    }
  };

  // Un rechazo de una escritura que no es agregar. `yaQuedo` decide si el reintento de algo que sí
  // entró merece aviso: si la cuenta recargada ya está como se pidió, no lo merece.
  const alRechazo = async (e: unknown, id: string, yaQuedo: (d: DraftView) => boolean) => {
    reportarResultado(e);
    // La recarga la pide esta tableta: su eco no es un cambio de otra.
    marcarPropia();
    if (esFalloDeRed(e)) {
      toaster.create({ title: 'No se guardó el cambio', description: mensajeDeError(e), type: 'error' });
      return;
    }
    const fresca = await recargar(id);
    if (e instanceof ApiError && (e.code === 'DRAFT_CHANGED' || e.status === 404)) {
      if (!fresca || !yaQuedo(fresca)) avisarCambio();
      return;
    }
    toaster.create({ title: mensajeDeError(e), type: 'error' });
  };

  const escribir = (cuenta: string, op: (id: string) => Promise<DraftView>, yaQuedo: (d: DraftView) => boolean) => {
    cuantoEnVuelo(1);
    encolar(cuenta, () => op(real(cuenta)))
      .then((v) => { marcarPropia(); reportarResultado(null); guardarVista(qc, v); })
      .catch((e) => alRechazo(e, real(cuenta), yaQuedo))
      .finally(() => cuantoEnVuelo(-1));
  };

  const propuesto = (): string | null => {
    const vivos = (vivas?.items ?? []).map((a) => a.folioName ?? '').filter(Boolean);
    return nombreLibre(folios?.items ?? [], vivos) || null;
  };

  // Un agregado: crea la cuenta si esta tableta la está haciendo nacer, o le suma un renglón. Se
  // decide AL EJECUTAR, no al tocar: si la creación falló, el siguiente toque la vuelve a intentar.
  const ejecutarAgregado = async (cuenta: string, linea: DraftLineInput): Promise<DraftView> => {
    const base = bases.get(cuenta);
    if (!base || !(cuenta in useCaptura.getState().naciendo)) {
      return posApi.addDraftLine(real(cuenta), linea);
    }
    useCaptura.setState((s) => ({ naciendo: { ...s.naciendo, [cuenta]: 'creando' } }));
    try {
      const v = await posApi.createDraft({ ...base, lines: [linea] });
      useCaptura.setState((s) => {
        const naciendo = { ...s.naciendo };
        delete naciendo[cuenta];
        const nuevoDePedido = { ...s.nuevoDePedido };
        if (base.orderId !== null && v.id !== cuenta) nuevoDePedido[base.orderId] = v.id;
        return { naciendo, nuevoDePedido };
      });
      if (v.id !== cuenta) adoptados.set(cuenta, v.id);
      bases.delete(cuenta);
      return v;
    } catch (e) {
      useCaptura.setState((s) => ({ naciendo: { ...s.naciendo, [cuenta]: 'fallo' } }));
      throw e;
    }
  };

  const correrAgregado = (pendiente: Pendiente, linea: DraftLineInput) => {
    const cuenta = pendiente.cuenta;
    useCaptura.setState((s) => ({ pendientes: [...s.pendientes, pendiente], enVuelo: s.enVuelo + 1 }));
    const fuera = () => useCaptura.setState((s) => ({
      pendientes: s.pendientes.filter((p) => p.opId !== pendiente.opId),
      enVuelo: Math.max(0, s.enVuelo - 1),
    }));
    encolar(cuenta, () => ejecutarAgregado(cuenta, linea))
      .then((v) => { marcarPropia(); reportarResultado(null); guardarVista(qc, v); fuera(); })
      .catch((e: unknown) => {
        fuera();
        reportarResultado(e);
        if (e instanceof ApiError && e.code === 'ORDER_CLOSED') {
          toaster.create({
            title: mensajeDeError(e), type: 'info',
            action: { label: 'Cuenta nueva', onClick: () => usePosStore.getState().cuentaNueva() },
          });
          return;
        }
        if (e instanceof ApiError && e.status === 404) {
          void recargar(real(cuenta));
          avisarCambio();
          return;
        }
        toaster.create({
          title: `No se guardó ${pendiente.name}`,
          description: mensajeDeError(e),
          type: 'error',
          action: { label: 'Reintentar', onClick: () => correrAgregado(pendiente, linea) },
        });
      });
  };

  const agregar = (p: ProductoAAgregar) => {
    if (sinConexion) {
      toaster.create({
        title: 'Sin conexión: no se agregó',
        description: `${p.name} no entró a la cuenta. Tócalo otra vez cuando regrese la red.`,
        type: 'warning',
      });
      return;
    }
    if (vista.tipo === 'pedido' && vista.esPlataforma) {
      toaster.create({ title: 'A un pedido de plataforma no se le agregan productos.', type: 'info' });
      return;
    }
    const st = usePosStore.getState();
    let cuenta: string;
    if (st.selected === null) {
      cuenta = uuid();
      bases.set(cuenta, {
        id: cuenta, orderId: null, folioName: propuesto(),
        header: {
          serviceType: st.nueva.serviceType,
          platformId: st.nueva.platformId,
          platformOrderRef: st.nueva.platformOrderRef.trim() || null,
          customerName: st.nueva.customerName.trim() || null,
          ...(st.nueva.serviceType === 'domicilio' && st.nueva.platformId === null
            ? { deliveryFee: envioPorDefecto.toFixed(2) } : {}),
        },
      });
      useCaptura.setState((s) => ({ naciendo: { ...s.naciendo, [cuenta]: 'creando' } }));
      st.seleccionar({ kind: 'draft', id: cuenta });
    } else if (st.selected.kind === 'draft') {
      cuenta = st.selected.id;
    } else if (cuentaLocal) {
      cuenta = cuentaLocal;
    } else {
      const orderId = st.selected.id;
      cuenta = uuid();
      bases.set(cuenta, { id: cuenta, orderId });
      useCaptura.setState((s) => ({
        naciendo: { ...s.naciendo, [cuenta]: 'creando' },
        nuevoDePedido: { ...s.nuevoDePedido, [orderId]: cuenta },
      }));
    }
    const opId = uuid();
    const notes = p.notes ?? '';
    correrAgregado(
      { opId, cuenta, productId: p.productId, name: p.name, qty: p.qty, unitPrice: p.unitPrice, modifiers: p.modifiers, notes },
      {
        opId, productId: p.productId, qty: String(p.qty), notes,
        modifiers: p.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
      },
    );
  };

  const deEsta = (fn: (cuenta: string) => void) => {
    if (!cuentaLocal) return;
    fn(cuentaLocal);
  };

  return {
    seleccion,
    vista,
    sinConexion,
    agregar,
    mas: (r) => deEsta((c) => {
      if (r.guardando) return;
      escribir(c, (id) => posApi.addDraftLine(id, { opId: uuid(), intoLineId: r.id, qty: '1' }), () => false);
    }),
    menos: (r) => deEsta((c) => {
      if (r.guardando) return;
      if (r.qty <= 1) {
        escribir(c, (id) => posApi.removeDraftLine(id, r.id, r.version),
          (d) => !(d.lines ?? []).some((l) => l.id === r.id));
        return;
      }
      const quiero = r.qty - 1;
      escribir(c, (id) => posApi.changeDraftLine(id, r.id, { expectedVersion: r.version, qty: String(quiero) }),
        (d) => (d.lines ?? []).some((l) => l.id === r.id && Number(l.qty) === quiero));
    }),
    quitar: (r) => deEsta((c) => {
      if (r.guardando) return;
      escribir(c, (id) => posApi.removeDraftLine(id, r.id, r.version),
        (d) => !(d.lines ?? []).some((l) => l.id === r.id));
    }),
    cambiar: (r, modifiers, notes) => deEsta((c) => {
      escribir(c, (id) => posApi.changeDraftLine(id, r.id, {
        expectedVersion: r.version, notes,
        modifiers: modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
      }), () => false);
    }),
    cabecera: (h) => {
      const st = usePosStore.getState();
      if (st.selected === null) {
        st.cambiarNueva({
          ...(h.serviceType !== undefined ? { serviceType: h.serviceType } : {}),
          ...(h.platformId !== undefined ? { platformId: h.platformId } : {}),
          ...(h.platformOrderRef !== undefined ? { platformOrderRef: h.platformOrderRef ?? '' } : {}),
          ...(h.customerName !== undefined ? { customerName: h.customerName ?? '' } : {}),
        });
        return;
      }
      if (st.selected.kind !== 'draft') return;
      const cuenta = st.selected.id;
      escribir(cuenta, async (id) => {
        const actual = qc.getQueryData<DraftView>(claveDraft(id)) ?? await posApi.getDraft(id);
        return posApi.patchDraft(id, { expectedHeaderVersion: actual.headerVersion, ...h });
      }, () => false);
    },
    esperar: async () => {
      if (!cuentaLocal) return;
      await (colas.get(cuentaLocal) ?? Promise.resolve()).catch(() => undefined);
    },
  };
}
