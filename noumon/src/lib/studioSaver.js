// Motor de guardado de Studio: rebote, reintentos, copia local y las carreras
// entre el usuario escribiendo y el servidor respondiendo.
//
// Vive fuera del componente a propósito. Aquí no se importa Svelte ni se toca el
// DOM: todo entra por parámetros y sale por callbacks, así que las carreras se
// pueden provocar en frío en un test en vez de a base de escribir rápido y mirar
// la pantalla. De este código salieron los peores fallos del editor ("no guarda
// nada", las fichas que desaparecían al publicar, la publicación que se quedaba
// pendiente) y ninguno tenía prueba que lo sujetara.

// Lo que el servidor manda y el cliente acata. El resto del documento —cuerpo,
// título, etiquetas— es del usuario mientras edita.
const ENVELOPE_FIELDS = [
  'revision',
  'status',
  'updated',
  'publishedRevision',
  'publicationKind',
  'publicationTarget',
  'published',
  'coverAssetId',
  // Si esto no viaja en el sobre, tras guardar se sigue viendo el estado de
  // publicación de antes del guardado, y el aviso llega siempre un paso tarde.
  'publicationOutdated',
];

// Copia sobre el documento SOLO el sobre, mutándolo en el sitio. Reemplazar el
// documento entero por la respuesta desmonta los contenteditable, pierde el foco
// y devuelve el lienzo hacia arriba; y si la respuesta llega sin normalizar, se
// lleva por delante lo que el usuario acaba de escribir.
export function mergeStudioEnvelope(document, updated) {
  if (!document || !updated) return document;
  for (const field of ENVELOPE_FIELDS) document[field] = updated[field];
  return document;
}

export function createStudioSaver({
  // Objeto de estado compartido con la vista: {dirty, saving, saved, offline}.
  // Se muta en el sitio para que las runas de Svelte lo vean sin que este módulo
  // sepa nada de Svelte.
  status,
  getDocument,
  toInput,
  save,
  normalize = (doc) => doc,
  saveRecovery = async () => {},
  clearRecovery = async () => {},
  onSaved = () => {},
  onError = () => {},
  isOnline = () => typeof navigator === 'undefined' || navigator.onLine !== false,
  setTimer = (fn, delay) => setTimeout(fn, delay),
  clearTimer = (handle) => clearTimeout(handle),
  saveDelay = 1200,
  recoveryDelay = 300,
  savedDelay = 1800,
  maxRetryDelay = 30000,
  maxRetryAttempts = 5,
} = {}) {
  let saveTimer = null;
  let savedTimer = null;
  let recoveryTimer = null;
  let retryTimer = null;
  let savePromise = null;
  // Cuenta cada cambio del usuario. Es lo que distingue "el servidor confirmó lo
  // que envié" de "confirmó una versión que ya se ha quedado atrás".
  let changeVersion = 0;
  let retryAttempt = 0;
  let active = true;

  function editable() {
    const document = getDocument();
    return document && document.status !== 'archived' ? document : null;
  }

  function touch() {
    if (!editable()) return;
    markDirty();
    scheduleRecovery();
  }

  // Hay cambios que no vienen de teclear —restaurar la copia local, por ejemplo—
  // y que ya están a salvo en disco: ensucian y piden guardado, pero no vuelven a
  // escribir la copia que acaban de leer.
  function markDirty() {
    if (!editable()) return;
    status.dirty = true;
    status.saved = false;
    changeVersion += 1;
    scheduleSave();
  }

  function scheduleRecovery(delay = recoveryDelay) {
    clearTimer(recoveryTimer);
    recoveryTimer = setTimer(() => {
      recoveryTimer = null;
      const document = getDocument();
      if (document && status.dirty) void saveRecovery(document);
    }, delay);
  }

  async function persistRecoveryNow() {
    clearTimer(recoveryTimer);
    recoveryTimer = null;
    const document = getDocument();
    if (document && status.dirty) await saveRecovery(document);
  }

  function scheduleSave(delay = saveDelay) {
    clearTimer(saveTimer);
    saveTimer = setTimer(() => {
      saveTimer = null;
      void saveNow();
    }, delay);
  }

  function clearRetry() {
    clearTimer(retryTimer);
    retryTimer = null;
    retryAttempt = 0;
  }

  // Espera creciente entre reintentos. Sólo se reintenta cuando la causa fue
  // quedarse sin red: un 409 o un error del servidor no se arregla insistiendo.
  function scheduleRetry(delay) {
    if (!active || !status.dirty || !status.offline) return;
    clearTimer(retryTimer);
    if (!isOnline()) return;
    const wait = delay ?? Math.min(maxRetryDelay, 1000 * (2 ** retryAttempt));
    retryAttempt = Math.min(retryAttempt + 1, maxRetryAttempts);
    retryTimer = setTimer(() => {
      retryTimer = null;
      if (active && status.dirty && status.offline) void saveNow();
    }, wait);
  }

  // Vuelve a intentarlo ya, sin esperar la escalera: lo llama la vista cuando el
  // navegador avisa de que hay red otra vez.
  function retryNow() {
    retryAttempt = 0;
    scheduleRetry(0);
  }

  async function saveNow() {
    clearTimer(saveTimer);
    saveTimer = null;
    // Nunca dos peticiones a la vez sobre el mismo documento: la segunda llegaría
    // con un baseRevision viejo y el servidor la rechazaría por conflicto. Se
    // espera a la que está en vuelo y, si entretanto se escribió más, se encadena.
    if (savePromise) {
      const previousOK = await savePromise;
      if (previousOK && status.dirty) return saveNow();
      return previousOK && !status.dirty;
    }
    const document = editable();
    if (!document || !status.dirty) return true;

    const documentId = document.id;
    const version = changeVersion;
    // Instantánea profunda: a partir de aquí el usuario puede seguir escribiendo
    // sin que lo que viaja por la red cambie debajo.
    const input = JSON.parse(JSON.stringify(toInput(document)));
    status.saving = true;
    status.offline = false;
    onError('');
    savePromise = (async () => {
      try {
        const updated = normalize(await save(documentId, input));
        clearRetry();
        status.offline = false;
        onSaved(updated);
        // El usuario cambió de documento mientras se guardaba: la respuesta es
        // del anterior y escribirla encima del nuevo lo corrompe.
        if (getDocument()?.id !== documentId) return true;

        if (changeVersion === version) {
          mergeStudioEnvelope(getDocument(), updated);
          status.dirty = false;
          status.saved = true;
          clearTimer(recoveryTimer);
          recoveryTimer = null;
          await clearRecovery(documentId);
          clearTimer(savedTimer);
          savedTimer = setTimer(() => { status.saved = false; }, savedDelay);
        } else {
          // El servidor guardó la instantánea enviada, pero el usuario siguió
          // escribiendo durante la petición. Se conservan esos cambios y sólo se
          // adelanta su baseRevision para el guardado siguiente.
          mergeStudioEnvelope(getDocument(), updated);
          status.dirty = true;
          scheduleRecovery(0);
          scheduleSave(0);
        }
        return true;
      } catch (e) {
        status.offline = !e.status;
        if (e.status === 409) onError('studio.conflict');
        else if (status.offline) {
          onError('studio.offline');
          await persistRecoveryNow();
          scheduleRetry();
        } else onError(e.code || e.message);
        if (!status.offline) clearRetry();
        return false;
      } finally {
        status.saving = false;
        savePromise = null;
      }
    })();
    return savePromise;
  }

  async function flushCurrent() {
    clearTimer(saveTimer);
    saveTimer = null;
    if (savePromise && !await savePromise) return false;
    if (!status.dirty) return true;
    return saveNow();
  }

  // Publicar exige que no quede nada sin guardar. Un flush puede terminar bien y
  // dejar el documento sucio otra vez (el usuario siguió escribiendo), así que se
  // repite; pero con tope, porque escribiendo sin parar no converge nunca y sin
  // tope esto se quedaba dando vueltas y la publicación "pendiente".
  async function flushUntilClean(attempts = 5) {
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      if (!await flushCurrent()) return false;
      if (!status.dirty) return true;
    }
    return !status.dirty;
  }

  // Documento recién abierto o recién restaurado: lo que hubiera pendiente ya no
  // es de éste. Con `flash` además se anuncia "guardado" un momento, que es lo
  // que corresponde tras restaurar una revisión: en el servidor ya está.
  function markClean({ flash = false } = {}) {
    changeVersion = 0;
    clearRetry();
    status.dirty = false;
    status.offline = false;
    status.saved = flash;
    if (flash) {
      clearTimer(savedTimer);
      savedTimer = setTimer(() => { status.saved = false; }, savedDelay);
    }
  }

  // Tirar por la borda lo que hubiera en marcha, sin intentar guardarlo. Lo pide
  // restaurar una revisión: descartar el borrador es justo la vía de salida
  // cuando el borrador es el que está roto, así que guardarlo antes lo estropea.
  async function abandon() {
    clearTimer(saveTimer);
    clearTimer(recoveryTimer);
    saveTimer = null;
    recoveryTimer = null;
    clearRetry();
    if (savePromise) await savePromise;
    clearTimer(saveTimer);
    clearTimer(recoveryTimer);
    saveTimer = null;
    recoveryTimer = null;
  }

  // La vista se va: se sueltan los temporizadores para que no disparen sobre un
  // componente desmontado. El vaciado final lo decide quien llama.
  function stop() {
    active = false;
    clearTimer(saveTimer);
    clearTimer(savedTimer);
    clearTimer(recoveryTimer);
    clearTimer(retryTimer);
    saveTimer = savedTimer = recoveryTimer = retryTimer = null;
  }

  return {
    touch,
    markDirty,
    markClean,
    abandon,
    scheduleSave,
    scheduleRecovery,
    persistRecoveryNow,
    saveNow,
    flushCurrent,
    flushUntilClean,
    retryNow,
    stop,
  };
}
