import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { restoreSession } from './api/client';
import { AppShell } from './app/AppShell';
import { RequireAuth } from './app/RequireAuth';
import { LoginPage } from './features/auth/LoginPage';
import { POSPage } from './features/pos/POSPage';
import { OrdersBoardPage } from './features/orders/OrdersBoardPage';
import { CashPage } from './features/backoffice/CashPage';
import { ExpensesPage } from './features/backoffice/ExpensesPage';
import { StockPage } from './features/backoffice/StockPage';
import { ReportsPage } from './features/backoffice/ReportsPage';
import { EmployeesPage } from './features/admin/EmployeesPage';
import { CatalogPage } from './features/admin/CatalogPage';
import { ProductsAdminPage } from './features/admin/ProductsAdminPage';
import { ModifierOptionsPage } from './features/admin/ModifierOptionsPage';
import { AppearancePage } from './features/admin/AppearancePage';

export const App = () => {
  useEffect(() => {
    // Purga el token que instalaciones previas dejaron en localStorage (ya no se persiste)
    // y re-emite la sesión con la cookie HttpOnly de refresh tras un reload en frío.
    localStorage.removeItem('egb:session:v1');
    void restoreSession();
  }, []);

  return (
  <BrowserRouter>
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <AppShell />
          </RequireAuth>
        }
      >
        <Route path="/pos" element={<POSPage />} />
        <Route path="/pedidos" element={<OrdersBoardPage />} />
        <Route path="/caja" element={<CashPage />} />
        <Route path="/gastos" element={<ExpensesPage />} />
        <Route path="/almacen" element={<StockPage />} />
        <Route path="/reportes" element={<ReportsPage />} />
        <Route path="/catalogo" element={<CatalogPage />}>
          <Route index element={<Navigate to="/catalogo/productos" replace />} />
          <Route path="productos" element={<ProductsAdminPage />} />
          <Route path="opciones" element={<ModifierOptionsPage />} />
        </Route>
        {/* rutas viejas → redirigen al hub (compatibilidad con enlaces existentes) */}
        <Route path="/productos" element={<Navigate to="/catalogo/productos" replace />} />
        <Route path="/opciones" element={<Navigate to="/catalogo/opciones" replace />} />
        <Route path="/empleados" element={<EmployeesPage />} />
        <Route path="/apariencia" element={<AppearancePage />} />
      </Route>
      <Route path="*" element={<Navigate to="/pos" replace />} />
    </Routes>
  </BrowserRouter>
  );
};
