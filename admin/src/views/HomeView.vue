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
        <p>Signed in as an admin.</p>
      </template>
      <template #footer>
        <div class="actions">
          <Button label="Quests" icon="pi pi-list-check" @click="router.push({ name: 'quests' })" />
          <Button label="News" icon="pi pi-megaphone" @click="router.push({ name: 'news' })" />
          <Button v-if="session.hasPassword" label="Change password" severity="secondary" @click="router.push({ name: 'password' })" />
          <Button label="Sign out" severity="secondary" @click="logout" />
        </div>
      </template>
    </Card>
  </main>
</template>

<style scoped>
.wrap { max-width: 960px; margin: 0 auto; padding: 16px; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; }
</style>
