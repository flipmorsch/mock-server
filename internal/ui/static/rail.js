// Rail app: the reactive rule list (ADR-0009 authoring island). Renders the same
// markup/classes as the old templ Rail so app.css and the command palette's
// .rail-item DOM scraping keep working.
import { ref } from 'vue'
import { store, mutate, save, toast, newRule, SETTINGS } from './store.js'
import { methodClass } from './helpers.js'

export const Rail = {
  setup() {
    const dragIndex = ref(-1)
    const overIndex = ref(-1)

    function select(id) {
      store.selectedId = id
      store.highlight = ''
    }
    function addRule() {
      const r = newRule()
      mutate(() => store.rules.push(r))
      select(r.id)
    }
    function openSettings() {
      store.selectedId = SETTINGS
    }
    async function doSave() {
      try {
        await save()
        toast('Saved to disk')
      } catch (e) {
        toast(e.message, 'error')
      }
    }
    function onDrop(i) {
      const from = dragIndex.value
      if (from < 0 || from === i) return
      mutate(() => {
        const [moved] = store.rules.splice(from, 1)
        store.rules.splice(i, 0, moved)
      })
    }

    return {
      store, methodClass, select, addRule, openSettings, doSave, onDrop, dragIndex, overIndex,
    }
  },
  // Mounted into the <aside class="rail" id="rail-root"> host, so this renders the
  // rail's inner content (a fragment), keeping .rail as the grid child.
  template: `
  <div class="rail-head">
    <span class="brand">mock<span class="brand-slash">/</span>server</span>
    <button class="btn btn-save" title="Write working copy to YAML (Ctrl+S)" @click="doSave">
      save<span v-if="store.dirty" class="unsaved-dot" title="unsaved changes"></span>
    </button>
  </div>
  <button class="btn btn-new" @click="addRule">+ new rule <kbd>^N</kbd></button>
  <div class="rail-list" id="rail-list">
    <div v-if="!store.rules.length" class="rail-empty">no rules yet<br>press <kbd>^N</kbd> to create one</div>
    <div v-for="(r, i) in store.rules" :key="r.id"
         class="rail-item" draggable="true"
         :class="{active: r.id === store.selectedId, dragging: i === dragIndex, 'drag-over': i === overIndex}"
         :data-id="r.id" :data-name="r.name" :data-method="r.request.method" :data-path="r.request.path"
         @click="select(r.id)"
         @dragstart="dragIndex = i"
         @dragover.prevent="overIndex = i"
         @drop.prevent="onDrop(i)"
         @dragend="dragIndex = -1; overIndex = -1">
      <span :class="methodClass(r.request.method)">{{ (r.request.method || '').toUpperCase() }}</span>
      <span class="rail-item-text">
        <span class="rail-item-name">{{ r.name || '(unnamed)' }}</span>
        <span class="rail-item-path">{{ r.request.path }}</span>
      </span>
      <span v-if="r.responses && r.responses.length" class="rail-seq" title="sequenced responses">▶{{ r.responses.length }}</span>
    </div>
  </div>
  <div class="rail-foot">
    <span class="rail-count">{{ store.rules.length }} rules</span>
    <button class="btn btn-ghost" @click="openSettings">settings</button>
    <span class="rail-hint"><kbd>⌘K</kbd> palette</span>
  </div>`,
}
