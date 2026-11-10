import { useState } from 'react';
import { Badge, Box, Button, Code, Flex, Grid, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuCheck, LuCopy, LuKeyRound, LuPlus, LuSquare, LuSquareCheck } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Page } from '../../components/Page';
import { Picker } from '../../components/Picker';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { InfoButton } from '../../components/InfoButton';
import { toaster } from '../../components/ui/toaster';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter } from '../../components/ui/drawer';
import { mensajeDeError } from '../../api/mensajes';
import { connectCommand, integrationsApi, mcpUrl, type ApiToken, type CreatedApiToken, type TokenScope } from '../../api/integrations';
import { useSessionStore } from '../../stores/session';
import { RADIUS } from '../../theme/ui';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

// Integraciones (spec 072, maqueta A): los tokens con los que un asistente como Claude opera el
// negocio con los permisos de quien lo crea. Lista a todo el ancho y alta en una hoja inferior.

const WRITE_SCOPES: { v: TokenScope; label: string; hint: string }[] = [
  { v: 'write:expenses', label: 'Gastos', hint: 'Capturar, pagar y anular gastos; dar de alta proveedores' },
  { v: 'write:cash', label: 'Caja', hint: 'Entradas y salidas de efectivo' },
  { v: 'write:inventory', label: 'Almacén', hint: 'Mermas y ajustes de existencia' },
  { v: 'write:catalog', label: 'Menú y precios', hint: 'Precio y disponibilidad, precios por plataforma' },
];
const LABEL: Record<TokenScope, string> = {
  read: 'Consultar', 'write:expenses': 'Gastos', 'write:cash': 'Caja', 'write:inventory': 'Almacén', 'write:catalog': 'Menú y precios',
};
const LIFETIMES = [{ value: '30', label: '30 días' }, { value: '90', label: '90 días' }, { value: '180', label: '180 días' }];


function copy(text: string, onOk: () => void) {
  void navigator.clipboard?.writeText(text).then(onOk,
    () => toaster.create({ title: 'No se pudo copiar', description: 'Selecciónalo y cópialo a mano.', type: 'error' }));
}

function TokenCard({ t, onRevoke }: { t: ApiToken; onRevoke?: () => void }) {
  const hora = useHoraDelNegocio();
  const fecha = (iso: string) => hora.soloFecha(iso);
  const cuando = (iso: string | null) => (iso ? hora.fechaYHora(iso) : 'nunca');
  const live = t.status === 'active';
  return (
    <Flex borderWidth="1px" borderRadius={RADIUS} p={3} gap={3} align="center" opacity={live ? 1 : 0.6}>
      <Box color="fg.muted"><LuKeyRound size={22} /></Box>
      <Box flex="1" minW={0}>
        <HStack gap={2} flexWrap="wrap">
          <Text fontWeight="700">{t.name}</Text>
          <Badge colorPalette={live ? 'green' : 'gray'}>{live ? 'Activo' : t.status === 'revoked' ? 'Revocado' : 'Vencido'}</Badge>
        </HStack>
        <Text fontSize="sm" color="fg.muted">
          {t.ownerName} · {live ? `vence ${fecha(t.expiresAt)}` : t.status === 'revoked' && t.revokedAt ? `revocado ${fecha(t.revokedAt)}` : `venció ${fecha(t.expiresAt)}`} · último uso {cuando(t.lastUsedAt)}
        </Text>
        <HStack gap={1} mt={1} flexWrap="wrap">
          {(t.scopes ?? []).map((s) => <Badge key={s} variant="subtle" colorPalette={s === 'read' ? 'gray' : 'brand'}>{LABEL[s]}</Badge>)}
        </HStack>
      </Box>
      {live && onRevoke && (
        <Button minH="44px" variant="outline" colorPalette="red" aria-label={`Revocar ${t.name}`} onClick={onRevoke}>Revocar</Button>
      )}
    </Flex>
  );
}

function CreateSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const [name, setName] = useState(`Claude de ${user?.name ?? 'mi computadora'}`);
  const [scopes, setScopes] = useState<TokenScope[]>([]);
  const [days, setDays] = useState<'30' | '90' | '180'>('90');
  const [password, setPassword] = useState('');
  const [created, setCreated] = useState<CreatedApiToken | null>(null);
  const [copied, setCopied] = useState(false);
  const [help, setHelp] = useState(false);

  const reset = () => {
    setName(`Claude de ${user?.name ?? 'mi computadora'}`);
    setScopes([]); setDays('90'); setPassword(''); setCreated(null); setCopied(false); setHelp(false);
  };
  const close = () => { reset(); onClose(); };
  const create = useMutation({
    mutationFn: () => integrationsApi.create({ name: name.trim(), scopes, expiresInDays: Number(days) as 30 | 90 | 180, password }),
    onSuccess: (t) => {
      setCreated(t);
      setPassword('');
      void qc.invalidateQueries({ queryKey: ['integrations', 'tokens'] });
    },
    onError: (e) => toaster.create({ title: 'No se creó el token', description: mensajeDeError(e), type: 'error' }),
  });
  const command = connectCommand(mcpUrl(), user?.companySlug);
  const toggle = (s: TokenScope) => setScopes((xs) => (xs.includes(s) ? xs.filter((x) => x !== s) : [...xs, s]));

  return (
    // Con el token a la vista la hoja solo se cierra con «Ya lo copié»: el token se muestra una sola
    // vez, y un toque en el fondo lo perdería sin aviso.
    <DrawerRoot open={open} placement="bottom" closeOnInteractOutside={!created} closeOnEscape={!created}
      onOpenChange={(e) => { if (!e.open && !created) close(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}><Text fontSize="lg" fontWeight="700">{created ? 'Tu token' : 'Crear token'}</Text></DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          {!created ? (
            <VStack align="stretch" gap={3}>
              <Flex gap={3}>
                <Box flex="1">
                  <Text fontSize="sm" fontWeight="600">Nombre</Text>
                  <Input id="token-name" aria-label="Nombre" size="lg" minH="48px" maxLength={60} value={name} onChange={(e) => setName(e.target.value)} />
                </Box>
                <Box w="180px">
                  <Text fontSize="sm" fontWeight="600">Vence en</Text>
                  <Picker value={days} options={LIFETIMES} onChange={(v) => setDays((v as '30' | '90' | '180') ?? '90')} title="Vence en" focusSearch={false} />
                </Box>
              </Flex>
              <Box>
                <Text fontSize="sm" fontWeight="600">Qué más puede hacer</Text>
                <Text fontSize="xs" color="fg.muted" mb={1}>
                  Siempre puede consultar ventas, caja, gastos, almacén, menú y bitácora. Nunca más de lo que tu usuario puede hacer.
                </Text>
                <Grid templateColumns="1fr 1fr" gap={2}>
                  {WRITE_SCOPES.map((s) => {
                    const on = scopes.includes(s.v);
                    return (
                      <Button key={s.v} role="checkbox" aria-checked={on} aria-label={s.label} variant="outline"
                        colorPalette={on ? 'brand' : 'gray'} justifyContent="flex-start" minH="52px" h="auto" py={2}
                        onClick={() => toggle(s.v)}>
                        {on ? <LuSquareCheck /> : <LuSquare />}
                        <Box textAlign="left" whiteSpace="normal">
                          <Text fontWeight="600">{s.label}</Text>
                          <Text fontSize="xs" color="fg.muted">{s.hint}</Text>
                        </Box>
                      </Button>
                    );
                  })}
                </Grid>
              </Box>
              <Box>
                <Text fontSize="sm" fontWeight="600">Tu contraseña</Text>
                <Input id="token-password" aria-label="Tu contraseña" type="password" size="lg" minH="48px" autoComplete="current-password"
                  value={password} onChange={(e) => setPassword(e.target.value)} />
              </Box>
            </VStack>
          ) : (
            <VStack align="stretch" gap={3}>
              <HStack color="green.600"><LuCheck /><Text fontWeight="700">Token creado. Cópialo ahora: es la única vez que se muestra.</Text></HStack>
              <Box borderWidth="1px" borderRadius={RADIUS} p={3} bg="bg.muted">
                <Code fontSize="sm" wordBreak="break-all" bg="transparent">{created.token}</Code>
              </Box>
              <HStack gap={2}>
                <Button flex="1" minH="52px" colorPalette="brand" onClick={() => copy(created.token, () => setCopied(true))}>
                  {copied ? <><LuCheck /> Copiado</> : <><LuCopy /> Copiar token</>}
                </Button>
                <InfoButton label="Cómo conectarlo" open={help} onToggle={() => setHelp((h) => !h)} />
              </HStack>
              {help && (
                <VStack align="stretch" gap={2} borderWidth="1px" borderRadius={RADIUS} p={3}>
                  <Text fontSize="sm">1. En tu computadora, guarda el token como POS_MCP_TOKEN.</Text>
                  <Text fontSize="sm">2. Pega este comando en una terminal:</Text>
                  <Box bg="bg.muted" borderRadius={RADIUS} p={2}>
                    <Code fontSize="sm" whiteSpace="pre-wrap" bg="transparent">{command}</Code>
                  </Box>
                  <Button minH="44px" variant="outline" colorPalette="gray"
                    onClick={() => copy(command, () => toaster.create({ title: 'Comando copiado', type: 'success' }))}>
                    <LuCopy /> Copiar comando
                  </Button>
                  <Text fontSize="sm">3. Abre Claude Code y pregunta «¿cuánto vendimos ayer?».</Text>
                </VStack>
              )}
            </VStack>
          )}
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          {!created ? (
            <HStack w="100%" gap={2}>
              <Button flex="1" minH="52px" variant="outline" colorPalette="gray" onClick={close}>Volver</Button>
              <Button flex="1" minH="52px" colorPalette="brand" loading={create.isPending}
                disabled={!name.trim() || !password} onClick={() => create.mutate()}>Crear token</Button>
            </HStack>
          ) : <Button w="100%" minH="52px" onClick={close}>Ya lo copié</Button>}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

export function IntegrationsPage() {
  const qc = useQueryClient();
  const tokens = useQuery({ queryKey: ['integrations', 'tokens'], queryFn: integrationsApi.list });
  const [creating, setCreating] = useState(false);
  const [showOld, setShowOld] = useState(false);
  const [revoking, setRevoking] = useState<ApiToken | null>(null);
  const revoke = useMutation({
    mutationFn: (id: number) => integrationsApi.revoke(id),
    onSuccess: () => {
      setRevoking(null);
      void qc.invalidateQueries({ queryKey: ['integrations', 'tokens'] });
      toaster.create({ title: 'Token revocado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se revocó', description: mensajeDeError(e), type: 'error' }),
  });

  const items = tokens.data?.items ?? [];
  const live = items.filter((t) => t.status === 'active');
  const old = items.filter((t) => t.status !== 'active');

  return (
    <Page fill>
      <HStack justify="space-between" mb={3} gap={3}>
        <Box minW={0}>
          <Text fontSize="2xl" fontWeight="700">Integraciones</Text>
          <Text fontSize="sm" color="fg.muted">Tokens para que un asistente como Claude opere el negocio con los permisos de quien lo crea.</Text>
        </Box>
        <Button minH="44px" colorPalette="brand" flexShrink={0} onClick={() => setCreating(true)}><LuPlus /> Crear token</Button>
      </HStack>
      <VStack align="stretch" gap={2} flex="1" minH={0} overflowY="auto">
        {tokens.isError && <Text color="fg.error">{mensajeDeError(tokens.error)}</Text>}
        {tokens.isSuccess && live.length === 0 && (
          <Text color="fg.muted">Todavía no hay tokens activos. Crea uno para conectar a Claude.</Text>
        )}
        {live.map((t) => <TokenCard key={t.id} t={t} onRevoke={() => setRevoking(t)} />)}
        {old.length > 0 && (
          <Button alignSelf="flex-start" minH="44px" variant="ghost" colorPalette="gray" onClick={() => setShowOld((v) => !v)}>
            {showOld ? 'Ocultar revocados y vencidos' : `Ver revocados y vencidos (${old.length})`}
          </Button>
        )}
        {showOld && old.map((t) => <TokenCard key={t.id} t={t} />)}
      </VStack>
      <CreateSheet open={creating} onClose={() => setCreating(false)} />
      <ConfirmSheet isOpen={!!revoking} title={`Revocar «${revoking?.name ?? ''}»`} destructive confirmLabel="Revocar"
        loading={revoke.isPending}
        description="Deja de funcionar en la siguiente llamada. No se puede deshacer: para volver a conectar se crea otro."
        onConfirm={() => revoking && revoke.mutate(revoking.id)} onCancel={() => setRevoking(null)} />
    </Page>
  );
}
