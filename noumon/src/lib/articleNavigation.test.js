import test from 'node:test';
import assert from 'node:assert/strict';
import { articleLinkTarget, navigationSnapshot, recordFrameNavigation } from './articleNavigation.js';

test('internal links use app history for HTTP and the Linux desktop gateway', () => {
  for (const origin of ['http://127.0.0.1:8090', 'wails://wails']) {
    const base = `${origin}/content/medical/Wikiproyecto:Medicina/Portada`;
    assert.deepEqual(articleLinkTarget('../Cardiolog%C3%ADa#Diagnóstico', base), { lib: 'medical', path: 'Cardiología#Diagn%C3%B3stico' });
    assert.equal(articleLinkTarget('#Temas', base), null);
    assert.equal(articleLinkTarget('https://example.org/article', base), null);
    assert.equal(articleLinkTarget('/api/libraries', base), null);
  }
});

test('native article navigation records the previous article, not just home', () => {
  const tab = {kind:'article',lib:'medical',path:'',back:[{kind:'home'}],fwd:[],title:'Medical'};
  recordFrameNavigation(tab,'medical','Portada','Portada',true);
  assert.equal(tab.back.length,1,'initial redirect replaces the empty library path');
  recordFrameNavigation(tab,'medical','Cardiología','Cardiología',false);
  assert.equal(tab.back.length,2);
  assert.equal(tab.back.at(-1).path,'Portada');
  const forward = navigationSnapshot(tab);
  Object.assign(tab,tab.back.pop());
  tab.fwd.push(forward);
  recordFrameNavigation(tab,'medical','Portada','Portada',true);
  assert.equal(tab.path,'Portada');
  assert.equal(tab.back.length,1,'back load does not add a new history entry');
  assert.equal(tab.fwd.at(-1).path,'Cardiología');
  recordFrameNavigation(tab,'medical','Portada','Portada',false);
  assert.equal(tab.back.length,1,'duplicate load does not add history');
});

test('late iframe loads cannot replace a home or settings view', () => {
  for (const kind of ['home','view','item']) {
    const tab={kind,back:[],fwd:[],lib:null,path:null};
    assert.equal(recordFrameNavigation(tab,'medical','Cardiología','Cardiología',false),false);
    assert.equal(tab.lib,null);
    assert.equal(tab.path,null);
    assert.equal(tab.back.length,0);
  }
});
