import type { Announcements, UniqueIdentifier } from '@dnd-kit/core';

// placeAnnouncements son los avisos para lectores de pantalla al arrastrar: dnd-kit los trae en
// inglés y con el id interno («Draggable item 95…»), y el texto de la pantalla es para quien opera.
export function placeAnnouncements(nameOf: (id: UniqueIdentifier) => string): Announcements {
  return {
    onDragStart: ({ active }) => `Moviendo «${nameOf(active.id)}».`,
    onDragOver: ({ active, over }) => (over ? `«${nameOf(active.id)}» sobre «${nameOf(over.id)}».` : undefined),
    onDragEnd: ({ active, over }) => (over ? `«${nameOf(active.id)}» quedó en el lugar de «${nameOf(over.id)}».` : `«${nameOf(active.id)}» se quedó donde estaba.`),
    onDragCancel: ({ active }) => `«${nameOf(active.id)}» se quedó donde estaba.`,
  };
}
