'use strict';

// Loads a small image from the secure site. It only loads once this device
// trusts the community certificate.
(function () {
  const secureURL = document.body.dataset.secureUrl;
  const card = document.getElementById('trust');
  const text = document.getElementById('trust-text');
  const link = document.getElementById('trust-link');

  function probe() {
    const img = new Image();
    img.onload = function () {
      card.classList.add('trusted');
      link.classList.remove('secondary');
      text.textContent = 'This device trusts the certificate.';
    };
    img.onerror = function () {
      setTimeout(probe, 3000);
    };
    img.src = secureURL + 'probe.gif?' + Date.now();
  }

  probe();
})();
