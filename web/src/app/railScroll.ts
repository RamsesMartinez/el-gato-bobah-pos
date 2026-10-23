// railScrollFor da el desplazamiento de la barra que deja completa la opción activa y empieza en el
// inicio de una opción (spec 045). «Lo justo para verla» dejaba la de arriba partida, y ajustar al
// renglón más cercano partía la activa abajo: se toma el primer inicio que la deja entera.
export function railScrollFor(list: HTMLElement, active: HTMLElement): number {
  const top = (el: HTMLElement) => el.getBoundingClientRect().top - list.getBoundingClientRect().top + list.scrollTop;
  const activeTop = top(active);
  const activeBottom = activeTop + active.getBoundingClientRect().height;
  if (activeTop >= list.scrollTop && activeBottom <= list.scrollTop + list.clientHeight) return list.scrollTop;
  // Arriba de lo visible: la activa queda primera. Abajo: el primer inicio que la deja entera.
  if (activeTop < list.scrollTop) return activeTop;
  const starts = [...list.children].map((c) => top(c as HTMLElement));
  return starts.find((s) => activeBottom - s <= list.clientHeight) ?? activeTop;
}

