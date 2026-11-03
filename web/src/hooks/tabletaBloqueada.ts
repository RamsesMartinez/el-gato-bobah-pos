import { createContext, useContext } from 'react';

// Si la pantalla de bloqueo está encima. La pone BloqueoPorInactividad; la lee quien monta un
// teclado de PIN debajo: `inert` no le quita a un oyente de `window` las teclas, y con dos teclados
// escuchando cada dígito caería en los dos PIN.
export const TabletaBloqueadaContext = createContext(false);

export const useTabletaBloqueada = () => useContext(TabletaBloqueadaContext);
