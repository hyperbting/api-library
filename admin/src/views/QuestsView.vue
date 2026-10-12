<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useSession } from '../session'

interface Reward { type: string; key: string; amount: number }
interface StepDef { id: string; checkpoint: boolean; rewards: Reward[] | null }
interface QuestDef { id: string; type: string; initial: boolean; followUp?: string; steps: StepDef[] }
interface Catalog { version: number; quests: QuestDef[] }
interface Progress {
  questId: string
  stepIndex: number
  checkpointIndex: number
  completed: boolean
  completedSteps: string[]
  startedAt: string
  updatedAt: string
  completedAt?: string
  inCatalog: boolean
}
interface UserQuests { uid: string; exists: boolean; catalogVersion: number; updatedAt?: string; quests: Progress[] }

// A row is a catalog quest (with or without progress) or progress whose quest left the catalog.
interface Row { id: string; def?: QuestDef; progress?: Progress }

const COMPLETED = -1

const session = useSession()
const catalog = ref<Catalog | null>(null)
const uid = ref('')
const loaded = ref<UserQuests | null>(null)
const edits = reactive<Record<string, number>>({})
const busy = ref('')
const error = ref('')
const notice = ref('')

class ApiError extends Error {}

async function call<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = init.body ? { 'Content-Type': 'application/json' } : {}
  const res = await session.api(`/admin/quests${path}`, { ...init, headers })
  const text = await res.text()
  let body: { error?: string } | null = null
  try { body = text ? JSON.parse(text) : null } catch { /* not JSON */ }
  if (!res.ok) throw new ApiError(body?.error ?? `The server answered ${res.status}.`)
  return body as T
}

async function run(label: string, fn: () => Promise<void>) {
  busy.value = label
  error.value = ''
  notice.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'Could not reach the server.'
  } finally {
    busy.value = ''
  }
}

onMounted(() => run('catalog', async () => { catalog.value = await call<Catalog>('/catalog') }))

const rows = computed<Row[]>(() => {
  if (!loaded.value) return []
  const byId = new Map(loaded.value.quests.map((p) => [p.questId, p]))
  const out: Row[] = (catalog.value?.quests ?? []).map((def) => ({ id: def.id, def, progress: byId.get(def.id) }))
  for (const p of loaded.value.quests) {
    if (!catalog.value?.quests.some((d) => d.id === p.questId)) out.push({ id: p.questId, progress: p })
  }
  return out
})

const versionBehind = computed(
  () => !!loaded.value?.exists && !!catalog.value && loaded.value.catalogVersion !== catalog.value.version,
)

function options(def: QuestDef) {
  return [
    ...def.steps.map((s, i) => ({ label: `${i + 1}. ${s.id}${s.checkpoint ? ' (checkpoint)' : ''}`, value: i })),
    { label: 'Completed', value: COMPLETED },
  ]
}

function status(r: Row): string {
  const p = r.progress
  if (!p) return 'Not started'
  if (p.completed) return 'Completed'
  const step = r.def?.steps[p.stepIndex]?.id ?? `#${p.stepIndex}`
  return `Step ${p.stepIndex + 1} of ${r.def?.steps.length ?? '?'}: ${step}`
}

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) || d.getFullYear() < 2000 ? '' : d.toLocaleString()
}

function show(data: UserQuests) {
  loaded.value = data
  for (const k of Object.keys(edits)) delete edits[k]
  for (const p of data.quests) edits[p.questId] = p.completed ? COMPLETED : p.stepIndex
  for (const d of catalog.value?.quests ?? []) edits[d.id] ??= 0
}

function load() {
  const target = uid.value.trim()
  if (!target) return Promise.resolve()
  return run('load', async () => {
    show(await call<UserQuests>(`/progress?uid=${encodeURIComponent(target)}`))
  })
}

function useMyUid() {
  uid.value = session.user?.uid ?? ''
  return load()
}

function save(r: Row) {
  if (!loaded.value || !r.def) return
  const target = loaded.value.uid
  const value = edits[r.id] ?? 0
  return run(`save:${r.id}`, async () => {
    await call<Progress>('/progress', {
      method: 'PUT',
      body: JSON.stringify({ uid: target, questId: r.id, stepIndex: value === COMPLETED ? 0 : value, completed: value === COMPLETED }),
    })
    show(await call<UserQuests>(`/progress?uid=${encodeURIComponent(target)}`))
    notice.value = `Saved ${r.id}.`
  })
}

function resetQuest(r: Row) {
  if (!loaded.value) return
  if (!window.confirm(`Reset "${r.id}" for ${loaded.value.uid}? Its progress is deleted.`)) return
  const target = loaded.value.uid
  return run(`reset:${r.id}`, async () => {
    await call('/progress/reset', { method: 'POST', body: JSON.stringify({ uid: target, questId: r.id }) })
    show(await call<UserQuests>(`/progress?uid=${encodeURIComponent(target)}`))
    notice.value = `Reset ${r.id}.`
  })
}

function resetAll() {
  if (!loaded.value) return
  if (!window.confirm(`Reset ALL quest progress for ${loaded.value.uid}? This cannot be undone.`)) return
  const target = loaded.value.uid
  return run('reset-all', async () => {
    await call('/reset', { method: 'POST', body: JSON.stringify({ uid: target }) })
    show(await call<UserQuests>(`/progress?uid=${encodeURIComponent(target)}`))
    notice.value = `Reset all quest progress for ${target}.`
  })
}
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Quests</template>
      <template #subtitle>View, edit and reset a player's quest progress by uid</template>
      <template #content>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <Message v-if="notice" severity="success" :closable="false">{{ notice }}</Message>

        <form class="lookup" @submit.prevent="load">
          <InputText v-model="uid" placeholder="Player uid (Firebase Authentication > Users, or the game's Dev tab)" fluid />
          <Button type="submit" label="Load" :loading="busy === 'load'" :disabled="!uid.trim() || !catalog" />
          <Button type="button" label="My uid" severity="secondary" :disabled="!catalog" @click="useMyUid" />
        </form>

        <template v-if="loaded">
          <p class="meta">
            <strong>{{ loaded.uid }}</strong>
            <span v-if="!loaded.exists"> has no quest document yet (a new account starts the initial quests).</span>
            <span v-else>
              · last written {{ when(loaded.updatedAt) || 'unknown' }} · catalog v{{ loaded.catalogVersion }}
              <span v-if="catalog"> (server: v{{ catalog.version }})</span>
            </span>
          </p>
          <Message v-if="versionBehind" severity="warn" :closable="false">
            This progress was written against catalog v{{ loaded.catalogVersion }}; the server is on v{{ catalog?.version }}.
          </Message>
          <Message severity="secondary" :closable="false">
            Edits write the stored state only: step conditions are not checked and no rewards are granted, so the
            Unlock rewards of a skipped step are never applied in the game. The running game picks the change up the
            next time it reports a step (it resyncs) or reloads its progress.
          </Message>

          <div class="scroll">
            <table class="rows">
              <thead>
                <tr><th>Quest</th><th>Status</th><th>Updated</th><th>Set to</th><th></th></tr>
              </thead>
              <tbody>
                <tr v-for="r in rows" :key="r.id">
                  <td>
                    <div class="qid">{{ r.id }}</div>
                    <div class="sub">
                      <span v-if="r.def">{{ r.def.type }}<span v-if="r.def.initial"> · initial</span><span v-if="r.def.followUp"> · then {{ r.def.followUp }}</span></span>
                      <Tag v-else severity="warn" value="not in the catalog" />
                    </div>
                  </td>
                  <td>
                    <Tag v-if="r.progress?.completed" severity="success" value="Completed" />
                    <span v-else>{{ status(r) }}</span>
                    <div v-if="r.progress" class="sub">checkpoint at {{ r.progress.checkpointIndex + 1 }}</div>
                  </td>
                  <td class="sub">{{ when(r.progress?.updatedAt) }}</td>
                  <td>
                    <Select
                      v-if="r.def"
                      v-model="edits[r.id]"
                      :options="options(r.def)"
                      option-label="label"
                      option-value="value"
                      class="pick"
                    />
                  </td>
                  <td class="actions">
                    <Button v-if="r.def" label="Save" size="small" :loading="busy === `save:${r.id}`" @click="save(r)" />
                    <Button
                      label="Reset"
                      size="small"
                      severity="danger"
                      outlined
                      :disabled="!r.progress"
                      :loading="busy === `reset:${r.id}`"
                      @click="resetQuest(r)"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div class="all">
            <Button
              label="Reset all quest progress"
              icon="pi pi-trash"
              severity="danger"
              :disabled="!loaded.exists"
              :loading="busy === 'reset-all'"
              @click="resetAll"
            />
          </div>
        </template>
      </template>
      <template #footer>
        <router-link :to="{ name: 'home' }">Back</router-link>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 1100px; margin: 0 auto; padding: 16px; }
.lookup { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0; }
.lookup > :first-child { flex: 1 1 320px; }
.meta { margin: 12px 0; word-break: break-all; }
.scroll { overflow-x: auto; margin: 12px 0; }
.rows { width: 100%; border-collapse: collapse; }
.rows th, .rows td { text-align: left; padding: 8px; border-bottom: 1px solid var(--p-content-border-color, #ddd); vertical-align: top; }
.qid { font-weight: 600; }
.sub { font-size: 0.85rem; opacity: 0.75; }
.pick { min-width: 220px; }
.actions { display: flex; gap: 8px; white-space: nowrap; }
.all { margin-top: 16px; }
</style>
