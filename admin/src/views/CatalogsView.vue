<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useSession } from '../session'

// GET /admin/catalogs: the embedded tables the running revision loaded at boot (read only; they live in git).
interface Pet { id: string; address: string; species: string; stars: number; category: string; role: string; pool?: number; maxStars?: number }
interface Accessory { id: string; slot: string; rarity: string; fits: string[] }
interface Tier { tier: number; name: string; rate: number }
interface RowConfig { freeIntervalSeconds: number; gemPrice1: number; gemPrice10: number; tiers: Tier[]; firstDraw?: string[] }
interface Gacha { version: number; rows: Record<string, RowConfig>; pets: Pet[]; accessories: Accessory[] }
interface ItemDef { id: string; type: string; key?: string; amount: number; note?: string }
interface Product { sku: string; items: { itemId: string; count: number }[]; note?: string }
interface Items { version: number; items: ItemDef[]; products?: Product[] }
interface CatalogFile<T> { path: string; version: number; data: T | null }
interface Answer {
  commit: string
  repo: string
  catalogs: { gacha?: CatalogFile<Gacha>; items?: CatalogFile<Items>; quests?: CatalogFile<null> }
}

const session = useSession()
const router = useRouter()
const answer = ref<Answer | null>(null)
const error = ref('')
const tab = ref<'gacha' | 'items'>('gacha')

onMounted(async () => {
  try {
    const res = await session.api('/admin/catalogs')
    if (res.ok) answer.value = (await res.json()) as Answer
    else error.value = `Could not read the catalogs (${res.status}).`
  } catch {
    error.value = 'Could not reach the server.'
  }
})

// A link to the file at the commit this revision was built from. A local run ("dev") or a build with uncommitted
// changes ("...-dirty") has no such commit on GitHub, so those get no link.
const linkable = computed(() => !!answer.value && /^[0-9a-f]{7,40}$/.test(answer.value.commit))
function gitUrl(path: string): string {
  return `${answer.value?.repo}/blob/${answer.value?.commit}/${path}`
}

const files = computed(() => {
  const c = answer.value?.catalogs
  if (!c) return []
  return (['gacha', 'items', 'quests'] as const).flatMap((name) => (c[name] ? [{ name, ...c[name]! }] : []))
})

const gacha = computed(() => answer.value?.catalogs.gacha?.data ?? null)
const items = computed(() => answer.value?.catalogs.items?.data ?? null)
const fits = (a: Accessory) => (a.fits.length > 6 ? `${a.fits.length} species` : a.fits.join(', '))
const hours = (s: number) => `${s / 3600} h`
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Catalogs</template>
      <template #subtitle>What the running revision loaded at boot. Edit them in git, not here.</template>
      <template #content>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <p v-else-if="!answer">Loading...</p>
        <template v-else>
          <dl class="rows">
            <dt>Build</dt>
            <dd>{{ answer.commit }}<span v-if="!linkable" class="muted"> (no commit on GitHub, so no file links)</span></dd>
            <template v-for="f in files" :key="f.name">
              <dt>{{ f.name }}</dt>
              <dd>
                v{{ f.version }},
                <a v-if="linkable" :href="gitUrl(f.path)" target="_blank" rel="noopener">{{ f.path }} at {{ answer.commit }}</a>
                <span v-else>{{ f.path }}</span>
              </dd>
            </template>
          </dl>
        </template>
      </template>
      <template #footer>
        <Button label="Back" severity="secondary" @click="router.push({ name: 'home' })" />
      </template>
    </Card>

    <Card v-if="answer" class="detail">
      <template #title>
        <div class="tabs">
          <Button label="Gacha" :severity="tab === 'gacha' ? undefined : 'secondary'" @click="tab = 'gacha'" />
          <Button label="Items" :severity="tab === 'items' ? undefined : 'secondary'" @click="tab = 'items'" />
        </div>
      </template>
      <template #content>
        <template v-if="tab === 'gacha' && gacha">
          <h3>Rows</h3>
          <table>
            <thead><tr><th>Row</th><th>Free every</th><th>1 draw</th><th>10 draws</th><th>Tiers (rate)</th><th>First draw</th></tr></thead>
            <tbody>
              <tr v-for="(r, name) in gacha.rows" :key="name">
                <td>{{ name }}</td>
                <td>{{ hours(r.freeIntervalSeconds) }}</td>
                <td>{{ r.gemPrice1 }}</td>
                <td>{{ r.gemPrice10 }}</td>
                <td>{{ r.tiers.map((t) => `${t.name} ${t.rate}`).join(', ') }}</td>
                <td>{{ r.firstDraw?.join(', ') ?? '' }}</td>
              </tr>
            </tbody>
          </table>
          <h3>Pets ({{ gacha.pets.length }})</h3>
          <table>
            <thead><tr><th>Id</th><th>Species</th><th>Stars</th><th>Role</th><th>Category</th><th>Pool</th></tr></thead>
            <tbody>
              <tr v-for="p in gacha.pets" :key="p.id">
                <td>{{ p.id }}</td>
                <td>{{ p.species }}</td>
                <td>{{ p.stars }}<span v-if="p.maxStars && p.maxStars !== p.stars"> (up to {{ p.maxStars }})</span></td>
                <td>{{ p.role }}</td>
                <td>{{ p.category }}</td>
                <td><Tag :value="`pool ${p.pool ?? 1}`" :severity="(p.pool ?? 1) > 1 ? 'warn' : 'secondary'" /></td>
              </tr>
            </tbody>
          </table>
          <h3>Accessories ({{ gacha.accessories.length }})</h3>
          <table>
            <thead><tr><th>Id</th><th>Slot</th><th>Rarity</th><th>Fits</th></tr></thead>
            <tbody>
              <tr v-for="a in gacha.accessories" :key="a.id">
                <td>{{ a.id }}</td><td>{{ a.slot }}</td><td>{{ a.rarity }}</td><td>{{ fits(a) }}</td>
              </tr>
            </tbody>
          </table>
        </template>
        <template v-else-if="tab === 'items' && items">
          <h3>Items ({{ items.items.length }})</h3>
          <table>
            <thead><tr><th>Id</th><th>Type</th><th>Key</th><th>Amount</th><th>Note</th></tr></thead>
            <tbody>
              <tr v-for="i in items.items" :key="i.id">
                <td>{{ i.id }}</td><td>{{ i.type }}</td><td>{{ i.key ?? '' }}</td><td>{{ i.amount }}</td><td>{{ i.note ?? '' }}</td>
              </tr>
            </tbody>
          </table>
          <template v-if="items.products?.length">
            <h3>Products ({{ items.products.length }})</h3>
            <table>
              <thead><tr><th>SKU</th><th>Gives</th><th>Note</th></tr></thead>
              <tbody>
                <tr v-for="p in items.products" :key="p.sku">
                  <td>{{ p.sku }}</td>
                  <td>{{ p.items.map((r) => `${r.itemId} x${r.count}`).join(', ') }}</td>
                  <td>{{ p.note ?? '' }}</td>
                </tr>
              </tbody>
            </table>
          </template>
        </template>
        <p v-else>Not loaded on this server.</p>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 960px; margin: 0 auto; padding: 16px; }
.detail { margin-top: 16px; }
.tabs { display: flex; gap: 8px; }
.rows { display: grid; grid-template-columns: max-content 1fr; gap: 6px 16px; margin: 0; }
.rows dt { font-weight: 600; }
.rows dd { margin: 0; overflow-wrap: anywhere; }
.muted { opacity: 0.7; }
h3 { margin: 16px 0 6px; }
table { width: 100%; border-collapse: collapse; font-size: 0.9rem; display: block; overflow-x: auto; }
th, td { text-align: left; padding: 4px 10px 4px 0; border-bottom: 1px solid var(--p-content-border-color, #ddd); white-space: nowrap; }
</style>
