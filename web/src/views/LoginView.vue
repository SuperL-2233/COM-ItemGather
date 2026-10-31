<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, setAuth } from '../api'
import type { User } from '../types'
const route = useRoute(); const router = useRouter(); const username = ref(''); const password = ref(''); const remember = ref(false); const busy = ref(false); const error = ref('')
async function submit() { error.value = ''; busy.value = true; try { const result = await api<{ user: User; csrf_token: string }>('/auth/login', { method: 'POST', body: JSON.stringify({ username: username.value, password: password.value, remember: remember.value, device_name: navigator.userAgent }) }); setAuth(result.user, result.csrf_token); router.replace(typeof route.query.redirect === 'string' ? route.query.redirect : '/ideas') } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template><main class="auth-page"><form class="auth-card" @submit.prevent="submit"><div class="auth-brand">COM · ItemGather</div><p class="muted">Call of Mommy 商品创意库</p><h1>登录</h1><div class="field"><label for="username">用户名</label><input id="username" v-model="username" autocomplete="username" required autofocus /></div><div class="field"><label for="password">密码</label><input id="password" v-model="password" type="password" autocomplete="current-password" required /></div><label class="checkbox-label"><input v-model="remember" type="checkbox" />记住此设备（最长 90 天）</label><p v-if="error" class="error-text" role="alert">{{ error }}</p><button class="primary" :disabled="busy">{{ busy ? '登录中…' : '登录' }}</button><p class="auth-note">账号由管理员邀请创建。忘记密码请联系管理员获取重置链接。</p></form></main></template>
