import { createRouter, createWebHistory } from 'vue-router'
import { auth, initializeAuth } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/ideas' },
    { path: '/login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
    { path: '/register', component: () => import('./views/RegisterView.vue'), meta: { public: true } },
    { path: '/reset-password', component: () => import('./views/ResetPasswordView.vue'), meta: { public: true } },
    { path: '/ideas', component: () => import('./views/IdeasView.vue') },
    { path: '/ideas/:id', component: () => import('./views/IdeaDetailView.vue') },
    { path: '/products', component: () => import('./views/ProductsView.vue') },
    { path: '/products/:id', component: () => import('./views/ProductDetailView.vue') },
    { path: '/search', component: () => import('./views/SearchView.vue') },
    { path: '/settings', component: () => import('./views/SettingsView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/ideas' },
  ],
})
router.beforeEach(async to => {
  if (!auth.ready) await initializeAuth()
  if (!to.meta.public && !auth.user) return { path: '/login', query: { redirect: to.fullPath } }
  if (to.meta.public && auth.user && to.path === '/login') return '/ideas'
  return true
})
export default router
