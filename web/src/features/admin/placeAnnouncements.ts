import type { Announcements, UniqueIdentifier } from '@dnd-kit/core';

// placeAnnouncements son los avisos para lectores de pantalla al arrastrar: dnd-kit los trae en
// inglés y con el id interno («Draggable item 95…»), y el texto de la pantalla es para quien opera.
// Dicen la posición y no «en el lugar de X»: eso se oía como si se hubiera juntado con X (quinta
// prueba sin contexto).
export function placeAnnouncements(nameOf: (id: UniqueIdentifier) => string, positionOf: (id: UniqueIdentifier) => number, total: number): Announcements {
  const at = (id: UniqueIdentifier) => `el lugar ${positionOf(id) + 1} de ${total}`;
  return {
    onDragStart: ({ active }) => `Moviendo «${nameOf(active.id)}».`,
    onDragOver: ({ active, over }) => (over ? `«${nameOf(active.id)}» iría en ${at(over.id)}.` : undefined),
    onDragEnd: ({ active, over }) => (over ? `«${nameOf(active.id)}» quedó en ${at(over.id)}.` : `«${nameOf(active.id)}» se quedó donde estaba.`),
    onDragCancel: ({ active }) => `«${nameOf(active.id)}» se quedó donde estaba.`,
  };
}
