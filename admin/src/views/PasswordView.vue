<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useSession } from '../session'

const session = useSession()
const current = ref('')
const next = ref('')
const confirm = ref('')
const busy = ref(false)
const changed = ref(false)
const problem = ref('')

onMounted(() => { session.error = '' })
onBeforeUnmount(() => { session.error = '' })

async function submit() {
  changed.value = false
  problem.value = ''
  if (next.value.length < 8 || next.value.length > 72) {
    problem.value = 'The new password must be 8 to 72 characters.'
    return
  }
  if (next.value !== confirm.value) {
    problem.value = 'The new passwords do not match.'
    return
  }
  if (next.value === current.value) {
    problem.value = 'The new password is the same as the current one.'
    return
  }
  busy.value = true
  try {
    changed.value = await session.changePassword(current.value, next.value)
    if (changed.value) current.value = next.value = confirm.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="wrap">
    <Card class="card">
      <template #title>Change password</template>
      <template #subtitle>{{ session.user?.email }}</template>
      <template #content>
        <Message v-if="changed" severity="success" :closable="false">Password changed. Other devices will be asked to sign in again.</Message>
        <Message v-if="session.error" severity="error" :closable="false">{{ session.error }}</Message>
        <Message v-if="problem" severity="error" :closable="false">{{ problem }}</Message>

        <form class="form" @submit.prevent="submit">
          <Password v-model="current" placeholder="Current password" :feedback="false" toggle-mask autocomplete="current-password" fluid />
          <Password v-model="next" placeholder="New password (8 to 72 characters)" :feedback="false" toggle-mask autocomplete="new-password" fluid />
          <Password v-model="confirm" placeholder="Repeat the new password" :feedback="false" toggle-mask autocomplete="new-password" fluid />
          <Button type="submit" label="Change password" :loading="busy" fluid />
        </form>

        <p class="back"><router-link :to="{ name: 'home' }">Back</router-link></p>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { min-height: 100vh; display: grid; place-items: center; padding: 16px; }
.card { width: 100%; max-width: 380px; }
.form { display: grid; gap: 12px; margin-top: 12px; }
.back { margin: 12px 0 0; font-size: 0.9rem; }
</style>
