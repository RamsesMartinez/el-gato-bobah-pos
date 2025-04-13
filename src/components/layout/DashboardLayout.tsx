import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Navbar } from './Navbar';
import { TicketPanel } from '../ticket/TicketPanel';
import { ProductsPanel } from '../products/ProductsPanel';
import { useTheme } from '../../hooks/useTheme';
import { Product } from '../../types/sales';
import productsData from '../../mocks/fudo.products.json';
import categoriesData from '../../mocks/fudo.categories.json';
import { adaptFudoProduct, adaptFudoCategory } from '../../adapters/fudo';

interface CurrentSale {
  items: Array<{
    product_id: string;
    quantity: number;
    unit_price: number;
    modifiers?: Array<{
      id: string;
      quantity: number;
    }>;
  }>;
  total_amount: number;
  status: 'pending' | 'completed' | 'cancelled';
}

export const DashboardLayout: React.FC = () => {
  const navigate = useNavigate();
  const { theme } = useTheme();
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<string[]>([]);
  const [selectedCategory, setSelectedCategory] = useState('');
  const [currentSale, setCurrentSale] = useState<CurrentSale>({
    items: [],
    total_amount: 0,
    status: 'pending'
  });
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    // Cargar productos y categorías de Fudo
    const adaptedProducts = productsData.data.map(adaptFudoProduct);
    const adaptedCategories = ['Todos los productos', ...categoriesData.data.map(adaptFudoCategory)];
    setProducts(adaptedProducts);
    setCategories(adaptedCategories);
    setSelectedCategory('Todos los productos');
  }, []);

  const handleBackClick = () => {
    if (hasChanges) {
      if (window.confirm('¿Estás seguro que deseas salir? Se perderán los cambios no guardados.')) {
        navigate('/ventas');
      }
    } else {
      navigate('/ventas');
    }
  };

  const handleAddGuest = () => {
    console.log('Add guest clicked');
  };

  const handlePay = async () => {
    try {
      // Aquí iría la llamada a la API de Fudo
      const response = await fetch('https://api.fu.do/sales', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(currentSale),
      });

      if (!response.ok) {
        throw new Error('Error al procesar el pago');
      }

      // Limpiar la venta actual después de un pago exitoso
      setCurrentSale({
        items: [],
        total_amount: 0,
        status: 'pending'
      });
      setHasChanges(false);

      // Redirigir a la lista de ventas después de un pago exitoso
      navigate('/ventas');

    } catch (error) {
      console.error('Error al procesar el pago:', error);
    }
  };

  const handleCategoryChange = (category: string) => {
    setSelectedCategory(category);
  };

  const handleProductSelect = (product: Product) => {
    setHasChanges(true);
    const newItem = {
      product_id: product.id,
      quantity: 1,
      unit_price: product.price
    };

    setCurrentSale(prev => ({
      ...prev,
      items: [...prev.items, newItem],
      total_amount: prev.total_amount + product.price
    }));
  };

  const filteredProducts = selectedCategory === 'Todos los productos'
    ? products
    : products.filter(product => product.category === selectedCategory);

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      backgroundColor: theme.colors.background.default,
    }}>
      {/* Header/Navbar principal */}
      <div style={{
        backgroundColor: theme.colors.secondary.dark,
        borderBottom: `1px solid ${theme.colors.secondary.main}`,
      }}>
        <div style={{
          maxWidth: '1400px',
          margin: '0 auto',
          padding: `${theme.spacing.md} ${theme.spacing.lg}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}>
          <div style={{
            display: 'flex',
            gap: theme.spacing.lg,
            alignItems: 'center',
          }}>
            <button
              onClick={handleBackClick}
              style={{
                background: 'none',
                border: 'none',
                color: theme.colors.text.disabled,
                cursor: 'pointer',
                padding: theme.spacing.sm,
                display: 'flex',
                alignItems: 'center',
                gap: theme.spacing.sm,
              }}
            >
              ← Volver
            </button>
            <div style={{
              height: '24px',
              width: '1px',
              backgroundColor: theme.colors.secondary.main,
            }} />
            <span style={{ color: theme.colors.text.disabled }}>
              Mesa 1
            </span>
          </div>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: theme.spacing.md,
          }}>
            <span style={{ color: theme.colors.text.disabled }}>
              Ramses
            </span>
            <span style={{ color: theme.colors.text.disabled }}>
              🔒
            </span>
          </div>
        </div>
      </div>

      {/* Subheader con botones de acción */}
      <div style={{
        backgroundColor: theme.colors.secondary.dark,
        borderBottom: `1px solid ${theme.colors.secondary.main}`,
        padding: `${theme.spacing.md} ${theme.spacing.lg}`,
      }}>
        <div style={{
          maxWidth: '1400px',
          margin: '0 auto',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}>
          <button
            onClick={handleAddGuest}
            style={{
              backgroundColor: theme.colors.success.main,
              color: 'white',
              border: 'none',
              padding: `${theme.spacing.sm} ${theme.spacing.lg}`,
              borderRadius: theme.borderRadius.md,
              cursor: 'pointer',
              fontWeight: theme.typography.fontWeights.medium,
              transition: 'background-color 0.2s',
            }}
          >
            Nuevo pedido
          </button>
          <div style={{
            display: 'flex',
            gap: theme.spacing.md,
          }}>
            <button
              style={{
                backgroundColor: 'transparent',
                color: theme.colors.text.disabled,
                border: `1px solid ${theme.colors.secondary.main}`,
                padding: `${theme.spacing.sm} ${theme.spacing.lg}`,
                borderRadius: theme.borderRadius.md,
                cursor: 'pointer',
              }}
            >
              Tipo de pedido
            </button>
          </div>
        </div>
      </div>

      {/* Contenido principal */}
      <div style={{
        display: 'flex',
        flex: 1,
        overflow: 'hidden',
      }}>
        <TicketPanel
          items={currentSale.items.map(item => {
            const product = products.find(p => p.id === item.product_id);
            return {
              id: item.product_id,
              name: product?.name || '',
              quantity: item.quantity,
              price: item.unit_price,
              total: item.quantity * item.unit_price,
              details: item.modifiers?.map(mod => {
                const modifier = product?.modifiers?.find(m => m.id === mod.id);
                return modifier ? `${modifier.name} x ${mod.quantity}` : '';
              }).filter(Boolean).join(', ')
            };
          })}
          onAddGuest={handleAddGuest}
          onPay={handlePay}
        />
        
        <ProductsPanel
          products={filteredProducts}
          selectedCategory={selectedCategory}
          onCategoryChange={handleCategoryChange}
          categories={categories}
          onProductSelect={handleProductSelect}
        />
      </div>
    </div>
  );
}; 