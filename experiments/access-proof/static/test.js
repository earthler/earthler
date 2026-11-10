'use strict';

// Reports what this browser allows on the current connection, and lets the
// person record the result on the server.
(function () {
  const table = document.getElementById('checks');
  const mediaButton = document.getElementById('media');
  const saveButton = document.getElementById('save');
  const status = document.getElementById('status');
  const checks = {};

  // show adds or updates one row. kind is "test" for yes or no answers that
  // matter to the proof, and "info" for facts that are only recorded.
  function show(name, label, value, kind) {
    checks[name] = String(value);
    let row = document.getElementById('check-' + name);
    if (!row) {
      row = table.insertRow();
      row.id = 'check-' + name;
      row.insertCell().textContent = label;
      row.insertCell();
    }
    const cell = row.cells[1];
    cell.textContent = '';
    const span = document.createElement('span');
    if (value === true) {
      span.textContent = 'yes';
    } else if (value === false) {
      span.textContent = 'no';
    } else {
      span.textContent = String(value);
    }
    cell.appendChild(span);
    if (kind === 'info' || typeof value !== 'boolean') {
      cell.className = String(value).indexOf('granted') === 0 ? 'yes' : 'info';
    } else {
      cell.className = value ? 'yes' : 'no';
    }
  }

  async function runChecks() {
    const media = !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia);
    show('secure_context', 'Secure context', window.isSecureContext === true);
    show('web_crypto', 'Web Crypto', !!(window.crypto && window.crypto.subtle));
    show('media_api', 'Camera and microphone available', media);
    show('persistent_storage_api', 'Persistent storage available', !!(navigator.storage && navigator.storage.persist));
    show('browser_online', 'Browser says it is online', navigator.onLine, 'info');
    const standalone = window.matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
    show('home_screen_icon', 'Opened from a home screen icon', standalone, 'info');
    if (!media) {
      mediaButton.disabled = true;
      mediaButton.textContent = 'Camera and microphone are not available here';
    }
    if (!('serviceWorker' in navigator)) {
      show('service_worker', 'Service worker, for offline use', false);
      return;
    }
    try {
      await navigator.serviceWorker.register('/sw.js');
      show('service_worker', 'Service worker, for offline use', true);
    } catch (err) {
      show('service_worker', 'Service worker, for offline use', 'failed: ' + err.name);
    }
  }

  async function testMedia() {
    mediaButton.disabled = true;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true });
      const kinds = stream.getTracks().map(function (t) { return t.kind; }).sort().join(' and ');
      stream.getTracks().forEach(function (t) { t.stop(); });
      show('media_access', 'Camera and microphone access', 'granted: ' + kinds);
    } catch (err) {
      show('media_access', 'Camera and microphone access', 'failed: ' + err.name);
    }
    mediaButton.disabled = false;
  }

  async function save() {
    const opened = document.querySelector('input[name="opened"]:checked');
    if (document.querySelector('input[name="opened"]') && !opened) {
      status.textContent = 'First say how this page opened.';
      return;
    }
    saveButton.disabled = true;
    status.textContent = 'Saving.';
    try {
      const res = await fetch('/api/result', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          scenario: document.getElementById('scenario').value,
          opened: opened ? opened.value : '',
          notes: document.getElementById('notes').value,
          checks: checks
        })
      });
      status.textContent = res.ok
        ? 'Saved at ' + new Date().toLocaleTimeString() + '.'
        : (res.status === 400 ? 'First say how this page opened.' : 'Could not save. The server answered ' + res.status + '.');
    } catch (err) {
      status.textContent = 'Could not reach the server.';
    }
    saveButton.disabled = false;
  }

  mediaButton.addEventListener('click', testMedia);
  saveButton.addEventListener('click', save);
  runChecks();
})();
