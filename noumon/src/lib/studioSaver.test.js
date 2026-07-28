import test from 'node:test';
import assert from 'node:assert/strict';
import { createStudioSaver, mergeStudioEnvelope } from './studioSaver.js';

const settle = () => new Promise((resolve) => setImmediate(resolve));

// Reloj de mentira: los tiempos del motor (rebote, escalera de reintentos) se
// comprueban por el retardo pedido, no esperando de verdad.
function createClock() {
  let now = 0;
  let seq = 0;
  const pending = new Map();
  const delays = [];
  return {
    delays,
    setTimer(fn, delay) {
      const id = ++seq;
      delays.push(delay);
      pending.set(id, { fn, at: now + delay });
      return id;
    },
    clearTimer(id) {
      if (id != null) pending.delete(id);
    },
    async advance(ms = 0) {
      now += ms;
      const due = [...pending].filter(([, entry]) => entry.at <= now).sort((a, b) => a[1].at - b[1].at);
      for (const [id, entry] of due) {
        if (!pending.has(id)) continue;
        pending.delete(id);
        entry.fn();
        await settle();
      }
      // Los temporizadores lanzan guardados sin esperarlos (`void saveNow()`), así
      // que hay que dejar drenar sus promesas antes de mirar el resultado.
      await settle();
    },
    waiting: () => pending.size,
  };
}

function createDocument(overrides = {}) {
  return {
    id: 'doc-1',
    status: 'draft',
    revision: 4,
    title: 'Renacimiento',
    content: { pages: [{ id: 'p1', blocks: [], infoCards: [{ id: 'c1', side: 'right' }] }] },
    ...overrides,
  };
}

// Respuesta típica del servidor: trae el sobre y un cuerpo que NO es el que el
// usuario tiene delante.
function serverReply(revision, extra = {}) {
  return {
    id: 'doc-1',
    revision,
    status: 'draft',
    updated: '2026-07-28T08:00:00Z',
    publishedRevision: 0,
    publicationKind: '',
    publicationTarget: '',
    published: false,
    coverAssetId: '',
    content: { pages: [] },
    title: 'lo que el servidor tenga guardado',
    ...extra,
  };
}

function harness({ save, document: initial, online = true } = {}) {
  const clock = createClock();
  const status = { dirty: false, saving: false, saved: false, offline: false };
  const state = {
    document: initial ?? createDocument(),
    errors: [],
    requests: [],
    recoverySaves: [],
    recoveryClears: [],
    savedNotices: [],
  };
  const defaultSave = async (id, input) => serverReply(input.baseRevision + 1);
  const saver = createStudioSaver({
    status,
    getDocument: () => state.document,
    toInput: (document) => ({
      title: document.title,
      content: document.content,
      baseRevision: document.revision,
    }),
    save: async (id, input) => {
      state.requests.push({ id, input });
      return (save ?? defaultSave)(id, input, state);
    },
    saveRecovery: async (document) => { state.recoverySaves.push(document.id); },
    clearRecovery: async (id) => { state.recoveryClears.push(id); },
    onSaved: (updated) => { state.savedNotices.push(updated.revision); },
    onError: (code) => { if (code) state.errors.push(code); },
    isOnline: () => online,
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer,
  });
  return { saver, status, state, clock };
}

test('solo copia el sobre y deja intacto el cuerpo que el usuario tiene delante', () => {
  const document = createDocument();
  const cards = document.content.pages[0].infoCards;
  mergeStudioEnvelope(document, serverReply(9, { publishedRevision: 9, published: true }));

  assert.equal(document.revision, 9);
  assert.equal(document.publishedRevision, 9);
  assert.equal(document.published, true);
  assert.equal(document.title, 'Renacimiento');
  // Misma referencia, no una copia: si se reemplazara, los contenteditable del
  // lienzo se desmontarían y las fichas laterales desaparecerían al publicar.
  assert.equal(document.content.pages[0].infoCards, cards);
});

test('el sobre trae el estado de publicación, que lo decide el servidor', async () => {
  const { saver, state } = harness({
    save: async (id, input) => serverReply(input.baseRevision + 1, {
      publishedRevision: 3,
      publicationOutdated: true,
    }),
  });

  saver.touch();
  await saver.saveNow();

  // El cliente no puede deducirlo restando revisiones: guardar sube la revisión
  // aunque no cambie nada. Si no viaja en el sobre, el aviso llega tarde.
  assert.equal(state.document.publicationOutdated, true);
  assert.equal(state.document.publishedRevision, 3);
});

test('no lanza dos peticiones a la vez: encadena y reenvía con la revisión nueva', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const { saver, state, status, clock } = harness({
    save: async (id, input) => {
      if (state.requests.length === 1) await gate;
      return serverReply(input.baseRevision + 1);
    },
  });

  saver.touch();
  const first = saver.saveNow();
  const second = saver.saveNow();
  assert.equal(state.requests.length, 1, 'la segunda llamada espera, no abre otra petición');

  release();
  assert.equal(await first, true);
  assert.equal(await second, true);
  await clock.advance(0);

  assert.equal(state.requests.length, 1);
  assert.equal(status.dirty, false);
  assert.equal(state.document.revision, 5);
});

test('lo que se escribe durante el guardado no se pierde y se reenvía sobre la revisión ya confirmada', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const { saver, state, status, clock } = harness({
    save: async (id, input) => {
      if (state.requests.length === 1) await gate;
      return serverReply(input.baseRevision + 1);
    },
  });

  saver.touch();
  const inFlight = saver.saveNow();
  // El usuario sigue escribiendo mientras la petición viaja.
  state.document.title = 'Renacimiento (corregido)';
  saver.touch();
  release();
  await inFlight;

  assert.equal(status.dirty, true, 'sigue sucio: lo confirmado no es lo que hay en pantalla');
  assert.equal(status.saved, false, 'no puede anunciar "guardado" con cambios sin enviar');
  assert.equal(state.document.title, 'Renacimiento (corregido)');
  assert.equal(state.document.revision, 5, 'el sobre avanza para que el reenvío no choque');

  await clock.advance(0);
  assert.equal(state.requests.length, 2);
  assert.equal(state.requests[1].input.baseRevision, 5);
  assert.equal(state.requests[1].input.title, 'Renacimiento (corregido)');
});

test('si se cambia de documento durante el guardado, la respuesta no toca al nuevo', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  const { saver, state, status } = harness({
    save: async (id, input) => {
      await gate;
      return serverReply(input.baseRevision + 1);
    },
  });

  saver.touch();
  const inFlight = saver.saveNow();
  const other = createDocument({ id: 'doc-2', revision: 1, title: 'Otro' });
  state.document = other;
  release();
  await inFlight;

  assert.equal(other.revision, 1, 'el documento abierto ahora conserva su revisión');
  assert.equal(other.title, 'Otro');
  assert.equal(status.saved, false);
  assert.equal(state.recoveryClears.length, 0, 'no borra la copia local del que ya no se ve');
});

test('sin red: guarda copia local, se queda sucio y reintenta con espera creciente', async () => {
  const { saver, state, status, clock } = harness({
    save: async () => { throw new Error('network'); },
  });

  saver.touch();
  assert.equal(await saver.saveNow(), false);

  assert.equal(status.offline, true);
  assert.equal(status.dirty, true, 'nunca se da por guardado lo que no llegó');
  assert.ok(state.errors.includes('studio.offline'));
  assert.deepEqual(state.recoverySaves, ['doc-1'], 'la copia local es la red de seguridad');
  assert.ok(clock.delays.includes(1000), 'primer reintento a 1s');

  await clock.advance(1000);
  assert.equal(state.requests.length, 2);
  assert.ok(clock.delays.includes(2000), 'el segundo espera el doble');
});

test('un conflicto no se reintenta: insistir no lo arregla', async () => {
  const { saver, state, status, clock } = harness({
    save: async () => { throw Object.assign(new Error('conflict'), { status: 409 }); },
  });

  saver.touch();
  assert.equal(await saver.saveNow(), false);

  assert.deepEqual(state.errors, ['studio.conflict']);
  assert.equal(status.offline, false);
  assert.equal(status.dirty, true);

  await clock.advance(60000);
  assert.equal(state.requests.length, 1, 'ni un reintento');
});

test('volver a tener red reintenta ya, sin esperar la escalera', async () => {
  const { saver, state, clock } = harness({
    save: async () => { throw new Error('network'); },
  });
  saver.touch();
  await saver.saveNow();
  const beforeRetry = state.requests.length;

  saver.retryNow();
  await clock.advance(0);

  assert.equal(state.requests.length, beforeRetry + 1);
});

test('flushUntilClean se rinde en vez de dar vueltas para siempre', async () => {
  const { saver, state, status } = harness({
    save: async (id, input, own) => {
      // El usuario no deja de escribir: el documento vuelve a ensuciarse dentro
      // de cada petición, así que nunca queda limpio.
      own.document.title += '!';
      saver.touch();
      return serverReply(input.baseRevision + 1);
    },
  });

  saver.touch();
  assert.equal(await saver.flushUntilClean(3), false, 'no puede decir que está limpio');
  assert.equal(state.requests.length, 3, 'se para en el tope pedido');
  assert.equal(status.dirty, true);
});

test('flushUntilClean falla si el guardado falla, y así publicar no sigue adelante', async () => {
  const { saver, state } = harness({
    save: async () => { throw Object.assign(new Error('boom'), { status: 500 }); },
  });

  saver.touch();
  assert.equal(await saver.flushUntilClean(), false);
  assert.equal(state.requests.length, 1, 'un error del servidor no se repite en bucle');
});

test('flushUntilClean confirma limpio cuando converge, y limpia la copia local', async () => {
  const { saver, state, status } = harness();

  saver.touch();
  assert.equal(await saver.flushUntilClean(), true);
  assert.equal(status.dirty, false);
  assert.deepEqual(state.recoveryClears, ['doc-1'], 'ya no hace falta la copia local');
  assert.deepEqual(state.savedNotices, [5]);
});

test('un documento archivado no se guarda ni se ensucia', async () => {
  const { saver, state, status } = harness({
    document: createDocument({ status: 'archived' }),
  });

  saver.touch();
  assert.equal(status.dirty, false);
  assert.equal(await saver.saveNow(), true);
  assert.equal(state.requests.length, 0);
});

test('el rebote agrupa una ráfaga de tecleo en un solo guardado', async () => {
  const { saver, state, clock } = harness();

  saver.touch();
  saver.touch();
  saver.touch();
  assert.equal(state.requests.length, 0, 'no guarda en cada tecla');

  await clock.advance(1200);
  assert.equal(state.requests.length, 1);
});

test('abandonar descarta lo pendiente sin intentar guardarlo', async () => {
  const { saver, state, clock } = harness();

  saver.touch();
  await saver.abandon();
  await clock.advance(60000);

  assert.equal(state.requests.length, 0, 'restaurar una revisión no debe guardar el borrador roto antes');
  assert.equal(clock.waiting(), 0);
});

test('abandonar espera a la petición en vuelo antes de seguir', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  let finished = false;
  const { saver, state } = harness({
    save: async (id, input) => {
      await gate;
      finished = true;
      return serverReply(input.baseRevision + 1);
    },
  });

  saver.touch();
  void saver.saveNow();
  const abandoning = saver.abandon();
  release();
  await abandoning;

  assert.equal(finished, true, 'no se restaura encima de una escritura a medias');
  assert.equal(state.requests.length, 1);
});

test('marcar limpio corta los reintentos pendientes del documento anterior', async () => {
  const { saver, status, state, clock } = harness({
    save: async () => { throw new Error('network'); },
  });

  saver.touch();
  await saver.saveNow();
  assert.equal(status.offline, true);

  // Se abre otro documento: lo que quedaba pendiente ya no es de éste.
  saver.markClean();
  await clock.advance(60000);

  assert.equal(status.dirty, false);
  assert.equal(status.offline, false);
  assert.equal(state.requests.length, 1, 'el reintento del anterior no revive');
});

test('marcar limpio con aviso anuncia guardado y luego se apaga solo', async () => {
  const { saver, status, clock } = harness();

  saver.markClean({ flash: true });
  assert.equal(status.saved, true);

  await clock.advance(1800);
  assert.equal(status.saved, false);
});

test('al soltar la vista los temporizadores dejan de disparar', async () => {
  const { saver, state, clock } = harness();

  saver.touch();
  saver.stop();
  await clock.advance(60000);

  assert.equal(state.requests.length, 0);
  assert.equal(clock.waiting(), 0);
});
