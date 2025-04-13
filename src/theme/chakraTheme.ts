import { extendTheme } from '@chakra-ui/react';
import { themeConfig } from './config';

const colors = {
  primary: themeConfig.colors.primary,
  secondary: themeConfig.colors.secondary,
  success: themeConfig.colors.success,
  error: themeConfig.colors.error,
  warning: themeConfig.colors.warning,
  text: themeConfig.colors.text,
  background: themeConfig.colors.background,
  border: themeConfig.colors.border,
  status: themeConfig.colors.status,
  gray: {
    50: '#F7FAFC',
    100: '#EDF2F7',
    200: '#E2E8F0',
    300: '#CBD5E0',
    400: '#A0AEC0',
    500: '#718096',
    600: '#4A5568',
    700: '#2D3748',
    800: '#1A202C',
    900: '#171923',
  }
};

const fonts = {
  heading: themeConfig.typography.fontFamily,
  body: themeConfig.typography.fontFamily,
};

const fontSizes = themeConfig.typography.fontSizes;
const fontWeights = themeConfig.typography.fontWeights;

const space = themeConfig.spacing;
const radii = themeConfig.borderRadius;
const shadows = themeConfig.shadows;

const components = {
  Button: {
    baseStyle: {
      fontWeight: 'semibold',
      borderRadius: 'md',
    },
    variants: {
      solid: (props: { colorScheme: string }) => ({
        bg: `${props.colorScheme}.main`,
        color: 'white',
        _hover: {
          bg: `${props.colorScheme}.dark`,
        },
      }),
      outline: (props: { colorScheme: string }) => ({
        border: '1px solid',
        borderColor: `${props.colorScheme}.main`,
        color: `${props.colorScheme}.main`,
        _hover: {
          bg: `${props.colorScheme}.main`,
          color: 'white',
        },
      }),
    },
    defaultProps: {
      colorScheme: 'primary',
    },
  },
  Text: {
    baseStyle: {
      color: 'text.primary',
    },
  },
  Heading: {
    baseStyle: {
      color: 'text.dark',
    },
  },
  Badge: {
    baseStyle: {
      borderRadius: 'sm',
      px: 2,
      py: 1,
      fontWeight: 'medium',
    },
    variants: {
      solid: (props: { colorScheme: string }) => ({
        bg: `${props.colorScheme}.main`,
        color: 'white',
      }),
      outline: (props: { colorScheme: string }) => ({
        bg: 'transparent',
        border: '1px solid',
        borderColor: `${props.colorScheme}.main`,
        color: `${props.colorScheme}.main`,
      }),
    },
    defaultProps: {
      variant: 'solid',
      colorScheme: 'primary',
    },
  },
};

export const chakraTheme = extendTheme({
  colors,
  fonts,
  fontSizes,
  fontWeights,
  space,
  radii,
  shadows,
  components,
}); 