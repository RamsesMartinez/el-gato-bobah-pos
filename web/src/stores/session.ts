import { create } from 'zustand';

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
  status: SessionStatus;
  setSession: (token: string, user: SessionUser) => void;
  clear: () => void;
}

export const useSessionStore = create<SessionState>()((set) => ({
  token: null,
  user: null,
  status: 'loading',
  setSession: (token, user) => set({ token, user, status: 'authed' }),
  clear: () => set({ token: null, user: null, status: 'anon' }),
}));
