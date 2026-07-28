// Genera los iconos A PARTIR de Logo.svelte, que es la marca que se ve dentro de
// la aplicación. Escritos a mano se quedaron atrás: el manifiesto sirvió la nube
// con el rayo de la primera versión mucho después de haber cambiado el logo, y
// desde el móvil no había forma de quitársela porque el fichero servido era ese.
//
// uso: node geniconos.mjs <Logo.svelte> <logo.svg> <logo-maskable.svg> [escritorio.svg]
import { readFileSync, writeFileSync } from 'node:fs';

const [, , origen, salidaAny, salidaMaskable, salidaEscritorio] = process.argv;

const fuente = readFileSync(origen, 'utf8');
const marca = [...fuente.matchAll(/<(path|circle)\b[^>]*\/>/g)].map((m) => m[0]);
if (marca.length < 2) {
  console.error('no encuentro la marca en Logo.svelte');
  process.exit(1);
}

// currentColor no resuelve en un fichero suelto: se quedaría negro. El icono
// lleva el morado fijo, el mismo de la animación de arranque.
const TINTA = '#8b7cf6';
const FONDO = '#141018';
const cuerpo = marca.join('\n    ').replaceAll('currentColor', TINTA);
const aviso = '<!-- Generado desde noumon/src/lib/Logo.svelte. No editar a mano. -->';

// "any": la marca sola, sin fondo, a sangre sobre el lienzo.
writeFileSync(salidaAny, `${aviso}
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="512" height="512">
  <g>
    ${cuerpo}
  </g>
</svg>
`);

// "maskable": fondo completo y la marca al 70%, dentro de la zona segura que
// recorta el sistema al aplicar su propia forma (círculo, cuadrado redondeado…).
writeFileSync(salidaMaskable, `${aviso}
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <rect width="512" height="512" fill="${FONDO}"/>
  <g transform="translate(76.8 76.8) scale(3.584)">
    ${cuerpo}
  </g>
</svg>
`);

console.log(`iconos web regenerados con ${marca.length} formas`);

// Icono de escritorio en vector: fondo redondeado, como lo presenta el sistema.
// El .ico del instalador NO sale de aquí —build.ps1 lo deriva del PNG—; este
// queda como fuente vectorial legible de la misma marca.
if (salidaEscritorio) {
  writeFileSync(salidaEscritorio, `${aviso}
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <rect width="512" height="512" rx="114" fill="${FONDO}"/>
  <g transform="translate(76.8 76.8) scale(3.584)">
    ${cuerpo}
  </g>
</svg>
`);
  console.log('icono de escritorio regenerado');
}
