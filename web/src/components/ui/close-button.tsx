import type { ButtonProps } from "@chakra-ui/react"
import { IconButton as ChakraIconButton } from "@chakra-ui/react"
import * as React from "react"
import { LuX } from "react-icons/lu"

export type CloseButtonProps = ButtonProps

export const CloseButton = React.forwardRef<
  HTMLButtonElement,
  CloseButtonProps
>(function CloseButton(props, ref) {
  return (
    // El recipe de Chakra deja el svg ~20px aunque el botón sea lg (48px), así que el X "se ve"
    // chico en tablet 7". Forzamos el ícono a 24px para que sea claramente visible y dedo-friendly.
    <ChakraIconButton
      variant="ghost"
      aria-label="Close"
      css={{ '& svg': { width: '1.5rem', height: '1.5rem' } }}
      ref={ref}
      {...props}
    >
      {props.children ?? <LuX />}
    </ChakraIconButton>
  )
})
