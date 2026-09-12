# Research: dónde cae el dedo

## 1. Qué se manda desde la tableta

**Decisión**: la **celda**, calculada en el navegador. Nunca `(x, y)`.

**Rationale**: un punto con precisión de píxel que viaja existe —en el cuerpo, en el log de un
proxy, en la memoria del servidor— aunque después se redondee. Redondear en el origen es lo único
que hace que el punto exacto no exista en ningún lado, y es lo que vuelve verdadera la promesa de
FR-003 sin depender de que nadie guarde de más.

| Alternativa | Por qué no |
|---|---|
| Mandar `(x, y)` y redondear en el servidor | El dato fino existió y pasó por tres sitios. «Se borra después» no es una garantía, es una intención |
| Mandar el elemento tocado (`data-medir="boton-cobrar"`) | Es otra feature —y la 017 ya la responde con las acciones con nombre—. Además el identificador de un elemento termina llevando texto de la pantalla, que FR-009 prohíbe |
| Mandar el porcentaje exacto (dos decimales) | Es un punto con otro nombre: 10,000 posiciones distinguibles reconstruyen el recorrido igual de bien |

## 2. Qué tan gruesa es la rejilla

**Decisión**: **12 × 7** celdas sobre el área visible (7 × 12 en vertical), fija en el código.

**Rationale**: en 1024×600 son zonas de ~85 × 86 px, aproximadamente un botón del POS. Es la
resolución en la que la pregunta *«¿esta parte se usa?»* tiene respuesta y la pregunta *«¿quién
tocó?»* no. Y acota las filas: 84 celdas por pantalla y por corte de rol, pase lo que pase.

| Alternativa | Por qué no |
|---|---|
| Rejilla fina (50×30) | 1,500 celdas por pantalla: vuelve el problema de volumen y empieza a distinguir dedos |
| Rejilla configurable | Un parámetro que nadie va a cambiar, con el costo de que dos periodos no se puedan comparar |
| Celdas según el layout real | Exigiría conocer el layout en el servidor, y el layout cambia con cada despliegue |

## 3. Toque contra arrastre

**Decisión**: cuenta el `pointerup` cuyo `pointerdown` estuvo a **menos de 10 px**; lo demás es
arrastre y se descarta.

**Rationale**: el POS tiene pantallas con 1,978 px de contenido en 600 visibles (medido), así que se
desplaza todo el día. Contar los arrastres llenaría el mapa del rastro del scroll en vez de las
intenciones, y las zonas más «calientes» serían por donde la gente arrastra, no por donde decide.

Diez píxeles es el umbral que usan los navegadores para distinguir un tap de un drag; menos convierte
un temblor de mano en arrastre.

## 4. Por dónde entra al servidor

**Decisión**: el **mismo** `POST /api/v1/usage` de la 017, con un campo más.

**Rationale**: todo lo que hace que la medición no estorbe —cola, lote, `keepalive`, uno en vuelo,
tope por usuario, descarte silencioso— ya está escrito y probado, con un caso de Playwright que
apaga la red. Un endpoint nuevo obligaría a reescribir esas garantías y a volver a demostrarlas.

| Alternativa | Por qué no |
|---|---|
| Endpoint propio | Duplica cola, limitador y pruebas para el mismo problema |
| Mandar el toque de inmediato, sin lote | Una petición por dedo en una hora pico son miles, por wifi de restaurante, compitiendo con el cobro |

## 5. Cómo se guarda

**Decisión**: una tabla de **conteos** `(empresa, día, pantalla, orientación, celda, rol) → veces`.
Sin fila por toque y sin instante.

**Rationale**: es la lección que la 017 ya pagó. Un renglón por evento con su marca de tiempo se
cruza con `register_sessions.closed_by` y etiqueta al empleado aunque su nombre no esté; y el
volumen, al tope del limitador, eran 4 GB en dos semanas. Con coordenadas sería peor: miles de
toques por turno.

Con conteos, mil toques en la misma zona suben un número y no crean filas.

## 6. Cuánto se conserva

**Decisión**: **92 días** (un trimestre), contra los 396 de la 017.

**Rationale**: un mapa de toques se mira para decidir un cambio de disposición **ahora**. La
comparación interesante es contra el mes pasado, no contra el año pasado, y guardar trece meses de
una tabla con 84 celdas por pantalla es pagar por algo que nadie va a abrir.

## 7. Qué se pinta

**Decisión**: la rejilla **sola**, con la proporción de la pantalla, celdas sombreadas y el número
dentro de las que tienen conteo.

**Rationale**: FR-009. Una captura de la pantalla de un cliente lleva nombres, pedidos e importes; no
puede salir del local. Y quien mira el mapa conoce el POS de memoria: la forma de la rejilla más la
posición ya dicen de qué zona se habla.

| Alternativa | Por qué no |
|---|---|
| Manchas sobre una captura real | Prohibido por FR-009; es la razón de ser de US2 |
| Manchas sobre una captura «de demostración» | Se desactualiza con cada cambio de la interfaz y termina mintiendo sobre dónde estaba el botón |
| Degradado suave entre celdas | Sugiere precisión que el dato no tiene: la celda es de 85 px |
