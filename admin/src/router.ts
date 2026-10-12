import { createRouter, createWebHistory } from 'vue-router'
import LoginView from './views/LoginView.vue'
import HomeView from './views/HomeView.vue'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/login', name: 'login', component: LoginView },
  ],
})

// Home needs a confirmed admin session; the server re-checks every API call anyway.
router.beforeEach(async (to) => {
  const session = useSession()
  await session.ready()
  if (to.name !== 'login' && !session.isAdmin) return { name: 'login' }
  if (to.name === 'login' && session.isAdmin) return { name: 'home' }
})
