// Draws what the screens share: dark mode, the glyphs of the apps, the line
// icons, the community's mark and the drawn map of the example community.
// A screen marks where each goes:
//
//   <span data-glyph="talk"></span>   the glyph of an app, sized by CSS
//   <i data-icon="search"></i>        a line icon
//   <img data-mark alt="">           the Earthler mark, from brand/
//   <div class="map" data-map="…">   the drawn map, for Maps
//
// "#dark" at the end of the address draws the screen in dark mode, and so
// does a system that prefers dark. The glyphs and the mark are the Earthler
// brand and are not under the code license: see TRADEMARKS.md.
(function () {
  'use strict';
  var root = document.documentElement;
  if (location.hash === '#dark' || (location.hash !== '#light' && window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    root.classList.add('dark');
  }

  var here = document.currentScript ? document.currentScript.src : location.href;
  var mark = new URL('../../brand/earthler-mark.png', here).href;

  var glyphs = {
    talk: '<circle class="gs" cx="67" cy="24" r="15"/><path class="gf" d="M4 66 C7 59 7 52 7 41 A21 21 0 0 1 28 20 H45 A21 21 0 0 1 66 41 A21 21 0 0 1 45 62 H24 C16 62 10 63.5 4 66 Z"/><path class="gf2" d="M93 90 C90 84 90 79 90 70 A17 17 0 0 0 73 53 H60 A17 17 0 0 0 43 70 A17 17 0 0 0 60 87 H76 C83 87 88 88 93 90 Z"/>',
    learn: '<circle class="gs" cx="48" cy="38" r="19"/><path class="gf" d="M45.5 87 C34 81 21 79 7 81 C5 67 5 54 5 41 C22 38 37 45 45.5 57 Z"/><path class="gf" d="M50.5 87 C62 81 75 79 89 81 C91 67 91 54 91 41 C74 38 59 45 50.5 57 Z"/>',
    decide: '<circle class="gs" cx="25" cy="25" r="14"/><path class="gf" d="M9.0 48.0 C6.9 65.0 26.0 84.6 43.0 83.0 C45.1 66.0 26.0 46.4 9.0 48.0 Z"/><path class="gf" d="M37.0 86.0 C57.9 85.3 87.6 43.9 90.0 12.0 C62.2 25.7 32.5 67.1 37.0 86.0 Z"/>',
    ask: '<path class="gt" d="M27 35 C27 22 36 14 48 14 C60 14 70 22 70 33 C70 42 64 46 57.5 51 C51.5 55.5 48.5 58 48.5 64"/><circle class="gs" cx="48.5" cy="82" r="9"/>',
    // Maps, proposed: the sun over two leaves that meet in a point, a pin
    // made of the pieces of the mark.
    maps: '<circle class="gs" cx="48" cy="25" r="16"/><path class="gf" d="M11 47 C8 66 27 86 45 90 C48 70 31 48 11 47 Z"/><path class="gf2" d="M85 47 C88 66 69 86 51 90 C48 70 65 48 85 47 Z"/>'
  };

  var icons = {
    talk: '<path d="M21 12a8 8 0 0 1-12 7l-5 1 1-5a8 8 0 1 1 16-3z"/>',
    phone: '<path d="M5 4h4l2 5-3 2a12 12 0 0 0 5 5l2-3 5 2v4a2 2 0 0 1-2 2A17 17 0 0 1 3 6a2 2 0 0 1 2-2z"/>',
    video: '<rect x="3" y="6" width="13" height="12" rx="2"/><path d="M16 10l5-3v10l-5-3z"/>',
    bell: '<path d="M6 17v-6a6 6 0 1 1 12 0v6l2 2H4zM10 22h4"/>',
    siren: '<path d="M12 3 2 20h20zM12 10v4M12 17h.01"/>',
    grid: '<rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/>',
    lesson: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M10 9l5 3-5 3z"/>',
    wiki: '<path d="M6 3h9l4 4v14H6zM9 12h7M9 16h7M9 8h3"/>',
    collection: '<path d="M3 8h18v12H3zM3 8l2-4h14l2 4M9 12h6"/>',
    cinema: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4"/>',
    vote: '<rect x="4" y="4" width="16" height="16" rx="3"/><path d="M8 12l3 3 5-6"/>',
    pin: '<path d="M12 21s7-6.5 7-12a7 7 0 1 0-14 0c0 5.5 7 12 7 12z"/><circle cx="12" cy="9" r="2.5"/>',
    people: '<circle cx="9" cy="8" r="3"/><path d="M3 20a6 6 0 0 1 12 0M16 5a3 3 0 0 1 0 6M21 20a6 6 0 0 0-4-5.6"/>',
    person: '<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>',
    bulb: '<path d="M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.8.8 1 1.5 1 2.5h6c0-1 .2-1.7 1-2.5A6 6 0 0 0 12 3z"/>',
    list: '<path d="M9 6h11M9 12h11M9 18h11M4 6h1M4 12h1M4 18h1"/>',
    missing: '<circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 1 1 3.6 2.2c-.7.4-1.1 1-1.1 1.8M12 17h.01"/>',
    lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    shield: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/>',
    key: '<circle cx="8" cy="15" r="4"/><path d="M11 12l9-9M17 6l3 3M15 8l2 2"/>',
    back: '<path d="M15 5l-7 7 7 7"/>',
    next: '<path d="M9 5l7 7-7 7"/>',
    down: '<path d="M6 9l6 6 6-6"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.5-4.5"/>',
    plus: '<path d="M12 6.5v11M6.5 12h11"/>',
    close: '<path d="M6 6l12 12M18 6 6 18"/>',
    check: '<path d="M5 12l5 5 9-10"/>',
    send: '<path d="M4 12 20 4l-6 16-3-7z"/>',
    mic: '<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M5 11a7 7 0 0 0 14 0M12 18v3"/>',
    attach: '<path d="M20 12l-8 8a5 5 0 0 1-7-7l9-9a3.5 3.5 0 0 1 5 5l-9 9a2 2 0 0 1-3-3l8-8"/>',
    camera: '<path d="M4 8h3l2-3h6l2 3h3v11H4z"/><circle cx="12" cy="13" r="3.5"/>',
    image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="M21 17l-5-5-9 8"/>',
    file: '<path d="M6 3h9l4 4v14H6z"/><path d="M15 3v4h4"/>',
    upload: '<path d="M12 16V4M7 9l5-5 5 5M4 20h16"/>',
    download: '<path d="M12 4v12M7 11l5 5 5-5M4 20h16"/>',
    edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/>',
    trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>',
    flag: '<path d="M5 21V4h11l-2 4 2 4H5"/>',
    share: '<path d="M12 15V3M8 7l4-4 4 4M7 11H5v10h14V11h-2"/>',
    more: '<circle cx="5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="19" cy="12" r="1.2"/>',
    menu: '<path d="M4 7h16M4 12h16M4 17h10"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    device: '<rect x="7" y="2" width="10" height="20" rx="2"/><path d="M11 18h2"/>',
    computer: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
    server: '<rect x="4" y="3" width="16" height="7" rx="2"/><rect x="4" y="14" width="16" height="7" rx="2"/><path d="M8 6.5h.01M8 17.5h.01"/>',
    offline: '<path d="M2 8.5a15 15 0 0 1 5-3M22 8.5a15 15 0 0 0-9.5-3.4M5 12a10 10 0 0 1 3-2M19 12a10 10 0 0 0-4-2.3M9 15.5a5 5 0 0 1 6 0M12 19h.01M3 3l18 18"/>',
    wifi: '<path d="M2 8.5a15 15 0 0 1 20 0M5 12a10 10 0 0 1 14 0M9 15.5a5 5 0 0 1 6 0M12 19h.01"/>',
    refresh: '<path d="M20 11a8 8 0 0 0-14-5l-2 2M4 4v4h4M4 13a8 8 0 0 0 14 5l2-2M20 20v-4h-4"/>',
    eye: '<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
    home: '<path d="M4 11l8-7 8 7v9h-5v-6H9v6H4z"/>',
    star: '<path d="M12 3l2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z"/>',
    link: '<path d="M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1"/>',
    play: '<path d="M8 5l11 7-11 7z" fill="currentColor" stroke="none"/>',
    pause: '<path d="M8 5v14M16 5v14"/>',
    book: '<path d="M4 5a2 2 0 0 1 2-2h14v16H6a2 2 0 0 0-2 2zM4 19V5"/>',
    history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2"/>',
    child: '<circle cx="12" cy="6" r="3"/><path d="M8 21v-6l-2-3h12l-2 3v6"/>',
    door: '<path d="M5 21V4h11v17M5 21h14M13 12h.01"/>',
    robot: '<rect x="5" y="8" width="14" height="11" rx="3"/><path d="M12 4v4M9 13h.01M15 13h.01M9 16h6"/>',
    hand: '<path d="M8 13V5a1.5 1.5 0 0 1 3 0v6M11 11V4a1.5 1.5 0 0 1 3 0v7M14 11V6a1.5 1.5 0 0 1 3 0v8a7 7 0 0 1-7 7H9a5 5 0 0 1-4-2l-2-4a1.5 1.5 0 0 1 2.5-1.5L8 15"/>',
    layers: '<path d="M12 3 2 8l10 5 10-5z"/><path d="M2 13l10 5 10-5M2 17.5l10 5 10-5"/>',
    locate: '<circle cx="12" cy="12" r="7"/><circle cx="12" cy="12" r="2.5"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3"/>',
    drop: '<path d="M12 3s6 6.5 6 11a6 6 0 0 1-12 0c0-4.5 6-11 6-11z"/>',
    sprout: '<path d="M12 21v-8"/><path d="M12 13c0-4.5 3.5-7.5 8.5-7.5 0 4.5-3.5 7.5-8.5 7.5z"/><path d="M12 15c0-3.5-2.8-6-7.5-6 0 3.5 2.8 6 7.5 6z"/>',
    tool: '<path d="M14.7 6.3a4.5 4.5 0 0 0-6 6L3 18l3 3 5.7-5.7a4.5 4.5 0 0 0 6-6l-3 3-3-3z"/>',
    route: '<circle cx="6" cy="19" r="2"/><circle cx="18" cy="5" r="2"/><path d="M8 19h7.5a3.5 3.5 0 0 0 0-7h-7a3.5 3.5 0 0 1 0-7H16"/>',
    area: '<path d="M4 8l7-4.5 9 5-3 10.5-11 1.5z"/><circle cx="4" cy="8" r="1.6"/><circle cx="11" cy="3.5" r="1.6"/><circle cx="20" cy="8.5" r="1.6"/><circle cx="17" cy="19" r="1.6"/><circle cx="6" cy="20.5" r="1.6"/>',
    walk: '<circle cx="13" cy="4" r="2"/><path d="M10 21l2-6 3 3v3M8 12l3-4 3 2 3 1M11 8l-1 5 2 2"/>',
    undo: '<path d="M9 14 4 9l5-5"/><path d="M4 9h10a6 6 0 0 1 0 12h-3"/>',
    plane: '<path d="M3 13l18-8-6 16-3-7z"/><path d="M12 14l3-3"/>'
  };

  function svg(viewBox, body, cls) {
    return '<svg class="' + cls + '" viewBox="' + viewBox + '" aria-hidden="true">' + body + '</svg>';
  }

  // A style written on the placeholder stays on the drawing.
  function keep(el, html) {
    var style = el.getAttribute('style');
    if (style) html = html.replace('<svg ', '<svg style="' + style.replace(/"/g, '&quot;') + '" ');
    el.outerHTML = html;
  }
  document.querySelectorAll('[data-glyph]').forEach(function (el) {
    var g = glyphs[el.getAttribute('data-glyph')];
    if (g) keep(el, svg('0 0 96 96', g, 'gl ' + (el.className || '')));
  });
  document.querySelectorAll('[data-icon]').forEach(function (el) {
    var name = el.getAttribute('data-icon');
    var g = icons[name];
    if (!g) { el.outerHTML = '<span style="color:red">[' + name + ']</span>'; return; }
    keep(el, svg('0 0 24 24', g, 'i ' + (el.className || '')));
  });
  // The drawn map of the example community, the same place on every Maps
  // screen. data-map holds the piece to show, as an SVG viewBox in the
  // coordinates of the whole map (390 by 768); the other attributes add what
  // lies over the land: data-garden, data-land, data-alert, data-houses and
  // data-me. A drawing, never a picture of a real place.
  var land = '<rect class="m-field" x="-400" y="-400" width="1200" height="1600"/>' +
    '<path class="m-forest" d="M390 236 C330 232 278 268 266 320 C256 372 292 424 352 432 C370 434 382 432 390 430 Z"/>' +
    '<path class="m-pasture" d="M0 640 C80 615 150 680 230 658 S330 628 390 645 V900 H0 Z"/>' +
    '<path class="m-contour" d="M0 600 C90 580 160 630 250 605 S340 570 390 585"/>' +
    '<path class="m-contour" d="M0 250 C80 232 150 280 230 258 S330 222 390 236"/>' +
    '<path class="m-water" d="M80 0 C110 90 150 160 172 230 S250 380 300 470 S370 600 390 640"/>' +
    '<path class="m-road" d="M190 900 C200 660 160 580 176 500 S236 330 210 230 S170 90 180 0"/>' +
    '<path class="m-road2" d="M176 500 C120 480 60 456 0 450"/>' +
    '<path class="m-trail" d="M214 330 C240 338 262 350 276 372"/>' +
    '<path class="m-trail" d="M176 500 C140 560 120 600 110 668"/>' +
    '<rect class="m-building" x="214" y="402" width="36" height="22" rx="2"/>' +
    '<rect class="m-building" x="262" y="538" width="20" height="16" rx="2"/>' +
    '<rect class="m-building" x="84" y="566" width="20" height="16" rx="2"/>' +
    '<rect class="m-building" x="40" y="520" width="16" height="13" rx="2"/>' +
    '<rect class="m-building" x="300" y="590" width="16" height="13" rx="2"/>' +
    '<circle class="m-spring" cx="368" cy="474" r="7"/>' +
    '<text class="m-text" x="300" y="418">Mata</text>' +
    '<text class="m-text quiet" x="250" y="460">Rio</text>' +
    '<text class="m-text" x="212" y="442">Escola</text>' +
    '<text class="m-text" x="150" y="736">Pasto de baixo</text>';
  var over = {
    'data-land': '<path class="m-land" d="M24 296 L140 278 L158 392 L38 412 Z"/>',
    'data-garden': '<path class="m-garden" d="M44 316 L124 304 L136 376 L56 390 Z"/><text class="m-text" x="46" y="350">Horta coletiva</text>',
    'data-alert': '<circle class="m-alert" cx="330" cy="326" r="60"/>',
    'data-houses': '<text class="m-text" x="228" y="574">Casa da Neide</text><text class="m-text" x="62" y="604">Casa do Joel</text>',
    'data-me': '<circle class="m-halo" cx="196" cy="470" r="22"/><circle class="m-me" cx="196" cy="470" r="8"/>'
  };
  document.querySelectorAll('[data-map]').forEach(function (el) {
    var body = land;
    Object.keys(over).forEach(function (k) { if (el.hasAttribute(k)) body += over[k]; });
    el.insertAdjacentHTML('afterbegin', '<svg viewBox="' + (el.getAttribute('data-map') || '0 0 390 768') +
      '" preserveAspectRatio="xMidYMid slice" aria-hidden="true">' + body + '</svg>');
  });

  document.querySelectorAll('img[data-mark]').forEach(function (el) {
    el.src = mark;
    if (!el.className) el.className = 'mark';
  });
})();
