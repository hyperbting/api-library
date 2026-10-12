import { defineStore } from 'pinia'
import { initializeApp, type FirebaseApp } from 'firebase/app'
import {
  EmailAuthProvider,
  GoogleAuthProvider,
  getAuth,
  onAuthStateChanged,
  reauthenticateWithCredential,
  signInWithEmailAndPassword,
  signInWithPopup,
  signOut,
  updatePassword,
  type Auth,
  type User,
} from 'firebase/auth'

export interface PublicConfig {
  login: { google: boolean; email: boolean }
  register: { email: boolean }
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
  getters: {
    // Only accounts that sign in with email/password have a password (a Google account does not).
    hasPassword: (state): boolean => !!state.user?.providerData.some((p) => p.providerId === 'password'),
  },
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
    // Creates the sign-in account only (POST /admin/register, public, rate limited). The account is not an
    // admin until its users/{uid} document gets UserRoles: ["admin"].
    async registerEmail(email: string, password: string): Promise<boolean> {
      await this.ready()
      this.error = ''
      try {
        const res = await fetch(`${this.config!.apiBase}/admin/register`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, password }),
        })
        if (res.status === 201) return true
        if (res.status === 409) this.error = 'This email is already registered.'
        else if (res.status === 429) this.error = 'Too many attempts. Try again in a minute.'
        else if (res.status === 400) this.error = 'Enter a valid email and a password of 8 to 72 characters.'
        else this.error = 'Registration failed.'
      } catch {
        this.error = 'Could not reach the server.'
      }
      return false
    },
    // Re-authenticates with the current password (Firebase demands a recent sign-in), then sets the new one.
    // Done in the browser with the Firebase SDK, so there is no password endpoint on the server.
    async changePassword(current: string, next: string): Promise<boolean> {
      this.error = ''
      const user = auth?.currentUser
      if (!user?.email) {
        this.error = 'Sign in again first.'
        return false
      }
      try {
        await reauthenticateWithCredential(user, EmailAuthProvider.credential(user.email, current))
        await updatePassword(user, next)
        return true
      } catch (e) {
        const code = (e as { code?: string }).code ?? ''
        if (code === 'auth/wrong-password' || code === 'auth/invalid-credential') this.error = 'The current password is wrong.'
        else if (code === 'auth/weak-password') this.error = 'The new password is too weak.'
        else if (code === 'auth/too-many-requests') this.error = 'Too many attempts. Try again later.'
        else this.error = 'Could not change the password.'
        return false
      }
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
