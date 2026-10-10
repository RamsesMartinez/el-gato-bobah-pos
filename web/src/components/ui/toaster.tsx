"use client"

import {
  Toaster as ChakraToaster,
  Portal,
  Spinner,
  Stack,
  Toast,
  createToaster,
} from "@chakra-ui/react"

// Arriba al centro: abajo a la derecha es el pie del ticket y la píldora de cobrar, y un aviso de
// varios segundos tapaba «Cobrar» y «Enviar» justo cuando el operador iba a tocarlos.
export const toaster = createToaster({
  placement: "top",
  pauseOnPageIdle: true,
})

export const Toaster = () => {
  return (
    <Portal>
      <ChakraToaster toaster={toaster} insetInline={{ mdDown: "4" }}>
        {(toast) => (
          // Sin capturar toques: arriba queda encima de la fila de cuentas y de los canales, y un
          // aviso de varios segundos no puede quedarse con el toque que iba a cambiar de cuenta.
          // Solo sus botones reciben el dedo. Con !important porque zag pone `auto` en línea.
          <Toast.Root width={{ md: "sm" }} css={{ pointerEvents: "none !important" }}>
            {toast.type === "loading" ? (
              <Spinner size="sm" color="blue.solid" />
            ) : (
              <Toast.Indicator />
            )}
            <Stack gap="1" flex="1" maxWidth="100%">
              {toast.title && <Toast.Title>{toast.title}</Toast.Title>}
              {toast.description && (
                <Toast.Description>{toast.description}</Toast.Description>
              )}
            </Stack>
            {toast.action && (
              // 44 px: el «Reintentar» de un producto que no se guardó se toca con el dedo.
              <Toast.ActionTrigger minH="44px" px={3} pointerEvents="auto">{toast.action.label}</Toast.ActionTrigger>
            )}
            {toast.closable && <Toast.CloseTrigger pointerEvents="auto" />}
          </Toast.Root>
        )}
      </ChakraToaster>
    </Portal>
  )
}
