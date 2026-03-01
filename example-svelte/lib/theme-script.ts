export const themeScript = `
(function() {
  function getThemePreference() {
    if (typeof localStorage !== 'undefined' && localStorage.getItem('theme')) {
      return localStorage.getItem('theme');
    }
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  
  const theme = getThemePreference();
  document.documentElement.classList.add(theme);
})();
`;
