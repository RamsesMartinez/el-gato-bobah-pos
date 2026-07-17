import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useSessionStore } from '../stores/session';

export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useSessionStore((s) => s.token);
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
