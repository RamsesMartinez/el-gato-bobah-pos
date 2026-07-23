import { useState } from 'react';
import { Box, Text, HStack, VStack, Button, Circle } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { systemApi, frontendVersion } from '../api/system';
import { useAppUpdate } from '../stores/appUpdate';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../components/ui/dialog';

// SHA corto para mostrar (CI ya manda corto; esto acota un SHA largo si alguien lo inyecta así).
function short(sha: string) {
  return sha.length > 7 ? sha.slice(0, 7) : sha;
}
function fmtDate(iso: string) {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('es-MX', { dateStyle: 'medium', timeStyle: 'short' });
}

// Pie de sistema: marca de agua tenue con la versión del frontend (siempre visible en el sidebar);
// al tocarla abre "Detalles del sistema" con FE + backend y, si hay versión nueva, el botón para
// actualizar. Un punto verde sobre la marca avisa de actualización pendiente sin robar atención.
export function SystemInfo() {
  const [open, setOpen] = useState(false);
  const needRefresh = useAppUpdate((s) => s.needRefresh);
  const apply = useAppUpdate((s) => s.apply);
  // La versión del backend no cambia dentro de una sesión (un deploy recarga el front vía SW).
  const { data: be } = useQuery({
    queryKey: ['system', 'version'],
    queryFn: systemApi.backendVersion,
    staleTime: Infinity,
    retry: false,
  });

  return (
    <>
      <Box
        as="button" onClick={() => setOpen(true)} title="Detalles del sistema" px={1} mt={1}
        display="flex" alignItems="center" gap={1} opacity={0.45} _hover={{ opacity: 0.95 }}
        transition="opacity 0.15s"
      >
        {needRefresh && <Circle size="6px" bg="green.400" flexShrink={0} />}
        <Text fontSize="9px" color="gray.400" lineClamp={1}>v{short(frontendVersion.version)}</Text>
      </Box>

      <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) setOpen(false); }} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Detalles del sistema</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            <VStack align="stretch" gap={4}>
              {needRefresh && (
                <Box borderWidth="1px" borderColor="green.400" borderRadius="md" p={3}>
                  <Text fontSize="sm" fontWeight="600" mb={2}>Hay una versión nueva de la app.</Text>
                  <Button size="sm" colorPalette="green" w="100%" onClick={apply}>Actualizar ahora</Button>
                </Box>
              )}
              <VStack align="stretch" gap={2}>
                <VersionRow label="Frontend" version={frontendVersion.version} builtAt={frontendVersion.builtAt} />
                <VersionRow label="Backend" version={be?.version ?? '—'} builtAt={be?.builtAt ?? ''} />
              </VStack>
            </VStack>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </>
  );
}

function VersionRow({ label, version, builtAt }: { label: string; version: string; builtAt: string }) {
  return (
    <HStack justify="space-between" align="start" borderBottomWidth="1px" borderColor="border.muted" pb={2}>
      <Text color="fg.muted" fontSize="sm">{label}</Text>
      <VStack align="end" gap={0}>
        <Text fontFamily="mono" fontWeight="600" fontSize="sm">{version === '—' ? '—' : short(version)}</Text>
        <Text fontSize="xs" color="fg.subtle">{fmtDate(builtAt)}</Text>
      </VStack>
    </HStack>
  );
}
