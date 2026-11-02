<script setup lang="ts">
defineProps<{ label: string; color?: string }>()
</script>

<template>
  <span class="status-badge" :style="{ '--status-color': color || '#7c3aed' }"><span class="status-dot" aria-hidden="true"></span>{{ label }}</span>
</template>

<style scoped>
.status-badge{display:inline-flex;align-items:center;gap:.4rem;max-width:100%;padding:.25rem .55rem;border:0;border-radius:5px;background:color-mix(in srgb,var(--status-color) 12%,var(--surface));color:var(--text);font-size:.75rem;line-height:1.4;overflow-wrap:anywhere}
.status-dot{width:7px;height:7px;flex:none;border-radius:50%;background:var(--status-color);box-shadow:0 0 0 1px color-mix(in srgb,var(--text) 15%,transparent)}
</style>
