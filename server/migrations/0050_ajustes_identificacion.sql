-- +goose Up
-- Cómo se identifica quien opera una estación, y cada cuánto deja de estar identificado.
--
-- Nace de un problema medible: hoy una sesión dura 30 días y nada revoca las anteriores, así que un
-- usuario de producción llegó a tener 4 vivas. Con dos estaciones cobrando contra el MISMO cajón,
-- eso vacía el desglose por cajero del arqueo: la tableta que alguien dejó abierta el viernes
-- atribuye a esa persona todo lo que se cobre el lunes.
--
-- Los tres nacen con el comportamiento seguro, así que el día del despliegue nada cambia salvo la
-- duración de la sesión.
alter table business_settings
  -- Si desbloquear pide SOLO el PIN y el sistema deduce quién es, en vez de elegir a la persona y
  -- después teclear el PIN. Apagado por default: un dedazo que caiga en el PIN de otro atribuye la
  -- venta a quien no fue, en silencio. Encenderlo exige PINs de 6 dígitos y únicos, y el servicio
  -- lo verifica antes de dejarlo.
  add column pin_only_unlock boolean not null default false,
  -- Segundos sin actividad antes de bloquear la pantalla. Tres minutos es lo que aguanta un
  -- mostrador a la vista; un negocio con la caja en una oficina cerrada querrá más.
  add column lock_after_seconds int not null default 180
    check (lock_after_seconds >= 0),
  -- Horas que dura una sesión antes de exigir usuario y contraseña otra vez. Ocho es un turno.
  -- Va en horas y no en segundos porque es lo que se captura en la pantalla de ajustes, y 28800
  -- es un número que nadie lee bien.
  add column session_hours int not null default 8
    check (session_hours between 1 and 720);

-- +goose Down
alter table business_settings
  drop column pin_only_unlock,
  drop column lock_after_seconds,
  drop column session_hours;
