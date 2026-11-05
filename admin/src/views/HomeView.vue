<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import { useSession } from '../session'

const session = useSession()
const router = useRouter()

// GET /admin/info: what this service is running (version, build, revision, catalogs, client limits).
interface ApiInfo {
  apiVersion: string
  apiBase: string
  env: string
  authMode: string
  commit: string
  goVersion: string
  startedAt: string
  serverTime: string
  uptimeSeconds: number
  cloudRun: { service: string; revision: string; configuration: string }
  client: { minVersion: string; latestVersion: string }
  admin: { panelEnabled: boolean; loginGoogle: boolean; loginEmail: boolean; registerEmail: boolean; acceptCustomToken: boolean }
  catalogs: Record<string, { version: number; entries: number; steps?: number }>
}

const info = ref<ApiInfo | null>(null)
const infoError = ref('')

onMounted(async () => {
  try {
    const res = await session.api('/admin/info')
    if (res.ok) info.value = (await res.json()) as ApiInfo
    else infoError.value = `Could not read the API info (${res.status}).`
  } catch {
    infoError.value = 'Could not reach the server.'
  }
})

function uptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return d > 0 ? `${d} d ${h} h` : h > 0 ? `${h} h ${m} min` : `${m} min`
}

const rows = computed<[string, string][]>(() => {
  const i = info.value
  if (!i) return []
  const on = (v: boolean) => (v ? 'on' : 'off')
  const out: [string, string][] = [
    ['API version', `${i.apiVersion} (${i.apiBase})`],
    ['Build', i.commit],
    ['Environment', `${i.env}, auth ${i.authMode}`],
    ['Revision', i.cloudRun.revision ? `${i.cloudRun.service} / ${i.cloudRun.revision}` : 'not on Cloud Run'],
    ['Started', `${new Date(i.startedAt).toLocaleString()} (up ${uptime(i.uptimeSeconds)})`],
    ['Go', i.goVersion],
    ['Client versions', `minimum ${i.client.minVersion || 'none'}, latest ${i.client.latestVersion || 'not set'}`],
    ['Admin sign-in', `Google ${on(i.admin.loginGoogle)}, email ${on(i.admin.loginEmail)}, register ${on(i.admin.registerEmail)}, custom token ${on(i.admin.acceptCustomToken)}`],
  ]
  return out
})

// The catalogs get their own block under the rows, in a different colour, so a version is easy to find.
const catalogRows = computed<[string, string][]>(() =>
  Object.entries(info.value?.catalogs ?? {}).map(([name, c]) => [name, `v${c.version}, ${c.entries} entries${c.steps ? `, ${c.steps} steps` : ''}`]),
)

async function logout() {
  await session.logout()
  await router.replace({ name: 'login' })
}
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Admin</template>
      <template #subtitle>{{ session.user?.email }}</template>
      <template #content>
        <p>Signed in as an admin.</p>
      </template>
      <template #footer>
        <div class="actions">
          <Button label="Quests" icon="pi pi-list-check" @click="router.push({ name: 'quests' })" />
          <Button label="Catalogs" icon="pi pi-book" @click="router.push({ name: 'catalogs' })" />
          <Button label="News" icon="pi pi-megaphone" @click="router.push({ name: 'news' })" />
          <Button label="Gold bars" icon="pi pi-gift" @click="router.push({ name: 'gems' })" />
          <Button label="Players" icon="pi pi-users" @click="router.push({ name: 'accounts' })" />
          <Button v-if="session.hasPassword" label="Change password" severity="secondary" @click="router.push({ name: 'password' })" />
          <Button label="Sign out" severity="secondary" @click="logout" />
        </div>
      </template>
    </Card>
    <Card class="info">
      <template #title>API</template>
      <template #content>
        <p v-if="infoError" class="error">{{ infoError }}</p>
        <p v-else-if="!info">Loading...</p>
        <dl v-else class="rows">
          <template v-for="[label, value] in rows" :key="label">
            <dt>{{ label }}</dt>
            <dd>{{ value }}</dd>
          </template>
        </dl>
        <div v-if="catalogRows.length" class="catalogs">
          <div class="catalogs-title">
            Catalogs
            <a class="catalogs-link" href="#" @click.prevent="router.push({ name: 'catalogs' })">open</a>
          </div>
          <dl class="rows">
            <template v-for="[label, value] in catalogRows" :key="label">
              <dt>{{ label }}</dt>
              <dd>{{ value }}</dd>
            </template>
          </dl>
        </div>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 960px; margin: 0 auto; padding: 16px; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
.info { margin-top: 16px; }
.rows { display: grid; grid-template-columns: max-content 1fr; gap: 6px 16px; margin: 0; }
.rows dt { font-weight: 600; }
.rows dd { margin: 0; overflow-wrap: anywhere; }
.catalogs { margin-top: 14px; padding: 10px 14px; border-radius: 8px; background: var(--p-primary-50, #eef6ff); color: var(--p-primary-700, #1d4e89); }
.catalogs-title { font-weight: 700; margin-bottom: 6px; display: flex; justify-content: space-between; }
.catalogs-link { font-weight: 400; color: inherit; }
@media (prefers-color-scheme: dark) { .catalogs { background: rgba(120, 170, 255, 0.14); color: var(--p-primary-200, #b9d4ff); } }
.error { color: var(--p-red-500, #c62828); }
</style>
