import React, { useState, useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ChakraProvider } from '@chakra-ui/react';
import { DashboardLayout } from './components/layout/DashboardLayout';
import { ActiveSales } from './pages/ActiveSales/ActiveSales';
import { SalesHistory } from './pages/SalesHistory/SalesHistory';
import { ThemeProvider } from './context/ThemeContext';
import { theme } from './theme/chakraTheme';
import './styles/globals.css';
import './App.css';
import NewOrder from './pages/NewOrder/NewOrder';
import { env } from './config/env';
import { ROUTES } from './constants/routes';

export const App: React.FC = () => {
  const [envError, setEnvError] = useState<string | null>(null);

  useEffect(() => {
    try {
      // Intentar acceder a env para validar las variables de entorno
      console.log('Validando configuración...', env.FUDO_API_URL);
    } catch (error) {
      if (error instanceof Error) {
        setEnvError(error.message);
      }
    }
  }, []);

  if (envError) {
    return (
      <ChakraProvider theme={theme}>
        <div style={{ 
          padding: '2rem', 
          maxWidth: '800px', 
          margin: '0 auto',
          fontFamily: 'monospace',
          whiteSpace: 'pre-wrap'
        }}>
          {envError}
        </div>
      </ChakraProvider>
    );
  }

  return (
    <ChakraProvider theme={theme}>
      <ThemeProvider>
        <Router>
          <Routes>
            <Route path="/" element={<Navigate to={ROUTES.SALES.ROOT} replace />} />
            <Route path={ROUTES.SALES.ROOT} element={<ActiveSales />} />
            <Route path={ROUTES.SALES.NEW} element={<NewOrder />} />
            <Route path={ROUTES.SALES.HISTORY} element={<SalesHistory />} />
            <Route path={ROUTES.SALES.CATEGORY} element={<NewOrder />} />
            <Route path={ROUTES.SALES.PRODUCTS} element={<NewOrder />} />
            {/* Redirigir rutas antiguas */}
            <Route path="/new-order" element={<Navigate to={ROUTES.SALES.NEW} replace />} />
            {/* Ruta 404 */}
            <Route path="*" element={<Navigate to={ROUTES.SALES.ROOT} replace />} />
          </Routes>
        </Router>
      </ThemeProvider>
    </ChakraProvider>
  );
};
