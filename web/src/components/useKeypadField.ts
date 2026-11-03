import { useRef, useState } from 'react';
import { pressKey } from './pressKey';

// useKeypadField lleva la casilla activa del teclado en pantalla (NumericKeypad). Dos cosas que el
// teclado solo no hace (spec 053, revisión de tableta y prueba con usuario nuevo):
// - La primera tecla tras tocar una casilla REEMPLAZA lo que traía: la cantidad llega precargada
//   («12») y teclear 8 daba «128»; había que borrar primero.
// - La casilla tocada se desplaza a la vista: el teclado ocupa el pie y tapaba el renglón de abajo.
export function useKeypadField<K extends string>() {
  const [field, setFieldState] = useState<K | null>(null);
  const fresh = useRef(false);
  const boxes = useRef<Record<string, HTMLElement | null>>({});

  const setField = (k: K | null) => {
    fresh.current = k !== null;
    setFieldState(k);
    if (k !== null) {
      // Después de que el teclado ocupe el pie y el cuerpo se encoja.
      requestAnimationFrame(() => boxes.current[k]?.scrollIntoView?.({ block: 'nearest' }));
    }
  };

  // apply aplica una tecla al valor de la casilla activa.
  const apply = (value: string, key: string, allowDecimal: boolean) => {
    const first = fresh.current;
    fresh.current = false;
    if (first) return key === 'back' ? '' : pressKey('', key, allowDecimal);
    return pressKey(value, key, allowDecimal);
  };

  // keep: la casilla cambió por otro control (− y +): la siguiente tecla ya no reemplaza ese ajuste.
  const keep = () => { fresh.current = false; };

  const ref = (k: K) => (el: HTMLElement | null) => { boxes.current[k] = el; };
  return { field, setField, apply, keep, ref };
}
