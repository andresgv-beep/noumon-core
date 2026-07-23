import {
  serverFetch, isShell, isGateway, getGatewayTarget, getServerBase,
} from './connection.js';

async function studioJSON(path, options = {}) {
  const response = await serverFetch(path, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.errorCode || `HTTP ${response.status}`);
    error.code = body.errorCode || '';
    error.status = response.status;
    error.details = body.details || {};
    throw error;
  }
  return body;
}

export async function getStudioCapabilities() {
  return studioJSON('/api/studio/capabilities');
}

export async function listStudioDocuments(status = 'all') {
  const body = await studioJSON(`/api/studio/documents?status=${encodeURIComponent(status)}`);
  return body.documents || [];
}

export async function getStudioDocument(id) {
  const body = await studioJSON(`/api/studio/documents/${encodeURIComponent(id)}`);
  return body.document;
}

export async function createStudioDocument(input) {
  const body = await studioJSON('/api/studio/documents', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  return body.document;
}

export async function updateStudioDocument(id, input) {
  const body = await studioJSON(`/api/studio/documents/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
  return body.document;
}

export async function archiveStudioDocument(id) {
  const body = await studioJSON(`/api/studio/documents/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
  return body.document;
}

export async function publishStudioDocument(id) {
  const body = await studioJSON(`/api/studio/documents/${encodeURIComponent(id)}/publish`, {
    method: 'POST',
  });
  return body.document;
}

export async function unpublishStudioDocument(id) {
  const body = await studioJSON(`/api/studio/documents/${encodeURIComponent(id)}/unpublish`, {
    method: 'POST',
  });
  return body.document;
}

export async function uploadStudioAsset(documentId, file) {
  const form = new FormData();
  form.append('file', file, file.name);
  const assetPath = `/api/studio/documents/${encodeURIComponent(documentId)}/assets`;
  let response;
  if (!isShell()) {
    response = await serverFetch(assetPath, { method: 'POST', body: form });
  } else {
    const grant = await studioJSON(
      `/api/studio/documents/${encodeURIComponent(documentId)}/upload-token`,
      { method: 'POST' },
    );
    const base = isGateway() ? await getGatewayTarget() : getServerBase();
    if (!base) throw new Error('studio.assets_unavailable');
    response = await fetch(
      `${String(base).replace(/\/+$/, '')}${assetPath}?ut=${encodeURIComponent(grant.token)}`,
      { method: 'POST', credentials: 'omit', body: form },
    );
  }
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.errorCode || `HTTP ${response.status}`);
    error.code = body.errorCode || '';
    error.status = response.status;
    error.details = body.details || {};
    throw error;
  }
  return body.asset;
}

export async function getStudioAssetBlob(documentId, assetId) {
  const response = await serverFetch(
    `/api/studio/documents/${encodeURIComponent(documentId)}/assets/${encodeURIComponent(assetId)}`,
  );
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.blob();
}

export async function deleteStudioAsset(documentId, assetId) {
  const response = await serverFetch(
    `/api/studio/documents/${encodeURIComponent(documentId)}/assets/${encodeURIComponent(assetId)}`,
    { method: 'DELETE' },
  );
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    const error = new Error(body.errorCode || `HTTP ${response.status}`);
    error.code = body.errorCode || '';
    error.status = response.status;
    throw error;
  }
}

export async function listPublishedDocuments() {
  const body = await studioJSON('/api/documents');
  return body.documents || [];
}

export async function getPublishedDocument(id) {
  const body = await studioJSON(`/api/documents/${encodeURIComponent(id)}`);
  return body.document;
}
