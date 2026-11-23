try {
  var t = localStorage.getItem('theme'), a = localStorage.getItem('accent');
  if (t) document.documentElement.dataset.theme = t;
  if (a) document.documentElement.style.setProperty('--accent', a);
} catch (e) {}
