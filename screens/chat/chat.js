'use strict';

// Keeps the conversation current without reloading: one connection on which
// the server sends each new message and each deletion. The page works
// without this script; it only saves the reload.
(function () {
  const list = document.getElementById('messages');
  if (!list || !window.EventSource) return;
  const empty = document.getElementById('empty');
  const me = list.dataset.me;

  // The last entry this page knows of: a message, or the deletion of one.
  function last() {
    let n = 0;
    list.querySelectorAll('li').forEach(function (li) {
      n = Math.max(n, Number(li.dataset.number) || 0, Number(li.dataset.deleted) || 0);
    });
    return n;
  }

  function atEnd() {
    return window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 120;
  }

  const source = new EventSource('/chat/live?after=' + last());

  source.addEventListener('posted', function (ev) {
    const holder = document.createElement('template');
    holder.innerHTML = ev.data.trim();
    const li = holder.content.firstElementChild;
    if (!li || list.querySelector('li[data-number="' + li.dataset.number + '"]')) return;
    if (li.dataset.account !== me) {
      li.classList.remove('mine');
      li.querySelectorAll('form').forEach(function (f) { f.remove(); });
    }
    const wasAtEnd = atEnd();
    list.appendChild(li);
    if (empty) empty.hidden = true;
    if (wasAtEnd) li.scrollIntoView({ block: 'end' });
  });

  source.addEventListener('deleted', function (ev) {
    const li = list.querySelector('li[data-number="' + ev.data + '"]');
    if (!li || li.dataset.deleted) return;
    li.dataset.deleted = ev.lastEventId;
    const p = li.querySelector('p');
    if (p) { p.textContent = list.dataset.deleted; p.className = 'muted'; }
    li.querySelectorAll('form').forEach(function (f) { f.remove(); });
  });

  source.addEventListener('reload', function () { location.reload(); });
})();
