import { useEffect, useState, useSyncExternalStore } from 'react';
import { ApiError } from '../../../api/client';

// La cola del autoguardado de la ficha (spec 043). Texto y números se mandan tras 2 s sin escribir o
// al salir del campo; las listas al elegir. Un guardado a la vez y en orden, cada uno con la versión
// que devolvió el anterior: dos en paralelo chocarían entre sí con 409. Ningún campo se deshabilita
// mientras guarda (cerraría el teclado de la tableta): lo que se siga escribiendo entra a la cola.

const PAUSE_MS = 2000;

export type AutosaveStatus = 'idle' | 'dirty' | 'saving' | 'saved' | 'error';

export interface AutosaveOptions<F extends Record<string, string>> {
  initial: F;
  version: string;
  // save recibe también lo último guardado de todos los campos, para mandar la ficha completa.
  save: (patch: Partial<F>, version: string, saved: F) => Promise<{ updatedAt: string }>;
  // validate devuelve el mensaje de un valor que no se puede guardar, o null.
  validate: (field: keyof F, value: string) => string | null;
  // complete dice si lo escrito ya es un valor: «1.» o «-» no lo son y no se mandan por la pausa.
  complete: (field: keyof F, value: string) => boolean;
  // onReject: el servidor rechazó el cambio (nombre repetido, dato inválido); el campo vuelve a saved.
  onReject: (field: keyof F, saved: string, error: unknown) => void;
  // onStale: otra persona cambió la ficha; hay que recargarla, y la cola ya no vale.
  onStale: () => void;
}

interface Entry<F> { field: keyof F; value: string }

export interface AutosaveView<F> {
  status: AutosaveStatus;
  unsaved: (keyof F)[];
  invalid: Partial<Record<keyof F, string>>;
}

export class AutosaveQueue<F extends Record<string, string>> {
  private saved: F;
  private ver: string;
  private queue: Entry<F>[] = [];
  private inFlight: Entry<F> | null = null;
  private failed: Entry<F> | null = null;
  private invalid: Partial<Record<keyof F, string>> = {};
  private typing = new Set<keyof F>();
  private timers = new Map<keyof F, ReturnType<typeof setTimeout>>();
  private waiters: ((allSaved: boolean) => void)[] = [];
  private savedOnce = false;
  private listeners = new Set<() => void>();
  private view: AutosaveView<F> = { status: 'idle', unsaved: [], invalid: {} };
  private opts: AutosaveOptions<F>;

  constructor(opts: AutosaveOptions<F>) {
    this.opts = opts;
    this.saved = { ...opts.initial };
    this.ver = opts.version;
  }

  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  snapshot = () => this.view;
  version = () => this.ver;

  private unsaved(): (keyof F)[] {
    return [...(this.failed ? [this.failed.field] : []), ...(Object.keys(this.invalid) as (keyof F)[])];
  }

  private refresh() {
    const status: AutosaveStatus = this.failed ? 'error'
      : this.inFlight || this.queue.length ? 'saving'
        : this.typing.size || Object.keys(this.invalid).length ? 'dirty'
          : this.savedOnce ? 'saved' : 'idle';
    this.view = { status, unsaved: this.unsaved(), invalid: { ...this.invalid } };
    this.listeners.forEach((fn) => fn());
  }

  private settle() {
    if (this.inFlight || this.queue.length) return;
    const ok = this.unsaved().length === 0;
    this.waiters.splice(0).forEach((w) => w(ok));
  }

  private pump() {
    if (this.inFlight || this.failed) return;
    const next = this.queue.shift();
    if (!next) { this.refresh(); this.settle(); return; }
    this.inFlight = next;
    this.refresh();
    // Un error dentro del éxito también cae en el manejador de abajo: si no, la cola se quedaría en
    // «Guardando…» para siempre.
    this.opts.save({ [next.field]: next.value } as Partial<F>, this.ver, this.saved).then((r) => {
      this.ver = r?.updatedAt ?? this.ver;
      this.saved = { ...this.saved, [next.field]: next.value };
      this.savedOnce = true;
      this.inFlight = null;
      this.pump();
    }).catch((e: unknown) => {
      this.inFlight = null;
      if (e instanceof ApiError && e.code === 'STALE') {
        this.queue = [];
        this.opts.onStale();
        this.refresh();
        this.settle();
        return;
      }
      if (e instanceof ApiError && e.status >= 400 && e.status < 500) {
        // Rechazo del servidor: reintentar mandaría lo mismo y volvería a fallar.
        this.opts.onReject(next.field, this.saved[next.field], e);
        this.pump();
        return;
      }
      // Sin red o error del servidor: lo escrito se conserva para «Reintentar».
      this.failed = next;
      this.refresh();
      this.waiters.splice(0).forEach((w) => w(false));
    });
  }

  private pendingValue(field: keyof F) {
    const queued = [...this.queue].reverse().find((e) => e.field === field);
    if (queued) return queued.value;
    if (this.failed?.field === field) return this.failed.value;
    if (this.inFlight?.field === field) return this.inFlight.value;
    return this.saved[field];
  }

  commit = (field: keyof F, value: string) => {
    clearTimeout(this.timers.get(field));
    this.typing.delete(field);
    const msg = this.opts.validate(field, value);
    if (msg) {
      this.invalid = { ...this.invalid, [field]: msg };
      this.refresh();
      return;
    }
    if (field in this.invalid) {
      const rest = { ...this.invalid };
      delete rest[field];
      this.invalid = rest;
    }
    if (value === this.pendingValue(field)) { this.refresh(); this.settle(); return; }
    if (this.failed?.field === field) this.failed = null;
    const waiting = this.queue.find((e) => e.field === field);
    if (waiting) waiting.value = value;
    else this.queue.push({ field, value });
    this.pump();
    this.refresh();
  };

  type = (field: keyof F, value: string) => {
    clearTimeout(this.timers.get(field));
    this.typing.add(field);
    this.refresh();
    this.timers.set(field, setTimeout(() => {
      if (this.opts.complete(field, value)) this.commit(field, value);
    }, PAUSE_MS));
  };

  retry = () => {
    const f = this.failed;
    if (!f) return;
    this.failed = null;
    this.queue.unshift(f);
    this.pump();
  };

  busy = (field: keyof F) => this.inFlight?.field === field || this.queue.some((e) => e.field === field);

  whenIdle = () => new Promise<boolean>((resolve) => {
    if (!this.inFlight && !this.queue.length) resolve(this.unsaved().length === 0);
    else this.waiters.push(resolve);
  });

  // reset vuelve a tomar la ficha como guardada: tras recargarla o tras una hoja que la cambió.
  reset = (initial: F, v: string) => {
    this.saved = { ...initial };
    this.ver = v;
    this.failed = null;
    this.invalid = {};
    this.refresh();
  };

  // setOptions toma los callbacks del render más reciente: guardan sobre el estado vigente de la ficha.
  setOptions = (opts: AutosaveOptions<F>) => { this.opts = opts; };

  dispose = () => { this.timers.forEach((id) => clearTimeout(id)); };
}

export function useAutosave<F extends Record<string, string>>(opts: AutosaveOptions<F>) {
  const [q] = useState(() => new AutosaveQueue(opts));
  useEffect(() => { q.setOptions(opts); });
  useEffect(() => q.dispose, [q]);
  const view = useSyncExternalStore(q.subscribe, q.snapshot);
  return { ...view, commit: q.commit, type: q.type, retry: q.retry, busy: q.busy, whenIdle: q.whenIdle, reset: q.reset, version: q.version };
}
