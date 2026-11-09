<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import type { ChoiceOption, Company } from '../types'
import { companyTitle, statusLabels } from '../utils'
const props = defineProps<{ current: Record<string, any>; submitted: Record<string, any> }>()
const emit = defineEmits<{ server: []; merge: []; cancel: [] }>()
const labels: Record<string, string> = { variant_name: '口味／样式', variant_description: '口味／样式差异说明', variant_group_id: '商品归组', content: '内容', display_title: '标题', summary: '摘要', notes: '备注', chinese_name: '中文名', english_name: '英文名', company_id: '所属公司', use_category: '使用类别', tool_settings: '道具使用与获得规则', aliases: '别名', name: '配方名称', output_quantity: '产出数量', ingredients: '材料', verification: '验证情况', company: '所属公司', category: '分类', description: '描述', development_status: '制作状态', attributes: '属性', idea_types: '灵感类型', product_directions: '商品方向' }
const differences = computed(() => Object.keys(props.submitted).filter(key => !['version', 'client_id'].includes(key) && JSON.stringify(props.current[key]) !== JSON.stringify(props.submitted[key])).map(key => ({ key, label: labels[key] || key, current: props.current[key], submitted: props.submitted[key] })))
const states = ref<Record<string,string>>({ ...statusLabels })
const companies = ref<Record<string,string>>({})
const show = (value: unknown, key: string) => {
  if (typeof value !== 'string') return JSON.stringify(value,null,2)
  if (key === 'development_status') return states.value[value] || (value === props.current.development_status ? props.current.development_status_label : '') || '自定义状态'
  if (key === 'company_id') return companies.value[value] || (value === props.current.company_id ? props.current.company : '') || (value ? '已选公司' : '未设置公司')
  return value
}
onMounted(async () => {
  const reads: Promise<void>[] = []
  if (differences.value.some(diff => diff.key === 'development_status')) reads.push(api<{items:ChoiceOption[]}>('/product-options/statuses').then(result => { states.value = { ...states.value, ...Object.fromEntries(result.items.map(option => [option.value,option.label])) } }))
  if (differences.value.some(diff => diff.key === 'company_id')) reads.push(api<{items:Company[]}>('/companies').then(result => { companies.value = Object.fromEntries(result.items.map(company => [company.id,companyTitle(company)])) }))
  await Promise.allSettled(reads)
})
</script>
<template>
  <div class="modal-backdrop"><section class="modal conflict-modal" role="dialog" aria-modal="true" aria-labelledby="conflict-title"><h2 id="conflict-title">检测到同时编辑</h2><p>服务器上已有新版本。请对照差异，选择使用服务器内容，或把你的修改应用到最新版本。</p><div v-if="differences.length" class="diff-list"><div v-for="diff in differences" :key="diff.key" class="diff-item"><strong>{{ diff.label }}</strong><div class="diff-columns"><div><small>服务器当前内容</small><pre>{{ show(diff.current,diff.key) || '（空）' }}</pre></div><div><small>你提交的内容</small><pre>{{ show(diff.submitted,diff.key) || '（空）' }}</pre></div></div></div></div><p v-else>记录版本已变化，但当前可见字段没有差异。</p><div class="modal-actions"><button class="secondary" @click="emit('server')">使用服务器版本</button><button class="secondary" @click="emit('cancel')">继续手动合并</button><button class="primary" @click="emit('merge')">将我的修改应用到最新版</button></div></section></div>
</template>
