# Runbook: impresión de tickets

Cómo dejar una caja lista para imprimir tickets de venta, y qué revisar cuando el papel sale mal.
La configuración del **contenido** del ticket (logo, textos, impresión automática) vive en el
sistema, en **Impresión**; esto es lo que hay que hacer **fuera** del sistema.

El sistema imprime **desde el navegador**, sin extensiones ni agentes: el ticket es un documento de
80mm que se manda a la impresora predeterminada del equipo. La versión ESC/POS por agente —y con
ella la impresora de cocina— es fase 2.

## 1. Impresora

Instalar el driver del fabricante y dejarla como **predeterminada** del equipo. El sistema no elige
impresora: manda a la default.

La caja de dev usa una **POS-80** (placa Zijiang genérica, `USB\VID_0416&PID_5011`) con el driver
`POS-80 11.3.0.x`. Sus ajustes viven en *Propiedades de impresora → Preferencias*, y estos tres
importan:

| Ajuste | Valor | Por qué |
| --- | --- | --- |
| `zjPrintMode` | **`zjGraphMode`** | En `zjSoftFontMode` el driver intenta mapear el texto a fuentes internas de la impresora. Lo que manda el navegador es una página rasterizada, y en ese modo **sale papel en blanco**. Éste es el primer ajuste que hay que revisar si no imprime nada |
| `zjPaperCutting` | `Option1` | Corta al final de cada página (`CMDID_CUTTING_AFTER_PAGE` en el GPD del driver) |
| `zjCashDrawer` | `zjNoCashDrawer` | Con `zjEject1BeforePrint` el cajón salta en **cada** impresión, incluidas reimpresiones y tickets de prueba |

Se leen y se cambian sin abrir la UI:

```powershell
Get-PrinterProperty -PrinterName "POS-80" | Select-Object PropertyName, Value
Set-PrinterProperty -PrinterName "POS-80" -PropertyName "Config:zjPrintMode" -Value "zjGraphMode"
```

## 2. Navegador en modo impresión directa

Sin esto, cada venta abre el cuadro de impresión y alguien tiene que cerrarlo — que es justo lo que
el interruptor de impresión automática viene a evitar. Los pasos para el operador están **dentro del
sistema**, en Impresión → el icono de ayuda del interruptor. El resumen técnico:

```powershell
msedge.exe --kiosk-printing https://app.tudominio.com
```

**Gotcha que cuesta media hora**: si Edge ya está abierto, la instancia nueva se une a la existente
y **el flag se ignora en silencio**. Para probar sin cerrar tu sesión, usa un perfil aparte:

```powershell
& "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" `
  --kiosk-printing --user-data-dir="$env:LOCALAPPDATA\edge-pos" http://localhost:3000
```

## 3. Cuando el papel sale mal

| Síntoma | Causa que ya se vio |
| --- | --- |
| **Sale en blanco** | `zjPrintMode` en `zjSoftFontMode`. También: el rollo montado al revés (el papel térmico solo marca de un lado) |
| **Texto tenue o entrecortado** | Grises en el documento. La impresora es de **1 bit**: no imprime gris, lo aproxima con puntos salteados. El ticket va todo en `#000` y en negritas, y hay un test que falla si vuelve a colarse un color |
| **Hoja larguísima en la vista previa** | La forma de papel del driver es `80 x 3276mm`. El contenido sale bien porque `zjPrintTrailingMarginOrNot` está en `zjNotPrintTrailingMargin`; si algún día alimenta metros de papel, baja la forma a `80 x 297mm` |
| **Se imprimió desde el navegador de VS Code** | El Simple Browser de VS Code es Electron y no imprime iframes: la vista previa sale vacía y el papel en blanco. Hay que abrirlo en Edge |
| **El logo sale manchado** | Es normal en térmica con degradados o mucho detalle. Un logo de trazo grueso se imprime bien |

## 4. Diagnóstico: mandar ESC/POS crudo

Separa la impresora del navegador y del driver. Si esto imprime, el hardware y el papel están bien y
el problema es del camino del driver:

```powershell
# Bytes directo al puerto, sin pasar por el driver (WritePrinter con datatype RAW).
# 1B 40 = init · texto · 1D 56 42 00 = avanzar y cortar.
```

El script completo vive en el historial del equipo; lo importante es la secuencia: `OpenPrinter` →
`StartDocPrinter` con `pDataType = "RAW"` → `WritePrinter` → `EndDocPrinter`.

## 5. Lo que NO cubre este runbook

- **Impresora de cocina y comandas**: fase 2. Necesita que el trabajo salga del backend y no del
  navegador, porque una comanda tiene que imprimirse aunque la tablet esté dormida.
- **Apertura del cajón de dinero desde el sistema**: hoy la dispara el driver, no el POS.
- **Impresión desde tablets Android o iOS**: el sistema es una PWA y el camino de impresión es el
  del navegador; en móviles no hay modo impresión directa.
