// Authoring island entry (ADR-0009): mounts the rail + editor Vue apps over the
// shared store, and bridges the observation surface (journal/palette/keyboard) in
// through DOM CustomEvents — the seam. No new transport: the same document.body
// event bus app.js already uses.
import { createApp } from 'vue'
import { store, load, save, undo, mutate, newRule, toEditRule, toast, SETTINGS } from './store.js'
import { Rail } from './rail.js'
import { Editor } from './editor.js'

async function boot() {
  await load()
  createApp(Rail).mount('#rail-root')
  createApp(Editor).mount('#editor-root')
}

const on = (name, fn) => document.body.addEventListener(name, fn)

on('mock:new-rule', () => {
  const r = newRule()
  mutate(() => store.rules.push(r))
  store.highlight = ''
  store.selectedId = r.id
})
on('mock:edit-rule', (e) => {
  store.highlight = (e.detail && e.detail.field) || ''
  store.selectedId = e.detail.id
})
on('mock:settings', () => { store.selectedId = SETTINGS })
on('mock:close-editor', () => { store.selectedId = null })
on('mock:save', async () => {
  try { await save(); toast('Saved to disk') } catch (err) { toast(err.message, 'error') }
})
// rule-from-request: the server still builds the pre-filled rule (ruleFromEntry);
// we fetch it and seed the client working copy (ADR-0010).
on('mock:seed-from', async (e) => {
  try {
    const wire = await (await fetch('/_ui/api/rule-from-entry?seq=' + e.detail.seq)).json()
    const r = toEditRule(wire)
    mutate(() => store.rules.push(r))
    store.highlight = ''
    store.selectedId = r.id
  } catch (err) {
    toast('could not seed rule: ' + err.message, 'error')
  }
})

window.addEventListener('keydown', (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
    const palette = document.getElementById('palette')
    if (palette && !palette.hidden) return
    if (e.target.closest('#editor-root, #rail-root')) { e.preventDefault(); undo() }
  }
})

window.addEventListener('beforeunload', (e) => {
  if (store.dirty) { e.preventDefault(); e.returnValue = '' }
})

boot()
