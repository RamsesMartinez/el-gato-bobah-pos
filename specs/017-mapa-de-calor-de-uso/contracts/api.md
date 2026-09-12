# Contratos: el mapa de calor de uso

Dos endpoints en dos superficies que no se tocan: el POS **escribe**, la consola **lee**.

## `POST /api/v1/usage` — el POS entrega un lote

Grupo del negocio, con `RequireAuth` y `WithTenant`. La empresa y el rol salen del **token**, nunca
del cuerpo.

```
{ "eventos": [ { "pantalla": "caja", "accion": "cerrar-turno" },
                { "pantalla": "pos" } ] }
→ 204 No Content   (siempre)
```

- **Siempre 204**, pase lo que pase: eventos desconocidos, lote vacío, cuerpo mal formado. El
  cliente no espera la respuesta ni la puede usar, y un 400 solo serviría para que alguien lo vea en
  la pestaña de red y crea que algo se rompió. Lo que no entra a la lista blanca se descarta y queda
  un contador en el log del servidor — ahí se ve si una versión del front manda nombres que no
  existen.
- **Sin `accion` = se abrió la pantalla.** Con `accion` = ocurrió esa acción dentro de esa pantalla.
- **Máximo 50 eventos por lote.** Lo que sobra se descarta: el cliente manda de a 20 y un lote de
  cientos solo puede venir de un bucle.
- **Sin fecha en el cuerpo.** La pone el servidor (spec 008: el reloj de la tableta no manda).
- **Sin identidad en el cuerpo.** Ni usuario, ni estación, ni nada que permita deducir a la persona
  (FR-002). El servidor escribe el rol del token, y lo suprime si identifica.
- Limitado por usuario con el limitador que ya existe. Pasado el tope: 204 igual, y no se escribe.

**Lo que este endpoint NO hace**: devolver datos, confirmar qué se guardó, o fallar de forma que el
POS se entere. Es de una sola dirección a propósito.

## `GET /api/v1/platform/usage?desde=…&hasta=…&empresa=…` — la consola lee

Grupo de plataforma, detrás de `RequireOperador`. Con una sesión del negocio: **401**.

```
→ 200 {
    "periodo": { "desde": "2026-09-01", "hasta": "2026-09-12" },
    "pantallas": [
      { "pantalla": "pos", "aperturas": 412,
        "acciones": [ { "accion": "cobrar", "veces": 180 } ],
        "porRol": [ { "rol": "cajero", "veces": 300 }, { "rol": null, "veces": 112 } ] }
    ],
    "empresas": [ { "id": 2, "slug": "gatobobah", "nombre": "El Gato Bobah" } ]
  }
```

- `empresa` ausente = **todas juntas** (FR-008). Con `empresa=<id>`, solo esa.
- Las pantallas vienen **ordenadas de más a menos usada**, que es la pregunta que la pantalla
  responde. Una pantalla de la lista blanca que nadie abrió viene con `aperturas: 0` y no se omite:
  el cero es justo el dato que se busca.
- `"rol": null` significa **sin corte**: ese uso existe pero no se puede atribuir a un rol sin
  identificar a una persona (FR-009). La pantalla lo dice con esas palabras, no con un hueco.
- **Sin una sola cifra de dinero, sin nombres y sin nada de la gente del cliente** (FR-003).
- Rango máximo: 13 meses, que es lo que se conserva. Un `desde` más viejo se rechaza como
  `ErrValidation`; **no se recorta en silencio a lo que hay** — un rango que devuelve otra cosa de
  la que pide es una pantalla que miente (principio V).

## Lo que no existe, a propósito

- Ninguna ruta que deje al **negocio** leer su propio uso. No es una pantalla del dueño del local:
  es investigación del producto, y mezclarlas convierte la medición en un reporte que alguien va a
  querer que se vea bonito.
- Ninguna ruta que devuelva **eventos**, solo conteos. El grano fino no sale de la base.
- Ningún borrado ni edición desde ninguna de las dos superficies. Lo viejo lo recorta el binario.

## Configuración nueva

**Ninguna.** Ni una variable de entorno, ni un interruptor. La medición está siempre encendida y no
se puede apagar desde una pantalla: un interruptor que alguien apaga «mientras probamos» y nadie
vuelve a encender es la forma más común de quedarse sin datos.

Si algún día hay que apagarla, se apaga como todo lo demás: con un despliegue.
