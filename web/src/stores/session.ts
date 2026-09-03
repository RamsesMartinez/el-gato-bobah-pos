import { create } from 'zustand';

import { empresaRecordada, hayQueLimpiar, recordarEmpresa } from '../app/tenantSwitch';

export interface SessionUser {
  id: number;
  companyId: number;
  companySlug?: string;
  name: string;
  role: string;
  mustChangePassword?: boolean; // tras alta/reset por admin: forzar cambio en el primer login
}

// 'loading' hasta que el arranque intenta canjear la cookie de refresh; luego 'authed' o
// 'anon'. Evita rebotar a /login antes de saber si hay una sesión viva tras un reload.
export type SessionStatus = 'loading' | 'authed' | 'anon';

interface SessionState {
  // El access token vive SOLO en memoria (nunca en localStorage): así un XSS no puede
  // robarlo del disco (A02/A05). Se re-emite con la cookie HttpOnly de refresh al recargar.
  token: string | null;
  user: SessionUser | null;
  // Por qué terminó la última sesión: 'caducada' cuando pasó el plazo del turno, null si se salió
  // a propósito o nunca hubo una.
  motivo: 'caducada' | null;
  status: SessionStatus;
  setSession: (token: string, user: SessionUser) => void;
  clear: (motivo?: 'caducada') => void;
}

// Qué tirar cuando el dispositivo entra con una empresa distinta a la anterior. Lo registra
// main.tsx con la caché de queries y el carrito; el store no los importa para no amarrar el
// estado de sesión a React Query. Sin registrar es un no-op (tests que montan el store solo).
let limpiarDatosDelTenant: () => void = () => {};

export function registrarLimpiezaDeTenant(fn: () => void): void {
  limpiarDatosDelTenant = fn;
}

export const useSessionStore = create<SessionState>()((set) => ({
  token: null,
  user: null,
  motivo: null,
  status: 'loading',
  // setSession es el único punto por el que se entra (login, y el canje del refresh al arrancar),
  // así que es donde se decide si lo que quedó en el dispositivo es de otra empresa. Se hace al
  // ENTRAR y no al salir: cerrar sesión también pasa cuando el refresh falla por un hipo de red, y
  // limpiar ahí le borraría el ticket a medias a un cajero que va a volver a la misma empresa.
  setSession: (token, user) => {
    if (hayQueLimpiar(empresaRecordada(), user.companyId)) {
      limpiarDatosDelTenant();
    }
    recordarEmpresa(user.companyId);
    set({ token, user, status: 'authed' });
  },
  // motivo dice POR QUÉ se cerró la sesión, para que la pantalla de login no muestre un error
  // genérico cuando lo que pasó fue que el turno terminó. Sin esto, quien llega en la mañana ve
  // "no autorizado" y cree que algo se rompió.
  clear: (motivo) => set({ token: null, user: null, status: 'anon', motivo: motivo ?? null }),
}));
