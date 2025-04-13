import { Routes, Route, Navigate } from 'react-router-dom';
import { ActiveSales } from '../pages/ActiveSales/ActiveSales';
import { SalesHistory } from '../pages/SalesHistory/SalesHistory';
import NewOrder from '../pages/NewOrder/NewOrder';
import { CategoryProducts } from '../pages/CategoryProducts/CategoryProducts';
import { ROUTES } from '../constants/routes';

export const AppRoutes = () => {
  return (
    <Routes>
      <Route path="/" element={<Navigate to={ROUTES.SALES.ACTIVE.ROOT} replace />} />
      <Route path={ROUTES.SALES.ROOT} element={<Navigate to={ROUTES.SALES.ACTIVE.COUNTER} replace />} />
      <Route path={ROUTES.SALES.ACTIVE.ROOT} element={<Navigate to={ROUTES.SALES.ACTIVE.COUNTER} replace />} />
      <Route path={ROUTES.SALES.ACTIVE.COUNTER} element={<ActiveSales />} />
      <Route path={ROUTES.SALES.ACTIVE.DELIVERY} element={<ActiveSales />} />
      <Route path={ROUTES.SALES.HISTORY} element={<SalesHistory />} />
      <Route path={ROUTES.SALES.NEW} element={<NewOrder />} />
      <Route path={ROUTES.SALES.CATEGORY} element={<CategoryProducts />} />
    </Routes>
  );
}; 