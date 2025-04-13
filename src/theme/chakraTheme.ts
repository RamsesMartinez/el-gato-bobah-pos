import { extendTheme } from '@chakra-ui/react';

const colors = {
  brand: {
    50: '#f7fafc',
    100: '#edf2f7',
    200: '#e2e8f0',
    300: '#cbd5e0',
    400: '#a0aec0',
    500: '#718096',
    600: '#4a5568',
    700: '#2d3748',
    800: '#1a202c',
    900: '#171923',
  },
  accent: {
    primary: '#FF6B6B',
    secondary: '#4ECDC4',
  }
};

const fonts = {
  heading: '"Poppins", sans-serif',
  body: '"Inter", sans-serif',
};

const components = {
  Button: {
    baseStyle: {
      fontWeight: 'bold',
      borderRadius: 'md',
    },
    variants: {
      solid: {
        bg: 'accent.primary',
        color: 'white',
        _hover: {
          bg: 'accent.secondary',
        },
      },
    },
  },
  Table: {
    defaultProps: {
      variant: 'simple',
    },
    variants: {
      simple: {
        th: {
          borderBottom: '1px',
          borderColor: 'gray.200',
          padding: '1rem',
          textTransform: 'none',
          letterSpacing: 'normal',
        },
        td: {
          borderBottom: '1px',
          borderColor: 'gray.200',
          padding: '1rem',
        },
      },
    },
  },
};

export const chakraTheme = extendTheme({
  colors,
  fonts,
  components,
}); 