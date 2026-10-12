import { defineStore } from 'pinia'
import { initializeApp, type FirebaseApp } from 'firebase/app'
import {
  GoogleAuthProvider,
  getAuth,
  onAuthStateChanged,
  signInWithEmailAndPassword,
  signInWithPopup,
  signOut,
  type Auth,
  type User,
} from 'firebase/auth'

export interface PublicConfig {
  login: { google: boolean; email: boolean }
  firebase: { apiKey: string; authDomain: string; projectId: string; appId: string }
  apiBase: string
}

let app: FirebaseApp | undefined
let auth: Auth | undefined
let readyPromise: Promise<void> | undefined

export const useSession = defineStore('session', {
  state: () => ({
    config: null as PublicConfig | null,
    user: null as User | null,
    isAdmin: false,
    error: '',
  }),
  actions: {
    // Loads /admin/config.json, starts Firebase and waits for the first auth state.
    ready(): Promise<void> {
      readyPromise ??= (async () => {
        const res = await fetch('/admin/config.json')
        this.config = (await res.json()) as PublicConfig
        app = initializeApp(this.config.firebase)
        auth = getAuth(app)
        await new Promise<void>((resolve) => {
          const off = onAuthStateChanged(auth!, async (u) => {
            this.user = u
            this.isAdmin = u ? await this.checkAdmin() : false
            off()
            resolve()
          })
        })
      })()
      return readyPromise
    },
    // The server decides: 200 only for an enabled sign-in method + the admin role.
    async checkAdmin(): Promise<boolean> {
      const res = await this.api('/admin/me')
      return res.ok
    },
    async api(path: string, init: RequestInit = {}): Promise<Response> {
      const token = await this.user?.getIdToken()
      return fetch(`${this.config!.apiBase}${path}`, {
        ...init,
        headers: { ...(init.headers ?? {}), Authorization: `Bearer ${token ?? ''}` },
      })
    },
    async finish(): Promise<boolean> {
      this.user = auth!.currentUser
      this.isAdmin = await this.checkAdmin()
      if (!this.isAdmin) {
        this.error = 'This account is not an admin, or this sign-in method is disabled.'
        await signOut(auth!)
        this.user = null
      } else {
        this.error = ''
      }
      return this.isAdmin
    },
    async loginEmail(email: string, password: string): Promise<boolean> {
      await this.ready()
      try {
        await signInWithEmailAndPassword(auth!, email, password)
      } catch {
        this.error = 'Wrong email or password.'
        return false
      }
      return this.finish()
    },
    async loginGoogle(): Promise<boolean> {
      await this.ready()
      try {
        await signInWithPopup(auth!, new GoogleAuthProvider())
      } catch {
        this.error = 'Google sign-in was cancelled or failed.'
        return false
      }
      return this.finish()
    },
    async logout() {
      if (auth) await signOut(auth)
      this.user = null
      this.isAdmin = false
    },
  },
})
