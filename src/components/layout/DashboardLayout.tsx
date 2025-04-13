import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { TicketPanel } from '../ticket/TicketPanel';
import { ProductsPanel } from '../products/ProductsPanel';
import { MainNav } from './MainNav';
import { useTheme } from '../../hooks/useTheme';
import { Product } from '../../types/sales';
import { FudoCategory } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';
import { productService } from '../../services/api/products';

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
  const [selectedProduct, setSelectedProduct] = useState<Product | null>(null);
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    // Cargar categorías y productos
    const loadData = async () => {
      try {
        // Cargar categorías
        const categoriesResponse = await categoryService.getCategories();
        setCategories(categoriesResponse.data);

        // Cargar todos los productos
        const productsResponse = await productService.getProducts();
        setProducts(productsResponse.data.map((p: any) => ({
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

  const handleNewSale = () => {
    setCurrentSale({
      items: [],
      total_amount: 0,
      status: 'pending'
    });
    setHasChanges(false);
  };

  const handleCategoryChange = (category: FudoCategory) => {
    setSelectedCategory(category.id);
  };

  const handleProductSelect = (product: Product) => {
    setSelectedProduct(product);
    setHasChanges(true);
  };

  const handleSave = () => {
    // Implementar lógica de guardado
    setHasChanges(false);
  };

  const handleCancel = () => {
    navigate('/sales');
    setHasChanges(false);
  };

  // Filtrar productos por categoría seleccionada
  const filteredProducts = selectedCategory
    ? products.filter(product => product.category === selectedCategory)
    : products;

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      backgroundColor: theme.colors.background.default,
    }}>
      <MainNav />

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
          onPay={handleNewSale}
        />
      </div>
    </div>
  );
}; 