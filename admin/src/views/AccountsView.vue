<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useSession } from '../session'

interface Provider { providerId: string; label: string; platformUid: string; displayName?: string }
interface Row { uid: string; providers: Provider[]; createdAt?: string; lastSignIn?: string; lastRefresh?: string }
interface Page { accounts: Row[]; scanned: number; nextPageToken?: string }

const session = useSession()
const rows = ref<Row[]>([])
const scanned = ref(0)
const nextToken = ref('')
const loading = ref(false)
const loaded = ref(false)
const error = ref('')

async function load(more = false) {
  loading.value = true
  error.value = ''
  try {
    const query = more && nextToken.value ? `?pageToken=${encodeURIComponent(nextToken.value)}` : ''
    const res = await session.api(`/admin/accounts${query}`)
    const body = (await res.json()) as Page & { error?: string }
    if (!res.ok) throw new Error(body.error ?? `The server answered ${res.status}.`)
    rows.value = more ? rows.value.concat(body.accounts) : body.accounts
    scanned.value = (more ? scanned.value : 0) + body.scanned
    nextToken.value = body.nextPageToken ?? ''
    loaded.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not reach the server.'
  } finally {
    loading.value = false
  }
}

onMounted(() => load())

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

async function copy(text: string) {
  try { await navigator.clipboard.writeText(text) } catch { /* the uid is selectable anyway */ }
}
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Linked players</template>
      <template #subtitle>
        Players whose progress is linked to Play Games or Game Center, read from Firebase Auth. Guests (anonymous
        accounts) are not listed; a page scans up to 500 accounts, so use "Scan more" to go on.
      </template>
      <template #content>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

        <div class="bar">
          <Button label="Refresh" icon="pi pi-refresh" severity="secondary" size="small" :loading="loading" @click="load()" />
          <span v-if="loaded" class="sub">{{ rows.length }} linked in {{ scanned }} accounts scanned<template v-if="nextToken">, more to scan</template></span>
        </div>

        <p v-if="loaded && !rows.length" class="empty">No linked player in the accounts scanned so far.</p>
        <div v-if="rows.length" class="scroll">
          <table class="rows">
            <thead><tr><th>Player (uid)</th><th>Linked to</th><th>Name</th><th>Created</th><th>Last sign-in</th></tr></thead>
            <tbody>
              <tr v-for="r in rows" :key="r.uid">
                <td class="uid">{{ r.uid }} <Button icon="pi pi-copy" size="small" text severity="secondary" aria-label="Copy uid" @click="copy(r.uid)" /></td>
                <td><Tag v-for="p in r.providers" :key="p.providerId" class="gap" severity="info" :value="p.label" /></td>
                <td>{{ r.providers.map((p) => p.displayName).filter(Boolean).join(', ') }}</td>
                <td class="sub">{{ when(r.createdAt) }}</td>
                <td class="sub">{{ when(r.lastSignIn) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="nextToken" class="bar">
          <Button label="Scan more" icon="pi pi-angle-double-down" :loading="loading" @click="load(true)" />
        </div>
      </template>
      <template #footer>
        <router-link :to="{ name: 'home' }">Back</router-link>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 1100px; margin: 0 auto; padding: 16px; }
.bar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin: 12px 0; }
.empty { opacity: 0.7; }
.scroll { overflow-x: auto; }
.rows { width: 100%; border-collapse: collapse; }
.rows th, .rows td { text-align: left; padding: 8px; border-bottom: 1px solid var(--p-content-border-color, #ddd); vertical-align: top; }
.uid { font-family: ui-monospace, monospace; font-size: 0.85rem; word-break: break-all; }
.sub { font-size: 0.85rem; opacity: 0.75; }
.gap { margin-right: 4px; }
</style>
