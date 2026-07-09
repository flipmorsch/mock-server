// Client-owned working copy (ADR-0010). One reactive singleton shared by the
// rail and editor apps; edits are optimistic, undo is a snapshot stack, and the
// whole copy is POSTed on save. The server owns nothing until save.
//
// The store holds an EDIT shape: headers/query are ordered [{k,v}] pairs (so the
// kv-editors can rename/reorder/add), and request body is a {bodyMode,bodyValue}
// pair. toWire()/toEdit() convert to and from the server's map-based JSON only at
// the save/load boundary, keeping every component free of shape-juggling.
import { reactive } from 'vue'

export const SETTINGS = '__settings__' // sentinel selectedId for the settings pane

export const store = reactive({
  listen: '',
  rules: [], // edit-shape rules (see toEditRule)
  selectedId: null,
  highlight: '', // failing dim to flash when opened via jump-to-rule
  dirty: false,
  loaded: false,
  _undo: [],
})

// uuid mints an RFC-4122 v4 id, matching the server's newID. crypto.getRandomValues
// (unlike crypto.randomUUID) works on plain-http non-localhost origins too.
export function uuid() {
  const b = crypto.getRandomValues(new Uint8Array(16))
  b[6] = (b[6] & 0x0f) | 0x40
  b[8] = (b[8] & 0x3f) | 0x80
  const h = [...b].map((x) => x.toString(16).padStart(2, '0'))
  return `${h.slice(0, 4).join('')}-${h.slice(4, 6).join('')}-${h.slice(6, 8).join('')}-${h.slice(8, 10).join('')}-${h.slice(10, 16).join('')}`
}

// ---- edit <-> wire conversion ------------------------------------------------

const mapToPairs = (m) => Object.entries(m || {}).map(([k, v]) => ({ k, v }))
function pairsToMap(pairs) {
  const m = {}
  ;(pairs || []).forEach((p) => {
    if (p.k) m[p.k] = p.v
  })
  return Object.keys(m).length ? m : undefined
}

export function blankResp() {
  return { status: 200, delay: '', template: false, body: '', body_file: '', headers: [] }
}
const editResp = (r) => ({
  status: r.status || 200,
  delay: r.delay || '',
  template: !!r.template,
  body: r.body || '',
  body_file: r.body_file || '',
  headers: mapToPairs(r.headers),
})
function wireResp(r) {
  const o = { status: parseInt(r.status, 10) || 0 }
  if (r.delay) o.delay = r.delay
  if (r.template) o.template = true
  if (r.body) o.body = r.body
  if (r.body_file) o.body_file = r.body_file
  const h = pairsToMap(r.headers)
  if (h) o.headers = h
  return o
}

export function toEditRule(r) {
  const req = r.request || {}
  const body = req.body
  return {
    id: r.id || uuid(),
    name: r.name || '',
    request: {
      method: req.method || 'GET',
      path: req.path || '',
      path_mode: req.path_mode || 'exact',
      headers: mapToPairs(req.headers),
      query: mapToPairs(req.query),
      bodyMode: body ? body.mode || 'exact' : 'none',
      bodyValue: body ? body.value || '' : '',
    },
    response: editResp(r.response || {}),
    responses: (r.responses || []).map(editResp),
  }
}

function toWireRule(r) {
  const req = r.request
  const out = { id: r.id, request: { method: req.method, path: req.path, path_mode: req.path_mode } }
  if (r.name) out.name = r.name
  const h = pairsToMap(req.headers)
  if (h) out.request.headers = h
  const q = pairsToMap(req.query)
  if (q) out.request.query = q
  if (req.bodyMode && req.bodyMode !== 'none') out.request.body = { mode: req.bodyMode, value: req.bodyValue }
  if (r.responses && r.responses.length) out.responses = r.responses.map(wireResp)
  else out.response = wireResp(r.response)
  return out
}

export function newRule() {
  return toEditRule({ id: uuid(), request: { method: 'GET', path: '', path_mode: 'exact' }, response: { status: 200 } })
}

export function selected() {
  return store.rules.find((r) => r.id === store.selectedId) || null
}

// ---- undo + mutation ---------------------------------------------------------

// snapshot pushes the current state for undo, deduping consecutive identical
// states so a focus that changes nothing doesn't cost an undo step.
export function snapshot() {
  const snap = JSON.stringify({ listen: store.listen, rules: store.rules })
  if (store._undo[store._undo.length - 1] === snap) return
  store._undo.push(snap)
  if (store._undo.length > 100) store._undo.shift() // ponytail: cap the stack; 100 undos is plenty
}

// mutate wraps a structural edit: snapshot for undo, run it, mark dirty.
export function mutate(fn) {
  snapshot()
  fn()
  store.dirty = true
}

export function undo() {
  if (!store._undo.length) return
  const prev = JSON.parse(store._undo.pop())
  store.listen = prev.listen
  store.rules = prev.rules
  store.dirty = true
  if (store.selectedId !== SETTINGS && !store.rules.some((r) => r.id === store.selectedId)) {
    store.selectedId = null
  }
}

export async function load() {
  const cfg = await (await fetch('/_ui/api/rules')).json()
  store.listen = cfg.listen || ''
  store.rules = (cfg.rules || []).map(toEditRule)
  store.selectedId = null
  store.dirty = false
  store._undo = []
  store.loaded = true
}

// save POSTs the whole working copy as JSON (ADR-0010). Throws on a validation
// failure so callers can surface the server's message.
export async function save() {
  const body = JSON.stringify({ listen: store.listen, rules: store.rules.map(toWireRule) })
  const resp = await fetch('/_ui/api/save', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
  })
  if (!resp.ok) {
    const e = await resp.json().catch(() => ({ error: 'save failed' }))
    throw new Error(e.error || 'save failed')
  }
  store.dirty = false
}

// wireRuleFor returns a single rule in the server's map shape — used by the Test
// tab (dry-run/preview send the rule under edit to the server engines).
export function wireRuleFor(r) {
  return toWireRule(r)
}

// toast reuses the existing app.js toast bus (a DOM CustomEvent on body).
export function toast(msg, type) {
  document.body.dispatchEvent(new CustomEvent('toast', { detail: { msg, type: type || 'success' } }))
}
