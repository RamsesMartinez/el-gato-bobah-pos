// Guard: la suite corre con jsdom, que ya usa módulos ESM cargados vía require() — soportado
// solo en Node >= 20.19 / 22.12. En Node más viejo vitest muere con un ERR_REQUIRE_ESM críptico
// (html-encoding-sniffer → @exodus/bytes). Aquí lo convertimos en un mensaje accionable.
// El repo fija Node 24 (.nvmrc); este es el piso técnico mínimo.
const [maj, min] = process.versions.node.split('.').map(Number);
if (maj < 20 || (maj === 20 && min < 19)) {
  console.error(
    `\n✖ Los tests necesitan Node ≥ 20.19 (jsdom hace require() de ESM). Tienes ${process.versions.node}.\n` +
    `  El repo usa Node 24 (.nvmrc). Corre:  nvm use\n`,
  );
  process.exit(1);
}
