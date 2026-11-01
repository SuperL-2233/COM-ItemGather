<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ current: Record<string, any>; submitted: Record<string, any> }>()
const emit = defineEmits<{ server: []; merge: []; cancel: [] }>()
const labels: Record<string, string> = { content: '内容', display_title: '标题', summary: '摘要', notes: '备注', chinese_name: '商品中文名', company: '所属公司', category: '分类', description: '描述', development_status: '制作状态', tags: '标签', attributes: '属性', name_candidates: '名称候选', idea_types: '灵感类型', product_directions: '商品方向' }
const differences = computed(() => Object.keys(props.submitted).filter(key => !['version', 'client_id'].includes(key) && JSON.stringify(props.current[key]) !== JSON.stringify(props.submitted[key])).map(key => ({ key, label: labels[key] || key, current: props.current[key], submitted: props.submitted[key] })))
const show = (value: unknown) => typeof value === 'string' ? value : JSON.stringify(value, null, 2)
</script>
<template>
  <div class="modal-backdrop"><section class="modal conflict-modal" role="dialog" aria-modal="true" aria-labelledby="conflict-title"><h2 id="conflict-title">检测到同时编辑</h2><p>服务器上已有新版本。请对照差异，选择使用服务器内容，或把你的修改应用到最新版本。</p><div v-if="differences.length" class="diff-list"><div v-for="diff in differences" :key="diff.key" class="diff-item"><strong>{{ diff.label }}</strong><div class="diff-columns"><div><small>服务器当前内容</small><pre>{{ show(diff.current) || '（空）' }}</pre></div><div><small>你提交的内容</small><pre>{{ show(diff.submitted) || '（空）' }}</pre></div></div></div></div><p v-else>记录版本已变化，但当前可见字段没有差异。</p><div class="modal-actions"><button class="secondary" @click="emit('server')">使用服务器版本</button><button class="secondary" @click="emit('cancel')">继续手动合并</button><button class="primary" @click="emit('merge')">将我的修改应用到最新版</button></div></section></div>
</template>
