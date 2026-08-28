import '@testing-library/jest-dom/vitest';

// jsdom no implementa matchMedia; el ColorModeProvider (next-themes) lo usa al montar. Sin este
// stub, cualquier test que renderice bajo el Provider de Chakra revienta.
if (!window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList;
}

// jsdom tampoco implementa ResizeObserver; lo usa useContainerWidth (POS y vista previa del
// ticket). El stub no observa nada: en jsdom no hay layout, así que el ancho medido es 0 y los
// componentes deben caer en su valor por defecto.
if (!window.ResizeObserver) {
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}
