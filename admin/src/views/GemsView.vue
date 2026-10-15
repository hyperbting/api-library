<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useSession } from '../session'

interface State {
  balance: number
  warpHoursUsed: number
  warpHoursLeft: number
  warpResetsAt: string
  boostSeconds: number
  serverNowMs: number
}
interface Reward { type: string; key?: string; amount: number; itemId?: string; count?: number }
interface GrantRow {
  id: string
  source: string
  sourceRef: string
  rewards: Reward[]
  createdAt: string
  claimed: boolean
  gems: number
  credited: boolean
}
interface SpendRow { requestId: string; product: string; price: number; at: string; balanceAfter: number; grantId?: string }
interface Overview { uid: string; state: State; spends: SpendRow[]; grants: GrantRow[]; hasRecords: boolean }
interface Order { orderId: string; uid: string; sku: string; grantId: string; createdAt: string }

// Items an admin can add to a gift besides gems (the server resolves them through its item catalog).
const ITEMS = [
  { label: 'None', value: '' },
  { label: 'Time warp 1 h (timewarp_1h)', value: 'timewarp_1h' },
  { label: 'Time warp 2 h (timewarp_2h)', value: 'timewarp_2h' },
  { label: 'Time warp 8 h (timewarp_8h)', value: 'timewarp_8h' },
  { label: 'Time warp 24 h (timewarp_24h)', value: 'timewarp_24h' },
  { label: 'Pull ticket (pull_ticket)', value: 'pull_ticket' },
  { label: 'Cash 10K (cash_10k)', value: 'cash_10k' },
]

const ERRORS: Record<string, string> = {
  empty_gift: 'Nothing to gift: enter gold bars or pick an item.',
  invalid_gift: 'The server refused the gift (too many gold bars or an unknown item).',
}

const session = useSession()
const uid = ref('')
const overview = ref<Overview | null>(null)
const orders = ref<Order[]>([])
const loading = ref(false)
const ordersLoading = ref(false)
const gifting = ref(false)
const error = ref('')
const notice = ref('')

const gift = reactive({ gems: 100, itemId: '', itemCount: 1, note: '' })

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
  notice.value = ''
  try {
    overview.value = await call<Overview>(`/admin/gems/user?uid=${encodeURIComponent(id)}`)
  } catch (e) {
    overview.value = null
    fail(e)
  } finally {
    loading.value = false
  }
}

async function loadOrders() {
  ordersLoading.value = true
  try {
    orders.value = (await call<{ orders: Order[] }>('/admin/iap/orders?limit=100')).orders
  } catch (e) {
    fail(e)
  } finally {
    ordersLoading.value = false
  }
}

onMounted(loadOrders)

async function sendGift() {
  if (!overview.value) return
  const items = gift.itemId ? [{ itemId: gift.itemId, count: Math.max(1, gift.itemCount) }] : []
  const what = [gift.gems > 0 ? `${gift.gems} gold bars` : '', ...items.map((i) => `${i.count} x ${i.itemId}`)].filter(Boolean).join(' + ')
  if (!what) {
    error.value = ERRORS.empty_gift
    return
  }
  if (!window.confirm(`Gift ${what} to ${overview.value.uid}?`)) return

  gifting.value = true
  error.value = ''
  notice.value = ''
  try {
    const res = await call<{ ok: boolean; errorCode?: string; repeated?: boolean }>('/admin/gems/gift', {
      method: 'POST',
      // One id per submit: a double click is answered, not paid twice.
      body: JSON.stringify({
        uid: overview.value.uid,
        requestId: crypto.randomUUID(),
        gems: Math.max(0, Math.floor(gift.gems || 0)),
        items,
        note: gift.note,
      }),
    })
    if (!res.ok) {
      error.value = ERRORS[res.errorCode ?? ''] ?? `Refused: ${res.errorCode}`
    } else {
      notice.value = `Gifted ${what}.`
      gift.note = ''
      await loadPlayer(overview.value.uid)
    }
  } catch (e) {
    fail(e)
  } finally {
    gifting.value = false
  }
}

function when(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
}

function rewardText(g: GrantRow): string {
  return g.rewards.map((r) => (r.type === 'Unlock' ? `Unlock ${r.key}` : `${r.amount} ${r.type === 'Gem' ? 'gold bars' : r.type}`)).join(', ')
}

function minutes(s: number): string {
  return s > 0 ? `${Math.floor(s / 60)} min ${s % 60} s` : 'none'
}

const sourceSeverity: Record<string, 'info' | 'success' | 'warn' | 'secondary'> = {
  iap: 'success',
  redeem: 'info',
  admin: 'warn',
  gems: 'secondary',
  ads: 'success',
  quest: 'info',
}
</script>

<template>
  <main class="wrap">
    <Card>
      <template #title>Gold bars</template>
      <template #subtitle>
        A player's gold bar balance, spends and grants by uid, a gift, and the newest store orders. A gift is a grant like
        any other: it shows in the player's history and is credited on their next read.
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
            This uid has no gold bar or grant records. Check that it is spelled right before gifting.
          </Message>

          <div class="stats">
            <div><span class="k">Balance</span><span class="v">{{ overview.state.balance }} gold bars</span></div>
            <div><span class="k">Warp hours today</span><span class="v">{{ overview.state.warpHoursUsed }} used, {{ overview.state.warpHoursLeft }} left</span></div>
            <div><span class="k">x2 boost left</span><span class="v">{{ minutes(overview.state.boostSeconds) }}</span></div>
            <div><span class="k">Allowance resets</span><span class="v">{{ when(overview.state.warpResetsAt) }}</span></div>
          </div>

          <h3>Gift</h3>
          <div class="form">
            <label>Gold bars <InputNumber v-model="gift.gems" :min="0" :max="100000" show-buttons fluid /></label>
            <label>Item <Select v-model="gift.itemId" :options="ITEMS" option-label="label" option-value="value" fluid /></label>
            <label v-if="gift.itemId">Count <InputNumber v-model="gift.itemCount" :min="1" :max="100" show-buttons fluid /></label>
            <label class="wide">Note (shown in the grant history) <InputText v-model="gift.note" maxlength="120" placeholder="why" fluid /></label>
            <Button label="Send gift" icon="pi pi-gift" :loading="gifting" @click="sendGift" />
          </div>

          <h3>Spends</h3>
          <p v-if="!overview.spends.length" class="empty">No spends.</p>
          <div v-else class="scroll">
            <table class="rows">
              <thead><tr><th>When</th><th>Product</th><th>Price</th><th>Balance after</th><th>Grant</th></tr></thead>
              <tbody>
                <tr v-for="s in overview.spends" :key="s.requestId">
                  <td class="sub">{{ when(s.at) }}</td>
                  <td>{{ s.product }}</td>
                  <td>{{ s.price }}</td>
                  <td>{{ s.balanceAfter }}</td>
                  <td class="sub">{{ s.grantId }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <h3>Grants (newest 100)</h3>
          <p v-if="!overview.grants.length" class="empty">No grants.</p>
          <div v-else class="scroll">
            <table class="rows">
              <thead><tr><th>When</th><th>Source</th><th>Rewards</th><th>Gold bars</th><th>Reference</th></tr></thead>
              <tbody>
                <tr v-for="g in overview.grants" :key="g.id">
                  <td class="sub">{{ when(g.createdAt) }}</td>
                  <td>
                    <Tag :severity="sourceSeverity[g.source] ?? 'secondary'" :value="g.source" />
                    <Tag v-if="!g.claimed" class="gap" severity="warn" value="waiting" />
                  </td>
                  <td>{{ rewardText(g) }}</td>
                  <td>
                    <template v-if="g.gems">{{ g.gems }} <Tag v-if="!g.credited" severity="danger" value="not credited yet" /></template>
                  </td>
                  <td class="sub">{{ g.id }}<br />{{ g.sourceRef }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <h3>Newest store orders</h3>
        <div class="bar">
          <Button label="Refresh" severity="secondary" size="small" :loading="ordersLoading" @click="loadOrders" />
        </div>
        <p v-if="!ordersLoading && !orders.length" class="empty">No orders yet.</p>
        <div v-if="orders.length" class="scroll">
          <table class="rows">
            <thead><tr><th>When</th><th>SKU</th><th>Order</th><th>Player</th><th></th></tr></thead>
            <tbody>
              <tr v-for="o in orders" :key="o.orderId">
                <td class="sub">{{ when(o.createdAt) }}</td>
                <td>{{ o.sku }}</td>
                <td class="sub">{{ o.orderId }}</td>
                <td class="sub">{{ o.uid }}</td>
                <td><Button label="Open" size="small" severity="secondary" @click="loadPlayer(o.uid)" /></td>
              </tr>
            </tbody>
          </table>
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
.bar { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0; }
.uid { flex: 1 1 320px; }
.empty { opacity: 0.7; }
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; margin: 12px 0; }
.stats > div { display: grid; padding: 12px; border: 1px solid var(--p-content-border-color, #ddd); border-radius: 8px; }
.k { font-size: 0.8rem; opacity: 0.7; }
.v { font-weight: 600; }
.form { display: flex; flex-wrap: wrap; gap: 12px; align-items: end; }
.form > label { display: grid; gap: 4px; flex: 1 1 200px; }
.form > .wide { flex: 2 1 320px; }
.scroll { overflow-x: auto; }
.rows { width: 100%; border-collapse: collapse; }
.rows th, .rows td { text-align: left; padding: 8px; border-bottom: 1px solid var(--p-content-border-color, #ddd); vertical-align: top; }
.sub { font-size: 0.85rem; opacity: 0.75; }
.gap { margin-left: 4px; }
h3 { margin: 20px 0 4px; }
</style>
