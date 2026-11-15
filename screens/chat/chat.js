'use strict';

// Keeps the conversation current without reloading: one connection on which
// the server sends each new message and each deletion. The page works
// without this script; it only saves the reload.
(function () {
  const list = document.getElementById('messages');
  if (!list || !window.EventSource) return;
  const empty = document.getElementById('empty');
  const me = list.dataset.me;
  const canRemove = list.dataset.canRemove === '1';
  const removeForm = document.getElementById('remove-form');

  function atEnd() {
    return window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 120;
  }

  // A message written on another machine can arrive after messages written
  // later, so each one goes in its place: by time, then by id.
  function before(a, b) {
    const pa = Number(a.dataset.place), pb = Number(b.dataset.place);
    return pa < pb || (pa === pb && Number(a.dataset.number) < Number(b.dataset.number));
  }

  function insert(li) {
    const items = list.querySelectorAll('li');
    for (let i = items.length - 1; i >= 0; i--) {
      if (!before(li, items[i])) {
        items[i].after(li);
        return;
      }
    }
    list.prepend(li);
  }

  // How far the page has seen, in the journal of each machine. The browser
  // sends back the position of the last message it received when it
  // reconnects.
  const source = new EventSource('/chat/live?after=' + encodeURIComponent(list.dataset.position || ''));

  source.addEventListener('posted', function (ev) {
    const holder = document.createElement('template');
    holder.innerHTML = ev.data.trim();
    const li = holder.content.firstElementChild;
    if (!li || list.querySelector('li[data-number="' + li.dataset.number + '"]')) return;
    if (li.dataset.account !== me) {
      li.classList.remove('mine');
      li.querySelectorAll('form').forEach(function (f) { f.remove(); });
      // Someone who may remove content gets the form to do it, with a reason.
      if (canRemove && removeForm && !li.dataset.deleted) {
        const form = removeForm.content.firstElementChild.cloneNode(true);
        form.querySelector('input[name="number"]').value = li.dataset.number;
        li.appendChild(form);
      }
    }
    const wasAtEnd = atEnd();
    insert(li);
    if (empty) empty.hidden = true;
    if (wasAtEnd && li === list.lastElementChild) li.scrollIntoView({ block: 'end' });
  });

  source.addEventListener('deleted', function (ev) {
    const li = list.querySelector('li[data-number="' + ev.data + '"]');
    if (!li || li.dataset.deleted) return;
    li.dataset.deleted = '1';
    const p = li.querySelector('p');
    if (p) { p.textContent = list.dataset.deleted; p.className = 'muted'; }
    li.querySelectorAll('form').forEach(function (f) { f.remove(); });
  });

  source.addEventListener('reload', function () { location.reload(); });
})();
