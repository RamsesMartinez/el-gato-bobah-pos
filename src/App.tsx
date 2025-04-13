import React from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider } from './context/ThemeContext';
import { DashboardLayout } from './components/layout/DashboardLayout';
import { SalesPage } from './pages/SalesPage';
import NewOrder from './pages/NewOrder/NewOrder';
import './styles/globals.css';
import './App.css';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/sales" element={<SalesPage />} />
          <Route path="/dashboard" element={<DashboardLayout />} />
          <Route path="/" element={<Navigate to="/sales" replace />} />
          <Route path="/new-order" element={<NewOrder />} />
        </Routes>
      </Router>
    </ThemeProvider>
  );
}

export default App;
