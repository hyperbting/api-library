import { createRouter, createWebHistory } from 'vue-router'
import LoginView from './views/LoginView.vue'
import RegisterView from './views/RegisterView.vue'
import PasswordView from './views/PasswordView.vue'
import HomeView from './views/HomeView.vue'
import QuestsView from './views/QuestsView.vue'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/login', name: 'login', component: LoginView },
    { path: '/register', name: 'register', component: RegisterView },
    { path: '/password', name: 'password', component: PasswordView },
    { path: '/quests', name: 'quests', component: QuestsView },
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
