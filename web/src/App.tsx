import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
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
import { ProductsAdminPage } from './features/admin/ProductsAdminPage';
import { AppearancePage } from './features/admin/AppearancePage';

export const App = () => (
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
        <Route path="/productos" element={<ProductsAdminPage />} />
        <Route path="/empleados" element={<EmployeesPage />} />
        <Route path="/apariencia" element={<AppearancePage />} />
      </Route>
      <Route path="*" element={<Navigate to="/pos" replace />} />
    </Routes>
  </BrowserRouter>
);
