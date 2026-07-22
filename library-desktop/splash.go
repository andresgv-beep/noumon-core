package main

import (
	"html/template"
	"net/http"
)

// serveSplash pinta el ARRANQUE de Noumon: el logo rueda desde el lateral hasta
// el centro, gira mientras se espera al servidor, y al conectar destella, saca
// el wordmark y una luz que rodea el buscador antes de entrar en la interfaz.
//
// Decisiones que conviene no perder (acordadas con Andrés):
//
//   - Es un MOMENTO DE MARCA FIJO: oscuro + neón lila para todo el mundo, como
//     el logo de una consola al encender. NO se viste con el tema del usuario;
//     al terminar se difumina y debajo entra su tema (ver el velo de index.html).
//   - La RODADURA ES LA ESPERA: no hay barra de progreso que finja saber cuánto
//     falta. Si el servidor tarda, el logo sigue girando en el centro.
//   - La entrada se ve SIEMPRE, aunque el servidor conteste al instante: el
//     recorrido del lateral al centro es la firma del arranque, no un relleno.
//   - Ya no hay <meta refresh>: recargar cada segundo reiniciaba la animación.
//     Se sondea /api/health con fetch y solo se navega al estar listo.
func serveSplash(w http.ResponseWriter, remote bool, target string) {
	message := "Conectando con el servicio local de Noumon Server..."
	if remote {
		message = "Conectando con " + target + "..."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pageStart + `<title>Noumon</title>` + bootStyle + `</head><body class="bootbody">` + chromeBar + `
<div class="stage" id="stage">
  <div class="mark" id="mark"><span class="spin" id="spin"><img src="data:image/svg+xml,` + escapedLogo + `" alt=""></span></div>
  <div class="flash"></div>
  <div class="wordmark">Noumon</div>
  <div class="shell" id="shell"><span class="ico">&#9906;</span><em>Busca en todas tus colecciones&hellip;</em><svg class="tracer" id="tracer"><g><path class="halo" pathLength="100"/><path class="halo" pathLength="100"/><path class="line" pathLength="100"/><path class="line" pathLength="100"/></g></svg></div>
  <p class="status" id="status">` + template.HTMLEscapeString(message) + `</p>
</div>
` + bootScript + chromeScript + `</body></html>`))
}

func serveSetup(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pageStart + `<title>Conectar Noumon</title>` + pageStyle + `</head><body>` + chromeBar + `<main class="setup"><img src="data:image/svg+xml,` + escapedLogo + `" alt=""><h1>Conectar a Noumon Server</h1><p>Escribe la direccion del equipo o NAS que guarda tu biblioteca.</p><form id="setup"><input id="target" type="url" required autofocus placeholder="https://library.ejemplo.local"><button>Conectar</button><small id="error">` + template.HTMLEscapeString(message) + `</small></form></main><script>
document.getElementById('setup').addEventListener('submit',async function(event){
 event.preventDefault();var button=this.querySelector('button'),error=document.getElementById('error');button.disabled=true;error.textContent='Comprobando...';
 try{var response=await fetch('/__noumon/gateway',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({target:document.getElementById('target').value})});var body=await response.json();if(!response.ok)throw new Error(body.error||'No se pudo guardar');location.reload();}
 catch(e){error.textContent=e.message;button.disabled=false;}
});
</script>` + chromeScript + `</body></html>`))
}

// serveDisconnected sustituye la página de error interna del WebView cuando el
// proxy no puede alcanzar el servidor: mensaje claro, reintento automático y,
// en modo remoto, la opción de conectar con otro servidor.
func serveDisconnected(w http.ResponseWriter, remote bool, target string) {
	message := "El servicio local de Noumon Server no responde."
	if remote && target != "" {
		message = "No se pudo contactar con " + target + "."
	}
	other := ""
	if remote {
		other = `<button type="button" class="ghost" id="showother">Conectar a otro servidor</button><form id="setup" hidden><input id="target" type="url" required placeholder="https://library.ejemplo.local"><button>Conectar</button><small id="error"></small></form>`
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pageStart + `<title>Noumon</title>` + pageStyle + `</head><body>` + chromeBar + `<main class="setup"><img src="data:image/svg+xml,` + escapedLogo + `" alt=""><h1>Se ha perdido la conexi&oacute;n con el servidor</h1><p>` + template.HTMLEscapeString(message) + `</p><p class="retry" id="retry">Reintentando autom&aacute;ticamente&hellip;</p>` + other + `</main><script>
var retrying=true;
async function ping(){if(!retrying)return;try{var r=await fetch('/api/health',{cache:'no-store'});if(r.ok)location.replace('/');}catch(e){}}
setInterval(ping,2000);
var show=document.getElementById('showother');
if(show)show.addEventListener('click',function(){retrying=false;document.getElementById('retry').hidden=true;this.hidden=true;var f=document.getElementById('setup');f.hidden=false;document.getElementById('target').focus();});
var form=document.getElementById('setup');
if(form)form.addEventListener('submit',async function(event){
 event.preventDefault();var button=this.querySelector('button'),error=document.getElementById('error');button.disabled=true;error.textContent='Comprobando...';
 try{var response=await fetch('/__noumon/gateway',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({target:document.getElementById('target').value})});var body=await response.json();if(!response.ok)throw new Error(body.error||'No se pudo guardar');location.replace('/');}
 catch(e){error.textContent=e.message;button.disabled=false;}
});
</script>` + chromeScript + `</body></html>`))
}

const pageStart = `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`

const pageStyle = `<style>
html,body{height:100%;margin:0}body{display:grid;place-items:center;background:#0e0e14;color:#e9e9f0;font:15px/1.45 system-ui,Segoe UI,sans-serif}main{width:min(440px,calc(100% - 48px));display:flex;flex-direction:column;align-items:center;text-align:center;gap:14px}img{width:82px;height:82px}h1{font-size:22px;margin:0}p{color:#9393a0;margin:0}form{width:100%;display:flex;flex-direction:column;gap:11px;margin-top:12px}input,button{box-sizing:border-box;width:100%;height:46px;border-radius:11px;font:inherit}input{border:1px solid #353543;background:#181820;color:#fff;padding:0 14px;outline:none}input:focus{border-color:#8b5cf6}button{border:0;background:linear-gradient(135deg,#6f5ee8,#9b4fe1);color:#fff;font-weight:650;cursor:pointer}button:disabled{opacity:.55;cursor:wait}button.ghost{width:auto;height:38px;padding:0 18px;background:transparent;border:1px solid #353543;color:#b9b9c6;font-weight:500;margin-top:8px}button.ghost:hover{border-color:#8b5cf6;color:#fff}small{min-height:20px;color:#f08094}.retry{animation:pulse 1.6s ease-in-out infinite}@keyframes pulse{50%{opacity:.4}}
` + chromeCSS + `
</style>`

const chromeCSS = `#chrome{position:fixed;top:0;left:0;right:0;height:38px;display:flex;align-items:stretch;--wails-draggable:drag}#chrome .space{flex:1}#chrome .wc{--wails-draggable:no-drag;width:46px;height:100%;border:0;border-radius:0;background:transparent;color:#9393a0;font:13px/1 system-ui,sans-serif;cursor:pointer}#chrome .wc:hover{background:#23232e;color:#fff}#chrome .wc.close:hover{background:#d3305a;color:#fff}`

// bootStyle: la escenografía del arranque. Paleta FIJA (oscuro + neón lila),
// deliberadamente ajena al tema del usuario: el arranque es de Noumon, no del
// usuario; su tema entra después, al levantarse el velo de index.html.
const bootStyle = `<style>
html,body{height:100%;margin:0;overflow:hidden;background:#0b0b11}
.bootbody{font:15px/1.45 system-ui,Segoe UI,sans-serif;color:#e9e9f0}
.stage{position:fixed;inset:0;opacity:1;transition:opacity .3s ease}
.stage.gone{opacity:0}
.mark{position:absolute;left:50%;top:46%;width:clamp(62px,8vw,104px);height:clamp(62px,8vw,104px);transform:translate(-50%,-50%);animation:roll-in 1s cubic-bezier(.22,.85,.24,1) both;filter:drop-shadow(0 0 14px rgba(139,124,246,.35))}
.stage.reveal .mark{filter:drop-shadow(0 0 26px #8b7cf6)}
.spin,.spin img{display:block;width:100%;height:100%}
@keyframes roll-in{from{transform:translate(calc(-50% - 46vw),-50%)}to{transform:translate(-50%,-50%)}}
.flash{position:absolute;left:50%;top:46%;width:10px;height:10px;border-radius:50%;opacity:0;transform:translate(-50%,-50%);background:#8b7cf6;box-shadow:0 0 40px 18px #8b7cf6,0 0 110px 65px rgba(139,124,246,.28)}
.stage.reveal .flash{animation:burst .55s ease-out both}
.wordmark{position:absolute;left:50%;top:58%;transform:translate(-50%,10px);font-size:clamp(40px,7vw,78px);font-weight:550;letter-spacing:-.05em;color:#f3f1f8;opacity:0}
.stage.word .wordmark{animation:rise .42s cubic-bezier(.2,.8,.2,1) both}
.shell{position:absolute;left:50%;top:76%;width:min(52%,620px);height:54px;box-sizing:border-box;transform:translate(-50%,10px);border-radius:11px;background:rgba(32,33,39,.74);display:flex;align-items:center;gap:12px;padding:0 18px;color:#92939e;opacity:0}
.shell em{font-style:normal;font-size:13px}
.stage.search .shell{animation:rise .34s ease-out both}
.tracer{position:absolute;inset:0;width:100%;height:100%;overflow:visible}
.line,.halo{vector-effect:non-scaling-stroke;fill:none;stroke-linecap:round;stroke-linejoin:round;opacity:0;stroke-dasharray:100 100;stroke-dashoffset:100}
.halo{stroke:#8b7cf6;stroke-width:7;filter:blur(3px) drop-shadow(0 0 10px #8b7cf6) drop-shadow(0 0 20px #8b7cf6)}
.line{stroke:#b3a8fa;stroke-width:2.6;filter:drop-shadow(0 0 4px #8b7cf6) drop-shadow(0 0 10px #8b7cf6)}
.stage.search .line{animation:draw 1.05s cubic-bezier(.3,0,.2,1) forwards}
.stage.search .halo{animation:halo 1.05s cubic-bezier(.3,0,.2,1) forwards}
.status{position:absolute;left:0;right:0;bottom:32px;margin:0;text-align:center;font-size:12.5px;color:#6f7080;opacity:0;transition:opacity .5s ease}
.status.on{opacity:1}
@keyframes burst{0%{opacity:0;transform:translate(-50%,-50%) scale(.2)}22%{opacity:1}100%{opacity:0;transform:translate(-50%,-50%) scale(7)}}
@keyframes rise{to{opacity:1;transform:translate(-50%,0)}}
@keyframes draw{0%{opacity:0;stroke-dashoffset:100}6%{opacity:.95}50%{stroke-dashoffset:0;opacity:.95}72%{opacity:.95}100%{stroke-dashoffset:0;opacity:0}}
@keyframes halo{0%{opacity:0;stroke-dashoffset:100}6%{opacity:.5}50%{stroke-dashoffset:0;opacity:.5}72%{opacity:.5}100%{stroke-dashoffset:0;opacity:0}}
@media(prefers-reduced-motion:reduce){.mark{animation:none}.flash,.line,.halo{display:none}}
` + chromeCSS + `
</style>`

// bootScript gobierna la secuencia. Dos relojes independientes que tienen que
// coincidir para entrar: el de la ANIMACIÓN (la rodadura hasta el centro, que se
// ve siempre) y el del SERVIDOR (/api/health). El que llegue último manda.
const bootScript = `<script>
(function(){
 var stage=document.getElementById('stage'),mark=document.getElementById('mark'),spin=document.getElementById('spin');
 var shell=document.getElementById('shell'),tracer=document.getElementById('tracer'),status=document.getElementById('status');
 var reduce=matchMedia('(prefers-reduced-motion: reduce)').matches;
 var rolled=reduce,ready=false,done=false,started=Date.now();

 // La luz se traza en pixeles REALES de la barra, con su mismo radio: un
 // viewBox 100x100 estirado deformaba las esquinas y el neon quedaba flotando
 // por fuera del reborde.
 function layout(){
  var w=shell.clientWidth,h=shell.clientHeight;if(!w||!h)return;
  tracer.setAttribute('viewBox','0 0 '+w+' '+h);
  var i=1.3,r=Math.min((parseFloat(getComputedStyle(shell).borderRadius)||11)-i/2,h/2-i);
  var right='M '+(w/2)+' '+i+' H '+(w-i-r)+' Q '+(w-i)+' '+i+' '+(w-i)+' '+(i+r)+' V '+(h-i-r)+' Q '+(w-i)+' '+(h-i)+' '+(w-i-r)+' '+(h-i)+' H '+(w/2);
  var left='M '+(w/2)+' '+i+' H '+(i+r)+' Q '+i+' '+i+' '+i+' '+(i+r)+' V '+(h-i-r)+' Q '+i+' '+(h-i)+' '+(i+r)+' '+(h-i)+' H '+(w/2);
  var p=tracer.querySelectorAll('path');
  p[0].setAttribute('d',right);p[1].setAttribute('d',left);p[2].setAttribute('d',right);p[3].setAttribute('d',left);
 }
 layout();addEventListener('resize',layout);

 // El giro ES el indicador de espera: sustituye a la barra de progreso, que
 // fingia saber cuanto faltaba cuando nadie puede saberlo.
 var idle=null;
 if(!reduce&&spin.animate)idle=spin.animate([{transform:'rotate(0deg)'},{transform:'rotate(360deg)'}],{duration:2200,iterations:Infinity,easing:'linear'});
 if(reduce)mark.style.transform='translate(-50%,-50%)';

 mark.addEventListener('animationend',function(){rolled=true;maybe();});
 setTimeout(function(){if(!done)status.classList.add('on');},2200);

 function maybe(){if(rolled&&ready&&!done){done=true;finish();}}

 function finish(){
  status.classList.remove('on');
  if(idle){ // el giro remata una vuelta entera en vez de frenar en seco
   var deg=(idle.currentTime%2200)/2200*360;
   idle.cancel();
   spin.animate([{transform:'rotate('+deg+'deg)'},{transform:'rotate(360deg)'}],{duration:520,easing:'cubic-bezier(.22,1,.36,1)',fill:'forwards'});
  }
  if(reduce){leave(240);return;}
  stage.classList.add('reveal');
  setTimeout(function(){stage.classList.add('word');},120);
  setTimeout(function(){layout();stage.classList.add('search');},300);
  leave(1320);
 }

 // El escenario se difumina y debajo entra el cliente ya vestido con el tema
 // del usuario: index.html levanta un velo del mismo negro y lo funde.
 function leave(wait){
  setTimeout(function(){
   try{sessionStorage.setItem('noumon-boot','1');}catch(e){}
   stage.classList.add('gone');
   setTimeout(function(){location.replace('/');},280);
  },wait);
 }

 async function ping(){
  if(done)return;
  try{var r=await fetch('/api/health',{cache:'no-store'});if(r.ok){ready=true;maybe();return;}}catch(e){}
  // Agotada la gracia del shell, recargar cede el paso a la pagina de
  // desconexion (reintentar / conectar con otro servidor), que ya existe.
  if(Date.now()-started>11000)location.reload();
 }
 setInterval(ping,350);ping();
})();
</script>`

// chromeBar dibuja una franja superior arrastrable con los controles de
// ventana: la app es frameless y normalmente los pinta la SPA, así que sin
// esto las páginas del shell dejan al usuario sin forma de cerrar o mover.
// chromeScript la oculta si el runtime de Wails no está disponible.
const chromeBar = `<div id="chrome" hidden><div class="space"></div><button class="wc" id="wmin" aria-label="Minimizar">&#8211;</button><button class="wc" id="wmax" aria-label="Maximizar">&#9633;</button><button class="wc close" id="wclose" aria-label="Cerrar">&#10005;</button></div>`

const chromeScript = `<script>
(function(){var bar=document.getElementById('chrome');if(!window.runtime||typeof window.runtime.WindowMinimise!=='function')return;bar.hidden=false;
document.getElementById('wmin').onclick=function(){window.runtime.WindowMinimise()};
document.getElementById('wmax').onclick=function(){window.runtime.WindowToggleMaximise()};
document.getElementById('wclose').onclick=function(){window.runtime.Quit()};})();
</script>`

// El SVG es el espiral de noumon/src/lib/Logo.svelte, con el color del
// acento fijado porque aqui no hay CSS del cliente.
const escapedLogo = `%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100' fill='%237f77dd'%3E%3Cpath d='M94.22 62.68L91.28 66.16L88.15 69.24L84.86 71.91L81.47 74.18L78.01 76.05L74.52 77.52L71.03 78.63L67.59 79.37L64.22 79.77L60.96 79.85L57.81 79.63L54.82 79.14L51.99 78.39L49.34 77.42L46.88 76.25L44.64 74.91L42.60 73.41L40.78 71.80L39.18 70.08L37.81 68.29L36.65 66.45L35.70 64.57L34.96 62.69L34.42 60.81L34.08 58.97L33.91 57.16L34.63 56.84L34.95 58.48L35.43 60.12L36.08 61.74L36.90 63.34L37.90 64.90L39.07 66.39L40.41 67.80L41.92 69.11L43.59 70.29L45.40 71.34L47.36 72.24L49.44 72.96L51.64 73.50L53.94 73.83L56.31 73.94L58.74 73.82L61.21 73.47L63.70 72.86L66.17 72.00L68.60 70.89L70.98 69.51L73.26 67.87L75.43 65.98L77.45 63.85L79.30 61.47L80.95 58.88Z'/%3E%3Cpath d='M61.13 94.63L56.64 93.83L52.41 92.66L48.45 91.15L44.79 89.34L41.45 87.28L38.42 84.99L35.73 82.53L33.36 79.92L31.33 77.20L29.63 74.41L28.24 71.58L27.17 68.74L26.40 65.92L25.92 63.14L25.71 60.43L25.75 57.81L26.02 55.30L26.51 52.92L27.20 50.67L28.06 48.58L29.08 46.66L30.23 44.90L31.49 43.32L32.85 41.92L34.27 40.69L35.75 39.65L36.39 40.11L35.13 41.20L33.95 42.44L32.87 43.82L31.90 45.33L31.05 46.97L30.34 48.73L29.79 50.60L29.41 52.56L29.22 54.59L29.22 56.69L29.42 58.83L29.84 61.00L30.47 63.17L31.33 65.32L32.42 67.44L33.74 69.48L35.28 71.44L37.05 73.29L39.03 75.00L41.21 76.55L43.59 77.92L46.15 79.08L48.87 80.01L51.73 80.70L54.71 81.11L57.79 81.24Z'/%3E%3Cpath d='M16.91 81.95L15.36 77.67L14.26 73.41L13.59 69.23L13.33 65.16L13.44 61.23L13.91 57.47L14.69 53.90L15.77 50.55L17.11 47.43L18.67 44.56L20.43 41.95L22.36 39.60L24.42 37.52L26.58 35.71L28.82 34.17L31.11 32.90L33.42 31.88L35.73 31.12L38.02 30.59L40.26 30.30L42.43 30.21L44.53 30.33L46.53 30.63L48.42 31.10L50.20 31.73L51.84 32.48L51.76 33.27L50.19 32.73L48.53 32.32L46.79 32.07L44.99 31.99L43.15 32.07L41.27 32.34L39.38 32.80L37.49 33.45L35.63 34.30L33.81 35.35L32.06 36.59L30.39 38.04L28.83 39.67L27.40 41.50L26.11 43.50L25.00 45.66L24.07 47.98L23.35 50.43L22.86 53.00L22.61 55.67L22.62 58.41L22.89 61.21L23.44 64.03L24.28 66.85L25.41 69.64L26.84 72.37Z'/%3E%3Cpath d='M5.78 37.32L8.72 33.84L11.85 30.76L15.14 28.09L18.53 25.82L21.99 23.95L25.48 22.48L28.97 21.37L32.41 20.63L35.78 20.23L39.04 20.15L42.19 20.37L45.18 20.86L48.01 21.61L50.66 22.58L53.12 23.75L55.36 25.09L57.40 26.59L59.22 28.20L60.82 29.92L62.19 31.71L63.35 33.55L64.30 35.43L65.04 37.31L65.58 39.19L65.92 41.03L66.09 42.84L65.37 43.16L65.05 41.52L64.57 39.88L63.92 38.26L63.10 36.66L62.10 35.10L60.93 33.61L59.59 32.20L58.08 30.89L56.41 29.71L54.60 28.66L52.64 27.76L50.56 27.04L48.36 26.50L46.06 26.17L43.69 26.06L41.26 26.18L38.79 26.53L36.30 27.14L33.83 28.00L31.40 29.11L29.02 30.49L26.74 32.13L24.57 34.02L22.55 36.15L20.70 38.53L19.05 41.12Z'/%3E%3Cpath d='M38.87 5.37L43.36 6.17L47.59 7.34L51.55 8.85L55.21 10.66L58.55 12.72L61.58 15.01L64.27 17.47L66.64 20.08L68.67 22.80L70.37 25.59L71.76 28.42L72.83 31.26L73.60 34.08L74.08 36.86L74.29 39.57L74.25 42.19L73.98 44.70L73.49 47.08L72.80 49.33L71.94 51.42L70.92 53.34L69.77 55.10L68.51 56.68L67.15 58.08L65.73 59.31L64.25 60.35L63.61 59.89L64.87 58.80L66.05 57.56L67.13 56.18L68.10 54.67L68.95 53.03L69.66 51.27L70.21 49.40L70.59 47.44L70.78 45.41L70.78 43.31L70.58 41.17L70.16 39.00L69.53 36.83L68.67 34.68L67.58 32.56L66.26 30.52L64.72 28.56L62.95 26.71L60.97 25.00L58.79 23.45L56.41 22.08L53.85 20.92L51.13 19.99L48.27 19.30L45.29 18.89L42.21 18.76Z'/%3E%3Cpath d='M83.09 18.05L84.64 22.33L85.74 26.59L86.41 30.77L86.67 34.84L86.56 38.77L86.09 42.53L85.31 46.10L84.23 49.45L82.89 52.57L81.33 55.44L79.57 58.05L77.64 60.40L75.58 62.48L73.42 64.29L71.18 65.83L68.89 67.10L66.58 68.12L64.27 68.88L61.98 69.41L59.74 69.70L57.57 69.79L55.47 69.67L53.47 69.37L51.58 68.90L49.80 68.27L48.16 67.52L48.24 66.73L49.81 67.27L51.47 67.68L53.21 67.93L55.01 68.01L56.85 67.93L58.73 67.66L60.62 67.20L62.51 66.55L64.37 65.70L66.19 64.65L67.94 63.41L69.61 61.96L71.17 60.33L72.60 58.50L73.89 56.50L75.00 54.34L75.93 52.02L76.65 49.57L77.14 47.00L77.39 44.33L77.38 41.59L77.11 38.79L76.56 35.97L75.72 33.15L74.59 30.36L73.16 27.63Z'/%3E%3Ccircle cx='50' cy='50' r='11' fill-opacity='0.45'/%3E%3C/svg%3E`
