import React from 'react';
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

export const App: React.FC = () => {
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
