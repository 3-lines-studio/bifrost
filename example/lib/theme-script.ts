export const themeScript = `
  (function() {
    const theme = localStorage.getItem('vite-ui-theme') || 'dark';
    const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const isDark = theme === 'dark' || (theme === 'system' && systemDark);
    document.documentElement.classList.add(isDark ? 'dark' : 'light');
  })();
`;
