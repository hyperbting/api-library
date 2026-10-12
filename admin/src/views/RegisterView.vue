<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useSession } from '../session'

const session = useSession()
const email = ref('')
const password = ref('')
const confirm = ref('')
const busy = ref(false)
const created = ref(false)
const mismatch = ref(false)

// A message left over from the sign-in form does not belong here (and the other way round).
onMounted(() => { session.error = '' })
onBeforeUnmount(() => { session.error = '' })

async function submit() {
  mismatch.value = password.value !== confirm.value
  if (mismatch.value) return
  busy.value = true
  try {
    created.value = await session.registerEmail(email.value.trim(), password.value)
    if (created.value) password.value = confirm.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="wrap">
    <Card class="card">
      <template #title>Create an account</template>
      <template #content>
        <Message v-if="created" severity="success" :closable="false">
          Account created. It cannot open the panel until an admin grants it access.
        </Message>
        <Message v-if="session.error" severity="error" :closable="false">{{ session.error }}</Message>
        <Message v-if="mismatch" severity="error" :closable="false">The passwords do not match.</Message>

        <form v-if="!created" class="form" @submit.prevent="submit">
          <InputText v-model="email" type="email" placeholder="Email" autocomplete="username" required fluid />
          <Password v-model="password" placeholder="Password (8 to 72 characters)" :feedback="false" toggle-mask autocomplete="new-password" fluid />
          <Password v-model="confirm" placeholder="Repeat the password" :feedback="false" toggle-mask autocomplete="new-password" fluid />
          <Button type="submit" label="Create account" :loading="busy" fluid />
        </form>

        <p class="back"><router-link :to="{ name: 'login' }">Back to sign in</router-link></p>
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
