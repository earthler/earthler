'use strict';

// Two jobs, and the pages work without either: noticing when this device
// starts trusting the certificate, and remembering which step the person is
// on, so the right card is highlighted when they come back from Settings.
(function () {
  const trust = document.getElementById('trust');
  const text = document.getElementById('trust-text');
  const link = document.getElementById('trust-link');
  const steps = document.getElementById('steps');
  const late = document.getElementById('late');
  const key = 'earthler.certificate.step';
  let trusted = false;

  function probe() {
    const img = new Image();
    img.onload = function () {
      trusted = true;
      document.body.classList.add('trusted');
      trust.classList.add('ready');
      link.classList.remove('quiet');
      text.textContent = trust.dataset.trusted;
      if (late) late.classList.add('hidden');
      try { localStorage.removeItem(key); } catch (e) { /* storage may be off */ }
    };
    img.onerror = function () {
      setTimeout(probe, 3000);
    };
    img.src = trust.dataset.secureUrl + 'probe.gif?' + Date.now();
  }

  function current() {
    try { return Number(localStorage.getItem(key)) || 1; } catch (e) { return 1; }
  }

  function show(step) {
    try { localStorage.setItem(key, String(step)); } catch (e) { /* storage may be off */ }
    steps.querySelectorAll('.step').forEach(function (card) {
      const n = Number(card.dataset.step);
      card.classList.toggle('current', n === step);
      card.classList.toggle('done', n < step);
      card.querySelectorAll('.confirm').forEach(function (b) { b.classList.toggle('hidden', n !== step); });
    });
    if (late && step >= Number(steps.dataset.count)) {
      setTimeout(function () { if (!trusted) late.classList.remove('hidden'); }, 20000);
    }
  }

  if (steps) {
    show(current());
    const download = document.getElementById('download');
    if (download) download.addEventListener('click', function () { show(2); });
    steps.querySelectorAll('.confirm').forEach(function (b) {
      b.addEventListener('click', function () { show(Number(b.dataset.next) + 1); });
    });
  }
  probe();
})();
