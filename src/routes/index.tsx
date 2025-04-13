import { Routes, Route, Navigate } from 'react-router-dom';
import { ActiveSales } from '../pages/ActiveSales/ActiveSales';
import { SalesHistory } from '../pages/SalesHistory/SalesHistory';
import NewOrder from '../pages/NewOrder/NewOrder';
import { CategoryProducts } from '../pages/CategoryProducts/CategoryProducts';

export const AppRoutes = () => {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/sales" replace />} />
      <Route path="/sales" element={<ActiveSales />} />
      <Route path="/sales/history" element={<SalesHistory />} />
      <Route path="/sales/new" element={<NewOrder />} />
      <Route path="/sales/category/:categoryId" element={<CategoryProducts />} />
    </Routes>
  );
}; 