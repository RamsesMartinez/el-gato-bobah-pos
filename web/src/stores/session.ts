import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface SessionUser {
  id: number;
  name: string;
  role: string;
}

interface SessionState {
  token: string | null;
  user: SessionUser | null;
  setSession: (token: string, user: SessionUser) => void;
  clear: () => void;
}

export const useSessionStore = create<SessionState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      setSession: (token, user) => set({ token, user }),
      clear: () => set({ token: null, user: null }),
    }),
    { name: 'egb:session:v1' },
  ),
);
