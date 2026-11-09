<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, auth, clearAuth } from './api'

const route = useRoute(); const router = useRouter(); const toastMessage = ref(''); const dark = ref(false); let toastTimer: ReturnType<typeof setTimeout> | undefined
const publicPage = computed(() => !!route.meta.public)
function showToast(event: Event) { toastMessage.value = (event as CustomEvent<string>).detail; clearTimeout(toastTimer); toastTimer = setTimeout(() => toastMessage.value = '', 3500) }
function expired() { if (!route.meta.public) router.push({ path: '/login', query: { redirect: route.fullPath } }) }
function toggleTheme() { dark.value = !dark.value; document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'; localStorage.setItem('com-theme', dark.value ? 'dark' : 'light') }
async function logout() { const userID = auth.user?.id; try { await api('/auth/logout', { method: 'POST' }) } finally { await clearAuth(true, userID); router.push('/login') } }
onMounted(() => { dark.value = localStorage.getItem('com-theme') === 'dark' || (!localStorage.getItem('com-theme') && matchMedia('(prefers-color-scheme: dark)').matches); document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'; window.addEventListener('toast', showToast); window.addEventListener('auth:expired', expired) })
onUnmounted(() => { window.removeEventListener('toast', showToast); window.removeEventListener('auth:expired', expired) })
</script>
<template>
  <div v-if="!publicPage && auth.user" class="app-shell">
    <header class="topbar">
      <RouterLink to="/ideas" class="brand"><span class="brand-mark">COM</span><span>游戏设定库</span></RouterLink>
      <nav class="main-nav" aria-label="主导航"><RouterLink to="/ideas">灵感</RouterLink><RouterLink to="/items" :class="{ 'router-link-active': !!route.meta.itemKind }" :aria-current="route.meta.itemKind ? 'page' : undefined">物品</RouterLink><RouterLink to="/recipes">合成配方</RouterLink><RouterLink to="/companies">公司</RouterLink><RouterLink to="/readme">读我</RouterLink><RouterLink to="/issues">问题</RouterLink><RouterLink to="/search">搜索</RouterLink></nav>
      <div class="top-actions"><button class="icon-button" :title="dark ? '切换亮色' : '切换暗色'" @click="toggleTheme">{{ dark ? '亮色' : '暗色' }}</button><RouterLink class="user-link" to="/settings">{{ auth.user.nickname }} · 设置</RouterLink><button class="text-button logout" @click="logout">退出</button></div>
    </header>
    <main class="page" :class="{ 'page-readme': route.path === '/readme' }"><RouterView :key="route.path" /></main>
    <footer class="mobile-note">先记录，后整理</footer>
  </div>
  <RouterView v-else-if="publicPage" />
  <main v-else class="auth-page" aria-busy="true">正在验证登录…</main>
  <div v-if="toastMessage" class="toast" role="status">{{ toastMessage }}</div>
</template>
