<script setup lang="ts">
import { useRoute } from 'vue-router'
const route = useRoute()
const collections = [
  { kind: 'product', label: '商品', path: '/products' },
  { kind: 'tool', label: '道具', path: '/tools' },
  { kind: 'other', label: '其他物品', path: '/other-items' },
]
</script>

<template>
  <section class="items-page" aria-label="物品">
    <h1 v-if="!route.params.id" class="items-title">物品</h1>
    <nav class="items-nav" aria-label="物品分类">
      <RouterLink v-for="collection in collections" :key="collection.kind" :to="collection.path" :class="{ active: route.meta.itemKind === collection.kind }" :aria-current="route.meta.itemKind === collection.kind ? 'page' : undefined">{{ collection.label }}</RouterLink>
    </nav>
    <RouterView :key="route.path" />
  </section>
</template>

<style scoped>
.items-title{margin:0 0 1rem;font-size:1.6rem}
.items-nav{display:flex;gap:.4rem;margin-bottom:1.5rem;border-bottom:1px solid var(--border)}
.items-nav a{padding:.75rem 1rem;border-bottom:2px solid transparent;color:var(--muted);font-weight:600}
.items-nav a:hover{color:var(--text);background:var(--surface-soft)}
.items-nav a.active{color:var(--accent);border-color:var(--accent)}
@media(max-width:600px){.items-nav{margin-bottom:1rem}.items-nav a{flex:1;padding:.7rem .5rem;text-align:center}}
</style>
