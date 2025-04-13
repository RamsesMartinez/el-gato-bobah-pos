import React from 'react';
import { Box, Text, Button, VStack, HStack, Divider } from '@chakra-ui/react';
import { useTheme } from '../../hooks/useTheme';
import { Product } from '../../types/sales';

interface OrderItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
  details?: string;
}

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

interface TicketPanelProps {
  currentSale: CurrentSale;
  onPay: () => void;
}

export const TicketPanel: React.FC<TicketPanelProps> = ({
  currentSale,
  onPay,
}) => {
  const { theme } = useTheme();

  return (
    <div style={{
      backgroundColor: theme.colors.background.paper,
      borderRight: `1px solid ${theme.colors.border.main}`,
      height: '100%',
      width: '400px',
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div style={{ flex: 1, overflowY: 'auto', padding: theme.spacing.md }}>
        {currentSale.items.map((item) => (
          <div
            key={item.product_id}
            style={{
              marginBottom: theme.spacing.md,
              padding: theme.spacing.md,
              backgroundColor: theme.colors.background.default,
              borderRadius: theme.borderRadius.md,
            }}
          >
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              marginBottom: theme.spacing.xs,
            }}>
              <span style={{ fontWeight: theme.typography.fontWeights.medium }}>
                {item.product_id}
              </span>
              <span>{(item.quantity * item.unit_price).toFixed(2)}</span>
            </div>
            {item.modifiers && item.modifiers.length > 0 && (
              <div style={{
                fontSize: theme.typography.fontSizes.sm,
                color: theme.colors.text.secondary,
              }}>
                {item.modifiers.map(mod => `${mod.id} x ${mod.quantity}`).join(', ')}
              </div>
            )}
            <div style={{
              display: 'flex',
              justifyContent: 'space-between',
              marginTop: theme.spacing.xs,
              fontSize: theme.typography.fontSizes.sm,
            }}>
              <span>Cantidad: {item.quantity}</span>
              <span>Precio: {item.unit_price.toFixed(2)}</span>
            </div>
          </div>
        ))}
      </div>

      <div style={{
        padding: theme.spacing.md,
        borderTop: `1px solid ${theme.colors.border.main}`,
      }}>
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: theme.spacing.md,
        }}>
          <span style={{ fontWeight: theme.typography.fontWeights.semibold }}>
            Precio total
          </span>
          <span style={{ fontWeight: theme.typography.fontWeights.bold }}>
            ${currentSale.total_amount.toFixed(2)}
          </span>
        </div>
        <button
          onClick={onPay}
          style={{
            width: '100%',
            padding: theme.spacing.md,
            backgroundColor: theme.colors.success.main,
            color: theme.colors.background.paper,
            border: 'none',
            borderRadius: theme.borderRadius.md,
            fontWeight: theme.typography.fontWeights.medium,
            cursor: 'pointer',
          }}
        >
          Pagar
        </button>
      </div>
    </div>
  );
}; 