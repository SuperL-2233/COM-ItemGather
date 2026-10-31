<script setup lang="ts">
import type { Attribute } from '../types'
const attributes = defineModel<Attribute[]>({ required: true })
function add() { attributes.value.push({ key: '', value: '', certainty: 'asserted' }) }
function remove(index: number) { attributes.value.splice(index, 1) }
</script>
<template>
  <div class="attribute-editor">
    <div v-for="(attribute, index) in attributes" :key="index" class="attribute-row">
      <input v-model="attribute.key" aria-label="属性名" placeholder="属性名，如：游戏效果" maxlength="80" />
      <input v-model="attribute.value" aria-label="属性值" placeholder="属性值" maxlength="2000" />
      <select v-model="attribute.certainty" aria-label="确定程度"><option value="asserted">明确</option><option value="tentative">暂定</option><option value="undecided">待定</option></select>
      <button type="button" class="icon-button danger-text" title="删除属性" @click="remove(index)">删除</button>
    </div>
    <button type="button" class="secondary small" @click="add">+ 添加属性</button>
  </div>
</template>
