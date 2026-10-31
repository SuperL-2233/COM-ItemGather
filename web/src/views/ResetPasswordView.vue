<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
const route = useRoute(); const password = ref(''); const confirmation = ref(''); const busy = ref(false); const error = ref(''); const done = ref(false)
async function submit() { error.value = ''; if (password.value.length < 10 || password.value !== confirmation.value) { error.value = '密码至少 10 个字符，且两次输入必须一致'; return }; busy.value = true; try { await api('/auth/reset-password', { method: 'POST', body: JSON.stringify({ token: route.query.token, password: password.value }) }); done.value = true } catch (e) { error.value = (e as Error).message } finally { busy.value = false } }
</script>
<template><main class="auth-page"><form class="auth-card" @submit.prevent="submit"><div class="auth-brand">COM · ItemGather</div><h1>重置密码</h1><template v-if="!done"><div class="field"><label for="new-password">新密码</label><input id="new-password" v-model="password" type="password" minlength="10" autocomplete="new-password" required /></div><div class="field"><label for="confirm-password">再次输入</label><input id="confirm-password" v-model="confirmation" type="password" minlength="10" autocomplete="new-password" required /></div><p v-if="error" class="error-text">{{ error }}</p><button class="primary" :disabled="busy">保存新密码</button></template><template v-else><p>密码已重置，其他设备的登录已撤销。</p><RouterLink class="primary" to="/login">使用新密码登录</RouterLink></template></form></main></template>
