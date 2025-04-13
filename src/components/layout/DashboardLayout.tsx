import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { TicketPanel } from '../ticket/TicketPanel';
import { ProductsPanel } from '../products/ProductsPanel';
import { useTheme } from '../../hooks/useTheme';
import { Product } from '../../types/sales';
import { FudoCategory } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';

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
  const [categories, setCategories] = useState<FudoCategory[]>([]);
  const [selectedCategory, setSelectedCategory] = useState<string>('');
  const [currentSale, setCurrentSale] = useState<CurrentSale>({
    items: [],
    total_amount: 0,
    status: 'pending'
  });
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    // Cargar categorías y productos
    const loadData = async () => {
      try {
        const categoriesResponse = await categoryService.getCategories();
        const allProductsCategory: FudoCategory = {
          type: 'ProductCategory',
          id: 'all',
          attributes: {
            enableOnlineMenu: true,
            name: 'Todos los productos',
            preparationTime: null,
            position: 0,
          },
          relationships: {
            kitchen: { data: null },
            parentCategory: { data: null },
            products: { data: [] },
          },
        };
        setCategories([allProductsCategory, ...categoriesResponse.data]);
        setSelectedCategory('all');

        // Cargar productos de todas las categorías
        const productsResponse = await categoryService.getCategoryProducts('all');
        setProducts(productsResponse.data.map(p => ({
          id: p.id,
          name: p.attributes.name,
          description: p.attributes.description || '',
          price: p.attributes.price,
          image_url: p.attributes.imageUrl || '',
          category: p.relationships.productCategory.data?.id || 'default',
          active: p.attributes.active,
          sellAlone: p.attributes.sellAlone,
        })));
      } catch (error) {
        console.error('Error loading data:', error);
      }
    };

    loadData();
  }, []);

  const handleBackClick = () => {
    if (hasChanges) {
      if (window.confirm('¿Estás seguro que deseas salir? Se perderán los cambios no guardados.')) {
        navigate('/sales');
      }
    } else {
      navigate('/sales');
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
      navigate('/sales');

    } catch (error) {
      console.error('Error al procesar el pago:', error);
    }
  };

  const handleCategoryChange = (category: FudoCategory) => {
    setSelectedCategory(category.id);
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

  const filteredProducts = selectedCategory === 'all'
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
        flex: 1,
        display: 'flex',
        width: '100%',
        height: 'calc(100vh - 120px)', // Restamos el alto de los headers
      }}>
        {/* Panel de productos */}
        <ProductsPanel
          categories={categories}
          selectedCategory={selectedCategory}
          onCategoryChange={handleCategoryChange}
          products={filteredProducts}
          onProductSelect={handleProductSelect}
        />

        {/* Panel del ticket */}
        <TicketPanel
          currentSale={currentSale}
          onPay={handlePay}
        />
      </div>
    </div>
  );
}; 