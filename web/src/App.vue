<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, auth, clearAuth } from './api'
import { clearOffline } from './offline'

const route = useRoute(); const router = useRouter(); const toastMessage = ref(''); const dark = ref(false); let toastTimer: ReturnType<typeof setTimeout> | undefined
const publicPage = computed(() => !!route.meta.public)
function showToast(event: Event) { toastMessage.value = (event as CustomEvent<string>).detail; clearTimeout(toastTimer); toastTimer = setTimeout(() => toastMessage.value = '', 3500) }
function expired() { if (!route.meta.public) router.push('/login') }
function toggleTheme() { dark.value = !dark.value; document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'; localStorage.setItem('com-theme', dark.value ? 'dark' : 'light') }
async function logout() { try { await api('/auth/logout', { method: 'POST' }) } finally { clearAuth(); await clearOffline().catch(() => {}); if ('caches' in window) (await caches.keys()).forEach(key => caches.delete(key)); router.push('/login') } }
onMounted(() => { dark.value = localStorage.getItem('com-theme') === 'dark' || (!localStorage.getItem('com-theme') && matchMedia('(prefers-color-scheme: dark)').matches); document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'; window.addEventListener('toast', showToast); window.addEventListener('auth:expired', expired) })
onUnmounted(() => { window.removeEventListener('toast', showToast); window.removeEventListener('auth:expired', expired) })
</script>
<template>
  <div v-if="!publicPage && auth.user" class="app-shell">
    <header class="topbar">
      <RouterLink to="/ideas" class="brand"><span class="brand-mark">COM</span><span>商品创意库</span></RouterLink>
      <nav class="main-nav" aria-label="主导航"><RouterLink to="/ideas">灵感</RouterLink><RouterLink to="/products">商品</RouterLink><RouterLink to="/search">搜索</RouterLink></nav>
      <div class="top-actions"><button class="icon-button" :title="dark ? '切换亮色' : '切换暗色'" @click="toggleTheme">{{ dark ? '亮色' : '暗色' }}</button><RouterLink class="user-link" to="/settings">{{ auth.user.nickname }} · 设置</RouterLink><button class="text-button logout" @click="logout">退出</button></div>
    </header>
    <main class="page"><RouterView /></main>
    <footer class="mobile-note">先记录，后整理</footer>
  </div>
  <RouterView v-else />
  <div v-if="toastMessage" class="toast" role="status">{{ toastMessage }}</div>
</template>
