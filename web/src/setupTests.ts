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
