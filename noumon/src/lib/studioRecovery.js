const DB_NAME = 'noumon-studio-recovery';
const STORE_NAME = 'drafts';
const MAX_RECOVERIES = 10;
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;
let recoveryQueue = Promise.resolve();

function openRecoveryDB() {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve) => {
    const request = indexedDB.open(DB_NAME, 1);
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(STORE_NAME)) {
        request.result.createObjectStore(STORE_NAME, { keyPath: 'documentId' });
      }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => resolve(null);
  });
}

async function withStore(mode, operation) {
  const db = await openRecoveryDB();
  if (!db) return null;
  return new Promise((resolve) => {
    const tx = db.transaction(STORE_NAME, mode);
    const store = tx.objectStore(STORE_NAME);
    let result = null;
    try { result = operation(store); } catch { db.close(); resolve(null); return; }
    tx.oncomplete = () => { db.close(); resolve(result?.result ?? null); };
    tx.onerror = () => { db.close(); resolve(null); };
    tx.onabort = () => { db.close(); resolve(null); };
  });
}

export async function saveStudioRecovery(document) {
  if (!document?.id) return;
  const savedAt = Date.now();
  const snapshot = JSON.parse(JSON.stringify(document));
  recoveryQueue = recoveryQueue.then(() => withStore('readwrite', (store) => {
    store.put({
      documentId: document.id,
      baseRevision: document.revision,
      savedAt,
      document: snapshot,
    });
    const all = store.getAll();
    all.onsuccess = () => {
      const staleBefore = savedAt - MAX_AGE_MS;
      const entries = (all.result || []).sort((a, b) => b.savedAt - a.savedAt);
      for (const entry of entries) {
        if (entry.savedAt < staleBefore) store.delete(entry.documentId);
      }
      for (const entry of entries.slice(MAX_RECOVERIES)) store.delete(entry.documentId);
    };
  }));
  await recoveryQueue;
}

export async function loadStudioRecovery(documentId) {
  await recoveryQueue;
  return withStore('readonly', (store) => store.get(documentId));
}

export async function clearStudioRecovery(documentId) {
  recoveryQueue = recoveryQueue.then(() =>
    withStore('readwrite', (store) => store.delete(documentId)));
  await recoveryQueue;
}
