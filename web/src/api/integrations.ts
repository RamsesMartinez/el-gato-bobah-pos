import { api } from './client';

// Los tokens del servidor MCP (spec 072). Las listas son opcionales para que el compilador obligue a
// la guarda si llegaran como null.
export type TokenScope = 'read' | 'write:expenses' | 'write:cash' | 'write:inventory' | 'write:catalog'
  | 'write:purchases' | 'write:orders';

export interface ApiToken {
  id: number;
  name: string;
  prefix: string;
  ownerId: number;
  ownerName: string;
  scopes?: TokenScope[];
  createdAt: string;
  expiresAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
  status: 'active' | 'revoked' | 'expired';
}

export interface CreatedApiToken extends ApiToken {
  token: string;
}

export interface CreateTokenInput {
  name: string;
  scopes: TokenScope[];
  expiresInDays: 30 | 90 | 180;
  password: string;
}

export const integrationsApi = {
  list: () => api.get<{ items?: ApiToken[] }>('/integrations/tokens'),
  create: (input: CreateTokenInput) => api.post<CreatedApiToken>('/integrations/tokens', input),
  revoke: (id: number) => api.post<void>(`/integrations/tokens/${id}/revoke`),
};

// mcpUrl es la dirección de /mcp de esta misma API: con la URL absoluta del build, su origen; en
// desarrollo (ruta relativa, con el proxy de Vite) el origen de la página.
export function mcpUrl(base: string = import.meta.env.VITE_API_URL || '/api/v1', origin: string = window.location.origin): string {
  try {
    return new URL(base).origin + '/mcp';
  } catch {
    return origin + '/mcp';
  }
}

// connectCommand es lo que se pega en una terminal: el token va por variable de entorno, con
// comillas simples para que se guarde la referencia y no el valor. En un solo renglón: la
// continuación con «\» es de bash, y en PowerShell partía el comando y lo registraba sin el header.
export function connectCommand(url: string, slug: string | undefined): string {
  const name = `pos-${(slug || 'negocio').replace(/[^a-z0-9-]/gi, '-').toLowerCase()}`;
  return `claude mcp add --transport http --scope user ${name} ${url} --header 'Authorization: Bearer \${POS_MCP_TOKEN}'`;
}
