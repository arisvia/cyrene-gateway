// 防主题闪烁（Zero-FOUC）首屏引导脚本。
// 必须是外部 classic script（而非 inline）：这样 CSP 可保持 script-src 'self' 严格模式，
// 无需退让到 'unsafe-inline'。外部 classic script 在 head 中同步加载，
// 仍在首屏绘制前执行完毕，闪烁抑制行为与 inline 版本一致。
(function () {
  if (localStorage.getItem('cyrene-theme') === 'light') document.documentElement.classList.add('light');
  const tc = localStorage.getItem('cyrene-theme-color');
  if (tc) document.documentElement.dataset.themeColor = tc;
  try {
    const bgCfg = JSON.parse(localStorage.getItem('cyrene_wallpaper_config') || '{}');
    if (bgCfg && bgCfg.enabled) {
      document.documentElement.classList.add('custom-background');
      if (bgCfg.surfaceAlpha) document.documentElement.style.setProperty('--app-surface-alpha', String(bgCfg.surfaceAlpha));
      if (bgCfg.glassBlur !== undefined) {
        document.documentElement.style.setProperty('--app-glass-blur', bgCfg.glassBlur + 'px');
        document.documentElement.style.setProperty('--glass-blur', bgCfg.glassBlur + 'px');
        document.documentElement.style.setProperty('--blur-lg', bgCfg.glassBlur + 'px');
        document.documentElement.style.setProperty('--blur-md', bgCfg.glassBlur + 'px');
      }
    }
  } catch (e) {}
})();
