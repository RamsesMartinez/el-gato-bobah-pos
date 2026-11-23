// Escape obligatorio de todo texto que escribe el negocio: un nombre de producto con «<script>» sale
// como texto. La plantilla no arma HTML con nada que no pase por aquí.
const MAP: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };

export function esc(s: string | null | undefined): string {
  return (s ?? '').replace(/[&<>"']/g, (c) => MAP[c]);
}
