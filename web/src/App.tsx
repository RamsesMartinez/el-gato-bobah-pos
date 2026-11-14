import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router';
import { restoreSession, watchPermissions } from './api/client';
import { AppShell } from './app/AppShell';
import { MedidorDeUso } from './app/MedidorDeUso';
import { MedidorDeToques } from './app/MedidorDeToques';
import { RequireAuth, RequirePermission } from './app/RequireAuth';
import { BloqueoPorInactividad } from './features/auth/BloqueoPorInactividad';
import { LoginPage } from './features/auth/LoginPage';
import { ForgotPasswordPage } from './features/auth/ForgotPasswordPage';
import { ResetPasswordPage } from './features/auth/ResetPasswordPage';
import { AccountPage } from './features/auth/AccountPage';
import { AttendancePage } from './features/attendance/AttendancePage';
import { WastePage } from './features/waste/WastePage';
import { SeAcaboPage } from './features/se-acabo/SeAcaboPage';
import { POSPage } from './features/pos/POSPage';
import { OrdersBoardPage } from './features/orders/OrdersBoardPage';
import { SalesPage } from './features/sales/SalesPage';
import { CashPage } from './features/caja/CashPage';
import { ExpensesPage } from './features/backoffice/ExpensesPage';
import { StockPage } from './features/backoffice/StockPage';
import { ReportsPage } from './features/backoffice/ReportsPage';
import { EmployeesPage } from './features/admin/EmployeesPage';
import { CatalogPage } from './features/admin/CatalogPage';
import { PlataformasPage } from './features/admin/PlataformasPage';
import { EmparejarConexionPage } from './features/admin/EmparejarConexionPage';
import { ProductsAdminPage } from './features/admin/ProductsAdminPage';
import { RecipesPage } from './features/admin/RecipesPage';
import { ModifierOptionsPage } from './features/admin/ModifierOptionsPage';
import { AppearancePage } from './features/admin/AppearancePage';
import { BusinessSettingsPage } from './features/admin/BusinessSettingsPage';
import { PrintSettingsPage } from './features/admin/PrintSettingsPage';
import { IntegrationsPage } from './features/admin/IntegrationsPage';

export const App = () => {
  useEffect(() => {
    // Purga el token que instalaciones previas dejaron en localStorage (ya no se persiste)
    // y re-emite la sesión con la cookie HttpOnly de refresh tras un reload en frío.
    localStorage.removeItem('egb:session:v1');
    void restoreSession();
  }, []);
  // Los permisos de la sesión se vuelven a pedir al volver al frente (roles por empresa, FR-018).
  useEffect(() => watchPermissions(), []);

  return (
  <BrowserRouter>
    <>
      {/* Mide por dónde va el operador. Va fuera de <Routes> para que no dependa de acertarle a
          una ruta concreta, y no pinta nada: si fallara, no hay pantalla que romper. */}
      <MedidorDeUso />
      <MedidorDeToques />
      <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/recuperar" element={<ForgotPasswordPage />} />
      <Route path="/reset" element={<ResetPasswordPage />} />
      <Route
        element={
          <RequireAuth>
            {/* El bloqueo va POR ENCIMA del shell, no en su lugar: si desmontara la aplicación se
                perderían las cuentas abiertas, y el operador aprendería a impedir que se bloquee.
                Con eso se cae toda la protección. */}
            <BloqueoPorInactividad>
              <AppShell />
            </BloqueoPorInactividad>
          </RequireAuth>
        }
      >
        <Route path="/pos" element={<POSPage />} />
        <Route path="/pedidos" element={<OrdersBoardPage />} />
        <Route path="/ventas" element={<RequirePermission path="/ventas"><SalesPage /></RequirePermission>} />
        <Route path="/caja/administrar" element={<RequirePermission path="/caja/administrar"><CashPage /></RequirePermission>} />
        <Route path="/caja/*" element={<RequirePermission path="/caja"><CashPage /></RequirePermission>} />
        <Route path="/gastos" element={<RequirePermission path="/gastos"><ExpensesPage /></RequirePermission>} />
        <Route path="/almacen" element={<RequirePermission path="/almacen"><StockPage /></RequirePermission>} />
        <Route path="/reportes" element={<RequirePermission path="/reportes"><ReportsPage /></RequirePermission>} />
        <Route path="/catalogo" element={<RequirePermission path="/catalogo"><CatalogPage /></RequirePermission>}>
          <Route index element={<Navigate to="/catalogo/productos" replace />} />
          <Route path="productos" element={<ProductsAdminPage />} />
          <Route path="opciones" element={<ModifierOptionsPage />} />
          <Route path="recetas" element={<RecipesPage />} />
        </Route>
        {/* Menús de plataforma (spec 020). `RequirePermission` es UX/espejo: la barrera real la pone
            `RequirePermission` en el router del backend. */}
        <Route path="/plataformas" element={<RequirePermission path="/plataformas"><PlataformasPage /></RequirePermission>} />
        <Route path="/plataformas/:id/emparejar" element={<RequirePermission path="/plataformas"><EmparejarConexionPage /></RequirePermission>} />
        {/* rutas viejas → redirigen al hub (compatibilidad con enlaces existentes) */}
        <Route path="/productos" element={<Navigate to="/catalogo/productos" replace />} />
        <Route path="/opciones" element={<Navigate to="/catalogo/opciones" replace />} />
        <Route path="/empleados" element={<RequirePermission path="/empleados"><EmployeesPage /></RequirePermission>} />
        <Route path="/negocio" element={<RequirePermission path="/negocio"><BusinessSettingsPage /></RequirePermission>} />
        <Route path="/impresion" element={<RequirePermission path="/impresion"><PrintSettingsPage /></RequirePermission>} />
        <Route path="/integraciones" element={<RequirePermission path="/integraciones"><IntegrationsPage /></RequirePermission>} />
        <Route path="/apariencia" element={<AppearancePage />} />
        <Route path="/cuenta" element={<AccountPage />} />
        <Route path="/asistencia" element={<AttendancePage />} />
        <Route path="/merma" element={<WastePage />} />
        <Route path="/se-acabo" element={<SeAcaboPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/pos" replace />} />
      </Routes>
    </>
  </BrowserRouter>
  );
};
