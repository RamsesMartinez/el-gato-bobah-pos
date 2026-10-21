import { useState } from 'react';
import { medirAccion } from '../../../api/uso';
import { Box, Button, HStack, Spinner, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleSheet } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { toaster } from '../../../components/ui/toaster';
import { CompositionSheet } from '../../../shared/CompositionSheet';
import type { CompositionKind } from '../../../api/admin';
import { formatBaseQty } from '../../../shared/units';

// Archivar y reactivar (spec 042, FR-007). Antes de archivar se ve cuánto queda y qué recetas lo usan.
// Una receta activa lo impide, pero no es un callejón: cada una abre su receta y se vuelve aquí
// (revisión de tableta, A5).

interface Props {
  article: ArticleSheet;
  onClose: () => void;
  onDone: () => void;
}

export function ArchiveSheet({ article, onClose, onDone }: Props) {
  const qc = useQueryClient();
  const check = useQuery({ queryKey: ['article-archive-check', article.id], queryFn: () => backofficeApi.articleArchiveCheck(article.id), enabled: article.isActive });
  const [recipe, setRecipe] = useState<{ kind: CompositionKind; id: number; name: string } | null>(null);
  const act = useMutation({
    mutationFn: () => (article.isActive ? backofficeApi.archiveArticle(article.id) : backofficeApi.reactivateArticle(article.id)),
    onSuccess: () => {
      medirAccion('inventario', 'archivar-articulo');
      toaster.create({ title: article.isActive ? 'Archivado' : 'Volvió a estar activo', type: 'success' });
      onDone();
    },
    onError: (e) => toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo', type: 'error' }),
  });
  const recipes = check.data?.activeRecipes ?? [];
  const onHand = check.data?.onHand ?? [];
  const blocked = recipes.length > 0 || !!check.data?.openCount;

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader>
          <Text fontSize="lg" fontWeight="700">{article.isActive ? `¿Archivar «${article.name}»?` : `¿Reactivar «${article.name}»?`}</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          {article.isActive && check.isLoading ? <Spinner /> : (
            <Stack gap={3}>
              {article.isActive ? (
                <>
                  <Text color="fg.muted">Archivado deja de ofrecerse en gastos, recetas y conteo. Su historia se conserva.</Text>
                  {onHand.length > 0 && (
                    <Text>Queda en existencia: {onHand.map((b) => `${formatBaseQty(b.qty, article.baseUnitCode)} en ${b.branch}`).join(', ')}.</Text>
                  )}
                  {(check.data?.pendingReceipts ?? 0) > 0 && (
                    <Text>Tiene {check.data!.pendingReceipts} compra(s) por recibir. Al llegar, sumarán aunque esté archivado.</Text>
                  )}
                  {check.data?.openCount && <Text color="red.fg">Un conteo abierto ya lo contó. Ciérralo o descártalo antes de archivar.</Text>}
                  {recipes.length > 0 && (
                    <Stack gap={1}>
                      <Text color="red.fg">Lo usan {recipes.length} receta(s) activa(s). Quítalo de cada una antes de archivar:</Text>
                      {recipes.map((r) => (
                        <Button key={`${r.kind}-${r.id}`} minH="48px" variant="outline" justifyContent="space-between"
                          onClick={() => setRecipe({ kind: r.kind, id: r.id, name: r.name })}>
                          {r.name}<Box as="span" color="fg.muted">Abrir receta ›</Box>
                        </Button>
                      ))}
                    </Stack>
                  )}
                </>
              ) : <Text>Vuelve a ofrecerse en gastos, recetas y conteo, tal como estaba.</Text>}
              <HStack gap={3} justify="flex-end" pt={2}>
                <Button minH="52px" variant="outline" onClick={onClose}>Cancelar</Button>
                <Button minH="52px" colorPalette={article.isActive ? 'red' : 'brand'} disabled={article.isActive && blocked}
                  loading={act.isPending} onClick={() => act.mutate()}>{article.isActive ? 'Archivar' : 'Reactivar'}</Button>
              </HStack>
            </Stack>
          )}
        </DrawerBody>
      </DrawerContent>
      {recipe && (
        <CompositionSheet kind={recipe.kind} id={recipe.id} name={recipe.name} open
          onClose={() => { setRecipe(null); void qc.invalidateQueries({ queryKey: ['article-archive-check', article.id] }); }} />
      )}
    </DrawerRoot>
  );
}
