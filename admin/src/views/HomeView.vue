<script setup lang="ts">
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import { useSession } from '../session'

const session = useSession()
const router = useRouter()

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
        <p>Signed in as an admin. Admin tools go here.</p>
      </template>
      <template #footer>
        <Button label="Sign out" severity="secondary" @click="logout" />
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 960px; margin: 0 auto; padding: 16px; }
</style>
