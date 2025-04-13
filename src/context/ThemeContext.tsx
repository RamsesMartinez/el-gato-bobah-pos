import React, { createContext, useCallback, useMemo } from 'react';
import { theme } from '../theme/chakraTheme';

type CustomTheme = typeof theme;

interface ThemeContextType {
  theme: CustomTheme;
  updateTheme: (newTheme: Partial<CustomTheme>) => void;
}

export const ThemeContext = createContext<ThemeContextType | null>(null);

interface ThemeProviderProps {
  children: React.ReactNode;
}

export const ThemeProvider: React.FC<ThemeProviderProps> = ({
  children,
}) => {
  const [currentTheme, setTheme] = React.useState<CustomTheme>(theme);

  const updateTheme = useCallback((newTheme: Partial<CustomTheme>) => {
    setTheme((prevTheme) => ({
      ...prevTheme,
      ...newTheme,
    }));
  }, []);

  const value = useMemo(
    () => ({
      theme: currentTheme,
      updateTheme,
    }),
    [currentTheme, updateTheme]
  );

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
}; 