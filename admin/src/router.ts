import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    { path: '/', name: 'home', component: () => import('./views/HomeView.vue') },
    { path: '/login', name: 'login', component: () => import('./views/LoginView.vue') },
    { path: '/register', name: 'register', component: () => import('./views/RegisterView.vue') },
    { path: '/password', name: 'password', component: () => import('./views/PasswordView.vue') },
    { path: '/quests', name: 'quests', component: () => import('./views/QuestsView.vue') },
    { path: '/news', name: 'news', component: () => import('./views/NewsView.vue') },
  ],
})

// Home needs a confirmed admin session; the server re-checks every API call anyway.
router.beforeEach(async (to) => {
  const session = useSession()
  await session.ready()
  // The register page exists only when the server mounted POST /admin/register (ADMIN_REGISTER_EMAIL).
  if (to.name === 'register') {
    if (session.isAdmin) return { name: 'home' }
    if (!session.config?.register.email) return { name: 'login' }
    return
  }
  if (to.name !== 'login' && !session.isAdmin) return { name: 'login' }
  // A Google account has no password to change.
  if (to.name === 'password' && !session.hasPassword) return { name: 'home' }
  if (to.name === 'login' && session.isAdmin) return { name: 'home' }
})
