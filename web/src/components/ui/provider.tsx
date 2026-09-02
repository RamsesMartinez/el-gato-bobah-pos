"use client"

import { ChakraProvider, createSystem, defaultConfig, defineConfig } from "@chakra-ui/react"
import {
  ColorModeProvider,
  type ColorModeProviderProps,
} from "./color-mode"

// Inter: diseñada para pantalla, x-height alto → legible en 7". tabular-nums global
// mantiene precios/cantidades alineados (los dígitos no "bailan" al cambiar de valor).
const fontStack = "'Inter Variable', system-ui, -apple-system, sans-serif"
const system = createSystem(
  defaultConfig,
  defineConfig({
    globalCss: { body: { fontVariantNumeric: "tabular-nums" } },
    theme: {
      tokens: {
        fonts: {
          heading: { value: fontStack },
          body: { value: fontStack },
        },
        // Escala de marca "Gato Bobah": rojo del logo (#E23B2E = brand.500).
        colors: {
          brand: {
            50: { value: "#fff1f0" },
            100: { value: "#ffdad6" },
            200: { value: "#ffb3ac" },
            300: { value: "#ff867c" },
            400: { value: "#f55a4e" },
            500: { value: "#e23b2e" },
            600: { value: "#c62b21" },
            700: { value: "#9e2018" },
            800: { value: "#7a1811" },
            900: { value: "#5c120d" },
            950: { value: "#360a07" },
          },
        },
      },
      // Radio único de controles: botones, inputs, menús (l2) y cards (l3)
      // comparten esquina. Coherencia visual global; ver src/theme/ui.ts.
      // EL PISO TÁCTIL, EN LA RECETA Y NO EN CADA PANTALLA.
      //
      // 44 px es el mínimo con el que un dedo acierta a la primera; por debajo, el operador toca dos
      // veces y la segunda cae en otra cosa — y aquí "otra cosa" significa cobrar el pedido de otra
      // mesa. Estaba escrito a mano once veces entre dos pantallas, más tres constantes `TAP`
      // locales que ni entre ellas coincidían (56, 44, 44).
      //
      // Puesto en la receta, un `<Button>` sin props ya cumple: nadie tiene que acordarse, que es
      // justo lo que falla cuando el que escribe la pantalla número doce no leyó las once anteriores.
      //
      // Va en el tamaño POR DEFECTO (`md`, que Chakra deja en 40 px) y no en `base`: `xs` y `sm` son
      // 83 controles de pantallas densas de administración —tablas de gastos, catálogo, corte— que
      // no se operan con el cliente enfrente y donde cada renglón que crece es un renglón de datos
      // que se pierde. Subirlos a ciegas cambiaría esas pantallas sin que nadie lo haya visto.
      // Usarlos donde el dedo decide dinero es la excepción, y se nota al leerlo.
      //
      // Los tamaños por encima del piso viven en theme/ui.ts (TAP_LG, TAP_XL).
      recipes: {
        button: { variants: { size: { md: { minH: "44px" } } } },
      },
      semanticTokens: {
        radii: {
          l2: { value: "{radii.lg}" },
          l3: { value: "{radii.lg}" },
        },
        // Roles del colorPalette "brand" para que colorPalette="brand" funcione
        // igual que las paletas nativas (solid/fg/muted/subtle… en claro y oscuro).
        colors: {
          brand: {
            solid: { value: "{colors.brand.500}" },
            contrast: { value: "white" },
            fg: { value: { base: "{colors.brand.700}", _dark: "{colors.brand.300}" } },
            muted: { value: { base: "{colors.brand.100}", _dark: "{colors.brand.900}" } },
            subtle: { value: { base: "{colors.brand.50}", _dark: "{colors.brand.950}" } },
            emphasized: { value: { base: "{colors.brand.200}", _dark: "{colors.brand.800}" } },
            focusRing: { value: "{colors.brand.500}" },
          },
        },
      },
    },
  }),
)

export function Provider(props: ColorModeProviderProps) {
  return (
    <ChakraProvider value={system}>
      <ColorModeProvider {...props} />
    </ChakraProvider>
  )
}
