import { Box, Button, HStack, Spinner, Stack, Text } from '@chakra-ui/react';
import { useInfiniteQuery } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi } from '../../../api/backoffice';
import { actionLabel } from '../../../shared/audit/labels';
import { useHoraDelNegocio } from '../../../hooks/useHoraDelNegocio';
import { whatChanged } from './articleAudit';

// La bitácora de un artículo (spec 042, FR-016): quién, cuándo y qué cambió, de 20 en 20. Solo lectura.

export function ArticleAuditSheet({ id, name, baseUnitCode, onClose }: { id: number; name: string; baseUnitCode: string; onClose: () => void }) {
  const hora = useHoraDelNegocio();
  const q = useInfiniteQuery({
    queryKey: ['article-audit', id],
    queryFn: ({ pageParam }) => backofficeApi.articleAudit(id, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (p) => p.next ?? undefined,
  });
  const items = q.data?.pages.flatMap((p) => p.items ?? []) ?? [];
  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader>
          <HStack justify="space-between">
            <Text fontSize="lg" fontWeight="700">Bitácora de «{name}»</Text>
            <Button minH="44px" variant="outline" onClick={onClose}>Cerrar</Button>
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={4}>
          {q.isLoading ? <Spinner /> : (
            <Stack gap={0}>
              {items.length === 0 && <Text color="fg.muted">Todavía no hay cambios registrados.</Text>}
              {items.map((e) => (
                <HStack key={e.id} minH="52px" borderBottomWidth="1px" gap={3} align="center">
                  <Text w="150px" flexShrink={0} fontSize="sm" color="fg.muted">{hora.fechaYHora(e.at)}</Text>
                  <Box flex="1" minW={0}>
                    <Text><b>{e.actor ?? 'Sistema'}</b> · {actionLabel(e.action)}</Text>
                    {whatChanged(e, baseUnitCode) && <Text fontSize="sm" color="fg.muted">{whatChanged(e, baseUnitCode)}</Text>}
                    {/* Los guardados seguidos de un mismo campo llegan juntos (spec 043): se dice cuántos. */}
                    {(e.changes ?? 1) > 1 && <Text fontSize="xs" color="fg.muted">{e.changes} cambios</Text>}
                  </Box>
                </HStack>
              ))}
              {q.hasNextPage && (
                <Button mt={2} minH="44px" variant="outline" loading={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>Ver más</Button>
              )}
            </Stack>
          )}
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
