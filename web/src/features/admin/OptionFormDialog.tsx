import { useEffect, useState } from 'react';
import { Input, Button, VStack } from '@chakra-ui/react';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Field } from '../../components/ui/field';
import { toaster } from '../../components/ui/toaster';
import { useMutation } from '@tanstack/react-query';
import { adminApi, type GroupOption } from '../../api/admin';
import { useUiStore } from '../../stores/ui';

// Crear/editar una opción de modificador (nombre, precio extra, máx por línea).
// option=null → crear en groupId; option set → editar.
export function OptionFormDialog({ groupId, option, isOpen, onClose, onSaved }: {
  groupId: number;
  option: GroupOption | null;
  isOpen: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const palette = useUiStore((s) => s.palette);
  const [name, setName] = useState('');
  const [price, setPrice] = useState('0');
  const [maxPerLine, setMaxPerLine] = useState('1');

  useEffect(() => {
    setName(option?.name ?? '');
    setPrice(String(option?.priceDelta ?? 0));
    setMaxPerLine(String(option?.maxPerLine ?? 1));
  }, [option, isOpen]);

  const save = useMutation({
    mutationFn: () => {
      const b = { name: name.trim(), priceDelta: parseFloat(price) || 0, maxPerLine: parseInt(maxPerLine, 10) || 1 };
      return option ? adminApi.updateOption(option.id, b) : adminApi.createOption(groupId, b).then(() => {});
    },
    onSuccess: () => { onSaved(); onClose(); toaster.create({ title: option ? 'Opción actualizada' : 'Opción creada', type: 'success' }); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>{option ? 'Editar opción' : 'Nueva opción'}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            <Field label="Nombre">
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label="Precio extra (0 o negativo permitido)">
              <Input type="number" value={price} onChange={(e) => setPrice(e.target.value)} />
            </Field>
            <Field label="Máx. por línea">
              <Input type="number" min={1} value={maxPerLine} onChange={(e) => setMaxPerLine(e.target.value)} />
            </Field>
          </VStack>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" mr={3} onClick={onClose}>Cancelar</Button>
          <Button loading={save.isPending} disabled={!name.trim()} onClick={() => save.mutate()}>Guardar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
