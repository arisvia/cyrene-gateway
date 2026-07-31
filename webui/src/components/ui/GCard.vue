<template>
  <div class="gcard">
    <slot />
  </div>
</template>

<style scoped>
.gcard {
  position: relative;
  background: var(--glass);
  backdrop-filter: blur(var(--glass-blur));
  -webkit-backdrop-filter: blur(var(--glass-blur));
  border: 1px solid var(--glass-border);
  border-radius: var(--radius);
  padding: 18px;
  transition: border-color var(--duration) ease, background var(--duration) ease, transform var(--duration) var(--ease-spring);
}
.gcard::before {
  content: '';
  position: absolute; inset: 0;
  border-radius: inherit;
  background: var(--glass-sheen);
  pointer-events: none;
}
.gcard:hover {
  border-color: var(--glass-border-hover);
  background: var(--glass-hover);
}
</style>
