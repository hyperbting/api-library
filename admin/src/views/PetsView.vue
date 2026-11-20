<script setup lang="ts">
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'
import { useSession } from '../session'

interface Source { grantId: string; source: string; sourceRef: string; createdAt: string; claimed: boolean }
interface OwnedPet { pet: string; starter: boolean; sources: Source[] }
interface Reward { type: string; key?: string; amount: number }
interface ArchivedGrant { id: string; source: string; sourceRef: string; rewards: Reward[]; createdAt: string }
interface LogEntry {
  requestId: string
  action: 'add' | 'remove' | 'reset'
  pets: string[]
  by: string
  note?: string
  at: string
  grantIds?: string[]
  archived?: ArchivedGrant[]
}
interface Overview { uid: string; owned: OwnedPet[]; known: string[]; log: LogEntry[]; hasRecords: boolean }
interface ChangeResponse { ok: boolean; errorCode?: string; repeated?: boolean; entry?: LogEntry; owned: string[] }

const ERRORS: Record<string, string> = {
  no_pets: 'Pick at least one pet.',
  unknown_pet: 'The server does not know one of those pets.',
  starter_pet: 'A starter pet cannot be removed.',
  nothing_to_do: 'Nothing changed: the pets are owned already, or none of them is owned.',
  too_many_grants: "The player's grant ledger is full.",
}

const session = useSession()
const uid = ref('')
const overview = ref<Overview | null>(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const toAdd = ref<string[]>([])
const note = ref('')
const openLog = ref<string | null>(null)

const notOwned = computed(() => {
  if (!overview.value) return []
  const owned = new Set(overview.value.owned.map((p) => p.pet))
  return overview.value.known.filter((p) => !owned.has(p)).map((p) => ({ label: petName(p), value: p }))
})
const removable = computed(() => overview.value?.owned.filter((p) => !p.starter).length ?? 0)

async function call<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = init.body ? { 'Content-Type': 'application/json' } : {}
  const res = await session.api(path, { ...init, headers })
  const text = await res.text()
  let body: { error?: string } | null = null
  try { body = text ? JSON.parse(text) : null } catch { /* not JSON */ }
  if (!res.ok) throw new Error(body?.error ?? `The server answered ${res.status}.`)
  return body as T
}

function fail(e: unknown) {
  error.value = e instanceof Error ? e.message : 'Could not reach the server.'
}

async function loadPlayer(target = uid.value) {
  const id = target.trim()
  if (!id) return
  uid.value = id
  loading.value = true
  error.value = ''
  try {
    overview.value = await call<Overview>(`/admin/pets/user?uid=${encodeURIComponent(id)}`)
    toAdd.value = []
  } catch (e) {
    overview.value = null
    fail(e)
  } finally {
    loading.value = false
  }
}

async function change(action: 'add' | 'remove' | 'reset', pets: string[], question: string) {
  if (!overview.value || !window.confirm(question)) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await call<ChangeResponse>('/admin/pets/change', {
      method: 'POST',
      // One id per submit: a double click is answered, not applied twice.
      body: JSON.stringify({ uid: overview.value.uid, requestId: crypto.randomUUID(), action, pets, note: note.value }),
    })
    if (!res.ok) {
      error.value = ERRORS[res.errorCode ?? ''] ?? `Refused: ${res.errorCode}`
    } else {
      const names = (res.entry?.pets ?? []).map(petName).join(', ')
      notice.value = `${action === 'add' ? 'Added' : 'Removed'}: ${names}. The player's game updates its list at its next start.`
      note.value = ''
    }
    await loadPlayer(overview.value.uid)
  } catch (e) {
    fail(e)
  } finally {
    busy.value = false
  }
}

function add() {
  if (!toAdd.value.length) { error.value = ERRORS.no_pets; return }
  change('add', toAdd.value, `Give ${toAdd.value.map(petName).join(', ')} to ${overview.value?.uid}?`)
}
function remove(p: OwnedPet) {
  change('remove', [p.pet], `Remove ${petName(p.pet)} from ${overview.value?.uid}? Its grants are archived in the history.`)
}
function reset() {
  change('reset', [], `Remove every pet but the starters from ${overview.value?.uid}? Their grants are archived in the history.`)
}

// "Pet_GermanShepherd" -> "GermanShepherd": the asset id without its prefix.
function petName(p: string): string {
  return p.startsWith('Pet_') ? p.slice(4) : p
}

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

const sourceSeverity: Record<string, 'info' | 'success' | 'warn' | 'secondary'> = {
  quest: 'info',
  gacha: 'success',
  admin: 'warn',
  redeem: 'info',
  iap: 'success',
}
const actionSeverity: Record<string, 'success' | 'warn' | 'danger'> = { add: 'success', remove: 'warn', reset: 'danger' }
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Pets</template>
      <template #subtitle>
        A player's owned pets and the grants they came from. Add, remove or reset them; every change is written to the
        player's history with your uid, and removed grants are kept there.
      </template>
      <template #content>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <Message v-if="notice" severity="success" :closable="false">{{ notice }}</Message>

        <div class="bar">
          <InputText v-model="uid" placeholder="Player uid (Menu > Player ID in the game)" class="uid" @keyup.enter="loadPlayer()" />
          <Button label="Look up" icon="pi pi-search" :loading="loading" :disabled="!uid.trim()" @click="loadPlayer()" />
        </div>

        <template v-if="overview">
          <Message v-if="!overview.hasRecords" severity="warn" :closable="false">
            This uid has no grant records. Check that it is spelled right before changing anything.
          </Message>

          <h3>Owned ({{ overview.owned.length }})</h3>
          <div class="scroll">
            <table class="rows">
              <thead><tr><th>Pet</th><th>From</th><th></th></tr></thead>
              <tbody>
                <tr v-for="p in overview.owned" :key="p.pet">
                  <td>{{ petName(p.pet) }}</td>
                  <td>
                    <Tag v-if="p.starter" severity="secondary" value="starter" />
                    <div v-for="s in p.sources" :key="s.grantId" class="src">
                      <Tag :severity="sourceSeverity[s.source] ?? 'secondary'" :value="s.source" />
                      <Tag v-if="!s.claimed" class="gap" severity="warn" value="waiting" />
                      <span class="sub gap">{{ when(s.createdAt) }} · {{ s.grantId }}</span>
                    </div>
                  </td>
                  <td>
                    <Button v-if="!p.starter" label="Remove" size="small" severity="danger" text :disabled="busy" @click="remove(p)" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <h3>Change</h3>
          <div class="form">
            <label class="wide">Add pets
              <MultiSelect v-model="toAdd" :options="notOwned" option-label="label" option-value="value" filter
                           placeholder="Pick pets the player does not own" display="chip" fluid />
            </label>
            <label class="wide">Note (shown in the history) <InputText v-model="note" maxlength="120" placeholder="why" fluid /></label>
            <Button label="Add" icon="pi pi-plus" :loading="busy" :disabled="!toAdd.length" @click="add" />
            <Button label="Reset to starters" icon="pi pi-refresh" severity="danger" :loading="busy" :disabled="!removable" @click="reset" />
          </div>

          <h3>History</h3>
          <p v-if="!overview.log.length" class="empty">No changes yet.</p>
          <div v-else class="scroll">
            <table class="rows">
              <thead><tr><th>When</th><th>Action</th><th>Pets</th><th>By</th><th>Note</th><th></th></tr></thead>
              <tbody>
                <template v-for="e in overview.log" :key="e.requestId">
                  <tr>
                    <td class="sub">{{ when(e.at) }}</td>
                    <td><Tag :severity="actionSeverity[e.action]" :value="e.action" /></td>
                    <td>{{ e.pets.map(petName).join(', ') }}</td>
                    <td class="sub">{{ e.by }}</td>
                    <td>{{ e.note }}</td>
                    <td>
                      <Button v-if="e.archived?.length" size="small" severity="secondary" text
                              :label="openLog === e.requestId ? 'Hide' : `${e.archived.length} archived`"
                              @click="openLog = openLog === e.requestId ? null : e.requestId" />
                    </td>
                  </tr>
                  <tr v-if="openLog === e.requestId">
                    <td colspan="6">
                      <div v-for="g in e.archived" :key="g.id" class="sub">
                        {{ when(g.createdAt) }} · {{ g.source }} · {{ g.id }} ·
                        {{ g.rewards.map((r) => (r.key ? petName(r.key) : r.type)).join(', ') }}
                      </div>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
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
.bar { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0; }
.uid { flex: 1 1 320px; }
.empty { opacity: 0.7; }
.form { display: flex; flex-wrap: wrap; gap: 12px; align-items: end; }
.form > label { display: grid; gap: 4px; flex: 1 1 200px; }
.form > .wide { flex: 2 1 320px; }
.scroll { overflow-x: auto; }
.rows { width: 100%; border-collapse: collapse; }
.rows th, .rows td { text-align: left; padding: 8px; border-bottom: 1px solid var(--p-content-border-color, #ddd); vertical-align: top; }
.src { margin: 2px 0; }
.sub { font-size: 0.85rem; opacity: 0.75; }
.gap { margin-left: 4px; }
h3 { margin: 20px 0 4px; }
</style>
