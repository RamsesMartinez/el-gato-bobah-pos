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

// El temporizador del focus-trap que corre DESPUÉS de que se desmontó el diálogo.
//
// Al cerrar una hoja, `@zag-js/focus-trap` agenda un `setTimeout` para devolverle el foco a quien
// la abrió. Testing Library desmonta al terminar el test y jsdom se va con el archivo, así que ese
// temporizador puede dispararse ya sin `document` y vitest lo reporta como *Uncaught Exception* —
// con los 608 tests en verde y el proceso en 1. Se lo atribuye al archivo que tocaba correr en ese
// momento, no al que abrió la hoja, así que perseguirlo cuesta una tarde.
//
// Visto UNA vez y no reproducido en 16 corridas después (8 con esto puesto y 8 sin), así que esto
// es mitigación y no un arreglo comprobado: se deja porque cuesta un tick y porque un gate que
// falla una vez de cada tantas es peor que uno que falla siempre — nadie lo cree la primera vez.
//
// Ceder un tick deja que corra mientras el documento todavía existe. No oculta nada: si el trabajo
// pendiente lanzara de verdad, lanza aquí, dentro del test que lo dejó pendiente.
afterEach(async () => {
  await new Promise((listo) => setTimeout(listo, 0));
});
