// Keep article URLs and the app's history in the same navigation flow.
export function articleLinkTarget(href, currentHref) {
  const url = new URL(href, currentHref);
  const current = new URL(currentHref);
  if (url.origin !== current.origin || !url.pathname.startsWith('/content/')) return null;
  // In-page anchors retain the native scroll/history behaviour.
  if (url.pathname === current.pathname && url.search === current.search && url.hash) return null;
  const rest = url.pathname.slice('/content/'.length);
  const slash = rest.indexOf('/');
  if (slash < 1) return null;
  return { lib: decodeURIComponent(rest.slice(0, slash)), path: decodeURIComponent(rest.slice(slash + 1)) + url.hash };
}

export const navigationSnapshot = (tab) => ({ kind: tab.kind, view: tab.view, lib: tab.lib, path: tab.path, itemId: tab.itemId, pageId: tab.pageId, open: tab.open, source: tab.source, title: tab.title });

export function recordFrameNavigation(tab, lib, path, title, initial = true) {
  // A late iframe load must never overwrite the home screen or another view.
  if (!tab || tab.kind !== 'article') return false;
  const same = tab.lib === lib && tab.path === path;
  // Native/script navigations still need history; the first load (and its
  // redirects) belongs to the navigation already recorded by the app.
  if (!initial && !same) {
    tab.back.push(navigationSnapshot(tab));
    tab.fwd = [];
  }
  tab.itemId = same ? (tab.itemId || '') : '';
  tab.lib = lib;
  tab.path = path;
  if (title) tab.title = title;
  return true;
}
