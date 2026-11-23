import { useContext, useEffect, useState, useSyncExternalStore } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { TabletaBloqueadaContext } from '../../hooks/tabletaBloqueada';
import { alarmaDeLaApp, type Alarma } from './alarma';
import { useColumnaPorAceptar } from './estado';
import { FranjaDeEntrantes } from './FranjaDeEntrantes';
import { useEntrantes } from './useEntrantes';

/**
 * AlertaDeEntrantes vive en el shell, así que suena y se ve en cualquier pantalla (spec 093). Une
 * lo que el servidor dice pendiente con la alarma y la franja; no decide nada por su cuenta.
 */
export function AlertaDeEntrantes({ alarma = alarmaDeLaApp }: { alarma?: Alarma }) {
  const { pedidos } = useEntrantes();
  const estado = useSyncExternalStore(alarma.suscribir, alarma.estado);
  const tabletaBloqueada = useContext(TabletaBloqueadaContext);
  const columnaVisible = useColumnaPorAceptar((s) => s.visible);
  const { pathname } = useLocation();
  const navigate = useNavigate();
  // Minutos y no segundos: lo que cambia una decisión son dos umbrales, no el segundero.
  const [ahora, setAhora] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 15_000);
    return () => clearInterval(t);
  }, []);

  const hay = pedidos.length > 0;
  useEffect(() => {
    alarma.sonar(hay);
  }, [alarma, hay]);
  // Al salir de la sesión se desmonta el shell: la alarma no se queda sonando en el login.
  useEffect(() => () => alarma.sonar(false), [alarma]);

  // El navegador no deja sonar hasta el primer toque: cualquiera lo destraba, en cualquier pantalla
  // (en captura, para que lo vea aunque una hoja lo detenga). Al volver a primer plano se intenta
  // otra vez: una tableta que se suspendió puede volver con el audio suspendido.
  useEffect(() => {
    const destrabar = () => alarma.desbloquear();
    const alVolver = () => {
      if (document.visibilityState === 'visible') alarma.desbloquear();
    };
    window.addEventListener('pointerdown', destrabar, true);
    window.addEventListener('keydown', destrabar, true);
    document.addEventListener('visibilitychange', alVolver);
    return () => {
      window.removeEventListener('pointerdown', destrabar, true);
      window.removeEventListener('keydown', destrabar, true);
      document.removeEventListener('visibilitychange', alVolver);
    };
  }, [alarma]);

  if (pathname === '/pedidos' && columnaVisible) return null;
  return (
    <FranjaDeEntrantes pedidos={pedidos} alarma={estado} tabletaBloqueada={tabletaBloqueada} ahora={ahora}
      onVer={() => navigate('/pedidos')} onActivarSonido={() => alarma.desbloquear()} />
  );
}
