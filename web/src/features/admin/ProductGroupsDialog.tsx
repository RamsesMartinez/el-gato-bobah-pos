import { Button } from '@chakra-ui/react';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { useUiStore } from '../../stores/ui';
import { ProductGroupsManager } from './ProductGroupsManager';

// Diálogo (catálogo → Productos) que envuelve el gestor de grupos reutilizable.
export function ProductGroupsDialog({ productId, productName, isOpen, onClose }: {
  productId: number | null;
  productName: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const palette = useUiStore((s) => s.palette);
  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }} size="lg">
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>Grupos de «{productName}»</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          {productId !== null && <ProductGroupsManager productId={productId} />}
        </DialogBody>
        <DialogFooter>
          <Button onClick={onClose}>Cerrar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
