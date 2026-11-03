import { useEffect, useRef, useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { AltNameChip } from './AltNameChip';
import { InfoButton } from './InfoButton';
import { altNameKey as key, cleanAltName as clean, MAX_ALT_NAME, MAX_ALT_NAMES, type SimilarAlt } from './altNames';

interface Props {
  ownName: string;
  value: string[];
  onChange: (next: string[]) => void;
  // El texto escrito y todavía no agregado vive en quien usa el campo: «Guardar» o «Volver» lo agrega
  // antes de cerrar (revisión de tableta, A1), así que tiene que poder leerlo.
  draft: string;
  onDraftChange: (draft: string) => void;
  similar: (name: string) => Promise<SimilarAlt[]>;
  // El choque que respondió el servidor (409 ALT_NAME_TAKEN), con su texto para quien opera.
  error?: string;
  disabled?: boolean;
}

// AltNamesField captura los otros nombres de un producto o artículo (spec 063, maqueta «Editor «Otros
// nombres»», opción A): chips que se quitan con una «×» y un campo con «Agregar». Antes de agregar
// avisa si ya hay otros con nombre parecido, sin impedirlo (pedido del dueño al elegir la maqueta).
export function AltNamesField({ ownName, value, onChange, draft, onDraftChange, similar, error, disabled }: Props) {
  const [help, setHelp] = useState(false);
  const [localError, setLocalError] = useState('');
  // Los parecidos de un texto exacto: así «Agregar» sabe si el aviso que se ve es del texto de hoy.
  const [found, setFound] = useState<{ text: string; items: SimilarAlt[] } | null>(null);
  const [warnedFor, setWarnedFor] = useState('');
  const [checking, setChecking] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  // Lo escrito AHORA: «Agregar» espera los parecidos y en ese rato se puede seguir escribiendo.
  const draftRef = useRef(draft);
  useEffect(() => { draftRef.current = draft; }, [draft]);
  const text = clean(draft);
  const full = value.length >= MAX_ALT_NAMES;

  // Pausa de 300 ms mientras se escribe; «Agregar» no la espera (pide los parecidos si faltan).
  useEffect(() => {
    if (!text) return;
    let alive = true;
    const t = setTimeout(() => {
      similar(text).then((items) => { if (alive) setFound({ text, items }); }).catch(() => {});
    }, 300);
    return () => { alive = false; clearTimeout(t); };
  }, [text, similar]);

  const reasonNotToAdd = (name: string) => {
    if (key(name) === key(ownName)) return `«${name}» ya es el nombre de este.`;
    if (value.some((v) => key(v) === key(name))) return `«${name}» ya está en la lista.`;
    if (name.length > MAX_ALT_NAME) return `Hasta ${MAX_ALT_NAME} letras.`;
    return '';
  };

  const add = async () => {
    if (!text || full) return;
    const reason = reasonNotToAdd(text);
    if (reason) { setLocalError(reason); return; }
    setLocalError('');
    let items = found?.text === text ? found.items : undefined;
    if (!items) {
      setChecking(true);
      try {
        items = await similar(text);
      } catch {
        items = [];
      } finally {
        setChecking(false);
      }
      setFound({ text, items });
      // Si siguió escribiendo, lo que se consultó ya no es lo que quiere agregar.
      if (clean(draftRef.current) !== text) return;
    }
    if (items.length > 0 && warnedFor !== text) {
      setWarnedFor(text);
      return;
    }
    onChange([...value, text]);
    onDraftChange('');
    setWarnedFor('');
  };

  const shown = found?.text === text ? found.items : [];
  const confirming = warnedFor !== '' && warnedFor === text && shown.length > 0;
  const message = localError || error;

  return (
    <VStack align="stretch" gap={2}>
      <HStack justify="space-between" gap={1}>
        <Text fontWeight="600" fontSize="sm">Otros nombres <Text as="span" color="fg.muted" fontWeight="normal">({value.length} de {MAX_ALT_NAMES})</Text></Text>
        <InfoButton label="¿Para qué sirven los otros nombres?" open={help} onToggle={() => setHelp((h) => !h)} />
      </HStack>
      {help && (
        <Text fontSize="sm" color="fg.muted">Sirven para encontrarlo al buscar. No salen en el ticket ni en la comanda.</Text>
      )}
      {value.length > 0 && (
        // Una sola línea con scroll horizontal: con 10 nombres, varias filas de chips se comían la ficha
        // en 600 px (revisión de tableta, A3).
        <HStack gap={2} overflowX="auto" flexWrap="nowrap" pb={1}>
          {value.map((v) => (
            <AltNameChip key={v} name={v} disabled={disabled} onRemove={() => onChange(value.filter((x) => x !== v))} />
          ))}
        </HStack>
      )}
      {full ? (
        <Text fontSize="sm" color="fg.muted">Ya tiene {MAX_ALT_NAMES}. Quita uno para agregar otro.</Text>
      ) : (
        <HStack gap={2}>
          <Input ref={inputRef} aria-label="Otro nombre" placeholder="Otro nombre con el que lo piden" minH="44px"
            value={draft} maxLength={MAX_ALT_NAME} disabled={disabled}
            onFocus={() => inputRef.current?.scrollIntoView?.({ block: 'center' })}
            onChange={(e) => { onDraftChange(e.target.value); setLocalError(''); }}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void add(); } }} />
          <Button minH="44px" variant="outline" flexShrink={0} loading={checking} disabled={disabled || !text} onClick={() => void add()}>
            {confirming ? 'Agregar de todos modos' : 'Agregar'}
          </Button>
        </HStack>
      )}
      {/* Con un error a la vista no se pinta también el aviso: en 600 px empujaba «Guardar» (revisión). */}
      {confirming && !(localError || error) && (
        <Box fontSize="sm" color="fg.warning">
          Se parece a: {shown.map((s, i) => (
            <span key={s.id}>{i > 0 && ', '}«{s.name}»{s.viaAlt && <> (otro nombre: «{s.viaAlt}»)</>}</span>
          ))}
        </Box>
      )}
      {message && (
        <Box role="alert" fontSize="sm" color="fg.error">{message}</Box>
      )}
    </VStack>
  );
}
