import { createRouter, createWebHistory } from 'vue-router'
import { auth, initializeAuth } from './api'
import { cancelPendingScroll, restoreScroll, savedListPosition } from './composables/listNavigation'

const router = createRouter({
  history: createWebHistory(),
  scrollBehavior(to, from, savedPosition) {
    if (to.path === from.path && !savedPosition) return false
    const position = savedPosition || savedListPosition(to.fullPath)
    return position ? restoreScroll(position) : { left: 0, top: 0 }
  },
  routes: [
    { path: '/', redirect: '/ideas' },
    { path: '/login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
    { path: '/register', component: () => import('./views/RegisterView.vue'), meta: { public: true } },
    { path: '/reset-password', component: () => import('./views/ResetPasswordView.vue'), meta: { public: true } },
    { path: '/ideas', component: () => import('./views/IdeasView.vue') },
    { path: '/ideas/:id', component: () => import('./views/IdeaDetailView.vue') },
    {
      path: '/items', component: () => import('./views/ItemsView.vue'),
      children: [
        { path: '', redirect: '/products' },
        { path: '/products', component: () => import('./views/ProductsView.vue'), meta: { itemKind: 'product' } },
        { path: '/products/:id', component: () => import('./views/ProductDetailView.vue'), meta: { itemKind: 'product' } },
        { path: '/tools', component: () => import('./views/ToolsView.vue'), meta: { itemKind: 'tool' } },
        { path: '/tools/:id', component: () => import('./views/ProductDetailView.vue'), meta: { itemKind: 'tool' } },
        { path: '/other-items', component: () => import('./views/ProductsView.vue'), props: { kind: 'other' }, meta: { itemKind: 'other' } },
        { path: '/other-items/:id', component: () => import('./views/ProductDetailView.vue'), meta: { itemKind: 'other' } },
      ],
    },
    { path: '/companies', component: () => import('./views/CompaniesView.vue') },
    { path: '/companies/:id', component: () => import('./views/CompanyDetailView.vue') },
    { path: '/readme', component: () => import('./views/ReadmeView.vue') },
    { path: '/recipes', component: () => import('./views/RecipesView.vue') },
    { path: '/recipes/:id', component: () => import('./views/RecipeDetailView.vue') },
    { path: '/search', component: () => import('./views/SearchView.vue') },
    { path: '/settings', component: () => import('./views/SettingsView.vue') },
    { path: '/settings/options', component: () => import('./views/OptionSettingsView.vue') },
    { path: '/mcp/authorize', component: () => import('./views/MCPAuthorizeView.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/ideas' },
  ],
})
router.beforeEach(async to => {
  cancelPendingScroll()
  if (!auth.ready) await initializeAuth()
  if (!to.meta.public && !auth.user) return { path: '/login', query: { redirect: to.fullPath } }
  if (to.meta.public && auth.user && to.path === '/login') return '/ideas'
  return true
})
export default router
