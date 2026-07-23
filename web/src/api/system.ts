import { api } from './client';

export interface BackendVersion {
  version: string; // SHA del build (o "dev")
  builtAt: string; // ISO-8601 (o "")
}

// Versión del frontend, horneada por vite.config.ts (define) en el build.
export const frontendVersion = { version: __APP_VERSION__, builtAt: __APP_BUILT_AT__ };

export const systemApi = {
  backendVersion: () => api.get<BackendVersion>('/version'),
};
