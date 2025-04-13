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
            <Route path="/" element={<Navigate to="/sales" replace />} />
            <Route path="/sales" element={<ActiveSales />} />
            <Route path="/sales/history" element={<SalesHistory />} />
            <Route path="/sales/new" element={<DashboardLayout />} />
            <Route path="/new-order" element={<NewOrder />} />
          </Routes>
        </Router>
      </ThemeProvider>
    </ChakraProvider>
  );
};
