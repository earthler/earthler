'use strict';

// The button that puts an app on the phone's home screen. On Android, Chrome
// offers its own confirmation once it allows it; until then, and on other
// browsers, the person is told where a browser keeps the option. On an
// iPhone, Apple does not let a site add itself, so the button shows the taps
// of Safari, and another browser of the iPhone is sent to Safari. Opened
// from its icon, an app shows none of this.
(function () {
  const invite = document.getElementById('invite');
  if (!invite) return;
  const app = document.body.dataset.app;
  const standalone = window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone === true;
  if (standalone) return;

  const key = 'earthler.later.' + app;
  function remembered() {
    try { return window.localStorage.getItem(key) === '1'; } catch (e) { return false; }
  }
  function remember() {
    try { window.localStorage.setItem(key, '1'); } catch (e) { /* the card just comes back next time */ }
  }

  // Coming from the portal's button. The mark leaves the address at once,
  // so an icon saved from here never opens the steps again.
  const params = new URLSearchParams(window.location.search);
  const asked = params.get('add') === '1';
  if (asked) {
    params.delete('add');
    const rest = params.toString();
    window.history.replaceState(null, '', window.location.pathname + (rest ? '?' + rest : '') + window.location.hash);
  }
  if (remembered() && !asked) return;
  invite.hidden = false;

  const ua = window.navigator.userAgent;
  const iphone = /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && window.navigator.maxTouchPoints > 1);
  const notSafari = iphone && /CriOS|FxiOS|EdgiOS|OPiOS|GSA\//.test(ua);
  const button = document.getElementById('add');
  const help = document.getElementById('add-help');
  const behind = Array.prototype.slice.call(document.querySelectorAll('body > header, body > main, body > nav'));
  let offer = null;

  window.addEventListener('beforeinstallprompt', function (ev) {
    ev.preventDefault();
    offer = ev;
    if (help) help.hidden = true;
  });
  window.addEventListener('appinstalled', function () { invite.hidden = true; });

  // A sheet over the page: what is behind it cannot be reached until it
  // closes, and closing it gives the focus back to the button.
  let open = null;
  function show(sheet) {
    if (!sheet) return;
    open = sheet;
    sheet.hidden = false;
    behind.forEach(function (el) { el.inert = true; });
    const ok = sheet.querySelector('button');
    if (ok) ok.focus();
  }
  function close() {
    if (!open) return;
    open.hidden = true;
    open = null;
    behind.forEach(function (el) { el.inert = false; });
    button.focus();
  }
  document.querySelectorAll('.veu').forEach(function (sheet) {
    sheet.querySelector('button').addEventListener('click', close);
    sheet.addEventListener('click', function (ev) { if (ev.target === sheet) close(); });
  });
  document.addEventListener('keydown', function (ev) { if (ev.key === 'Escape') close(); });

  button.addEventListener('click', function () {
    if (notSafari) { show(document.getElementById('ios-other')); return; }
    if (iphone) { show(document.getElementById('ios')); return; }
    if (offer) {
      offer.prompt();
      offer.userChoice.then(function (choice) {
        if (choice.outcome === 'accepted') invite.hidden = true;
      });
      offer = null;
      return;
    }
    if (help) help.hidden = false;
  });

  document.getElementById('later').addEventListener('click', function () {
    remember();
    invite.hidden = true;
  });

  if (asked) {
    invite.classList.add('asked');
    if (iphone) show(document.getElementById(notSafari ? 'ios-other' : 'ios'));
  }
})();
