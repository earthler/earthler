'use strict';

// Loads a small image from the secure site. It only loads once this device
// trusts the community certificate. The page works without this script: the
// link to the secure site is always there.
(function () {
  const card = document.getElementById('trust');
  const text = document.getElementById('trust-text');
  const link = document.getElementById('trust-link');

  function probe() {
    const img = new Image();
    img.onload = function () {
      card.classList.add('ready');
      link.classList.remove('quiet');
      text.textContent = card.dataset.trusted;
    };
    img.onerror = function () {
      setTimeout(probe, 3000);
    };
    img.src = card.dataset.secureUrl + 'probe.gif?' + Date.now();
  }

  probe();
})();
