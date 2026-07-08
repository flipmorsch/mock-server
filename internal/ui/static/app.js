// mock/server console — SSE stream, keyboard map, command palette.

// ---- Alpine component factories --------------------------------------------

function kvList(prefix, items) {
  return { prefix: prefix, items: items };
}

// ---- helpers -----------------------------------------------------------------

const $ = (sel, root) => (root || document).querySelector(sel);
const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));

function showToast(msg, type) {
  const t = document.createElement('div');
  t.className = 'toast toast-' + (type || 'success');
  t.textContent = msg;
  $('#toasts').appendChild(t);
  setTimeout(() => t.remove(), 2600);
}

// ---- editor pane state --------------------------------------------------------

function editorOpen() {
  return $('#editor-pane').children.length > 0;
}

function syncEditingClass() {
  document.body.classList.toggle('editing', editorOpen());
}

function closeEditor() {
  $('#editor-pane').innerHTML = '';
  $$('.rail-item.active').forEach(el => el.classList.remove('active'));
  syncEditingClass();
}

document.addEventListener('htmx:afterSwap', (e) => {
  if (e.detail.target.id === 'editor-pane' || e.detail.target.id === 'editor') syncEditingClass();
  if (window.Alpine) { window.Alpine.initTree(e.detail.target); }
});

document.addEventListener('click', (e) => {
  if (e.target.closest('#editor-close')) closeEditor();
  const item = e.target.closest('.rail-item');
  if (item) {
    $$('.rail-item.active').forEach(el => el.classList.remove('active'));
    item.classList.add('active');
  }
});

document.body.addEventListener('editor-closed', closeEditor);

// ---- unsaved flag + beforeunload -----------------------------------------------

let unsaved = false;
document.body.addEventListener('unsaved', (e) => { unsaved = !!e.detail.value; });
window.addEventListener('beforeunload', (e) => {
  if (unsaved) { e.preventDefault(); e.returnValue = ''; }
});

document.body.addEventListener('toast', (e) => showToast(e.detail.msg, e.detail.type));

// ---- journal: filter, pause, clear, SSE ------------------------------------------

let paused = false;
const pauseBuffer = [];
const MAX_ROWS = 200;

function filterTerms() {
  const f = $('#jfilter');
  return f && f.value ? f.value.toLowerCase().split(/\s+/).filter(Boolean) : [];
}

function rowVisible(row, terms) {
  const hay = row.dataset.hay || '';
  return terms.every(t => hay.includes(t));
}

function applyFilter() {
  const terms = filterTerms();
  $$('#stream .jrow').forEach(row => row.classList.toggle('fhide', !rowVisible(row, terms)));
}

function updateCount() {
  const c = $('#jcount');
  if (c) c.textContent = $$('#stream .jrow').length;
}

function insertRow(html) {
  const stream = $('#stream');
  if (!stream) return;
  const tpl = document.createElement('template');
  tpl.innerHTML = html.trim();
  const row = tpl.content.firstElementChild;
  if (!row) return;
  row.classList.toggle('fhide', !rowVisible(row, filterTerms()));
  stream.prepend(row);
  htmx.process(row);
  const rows = $$('#stream .jrow');
  for (let i = MAX_ROWS; i < rows.length; i++) rows[i].remove();
  updateCount();
}

function connectStream() {
  const es = new EventSource('/_ui/api/events');
  es.onopen = () => $('#live-dot') && $('#live-dot').classList.add('on');
  es.onerror = () => $('#live-dot') && $('#live-dot').classList.remove('on');
  es.onmessage = (e) => {
    if (paused) { pauseBuffer.push(e.data); return; }
    insertRow(e.data);
  };
}

function togglePause() {
  paused = !paused;
  const btn = $('#jpause');
  if (btn) {
    btn.textContent = paused ? 'resume' : 'pause';
    btn.classList.toggle('paused', paused);
  }
  if (!paused) {
    while (pauseBuffer.length) insertRow(pauseBuffer.shift());
  }
}

function clearJournal() {
  fetch('/__admin/requests', { method: 'DELETE' }).then(() => {
    $$('#stream .jrow').forEach(r => r.remove());
    pauseBuffer.length = 0;
    updateCount();
  });
}

document.addEventListener('input', (e) => {
  if (e.target.id === 'jfilter') applyFilter();
});
document.addEventListener('click', (e) => {
  if (e.target.id === 'jpause') togglePause();
  if (e.target.id === 'jclear') clearJournal();
});

// ---- journal keyboard selection ---------------------------------------------------

function visibleRows() {
  return $$('#stream .jrow').filter(r => !r.classList.contains('fhide'));
}

function selectedRow() {
  return $('#stream .jrow.sel');
}

function moveSelection(delta) {
  const rows = visibleRows();
  if (!rows.length) return;
  const cur = selectedRow();
  let idx = cur ? rows.indexOf(cur) + delta : (delta > 0 ? 0 : rows.length - 1);
  idx = Math.max(0, Math.min(rows.length - 1, idx));
  if (cur) cur.classList.remove('sel');
  rows[idx].classList.add('sel');
  rows[idx].scrollIntoView({ block: 'nearest' });
}

// ---- command palette -----------------------------------------------------------------

const paletteActions = [
  { label: 'new rule', hint: '^N', run: () => htmx.ajax('GET', '/_ui/partials/new-rule', { target: '#editor-pane', swap: 'innerHTML' }) },
  { label: 'save to disk', hint: '^S', run: () => htmx.ajax('POST', '/_ui/api/save', { swap: 'none' }) },
  { label: 'settings', hint: '', run: () => htmx.ajax('GET', '/_ui/partials/settings', { target: '#editor-pane', swap: 'innerHTML' }) },
  { label: 'clear journal', hint: '', run: clearJournal },
  { label: 'pause / resume stream', hint: '', run: togglePause },
  { label: 'focus filter', hint: '/', run: () => $('#jfilter') && $('#jfilter').focus() },
  { label: 'close editor', hint: 'esc', run: closeEditor },
];

let palSel = 0;
let palItems = [];

function paletteItems() {
  const rules = $$('.rail-item').map(el => ({
    label: (el.dataset.name || '(unnamed)') + '  ' + el.dataset.path,
    method: el.dataset.method,
    hint: 'open rule',
    run: () => htmx.ajax('GET', '/_ui/partials/rule-editor/' + el.dataset.id, { target: '#editor-pane', swap: 'innerHTML' }),
  }));
  return rules.concat(paletteActions);
}

// Subsequence fuzzy match; lower score = better (earlier, tighter match).
function fuzzy(q, s) {
  s = s.toLowerCase();
  let score = 0, pos = -1;
  for (const ch of q) {
    pos = s.indexOf(ch, pos + 1);
    if (pos === -1) return -1;
    score += pos;
  }
  return score;
}

function paletteOpen() {
  return !$('#palette').hidden;
}

function renderPalette() {
  const q = $('#palette-input').value.trim().toLowerCase();
  let items = paletteItems();
  if (q) {
    items = items
      .map(it => ({ it, score: fuzzy(q, it.label) }))
      .filter(x => x.score >= 0)
      .sort((a, b) => a.score - b.score)
      .map(x => x.it);
  }
  palItems = items.slice(0, 12);
  palSel = Math.max(0, Math.min(palSel, palItems.length - 1));
  const box = $('#palette-results');
  box.innerHTML = '';
  if (!palItems.length) {
    box.innerHTML = '<div class="palette-empty">nothing matches</div>';
    return;
  }
  palItems.forEach((it, i) => {
    const el = document.createElement('div');
    el.className = 'palette-item' + (i === palSel ? ' sel' : '');
    if (it.method) {
      const m = document.createElement('span');
      m.className = 'method method-' + it.method.toUpperCase();
      m.textContent = it.method.toUpperCase();
      el.appendChild(m);
    }
    const label = document.createElement('span');
    label.className = 'palette-item-label';
    label.textContent = it.label;
    el.appendChild(label);
    if (it.hint) {
      const hint = document.createElement('span');
      hint.className = 'palette-item-hint';
      hint.textContent = it.hint;
      el.appendChild(hint);
    }
    el.addEventListener('click', () => { hidePalette(); it.run(); });
    box.appendChild(el);
  });
}

function showPalette() {
  palSel = 0;
  const p = $('#palette');
  p.hidden = false;
  const input = $('#palette-input');
  input.value = '';
  renderPalette();
  input.focus();
}

function hidePalette() {
  $('#palette').hidden = true;
}

function paletteExec() {
  const it = palItems[palSel];
  hidePalette();
  if (it) it.run();
}

document.addEventListener('input', (e) => {
  if (e.target.id === 'palette-input') { palSel = 0; renderPalette(); }
});

document.addEventListener('click', (e) => {
  if (e.target.id === 'palette') hidePalette();
});

// ---- global keyboard map ---------------------------------------------------------------

document.addEventListener('keydown', (e) => {
  const mod = e.ctrlKey || e.metaKey;

  if (paletteOpen()) {
    if (e.key === 'Escape') { e.preventDefault(); hidePalette(); }
    if (e.key === 'ArrowDown') { e.preventDefault(); palSel++; renderPalette(); }
    if (e.key === 'ArrowUp') { e.preventDefault(); palSel = Math.max(0, palSel - 1); renderPalette(); }
    if (e.key === 'Enter') { e.preventDefault(); paletteExec(); }
    return;
  }

  if (mod && e.key === 'k') { e.preventDefault(); showPalette(); return; }
  if (mod && e.key === 's') { e.preventDefault(); htmx.ajax('POST', '/_ui/api/save', { swap: 'none' }); return; }
  if (mod && e.key === 'n') { e.preventDefault(); htmx.ajax('GET', '/_ui/partials/new-rule', { target: '#editor-pane', swap: 'innerHTML' }); return; }

  const inField = e.target.closest('input, textarea, select');
  if (e.key === 'Escape') {
    if (inField) { e.target.blur(); return; }
    closeEditor();
    return;
  }
  if (inField) return;

  switch (e.key) {
    case '/':
      e.preventDefault();
      if ($('#jfilter')) $('#jfilter').focus();
      break;
    case 'j':
      moveSelection(1);
      break;
    case 'k':
      moveSelection(-1);
      break;
    case 'Enter': {
      const row = selectedRow();
      if (row) { e.preventDefault(); row.open = !row.open; }
      break;
    }
    case 'e': {
      const row = selectedRow();
      if (row) {
        const link = $('.vrule', row);
        if (link) { row.open = true; link.click(); }
      }
      break;
    }
  }
});

// ---- rail drag & drop reorder --------------------------------------------------------------

let dragged = null;

document.addEventListener('dragstart', (e) => {
  const item = e.target.closest('.rail-item');
  if (!item) return;
  dragged = item;
  item.classList.add('dragging');
});

document.addEventListener('dragover', (e) => {
  const item = e.target.closest('.rail-item');
  if (!item || !dragged || item === dragged) return;
  e.preventDefault();
  $$('.rail-item.drag-over').forEach(el => el.classList.remove('drag-over'));
  item.classList.add('drag-over');
});

document.addEventListener('drop', (e) => {
  const target = e.target.closest('.rail-item');
  if (!target || !dragged || target === dragged) return;
  e.preventDefault();
  const list = $('#rail-list');
  const items = $$('.rail-item', list);
  if (items.indexOf(dragged) < items.indexOf(target)) {
    target.after(dragged);
  } else {
    target.before(dragged);
  }
  const ids = $$('.rail-item', list).map(el => el.dataset.id);
  fetch('/_ui/api/rules/reorder', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ids),
  }).then((resp) => {
    if (resp.ok) {
      unsaved = true;
      document.body.dispatchEvent(new CustomEvent('rail-refresh', { bubbles: true }));
    }
  });
});

document.addEventListener('dragend', () => {
  $$('.rail-item').forEach(el => el.classList.remove('dragging', 'drag-over'));
  dragged = null;
});

// ---- boot ------------------------------------------------------------------------------------

connectStream();
updateCount();
