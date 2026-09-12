import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Consola } from './Consola';
import './consola.css';

// Punto de entrada propio: este archivo NO lo carga el POS, y el `main.tsx` del POS no carga éste.
// Son dos builds distintos (vite.config.ts y vite.consola.config.ts) hacia dos proyectos de Pages.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Consola />
  </StrictMode>,
);
