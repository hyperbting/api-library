<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useSession } from '../session'

type Audience = 'public' | 'dev'
interface NewsItem {
  id: string
  audience: Audience
  title: Record<string, string>
  body: Record<string, string>
  version?: string
  pinned: boolean
  publishedAt: string
  expiresAt?: string
}

// The languages the game has; English is required.
const LOCALES = [
  { code: 'en', label: 'English (required)' },
  { code: 'zh-Hant', label: '繁體中文' },
  { code: 'ja', label: '日本語' },
]
const AUDIENCES = [
  { label: 'Public: every player', value: 'public' },
  { label: 'Dev: admin accounts only', value: 'dev' },
]

const session = useSession()
const items = ref<NewsItem[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const filter = ref<'all' | Audience>('all')

const dialog = ref(false)
const form = reactive({
  id: '',
  audience: 'public' as Audience,
  version: '',
  pinned: false,
  expires: '', // value of <input type="datetime-local">, local time
  title: { en: '', 'zh-Hant': '', ja: '' } as Record<string, string>,
  body: { en: '', 'zh-Hant': '', ja: '' } as Record<string, string>,
})

const shown = computed(() => items.value.filter((i) => filter.value === 'all' || i.audience === filter.value))

async function call<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = init.body ? { 'Content-Type': 'application/json' } : {}
  const res = await session.api(path, { ...init, headers })
  const text = await res.text()
  let body: { error?: string } | null = null
  try { body = text ? JSON.parse(text) : null } catch { /* not JSON */ }
  if (!res.ok) throw new Error(body?.error ?? `The server answered ${res.status}.`)
  return body as T
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = (await call<{ items: NewsItem[] }>('/admin/news')).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not reach the server.'
  } finally {
    loading.value = false
  }
}

onMounted(load)

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

function isExpired(i: NewsItem): boolean {
  return !!i.expiresAt && new Date(i.expiresAt).getTime() <= Date.now()
}

// <input type="datetime-local"> wants "YYYY-MM-DDTHH:mm" in local time.
function toLocalInput(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function openNew() {
  Object.assign(form, { id: '', audience: 'public', version: '', pinned: false, expires: '' })
  for (const l of LOCALES) { form.title[l.code] = ''; form.body[l.code] = '' }
  error.value = ''
  dialog.value = true
}

function openEdit(i: NewsItem) {
  Object.assign(form, { id: i.id, audience: i.audience, version: i.version ?? '', pinned: i.pinned, expires: toLocalInput(i.expiresAt) })
  for (const l of LOCALES) { form.title[l.code] = i.title[l.code] ?? ''; form.body[l.code] = i.body[l.code] ?? '' }
  error.value = ''
  dialog.value = true
}

async function save() {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload = {
      id: form.id,
      audience: form.audience,
      version: form.version,
      pinned: form.pinned,
      expiresAt: form.expires ? new Date(form.expires).toISOString() : null,
      title: { ...form.title },
      body: { ...form.body },
    }
    await call<NewsItem>('/admin/news', { method: 'POST', body: JSON.stringify(payload) })
    dialog.value = false
    notice.value = form.id ? 'Saved.' : 'Published.'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not reach the server.'
  } finally {
    saving.value = false
  }
}

async function remove(i: NewsItem) {
  if (!window.confirm(`Delete "${i.title.en}"? This cannot be undone.`)) return
  error.value = ''
  notice.value = ''
  try {
    await call('/admin/news/delete', { method: 'POST', body: JSON.stringify({ id: i.id }) })
    notice.value = 'Deleted.'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not reach the server.'
  }
}
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>News</template>
      <template #subtitle>
        Public items reach every player (menu News tab). Dev items reach admin accounts only; the server leaves them out
        for everyone else.
      </template>
      <template #content>
        <Message v-if="error && !dialog" severity="error" :closable="false">{{ error }}</Message>
        <Message v-if="notice" severity="success" :closable="false">{{ notice }}</Message>

        <div class="bar">
          <Button label="New item" icon="pi pi-plus" @click="openNew" />
          <Select
            v-model="filter"
            :options="[
              { label: 'All', value: 'all' },
              { label: 'Public', value: 'public' },
              { label: 'Dev', value: 'dev' },
            ]"
            option-label="label"
            option-value="value"
          />
          <Button label="Refresh" severity="secondary" :loading="loading" @click="load" />
        </div>

        <p v-if="!loading && shown.length === 0" class="empty">No items.</p>

        <div v-if="shown.length" class="scroll">
          <table class="rows">
            <thead>
              <tr><th>Title</th><th>Audience</th><th>Version</th><th>Published</th><th>Expires</th><th></th></tr>
            </thead>
            <tbody>
              <tr v-for="i in shown" :key="i.id" :class="{ expired: isExpired(i) }">
                <td>
                  <div class="t">{{ i.title.en }}</div>
                  <div class="sub">{{ i.body.en }}</div>
                </td>
                <td>
                  <Tag :severity="i.audience === 'dev' ? 'warn' : 'info'" :value="i.audience" />
                  <Tag v-if="i.pinned" class="gap" severity="secondary" value="pinned" />
                  <Tag v-if="isExpired(i)" class="gap" severity="danger" value="expired" />
                </td>
                <td>{{ i.version }}</td>
                <td class="sub">{{ when(i.publishedAt) }}</td>
                <td class="sub">{{ when(i.expiresAt) }}</td>
                <td class="actions">
                  <Button label="Edit" size="small" severity="secondary" @click="openEdit(i)" />
                  <Button label="Delete" size="small" severity="danger" outlined @click="remove(i)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
      <template #footer>
        <router-link :to="{ name: 'home' }">Back</router-link>
      </template>
    </Card>

    <Dialog v-model:visible="dialog" modal :header="form.id ? 'Edit item' : 'New item'" :style="{ width: 'min(640px, 96vw)' }">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="form">
        <div class="row">
          <label>Audience <Select v-model="form.audience" :options="AUDIENCES" option-label="label" option-value="value" fluid /></label>
          <label>App version (optional) <InputText v-model="form.version" placeholder="1.0" fluid /></label>
        </div>
        <div class="row">
          <label class="check"><Checkbox v-model="form.pinned" binary /> Pinned (shown first)</label>
          <label>Expires (optional) <input v-model="form.expires" type="datetime-local" class="native" /></label>
        </div>
        <fieldset v-for="l in LOCALES" :key="l.code">
          <legend>{{ l.label }}</legend>
          <InputText v-model="form.title[l.code]" placeholder="Title (80 characters at most)" maxlength="80" fluid />
          <Textarea v-model="form.body[l.code]" placeholder="Text (1000 characters at most)" maxlength="1000" rows="4" auto-resize fluid />
        </fieldset>
      </div>
      <template #footer>
        <Button label="Cancel" severity="secondary" @click="dialog = false" />
        <Button :label="form.id ? 'Save' : 'Publish'" :loading="saving" @click="save" />
      </template>
    </Dialog>
  </main>
</template>

<style scoped>
.wrap { max-width: 1100px; margin: 0 auto; padding: 16px; }
.bar { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0; }
.empty { opacity: 0.7; }
.scroll { overflow-x: auto; }
.rows { width: 100%; border-collapse: collapse; }
.rows th, .rows td { text-align: left; padding: 8px; border-bottom: 1px solid var(--p-content-border-color, #ddd); vertical-align: top; }
.rows tr.expired { opacity: 0.55; }
.t { font-weight: 600; }
.sub { font-size: 0.85rem; opacity: 0.75; max-width: 420px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.gap { margin-left: 4px; }
.actions { display: flex; gap: 8px; white-space: nowrap; }
.form { display: grid; gap: 12px; }
.row { display: flex; flex-wrap: wrap; gap: 12px; }
.row > label { display: grid; gap: 4px; flex: 1 1 220px; }
.check { display: flex !important; align-items: center; gap: 8px; }
.native { padding: 8px; border: 1px solid var(--p-content-border-color, #ccc); border-radius: 6px; background: transparent; color: inherit; }
fieldset { display: grid; gap: 8px; border: 1px solid var(--p-content-border-color, #ccc); border-radius: 8px; padding: 8px 12px 12px; }
legend { padding: 0 6px; font-weight: 600; }
</style>
