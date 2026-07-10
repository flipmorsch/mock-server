// Editor app: the reactive rule editor (ADR-0009 authoring island). Edits the
// selected rule from the shared store directly (optimistic, no round-trips);
// undo snapshots on focus-in and on structural changes. Test-tab computations
// (dry-run/probe/preview) stay server-side — this only orchestrates + renders.
import { ref, computed, watch, nextTick } from 'vue'
import { store, selected, mutate, snapshot, save, toast, uuid, blankResp, wireRuleFor, SETTINGS } from './store.js'
import { methodClass, statusClass, preview, dimLabel, gotDisplay } from './helpers.js'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

const TPL_SUGGESTIONS = [
  { label: '.Method', sig: '', full: '{{.Method}}', group: 'field' },
  { label: '.Path', sig: '', full: '{{.Path}}', group: 'field' },
  { label: '.Body', sig: '', full: '{{.Body}}', group: 'field' },
  { label: '.Header', sig: '(name string)', full: '{{.Header "name"}}', group: 'field' },
  { label: '.Query', sig: '(name string)', full: '{{.Query "name"}}', group: 'field' },
  { label: '.Param', sig: '(name string)', full: '{{.Param "name"}}', group: 'field' },
  { label: 'now', sig: '', full: '{{now}}', group: 'func' },
  { label: 'nowFormat', sig: '(layout string)', full: '{{nowFormat "layout"}}', group: 'func' },
  { label: 'randomInt', sig: '(min, max int)', full: '{{randomInt 0 100}}', group: 'func' },
  { label: 'randomString', sig: '(n int)', full: '{{randomString 8}}', group: 'func' },
  { label: 'counter', sig: '', full: '{{counter}}', group: 'func' },
  { label: 'requestCount', sig: '(method?, path?)', full: '{{requestCount "GET" "/users"}}', group: 'func' },
]

const TemplateAutocomplete = {
  props: {
    modelValue: String,
    rows: { type: [String, Number], default: '7' },
    placeholder: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    const ta = ref(null)
    const visible = ref(false)
    const items = ref([])
    const selIdx = ref(0)
    const dismissed = ref(false)
    const lastStart = ref(-1)

    function parseCtx() {
      const el = ta.value
      if (!el) return null
      const pos = el.selectionStart
      const text = el.value
      let i = pos - 1
      while (i >= 0) {
        if (text[i] === '{' && text[i + 1] === '{') {
          const after = text.slice(i + 2, pos)
          if (after.includes('}}')) return null
          return { start: i, filter: after, dotted: after.startsWith('.') }
        }
        if (text[i] === '}' && text[i + 1] === '}') return null
        i--
      }
      return null
    }
    function update() {
      const ctx = parseCtx()
      if (!ctx) { visible.value = false; dismissed.value = false; lastStart.value = -1; return }
      if (ctx.start !== lastStart.value) { dismissed.value = false; lastStart.value = ctx.start }
      if (dismissed.value) return
      let cs = TPL_SUGGESTIONS
      if (ctx.dotted) cs = cs.filter(s => s.group === 'field')
      const pf = ctx.filter.toLowerCase()
      if (pf) cs = cs.filter(s => s.label.toLowerCase().startsWith(pf))
      const changed = cs.length !== items.value.length || cs.some((s, i) => s.label !== items.value[i]?.label)
      items.value = cs
      visible.value = cs.length > 0
      if (changed) selIdx.value = 0
      else if (selIdx.value >= cs.length) selIdx.value = Math.max(0, cs.length - 1)
    }

    function select(item) {
      const el = ta.value
      const ctx = parseCtx()
      if (!ctx) return
      const before = el.value.slice(0, ctx.start)
      const after = el.value.slice(el.selectionStart)
      emit('update:modelValue', before + item.full + after)
      visible.value = false
      nextTick(() => {
        const m = item.full.match(/"([^"]+)"/)
        if (m) {
          const s = before.length + item.full.indexOf(m[0]) + 1
          el.focus()
          el.setSelectionRange(s, s + m[1].length)
        } else {
          const e = before.length + item.full.length
          el.focus()
          el.setSelectionRange(e, e)
        }
      })
    }

    function onInput(e) { emit('update:modelValue', e.target.value) }

    function onKeydown(e) {
      if (!visible.value) return
      if (e.key === 'ArrowDown') { e.preventDefault(); selIdx.value = Math.min(selIdx.value + 1, items.value.length - 1); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); selIdx.value = Math.max(selIdx.value - 1, 0); return }
      if (e.key === 'Enter' || e.key === 'Tab') {
        if (items.value.length > 0) { e.preventDefault(); select(items.value[selIdx.value]); return }
      }
      if (e.key === 'Escape') { e.preventDefault(); visible.value = false; dismissed.value = true; return }
    }

    watch(() => props.modelValue, () => update())

    return { ta, visible, items, selIdx, select, onInput, onKeydown, update }
  },
  template: `
<div class="tpl-ac">
  <textarea ref="ta" :value="modelValue" @input="onInput" @keydown="onKeydown" @click="update" @keyup="update"
            :rows="rows" :placeholder="placeholder" autocomplete="off"></textarea>
  <div v-show="visible" class="tpl-ac-dd">
    <div v-for="(item, idx) in items" :key="item.label"
         class="tpl-ac-item" :class="{'tpl-ac-sel': idx === selIdx}"
         @mousedown.prevent="select(item)" @mouseenter="selIdx = idx">
      <span class="tpl-ac-label">{{ item.label }}</span>
      <span v-if="item.sig" class="tpl-ac-sig">{{ item.sig }}</span>
    </div>
  </div>
</div>`,
}

const KvEditor = {
  props: ['pairs'],
  template: `
<div class="kv">
  <div v-for="(item, idx) in pairs" :key="idx" class="kv-row">
    <input type="text" v-model="item.k" placeholder="key" autocomplete="off" aria-label="key">
    <input type="text" v-model="item.v" placeholder="value" autocomplete="off" aria-label="value">
    <button type="button" class="btn btn-ghost kv-del" @click="pairs.splice(idx, 1)">✕</button>
  </div>
  <button type="button" class="btn btn-ghost kv-add" @click="pairs.push({k:'', v:''})">+ add</button>
</div>`,
}

const SequenceEditor = {
  props: ['rule'],
  components: { KvEditor, TemplateAutocomplete },
  setup(props) {
    const moveUp = (i) => {
      if (i > 0) mutate(() => { const a = props.rule.responses;[a[i - 1], a[i]] = [a[i], a[i - 1]] })
    }
    const moveDown = (i) => {
      const a = props.rule.responses
      if (i < a.length - 1) mutate(() => { [a[i + 1], a[i]] = [a[i], a[i + 1]] })
    }
    const remove = (i) => mutate(() => {
      const r = props.rule
      r.responses.splice(i, 1)
      if (r.responses.length === 1) { r.response = r.responses[0]; r.responses = [] } // collapse back to single
    })
    const add = () => mutate(() => props.rule.responses.push(blankResp()))
    return { moveUp, moveDown, remove, add, statusClass, preview }
  },
  template: `
<div class="form-section">
  <div class="banner-info">
    sequenced — the Nth matching request gets the Nth response, last sticks. Order matters; editing here does not rewind a running sequence (use Reset).
  </div>
  <details v-for="(r, i) in rule.responses" :key="i" class="seq-item" open>
    <summary class="seq-summary">
      <span class="seq-idx">{{ i + 1 }}</span>
      <span :class="statusClass(r.status)">{{ r.status }}</span>
      <span v-if="r.delay" class="dim">{{ r.delay }}</span>
      <span v-if="r.template" class="dim">tpl</span>
      <code class="seq-body">{{ preview(r.body) }}</code>
      <span class="seq-controls">
        <button type="button" class="btn btn-ghost" @click.prevent="moveUp(i)" :disabled="i === 0" title="move up" aria-label="move response up">↑</button>
        <button type="button" class="btn btn-ghost" @click.prevent="moveDown(i)" :disabled="i === rule.responses.length - 1" title="move down" aria-label="move response down">↓</button>
        <button type="button" class="btn btn-ghost" @click.prevent="remove(i)" title="remove" aria-label="remove response">✕</button>
      </span>
    </summary>
    <div class="seq-fields">
      <div class="form-row">
        <label class="field"><span class="field-label">status</span><input type="text" size="4" autocomplete="off" v-model="r.status"></label>
        <label class="field"><span class="field-label">delay</span><input type="text" size="8" autocomplete="off" placeholder="500ms" v-model="r.delay"></label>
        <label class="field field-check"><input type="checkbox" v-model="r.template"><span>template</span></label>
      </div>
      <div class="form-section-title sub">response headers</div>
      <kv-editor :pairs="r.headers"></kv-editor>
      <label class="field"><span class="field-label">body</span><template-autocomplete v-model="r.body" rows="6" placeholder='{"ok": true}'></template-autocomplete></label>
      <label class="field"><span class="field-label">body file</span><input type="text" v-model="r.body_file" placeholder="./fixtures/resp.json" autocomplete="off"></label>
    </div>
  </details>
  <button type="button" class="btn btn-ghost seq-add" @click.prevent="add()">+ add response</button>
</div>`,
}

const TestPanel = {
  props: ['rule'],
  setup(props) {
    const probe = ref({ method: props.rule.request.method || 'GET', path: props.rule.request.path || '/', headerText: '', body: '' })
    const result = ref(null) // {kind:'dry'|'probe'|'preview'|'error', ...}

    async function post(url, payload) {
      try {
        const resp = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
        const data = await resp.json()
        if (!resp.ok) { result.value = { kind: 'error', msg: data.error || 'request failed' }; return null }
        return data
      } catch (e) {
        result.value = { kind: 'error', msg: e.message }
        return null
      }
    }
    async function dryRun() {
      const v = await post('/_ui/api/test-dry', { rule: wireRuleFor(props.rule), probe: probe.value })
      if (v) result.value = { kind: 'dry', verdict: v }
    }
    async function doProbe() {
      const v = await post('/_ui/api/test-probe', probe.value)
      if (v) result.value = { kind: 'probe', ...v }
    }
    async function doPreview() {
      const v = await post('/_ui/api/template-preview', {
        tpl_body: props.rule.response.body,
        rule_path_mode: props.rule.request.path_mode,
        rule_path: props.rule.request.path,
        probe: probe.value,
      })
      if (v) result.value = { kind: 'preview', output: v.output }
    }
    const tpl = computed(() => props.rule.responses.length === 0 && props.rule.response.template)
    return { METHODS, probe, result, dryRun, doProbe, doPreview, tpl, statusClass, dimLabel, gotDisplay }
  },
  template: `
<div class="form-section test-panel">
  <div class="form-section-title">test request</div>
  <div class="form-row">
    <label class="field"><span class="field-label">method</span>
      <select v-model="probe.method"><option v-for="m in METHODS" :value="m">{{ m }}</option></select>
    </label>
    <label class="field field-grow"><span class="field-label">path</span><input type="text" v-model="probe.path" autocomplete="off"></label>
  </div>
  <div class="form-row">
    <label class="field field-grow"><span class="field-label">headers <span class="dim">(Key: Value, one per line)</span></span>
      <textarea rows="2" v-model="probe.headerText" placeholder="Content-Type: application/json"></textarea></label>
    <label class="field field-grow"><span class="field-label">body</span><textarea rows="2" v-model="probe.body"></textarea></label>
  </div>
  <div class="form-row test-buttons">
    <button type="button" class="btn" @click="dryRun" title="evaluate this rule's criteria against the test request — nothing is sent">dry run</button>
    <button type="button" class="btn" @click="doProbe" title="send a real request to the running server (saved rules)">probe</button>
    <button v-if="tpl" type="button" class="btn" @click="doPreview" title="render the response body template against the test request">preview template</button>
  </div>
  <div class="test-result-slot">
    <div v-if="result" class="test-result">
      <template v-if="result.kind === 'dry'">
        <div v-if="result.verdict.matched" class="test-verdict ok">✓ MATCH — this rule matches the test request</div>
        <div v-else class="test-verdict fail">✖ NO MATCH</div>
        <div v-for="v in result.verdict.verdicts" :class="['vline', v.ok ? 'vok' : 'vfail']">
          <template v-if="v.ok">✓ {{ dimLabel(v.dim) }} <span class="vwant">{{ v.want }}</span></template>
          <template v-else>✖ {{ dimLabel(v.dim) }} <span class="vdiff">expected <b>{{ v.want }}</b> · got <b>{{ gotDisplay(v.got) }}</b></span></template>
        </div>
      </template>
      <template v-else-if="result.kind === 'probe'">
        <div class="test-verdict">response <span :class="statusClass(result.status)">{{ result.status }}</span></div>
        <details v-if="result.headers && Object.keys(result.headers).length">
          <summary class="dim">headers ({{ Object.keys(result.headers).length }})</summary>
          <div class="kv-table">
            <div v-for="(v, k) in result.headers" class="kv-line"><span class="kv-k">{{ k }}</span><span class="kv-v">{{ v }}</span></div>
          </div>
        </details>
        <pre v-if="result.body" class="json-body">{{ result.body }}</pre>
      </template>
      <template v-else-if="result.kind === 'preview'">
        <div class="test-verdict ok">template output</div>
        <pre class="json-body">{{ result.output }}</pre>
      </template>
      <div v-else class="test-verdict err">{{ result.msg }}</div>
    </div>
  </div>
</div>`,
}

export const Editor = {
  components: { KvEditor, SequenceEditor, TestPanel, TemplateAutocomplete },
  setup() {
    const activeTab = ref('request')
    const sel = computed(selected)
    const isSettings = computed(() => store.selectedId === SETTINGS)
    const isSeq = computed(() => sel.value && sel.value.responses.length > 0)

    function tabFor(hl) {
      return hl && (hl.startsWith('status') || hl.startsWith('delay')) ? 'response' : 'request'
    }
    function flashSection(section) {
      const hl = store.highlight
      if (!hl) return false
      if (hl.startsWith('header:')) return section === 'headers'
      if (hl.startsWith('query:')) return section === 'query'
      return hl === section // method, path, body
    }
    // Toggle body.editing (app.css hides the editor pane until it is set) and
    // reset the tab / honour a jump-to-rule highlight whenever the selection changes.
    watch(() => store.selectedId, (id) => {
      document.body.classList.toggle('editing', id !== null)
      activeTab.value = tabFor(store.highlight)
      if (store.highlight) setTimeout(() => { store.highlight = '' }, 1600) // let the flash play once
    }, { immediate: true })

    function close() { store.selectedId = null }
    function del() {
      const id = store.selectedId
      mutate(() => { store.rules = store.rules.filter((r) => r.id !== id) })
      store.selectedId = null
    }
    function duplicate() {
      const src = sel.value
      const copy = JSON.parse(JSON.stringify(src))
      copy.id = uuid()
      mutate(() => {
        const i = store.rules.findIndex((r) => r.id === src.id)
        store.rules.splice(i + 1, 0, copy)
      })
      store.selectedId = copy.id
    }
    function addResponse() {
      const r = sel.value
      mutate(() => {
        r.responses = [{ ...r.response, headers: [...r.response.headers] }, blankResp()]
        r.response = blankResp() // clear singular: response/responses are mutually exclusive
      })
    }
    async function doSave() {
      try { await save(); toast('Saved to disk') } catch (e) { toast(e.message, 'error') }
    }
    const markDirty = () => { store.dirty = true }
    // Kept as a JS string to avoid Vue interpolation conflicts with {{ }} in the template.
    const tplHint = 'Type {{ for autocomplete.'

    return {
      store, sel, isSettings, isSeq, activeTab, METHODS, flashSection, tplHint,
      close, del, duplicate, addResponse, doSave, markDirty, snapshot,
    }
  },
  template: `
<div v-if="isSettings" class="editor" id="editor">
  <div class="editor-head">
    <span class="editor-title">settings</span>
    <button class="btn btn-ghost editor-close" @click="close" title="close (Esc)">✕</button>
  </div>
  <div class="editor-form" @input="markDirty" @focusin="snapshot">
    <div class="form-section">
      <label class="field"><span class="field-label">listen address</span>
        <input type="text" v-model="store.listen" placeholder="127.0.0.1:8080" autocomplete="off"></label>
      <p class="dim">written to the YAML on save; the running server keeps its current address until restarted</p>
    </div>
  </div>
</div>

<div v-else-if="!sel" class="editor-blank">
  <p class="dim">select a rule, or press <kbd>^N</kbd> for a new one</p>
</div>

<div v-else class="editor" id="editor" :key="sel.id">
  <div class="editor-head">
    <span class="editor-title">{{ sel.name || '(unnamed)' }}</span>
    <button class="btn btn-ghost editor-close" @click="close" title="close (Esc)">✕</button>
  </div>
  <div class="tabs">
    <button type="button" class="tab-btn" :class="{'tab-active': activeTab === 'request'}" @click="activeTab = 'request'">Request</button>
    <button type="button" class="tab-btn" :class="{'tab-active': activeTab === 'response'}" @click="activeTab = 'response'">Response</button>
    <button type="button" class="tab-btn" :class="{'tab-active': activeTab === 'test'}" @click="activeTab = 'test'">Test</button>
  </div>
  <div class="editor-form" @input="markDirty" @change="markDirty" @focusin="snapshot">
    <div v-show="activeTab === 'request'" class="tab-content">
      <div class="form-section">
        <label class="field"><span class="field-label">name</span>
          <input type="text" v-model="sel.name" placeholder="e.g. get users" autocomplete="off"></label>
      </div>
      <div class="form-section" :class="{flash: flashSection('method') || flashSection('path')}">
        <div class="form-section-title">request match</div>
        <div class="form-row">
          <label class="field field-method"><span class="field-label">method</span>
            <select v-model="sel.request.method"><option v-for="m in METHODS" :value="m">{{ m }}</option></select></label>
          <label class="field field-grow"><span class="field-label">path</span>
            <input type="text" v-model="sel.request.path" placeholder="/api/users" autocomplete="off"></label>
          <label class="field"><span class="field-label">mode</span>
            <select v-model="sel.request.path_mode">
              <option v-for="m in ['exact','prefix','regex','pattern']" :value="m">{{ m }}</option>
            </select></label>
        </div>
      </div>
      <div class="form-section" :class="{flash: flashSection('headers')}">
        <div class="form-section-title">match headers</div>
        <kv-editor :pairs="sel.request.headers"></kv-editor>
      </div>
      <div class="form-section" :class="{flash: flashSection('query')}">
        <div class="form-section-title">match query params</div>
        <kv-editor :pairs="sel.request.query"></kv-editor>
      </div>
      <div class="form-section" :class="{flash: flashSection('body')}">
        <div class="form-section-title">match body</div>
        <div class="form-row">
          <label class="field"><span class="field-label">mode</span>
            <select v-model="sel.request.bodyMode">
              <option value="none">none</option><option value="exact">exact</option>
              <option value="contains">contains</option><option value="json">json</option>
            </select></label>
          <label v-show="sel.request.bodyMode !== 'none'" class="field field-grow"><span class="field-label">value</span>
            <textarea v-model="sel.request.bodyValue" rows="2"></textarea></label>
        </div>
      </div>
    </div>

    <div v-show="activeTab === 'response'" class="tab-content">
      <div v-if="!isSeq" class="form-section form-section-resp">
        <div class="form-row">
          <label class="field"><span class="field-label">status</span>
            <input type="text" v-model="sel.response.status" size="4" autocomplete="off"></label>
          <label class="field"><span class="field-label">delay</span>
            <input type="text" v-model="sel.response.delay" placeholder="500ms" size="8" autocomplete="off"></label>
          <label class="field field-check"><input type="checkbox" v-model="sel.response.template"><span>template</span></label>
        </div>
        <div class="form-section-title sub">response headers</div>
        <kv-editor :pairs="sel.response.headers"></kv-editor>
        <label class="field"><span class="field-label">body
            <span v-if="sel.response.body_file" class="dim">(file: {{ sel.response.body_file }})</span></span>
          <template-autocomplete v-model="sel.response.body" rows="7" placeholder='{"ok": true}'></template-autocomplete></label>
        <label class="field"><span class="field-label">body file</span>
          <input type="text" v-model="sel.response.body_file" placeholder="./fixtures/resp.json" autocomplete="off"></label>
        <div v-show="sel.response.template" class="tpl-hint">{{ tplHint }}</div>
        <div class="seq-convert">
          <button type="button" class="btn btn-ghost seq-add" @click.prevent="addResponse">+ add response</button>
          <span class="dim">a second response makes this rule sequenced — Nth request gets the Nth, last sticks</span>
        </div>
      </div>
      <sequence-editor v-else :rule="sel"></sequence-editor>
    </div>

    <div v-show="activeTab === 'test'" class="tab-content">
      <test-panel :rule="sel" :key="sel.id"></test-panel>
    </div>

    <div class="editor-actions">
      <button type="button" class="btn btn-accent" @click="doSave">save to disk</button>
      <button type="button" class="btn" @click="duplicate" title="create a copy of this rule">duplicate</button>
      <button type="button" class="btn btn-danger" @click="del">delete</button>
      <span class="editor-note">edits are in the working copy — <b>save</b> writes to disk</span>
    </div>
  </div>
</div>`,
}
