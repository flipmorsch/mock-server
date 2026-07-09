// mock/server console — observation surface (ADR-0009): SSE stream, journal
// filter/keyboard, command palette. The authoring surface is the Vue island
// (authoring.js); this talks to it only through document.body CustomEvents.

const $ = (sel, root) => (root || document).querySelector(sel)
const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel))

const emit = (name, detail) => document.body.dispatchEvent(new CustomEvent(name, { detail, bubbles: true }))

// ---- toasts -------------------------------------------------------------------

function showToast(msg, type) {
  const t = document.createElement('div')
  t.className = 'toast toast-' + (type || 'success')
  t.textContent = msg
  $('#toasts').appendChild(t)
  setTimeout(() => t.remove(), 2600)
}
document.body.addEventListener('toast', (e) => showToast(e.detail.msg, e.detail.type))

// ---- journal -> island seam (jump-to-rule, rule-from-request) ------------------

document.addEventListener('click', (e) => {
  const jump = e.target.closest('.vrule')
  if (jump) {
    e.preventDefault()
    emit('mock:edit-rule', { id: jump.dataset.ruleId, field: jump.dataset.field || '' })
    return
  }
  const seed = e.target.closest('.rule-from-req')
  if (seed) {
    e.preventDefault()
    emit('mock:seed-from', { seq: seed.dataset.seq })
  }
})

// ---- journal: filter, pause, clear, SSE ---------------------------------------

let paused = false
const pauseBuffer = []
const MAX_ROWS = 200

function filterTerms() {
  const f = $('#jfilter')
  return f && f.value ? f.value.toLowerCase().split(/\s+/).filter(Boolean) : []
}
function rowVisible(row, terms) {
  const hay = row.dataset.hay || ''
  return terms.every((t) => hay.includes(t))
}
function applyFilter() {
  const terms = filterTerms()
  $$('#stream .jrow').forEach((row) => row.classList.toggle('fhide', !rowVisible(row, terms)))
}
function updateCount() {
  const c = $('#jcount')
  if (c) c.textContent = $$('#stream .jrow').length
}
function insertRow(html) {
  const stream = $('#stream')
  if (!stream) return
  const tpl = document.createElement('template')
  tpl.innerHTML = html.trim()
  const row = tpl.content.firstElementChild
  if (!row) return
  row.classList.toggle('fhide', !rowVisible(row, filterTerms()))
  stream.prepend(row)
  const rows = $$('#stream .jrow')
  for (let i = MAX_ROWS; i < rows.length; i++) rows[i].remove()
  updateCount()
}
function connectStream() {
  const es = new EventSource('/_ui/api/events')
  es.onopen = () => $('#live-dot') && $('#live-dot').classList.add('on')
  es.onerror = () => $('#live-dot') && $('#live-dot').classList.remove('on')
  es.onmessage = (e) => {
    if (paused) { pauseBuffer.push(e.data); return }
    insertRow(e.data)
  }
}
function togglePause() {
  paused = !paused
  const btn = $('#jpause')
  if (btn) {
    btn.textContent = paused ? 'resume' : 'pause'
    btn.classList.toggle('paused', paused)
  }
  if (!paused) while (pauseBuffer.length) insertRow(pauseBuffer.shift())
}
function clearJournal() {
  fetch('/__admin/requests', { method: 'DELETE' }).then(() => {
    $$('#stream .jrow').forEach((r) => r.remove())
    pauseBuffer.length = 0
    updateCount()
  })
}

document.addEventListener('input', (e) => {
  if (e.target.id === 'jfilter') applyFilter()
})
document.addEventListener('click', (e) => {
  if (e.target.id === 'jpause') togglePause()
  if (e.target.id === 'jclear') clearJournal()
})

// ---- journal keyboard selection -----------------------------------------------

const visibleRows = () => $$('#stream .jrow').filter((r) => !r.classList.contains('fhide'))
const selectedRow = () => $('#stream .jrow.sel')

function moveSelection(delta) {
  const rows = visibleRows()
  if (!rows.length) return
  const cur = selectedRow()
  let idx = cur ? rows.indexOf(cur) + delta : delta > 0 ? 0 : rows.length - 1
  idx = Math.max(0, Math.min(rows.length - 1, idx))
  if (cur) cur.classList.remove('sel')
  rows[idx].classList.add('sel')
  rows[idx].scrollIntoView({ block: 'nearest' })
}

// ---- command palette ----------------------------------------------------------

const paletteActions = [
  { label: 'new rule', hint: '^N', run: () => emit('mock:new-rule') },
  { label: 'save to disk', hint: '^S', run: () => emit('mock:save') },
  { label: 'settings', hint: '', run: () => emit('mock:settings') },
  { label: 'clear journal', hint: '', run: clearJournal },
  { label: 'pause / resume stream', hint: '', run: togglePause },
  { label: 'focus filter', hint: '/', run: () => $('#jfilter') && $('#jfilter').focus() },
]

let palSel = 0
let palItems = []

function paletteItems() {
  const rules = $$('.rail-item').map((el) => ({
    label: (el.dataset.name || '(unnamed)') + '  ' + el.dataset.path,
    method: el.dataset.method,
    hint: 'open rule',
    run: () => emit('mock:edit-rule', { id: el.dataset.id }),
  }))
  return rules.concat(paletteActions)
}

// Subsequence fuzzy match; lower score = better (earlier, tighter match).
function fuzzy(q, s) {
  s = s.toLowerCase()
  let score = 0, pos = -1
  for (const ch of q) {
    pos = s.indexOf(ch, pos + 1)
    if (pos === -1) return -1
    score += pos
  }
  return score
}

const paletteOpen = () => !$('#palette').hidden

function renderPalette() {
  const q = $('#palette-input').value.trim().toLowerCase()
  let items = paletteItems()
  if (q) {
    items = items
      .map((it) => ({ it, score: fuzzy(q, it.label) }))
      .filter((x) => x.score >= 0)
      .sort((a, b) => a.score - b.score)
      .map((x) => x.it)
  }
  palItems = items.slice(0, 12)
  palSel = Math.max(0, Math.min(palSel, palItems.length - 1))
  const box = $('#palette-results')
  box.innerHTML = ''
  if (!palItems.length) {
    box.innerHTML = '<div class="palette-empty">nothing matches</div>'
    return
  }
  palItems.forEach((it, i) => {
    const el = document.createElement('div')
    el.className = 'palette-item' + (i === palSel ? ' sel' : '')
    if (it.method) {
      const m = document.createElement('span')
      m.className = 'method method-' + it.method.toUpperCase()
      m.textContent = it.method.toUpperCase()
      el.appendChild(m)
    }
    const label = document.createElement('span')
    label.className = 'palette-item-label'
    label.textContent = it.label
    el.appendChild(label)
    if (it.hint) {
      const hint = document.createElement('span')
      hint.className = 'palette-item-hint'
      hint.textContent = it.hint
      el.appendChild(hint)
    }
    el.addEventListener('click', () => { hidePalette(); it.run() })
    box.appendChild(el)
  })
}

function showPalette() {
  palSel = 0
  $('#palette').hidden = false
  const input = $('#palette-input')
  input.value = ''
  renderPalette()
  input.focus()
}
const hidePalette = () => { $('#palette').hidden = true }
function paletteExec() {
  const it = palItems[palSel]
  hidePalette()
  if (it) it.run()
}

document.addEventListener('input', (e) => {
  if (e.target.id === 'palette-input') { palSel = 0; renderPalette() }
})
document.addEventListener('click', (e) => {
  if (e.target.id === 'palette') hidePalette()
})

// ---- global keyboard map ------------------------------------------------------

document.addEventListener('keydown', (e) => {
  const mod = e.ctrlKey || e.metaKey

  if (paletteOpen()) {
    if (e.key === 'Escape') { e.preventDefault(); hidePalette() }
    if (e.key === 'ArrowDown') { e.preventDefault(); palSel++; renderPalette() }
    if (e.key === 'ArrowUp') { e.preventDefault(); palSel = Math.max(0, palSel - 1); renderPalette() }
    if (e.key === 'Enter') { e.preventDefault(); paletteExec() }
    return
  }

  if (mod && e.key === 'k') { e.preventDefault(); showPalette(); return }
  if (mod && e.key === 's') { e.preventDefault(); emit('mock:save'); return }
  if (mod && e.key === 'n') { e.preventDefault(); emit('mock:new-rule'); return }

  const inField = e.target.closest('input, textarea, select')
  if (e.key === 'Escape') {
    if (inField) { e.target.blur(); return }
    emit('mock:close-editor')
    return
  }
  if (inField) return

  switch (e.key) {
    case '/':
      e.preventDefault()
      if ($('#jfilter')) $('#jfilter').focus()
      break
    case 'j':
      moveSelection(1)
      break
    case 'k':
      moveSelection(-1)
      break
    case 'Enter': {
      const row = selectedRow()
      if (row) { e.preventDefault(); row.open = !row.open }
      break
    }
    case 'e': {
      const row = selectedRow()
      if (row) {
        const link = $('.vrule', row)
        if (link) { row.open = true; emit('mock:edit-rule', { id: link.dataset.ruleId, field: link.dataset.field || '' }) }
      }
      break
    }
  }
})

// ---- boot ---------------------------------------------------------------------

connectStream()
updateCount()
