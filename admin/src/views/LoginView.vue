<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useSession } from '../session'

const session = useSession()
const router = useRouter()
const email = ref('')
const password = ref('')
const busy = ref(false)

async function run(fn: () => Promise<boolean>) {
  busy.value = true
  try {
    if (await fn()) await router.replace({ name: 'home' })
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="wrap">
    <Card class="card">
      <template #title>Admin sign in</template>
      <template #content>
        <Message v-if="session.error" severity="error" :closable="false">{{ session.error }}</Message>
        <Message v-if="session.config && !session.config.login.email && !session.config.login.google" severity="warn" :closable="false">
          No sign-in method is enabled (ADMIN_LOGIN_EMAIL / ADMIN_LOGIN_GOOGLE).
        </Message>

        <form v-if="session.config?.login.email" class="form" @submit.prevent="run(() => session.loginEmail(email, password))">
          <InputText v-model="email" type="email" placeholder="Email" autocomplete="username" fluid />
          <Password v-model="password" placeholder="Password" :feedback="false" toggle-mask autocomplete="current-password" fluid />
          <Button type="submit" label="Sign in" :loading="busy" fluid />
        </form>

        <Button
          v-if="session.config?.login.google"
          class="google"
          icon="pi pi-google"
          label="Sign in with Google"
          severity="secondary"
          :loading="busy"
          fluid
          @click="run(() => session.loginGoogle())"
        />
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { min-height: 100vh; display: grid; place-items: center; padding: 16px; }
.card { width: 100%; max-width: 380px; }
.form { display: grid; gap: 12px; margin-top: 12px; }
.google { margin-top: 12px; }
</style>
