// The browse page: look through a Box, correct what is in it, and see what
// the tool cannot explain. It writes sidecars and never a scan, which is why
// an edit here is instant and safe — the file does not move and its digest
// stays valid.
//
// There is no search box on purpose. With no text layer and no OCR, the only
// searchable words are the ones typed at intake, and much of a Box is taken in
// as unsorted. Filters are built on what is actually known.

const state = {
  config: {},
  types: [],
  scans: [],
  totals: [],
  selected: null,
  // selectedSplit is which split of the selected scan its card stands for,
  // when splits are shown as documents; null for the whole scan.
  selectedSplit: null,
  // shown is the scan the detail was last filled from, and draft is what
  // has been changed about its splits and not yet saved.
  shown: null,
  draft: {},
  // marked is the set of digests a batch would be given to. Batch editing is
  // necessary — three hundred scans one at a time is an hour nobody finishes —
  // and it is also the only way to get three hundred records wrong at once,
  // which is what the log is for.
  marked: new Set(),
  // tags is every tag the Box uses, most used first; facets is how many of
  // the cards now shown carry each, which is what the filter offers.
  tags: [],
  facets: new Map(),
};

const el = (id) => document.getElementById(id);

const pages = pageView(
  {
    strip: 'detail-page-strip',
    toggle: 'detail-pages-toggle',
    image: 'detail-image',
    label: 'detail-page-label',
  },
  (item, page) => split.decorate(item, page),
  (page) => {
    const before = split.active();
    split.follow(page);
    if (split.active() !== before) drawFields();
    else split.drawHead();
  },
);

// A filed scan's splits are corrected here the way they were made at intake:
// the same strip, the same Space marking, and the detail's own fields
// describing whichever split is picked. Only the sidecar changes.
const split = splitter({
  pages,
  scan: () => selected(),
  draft: () => state.draft,
  showError,
  again: 'Save again',
  changed: () => drawFields(),
});

const zones = zonePicker(el('detail-event-zone'), el('detail-zone-options'), (name) => {
  const document = split.editable();
  if (document) document.eventZone = name;
});

// DETAIL maps the detail's inputs to the names a scan and a split share.
const DETAIL = {
  'detail-type': 'type',
  'detail-description': 'description',
  'detail-event-date': 'eventDate',
  'detail-event-zone': 'eventZone',
  'detail-total': 'total',
};

// The detail's tags describe whichever split is picked, as its other fields do.
const detailTags = tagField(el('detail-tags'), {
  known: () => state.tags,
  onChange: (list) => {
    const document = split.editable();
    if (document) document.tags = list;
  },
});

// A batch adds tags and never removes one: a list typed once for three
// hundred scans must not replace what each of them already carries.
const markedTags = tagField(el('marked-tags'), {
  known: () => state.tags,
  placeholder: 'none',
});

// The tag filter takes only tags that exist, and narrows with each one: a card
// is shown when it carries every tag chosen. Beside each suggestion is how many
// of the cards now shown carry it, so a tag that would empty the grid says so.
const filterTags = tagField(el('filter-tags'), {
  known: () => state.tags.filter((use) => state.facets.has(use.name) || filterTags?.get().includes(use.name)),
  count: (name) => state.facets.get(name) ?? 0,
  only: true,
  placeholder: 'any',
  onChange: () => filtersChanged(),
});

// drawFields fills the detail from what it is describing: the picked split,
// or the scan itself. The scan's own values are only put back when another
// scan is opened or a save has landed, so typing is never overwritten.
function drawFields(fromScan) {
  const scan = selected();
  if (!scan) return;
  split.drawHead();
  el('split-tools').hidden = !split.applies();
  const document = split.active();
  el('detail-expires').disabled = Boolean(document);
  if (document) {
    el('detail-type').value = document.type || 'unsorted';
    el('detail-description').value = document.description || '';
    el('detail-event-date').value = document.eventDate || '';
    el('detail-event-zone').value = document.eventZone || state.config.zone || '';
    el('detail-total').value = document.total || '';
    detailTags.set(document.tags);
    return;
  }
  if (!fromScan) return;
  el('detail-type').value = scan.type;
  el('detail-description').value = scan.description || '';
  el('detail-event-date').value = scan.eventDate || '';
  el('detail-event-zone').value = scan.eventZone || '';
  el('detail-expires').value = scan.expiresAt || '';
  el('detail-total').value = scan.total || '';
  detailTags.set(scan.tags);
}

async function start() {
  state.config = await api.config();
  el('sample-banner').hidden = !state.config.sample;
  el('paths').textContent = state.config.root || '';
  const types = await api.types();
  state.types = types.types;
  fillTypeOptions();
  readTagsFromURL();
  await reload();
  wireFilters();
  wireDetail();
  wireMarked();
  wireDetailResize();
  await drawExceptionCount();
  await drawTrash();
}

// drawTrash is advice and nothing else. box never removes a file: emptying
// trash/ stays a decision made by hand, and box.trash.keep only decides when
// to mention it.
async function drawTrash() {
  let summary;
  try {
    summary = await api.trashSummary();
  } catch {
    return;
  }
  // The trash is a standing fact about the Box, so it is the bar's state:
  // a label, the count as the figure, and the dates as a quiet note. Emptying
  // trash/ stays a decision made by hand; the warning only says it is due.
  if (!summary.count) {
    statusBar.setState('');
    return;
  }
  let html = `<span class="sb-label">Trash</span><span class="sb-figure">${summary.count}</span>`;
  if (summary.oldest) html += `<span class="sb-note">oldest ${summary.oldest}</span>`;
  if (summary.keepDays && summary.overdue) {
    html += `<span class="sb-note sb-warn">${summary.overdue} past ${summary.keepDays} days</span>`;
  }
  statusBar.setState(html, { html: true });
}

function fillTypeOptions() {
  for (const type of state.types) {
    el('filter-type').append(new Option(type.label, type.name));
    el('detail-type').append(new Option(`${type.label} (${type.name})`, type.name));
    el('marked-type').append(new Option(type.label, type.name));
  }
}

async function reload() {
  const body = await api.scans();
  state.scans = body.scans;
  state.totals = body.totals;
  try {
    state.tags = (await api.tags()).tags;
  } catch {
    state.tags = [];
  }
  // Whether a bubble is a tag the Box has never seen can change with a save.
  for (const field of [detailTags, markedTags, filterTags]) field.redraw();
  fillDerivedFilters();
  draw();
}

function fillDerivedFilters() {
  const years = new Set();
  const currencies = new Set();
  const producers = new Set();
  const zones = new Set();
  for (const scan of state.scans) {
    for (const zone of zonesOf(scan)) zones.add(zone);
    const year = sortDay(scan).slice(0, 4);
    if (year) years.add(year);
    if (scan.total) currencies.add(scan.total.split(' ')[0]);
    if (scan.producer) producers.add(scan.producer);
  }
  refill('filter-year', [...years].sort().reverse());
  refill('filter-currency', [...currencies].sort());
  refill('filter-producer', [...producers].sort());
  // "none" is kept as the last choice whatever the Box holds: a scan with no
  // zone is one whose date was never pinned to a place.
  const zoneSelect = el('filter-zone');
  const noZone = zoneSelect.value === '-';
  refill('filter-zone', [...zones].sort());
  zoneSelect.append(new Option('none', '-'));
  if (noZone) zoneSelect.value = '-';
}

function refill(id, values) {
  const select = el(id);
  const chosen = select.value;
  select.innerHTML = '<option value="">any</option>';
  for (const value of values) select.append(new Option(value, value));
  select.value = values.includes(chosen) ? chosen : '';
}

// sortDay is the event date where there is one and the scan date otherwise.
// Which one it used is said on the card, because a column sorted by two
// different meanings looks like one ordering that is subtly wrong.
function sortDay(scan) {
  return scan.eventDate || localDate(scan.scannedAt) || '';
}

// searchText is what the search reads for a card: its description, its
// splits' descriptions and its filename, folded so case and accents do not
// decide a match.
function searchText(scan) {
  const words = [scan.description, scan.filename, scan.split ? '' : (scan.documents || []).map((d) => d.description).join(' ')];
  return fold(words.filter(Boolean).join(' '));
}

function fold(text) {
  return text.normalize('NFKD').replace(/[\u0300-\u036f]/g, '').toLocaleLowerCase();
}

// searchTerms are the words typed; a card matches when it holds every one,
// anywhere, so "nikon bino" finds "Nikon 8x42 Monarch M5 Binocular".
function searchTerms() {
  return fold(el('filter-text').value).split(/\s+/).filter(Boolean);
}

// zonesOf is every zone a card stands for: a split's own, or a scan's and its
// splits'.
function zonesOf(entry) {
  const zones = [entry.eventZone, ...(entry.split ? [] : (entry.documents || []).map((d) => d.eventZone))];
  return [...new Set(zones.filter(Boolean))];
}

function matches(scan) {
  const terms = searchTerms();
  if (terms.length) {
    const text = searchText(scan);
    if (!terms.every((term) => text.includes(term))) return false;
  }
  const year = el('filter-year').value;
  if (year && !sortDay(scan).startsWith(year)) return false;
  const type = el('filter-type').value;
  if (type && scan.type !== type) return false;
  const scanState = el('filter-state').value;
  if (scanState && scan.state !== scanState) return false;
  const reviewed = el('filter-reviewed').value;
  if (reviewed === 'yes' && !scan.reviewed) return false;
  if (reviewed === 'no' && scan.reviewed) return false;
  const currency = el('filter-currency').value;
  if (currency && (!scan.total || !scan.total.startsWith(currency))) return false;
  const zone = el('filter-zone').value;
  if (zone === '-' && zonesOf(scan).length) return false;
  if (zone && zone !== '-' && !zonesOf(scan).includes(zone)) return false;
  const producer = el('filter-producer').value;
  if (producer && scan.producer !== producer) return false;
  const carried = new Set(tagsOf(scan));
  if (!filterTags.get().every((tag) => carried.has(tag))) return false;
  switch (el('filter-deferred').value) {
    case 'needsSplit':
      if (!scan.needsSplit) return false;
      break;
    case 'needsRender':
      if (!scan.needsRender) return false;
      break;
    case 'unknownType':
      if (scan.typeKnown) return false;
      break;
    case 'noExpiry':
      if (scan.expiry || scan.expiryCleared) return false;
      break;
    default:
      break;
  }
  return true;
}

// tagsOf is every tag a card stands for, spelled as the filter spells it: a
// scan's own and its splits', or a split's own and the file's.
function tagsOf(entry) {
  if (entry.split) return tagList(entry.tags);
  return tagList([...(entry.tags || []), ...(entry.documents || []).flatMap((document) => document.tags || [])]);
}

// entries is what the grid lists: every scan, or, with splits shown as
// documents, each split of a split scan in its place, carrying the split's own
// description, type, date, amount and tags so filters and totals read it.
function entries() {
  if (!el('view-splits').checked) return state.scans;
  const known = new Set(state.types.map((type) => type.name));
  return state.scans.flatMap((scan) => {
    if (!scan.documents?.length) return [scan];
    return scan.documents.map((document, index) => ({
      ...scan,
      type: document.type || scan.type,
      typeKnown: document.type ? known.has(document.type) : scan.typeKnown,
      description: document.description || '',
      eventDate: document.eventDate || '',
      eventZone: document.eventZone || scan.eventZone || '',
      total: document.total || '',
      tags: [...(scan.tags || []), ...(document.tags || [])],
      documents: [],
      split: { index, pages: document.pages, count: scan.documents.length },
    }));
  });
}

// sortKeys are the orders Sort offers, each a value that compares as it
// should: dates and stamps as strings or milliseconds, text case-folded, an
// amount by currency first and then by number, since amounts in two
// currencies are not one scale.
const sortKeys = {
  event: sortDay,
  added: (scan) => (scan.ingestedAt ? new Date(scan.ingestedAt).getTime() : ''),
  edited: (scan) => (scan.editedAt ? new Date(scan.editedAt).getTime() : ''),
  title: (scan) => cardTitle(scan).toLocaleLowerCase(),
  type: (scan) => scan.type || '',
  total: (scan) => {
    if (!scan.total) return '';
    const [code, value] = scan.total.split(' ');
    return [code, Number(value)];
  },
  producer: (scan) => (scan.producer || '').toLocaleLowerCase(),
  pages: (scan) => (scan.split ? pagesOf(scan.split.pages).length : scan.pages || ''),
  state: (scan) => scan.state || '',
};

// Dates start newest first and names from A, which is what a click on a
// column heading picks when it changes the order.
const newestFirst = new Set(['event', 'added', 'edited', 'total', 'pages']);

function compareValues(a, b) {
  if (Array.isArray(a)) {
    if (a[0] !== b[0]) return a[0] < b[0] ? -1 : 1;
    return a[1] - b[1];
  }
  return a < b ? -1 : a > b ? 1 : 0;
}

// sorted orders what is shown by the chosen key and direction. Event date falls
// back to the scan date as the card says; date added is the sidecar's
// ingested_at; last edited is when the sidecar was last written. A scan with
// no value sorts last either way, and ties keep the server's order.
function sorted(scans) {
  const key = sortKeys[el('view-sort').value] || sortDay;
  const sign = el('view-direction').value === 'asc' ? 1 : -1;
  return scans
    .map((scan, position) => ({ scan, position, value: key(scan) }))
    .sort((a, b) => {
      const aEmpty = a.value === '' || a.value == null;
      const bEmpty = b.value === '' || b.value == null;
      if (aEmpty || bEmpty) return aEmpty === bEmpty ? a.position - b.position : aEmpty ? 1 : -1;
      return sign * compareValues(a.value, b.value) || a.position - b.position;
    })
    .map((each) => each.scan);
}

function draw() {
  const all = entries();
  const shown = sorted(all.filter(matches));
  el('count').textContent = `${shown.length} of ${all.length}`;
  el('grid-empty').hidden = shown.length > 0;
  drawTotals(shown);
  countFacets(shown);
  drawTagStrip();
  const list = el('view-layout').value === 'list';
  const grid = el('grid');
  const body = el('list-body');
  grid.hidden = list;
  el('list').hidden = !list;
  grid.innerHTML = '';
  body.innerHTML = '';
  if (list) {
    drawListHead();
    for (const scan of shown) body.append(row(scan));
  } else {
    for (const scan of shown) grid.append(card(scan));
  }
  drawMarked();
  drawDetail();
}

// drawTotals sums per currency and never across them: one combined figure
// would need a rate, which is a fact about an instant and is not available
// offline.
function drawTotals(shown) {
  const sums = new Map();
  for (const scan of shown) {
    if (!scan.total) continue;
    const [code, value] = scan.total.split(' ');
    const line = sums.get(code) || { minor: 0, count: 0, decimals: (value.split('.')[1] || '').length };
    const scaled = Math.round(Number(value) * 10 ** line.decimals);
    line.minor += scaled;
    line.count += 1;
    sums.set(code, line);
  }
  // The totals are the reading of what is on screen, so they sit at the right
  // of the bar: per currency a quiet code, the amount as the figure, and how
  // many documents make it up.
  let html = '<span class="sb-label">Amounts</span>';
  if (!sums.size) {
    html += '<span class="sb-note">none in this selection</span>';
  } else {
    html += [...sums.entries()]
      .sort()
      .map(([code, line]) => {
        const amount = (line.minor / 10 ** line.decimals).toFixed(line.decimals);
        const docs = `${line.count} document${line.count === 1 ? '' : 's'}`;
        return `<span class="sb-total" title="${code} ${amount} over ${docs}"><span class="sb-code">${code}</span><span class="sb-figure">${amount}</span><span class="sb-note">×${line.count}</span></span>`;
      })
      .join('');
  }
  statusBar.setHints(html, { html: true });
}

function escapeText(text) {
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// cardTitle prefers what the user wrote: the scan's own description, else
// those of the documents split from it, and the filename only when neither.
function firstPage(range) {
  return Math.min(...pagesOf(range));
}

function cardTitle(scan) {
  if (scan.description) return scan.description;
  if (scan.split) return `${scan.filename} · p ${scan.split.pages}`;
  const described = (scan.documents || []).map((document) => document.description).filter(Boolean);
  return described.length ? described.join(' · ') : scan.filename;
}

function card(scan) {
  const item = document.createElement('li');
  item.className = `card state-${scan.state}`;
  const splitIndex = scan.split?.index ?? null;
  const picked = state.selected === scan.digest && (!scan.split || state.selectedSplit === splitIndex);
  if (picked) item.classList.add('card-selected');
  if (state.marked.has(scan.digest)) item.classList.add('card-marked');
  const fellBack = !scan.eventDate;
  const badges = [];
  if (!scan.reviewed) badges.push('<span class="badge badge-guess">guess</span>');
  if (!scan.typeKnown) badges.push(`<span class="badge badge-warn">unknown type</span>`);
  if (scan.state === 'dead') badges.push('<span class="badge badge-dead">dead</span>');
  if (!scan.documents?.length && scan.needsSplit) badges.push('<span class="badge">split</span>');
  if (!scan.split) badges.push(badgeHTML(handling(scan)));
  badges.push(tagButtons(scan));
  // Every card has the same rows in the same places, empty or not, so a
  // grid of them lines up: title (two lines at most, the full text on hover),
  // type and date on one row, the amount at the right, badges at the foot.
  const title = escapeText(cardTitle(scan));
  const typeName = escapeText(scan.type);
  item.innerHTML =
    `<div class="card-thumb-wrap">` +
    `<img class="card-thumb" loading="lazy" src="${api.image(scan.digest, 'thumb', scan.split ? firstPage(scan.split.pages) : 1)}" alt="">` +
    (state.config.sample
      ? ''
      : `<button type="button" class="card-reveal" title="Show in Finder" aria-label="Show in Finder">` +
        `<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M1.5 3.5h4.5l1.5 1.5h7v8.5h-13z"/></svg></button>`) +
    `</div>` +
    `<div class="card-body">` +
    `<p class="card-title" title="${title}">${title}</p>` +
    `<p class="card-meta"><span class="card-type" title="${typeName}">${typeName}</span>` +
    `${scan.split ? `<span class="card-pages" title="Split ${splitIndex + 1} of ${scan.split.count}">p ${escapeText(scan.split.pages)}</span>` : ''}` +
    `<span class="card-date${fellBack ? ' card-fallback' : ''}"` +
    `${fellBack ? ' title="No event date: sorted by the scan date"' : ''}>${sortDay(scan) || 'no date'}</span></p>` +
    `<p class="card-total">${escapeText(scan.total || '')}</p>` +
    `<p class="card-badges">${badges.join('')}</p>` +
    `</div>`;
  item.querySelector('.card-thumb').addEventListener(
    'error',
    () => item.querySelector('.card-thumb-wrap').classList.add('no-picture'),
    { once: true },
  );
  wireEntry(item, scan, splitIndex);
  return item;
}

// wireEntry gives a card or a list row what a click on either does: the folder
// button shows the file, a tag filters, a click opens, a modified click marks
// and a double-click reads.
function wireEntry(item, scan, splitIndex) {
  item.querySelector('.card-reveal')?.addEventListener('click', async (event) => {
    // Showing the file is not opening the card.
    event.stopPropagation();
    try {
      await api.reveal(scan.digest);
    } catch (error) {
      showError(error);
    }
  });
  for (const badge of item.querySelectorAll('.badge-tag')) {
    badge.addEventListener('click', (event) => {
      // A tag on a card is a filter, not a way of opening the card.
      event.stopPropagation();
      filterTags.toggle(badge.dataset.tag);
    });
  }
  item.addEventListener('click', (event) => {
    // Shift or the platform modifier marks a card for a batch instead of
    // opening it, so a run of scans is one edit rather than three hundred.
    if (event.shiftKey || event.metaKey || event.ctrlKey) {
      if (state.marked.has(scan.digest)) state.marked.delete(scan.digest);
      else state.marked.add(scan.digest);
      draw();
      return;
    }
    state.selected = scan.digest;
    state.selectedSplit = splitIndex;
    draw();
    // A split's card opens the scan at that split, so the detail describes it.
    if (scan.split) pages.go(firstPage(scan.split.pages));
  });
  // A double-click reads what the card stands for: the scan, or its split.
  item.addEventListener('dblclick', (event) => {
    if (event.shiftKey || event.metaKey || event.ctrlKey) return;
    read();
  });
}

// wireListColumns puts a drag line at the right edge of every list heading.
// Dragging sets that column's width; a double-click on the line fits the
// column to its widest cell. Once any width is set the table lays out by the
// headings alone, so every column holds the width given to it. Widths are a
// way of looking and the browser remembers them.
function wireListColumns() {
  const table = el('list');
  const heads = [...table.querySelectorAll('thead th')];
  const keyOf = (th, index) => th.dataset.sort || th.dataset.column || String(index);
  let widths = {};
  try {
    widths = JSON.parse(localStorage.getItem('box.browse.columns') || '{}') || {};
  } catch {}
  const remember = () => {
    try {
      localStorage.setItem('box.browse.columns', JSON.stringify(widths));
    } catch {}
  };
  // Fixing the layout freezes every column at the width it has now, so moving
  // one line does not reflow the others.
  const fix = () => {
    if (table.classList.contains('list-fixed')) return;
    heads.forEach((th, index) => {
      const key = keyOf(th, index);
      if (!widths[key]) widths[key] = th.offsetWidth;
      th.style.width = `${widths[key]}px`;
    });
    table.classList.add('list-fixed');
  };
  const apply = (th, index, width) => {
    widths[keyOf(th, index)] = Math.max(32, Math.round(width));
    th.style.width = `${widths[keyOf(th, index)]}px`;
  };
  if (Object.keys(widths).length) {
    heads.forEach((th, index) => {
      const width = widths[keyOf(th, index)];
      if (width) th.style.width = `${width}px`;
    });
    table.classList.add('list-fixed');
  }
  heads.forEach((th, index) => {
    const grip = document.createElement('span');
    grip.className = 'list-grip';
    grip.title = 'Drag to change the width; double-click to fit';
    th.append(grip);
    // Neither dragging nor fitting is a click on the heading, which sorts.
    grip.addEventListener('click', (event) => event.stopPropagation());
    grip.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      event.stopPropagation();
      fix();
      const startX = event.clientX;
      const startWidth = th.offsetWidth;
      grip.setPointerCapture(event.pointerId);
      table.classList.add('list-resizing');
      const move = (moved) => apply(th, index, startWidth + moved.clientX - startX);
      const end = () => {
        grip.removeEventListener('pointermove', move);
        table.classList.remove('list-resizing');
        remember();
      };
      grip.addEventListener('pointermove', move);
      grip.addEventListener('pointerup', end, { once: true });
      grip.addEventListener('pointercancel', end, { once: true });
    });
    grip.addEventListener('dblclick', (event) => {
      event.stopPropagation();
      fix();
      // A cell's scrollWidth is its content at full length, since cells do not
      // wrap; the heading counts too, so its name and arrow stay whole.
      let widest = th.scrollWidth;
      for (const tr of table.tBodies[0].rows) {
        const cell = tr.cells[index];
        if (cell) widest = Math.max(widest, cell.scrollWidth);
      }
      apply(th, index, widest + 2);
      remember();
    });
  });
}

// drawListHead says on the column heading which order the list is in.
function drawListHead() {
  const key = el('view-sort').value;
  const asc = el('view-direction').value === 'asc';
  for (const th of document.querySelectorAll('#list th[data-sort]')) {
    const on = th.dataset.sort === key;
    th.classList.toggle('list-sorted', on);
    th.setAttribute('aria-sort', on ? (asc ? 'ascending' : 'descending') : 'none');
    th.dataset.arrow = on ? (asc ? '▲' : '▼') : '';
  }
}

function tagButtons(scan) {
  const chosen = new Set(filterTags.get());
  return tagsOf(scan)
    .map((tag) => {
      const on = chosen.has(tag);
      return (
        `<button type="button" class="badge badge-tag${on ? ' badge-tag-on' : ''}" data-tag="${escapeText(tag)}"` +
        ` title="${on ? 'Stop filtering by' : 'Show only cards tagged'} ${escapeText(tag)}">${escapeText(tag)}</button>`
      );
    })
    .join('');
}

// instantText shows a stored instant in the viewer's zone. The sidecar keeps
// it in UTC; which zone reads it is a fact about the viewer, not the scan.
function instantText(stamp) {
  if (!stamp) return '';
  const when = new Date(stamp);
  if (Number.isNaN(when.getTime())) return '';
  const pad = (n) => String(n).padStart(2, '0');
  return `${when.getFullYear()}-${pad(when.getMonth() + 1)}-${pad(when.getDate())} ${pad(when.getHours())}:${pad(when.getMinutes())}`;
}

// row is one scan, or one split, as a line of the list: the same facts a card
// shows, each in its own column, plus producer, pages and the two stamps.
function row(scan) {
  const item = document.createElement('tr');
  item.className = `list-row state-${scan.state}`;
  const splitIndex = scan.split?.index ?? null;
  const picked = state.selected === scan.digest && (!scan.split || state.selectedSplit === splitIndex);
  if (picked) item.classList.add('card-selected');
  if (state.marked.has(scan.digest)) item.classList.add('card-marked');
  const fellBack = !scan.eventDate;
  const flags = [];
  if (!scan.reviewed) flags.push('<span class="badge badge-guess">guess</span>');
  if (!scan.typeKnown) flags.push('<span class="badge badge-warn">unknown type</span>');
  const title = escapeText(cardTitle(scan));
  const pagesText = scan.split ? `p ${scan.split.pages}` : scan.pages || '';
  item.innerHTML =
    `<td class="list-thumb-col"><div class="card-thumb-wrap"><img class="card-thumb" loading="lazy" src="${api.image(scan.digest, 'thumb', scan.split ? firstPage(scan.split.pages) : 1)}" alt=""></div></td>` +
    `<td class="list-title" title="${title}">${title}</td>` +
    `<td>${escapeText(scan.type)} ${flags.join('')}</td>` +
    `<td class="list-date${fellBack ? ' card-fallback' : ''}"${fellBack ? ' title="No event date: the scan date"' : ''}>${sortDay(scan) || 'no date'}</td>` +
    `<td class="list-num">${escapeText(scan.total || '')}</td>` +
    `<td>${escapeText(scan.producer || '')}</td>` +
    `<td class="list-num">${escapeText(pagesText)}</td>` +
    `<td>${escapeText(scan.state || '')}</td>` +
    `<td class="list-tags">${tagButtons(scan)}</td>` +
    `<td class="list-date">${instantText(scan.ingestedAt)}</td>` +
    `<td class="list-date">${instantText(scan.editedAt)}</td>`;
  item.querySelector('.card-thumb').addEventListener(
    'error',
    () => item.querySelector('.card-thumb-wrap').classList.add('no-picture'),
    { once: true },
  );
  wireEntry(item, scan, splitIndex);
  return item;
}

const reader = readerOf();

// read opens the reader on the selected card. A split's card is read as its
// own document, only its pages; a scan's card reads the whole scan.
//
// Reading is a step in the browser's history, so Back closes the reader and
// stays on Browse rather than leaving for whatever page came before. The
// details go with it, beside the pages, so a date or a tag is set while the
// document is in view; closing puts them back.
function read() {
  const scan = selected();
  if (!scan) return;
  const document = state.selectedSplit === null ? null : scan.documents?.[state.selectedSplit];
  const options = { closed: readerClosed };
  if (document?.pages) {
    const page = pages.page();
    Object.assign(options, {
      pages: document.pages,
      page: pagesOf(document.pages).has(page) ? page : null,
      title: `${document.description || scan.description || scan.filename} · p ${document.pages}`,
    });
  } else {
    options.page = pages.page();
  }
  const wasOpen = reader.isOpen();
  reader.open(scan, options);
  lendDetail();
  if (!wasOpen && history.state?.reader !== true) history.pushState({ reader: true }, '', window.location.href);
}

// The details are one element, moved rather than copied, so every field keeps
// its wiring and there is one form to save.
const detailHome = { parent: null, next: null };

function lendDetail() {
  const aside = document.querySelector('aside.detail');
  const body = el('reader-body');
  if (aside.parentElement === body) return;
  detailHome.parent = aside.parentElement;
  detailHome.next = aside.nextSibling;
  const width = getComputedStyle(document.querySelector('.browse-main')).getPropertyValue('--detail-width');
  if (width) body.style.setProperty('--detail-width', width);
  body.classList.add('reader-with-detail');
  body.append(aside);
}

function readerClosed() {
  const aside = document.querySelector('aside.detail');
  if (detailHome.parent && aside.parentElement !== detailHome.parent) {
    detailHome.parent.insertBefore(aside, detailHome.next);
  }
  el('reader-body').classList.remove('reader-with-detail');
  // Closed by Esc or its button rather than by Back: take the step back out
  // of the history, so Back afterwards leaves Browse as it would have.
  if (history.state?.reader === true) history.back();
}

window.addEventListener('popstate', () => {
  if (reader.isOpen()) reader.close();
});

// drawMarked shows what a batch would touch and what it would do to them. The
// batch fields are deliberately the ones that are the same for many documents:
// a description and an amount belong to one.
function drawMarked() {
  el('marked').hidden = state.marked.size === 0;
  el('marked-count').textContent = String(state.marked.size);
}

function markedDigests() {
  return state.scans.filter((scan) => state.marked.has(scan.digest)).map((scan) => scan.digest);
}

function reportBatch(result) {
  const failed = result.failed || [];
  const changed = (result.changed || []).length;
  if (failed.length) {
    statusBar.showError(`${changed} changed, ${failed.length} refused: ${failed[0].error}`);
  } else {
    statusBar.show(`${changed} changed. Every one of them is one line in the log.`);
  }
}

function wireMarked() {
  el('marked-clear').addEventListener('click', () => {
    state.marked.clear();
    draw();
  });
  el('marked-all').addEventListener('click', () => {
    for (const scan of state.scans.filter(matches)) state.marked.add(scan.digest);
    draw();
  });
  el('marked-apply').addEventListener('click', async () => {
    const digests = markedDigests();
    if (!digests.length) return;
    const edit = {};
    if (el('marked-type').value) edit.type = el('marked-type').value;
    if (markedTags.get().length) edit.addTags = markedTags.get();
    if (el('marked-reviewed').checked) edit.reviewed = true;
    if (!Object.keys(edit).length) return;
    let result;
    try {
      result = await api.patchMany(digests, edit);
    } catch (err) {
      showError(err);
      return;
    }
    reportBatch(result);
    markedTags.set([]);
    await reload();
  });
  el('marked-trash').addEventListener('click', async () => {
    const digests = markedDigests();
    if (!digests.length) return;
    // Nothing is deleted: this moves the files into the Box's trash.
    if (!window.confirm(`Move ${digests.length} scans to the trash?`)) return;
    let result;
    try {
      result = await api.trashMany(digests, 'discarded in browse');
    } catch (err) {
      showError(err);
      return;
    }
    state.marked.clear();
    reportBatch(result);
    await reload();
    await drawTrash();
  });
}

function selected() {
  return state.scans.find((scan) => scan.digest === state.selected);
}

// One fact per row, label on the left, so a filename and a digest are read
// on their own instead of run together.
function drawFacts(rows) {
  const list = el('detail-facts');
  list.innerHTML = '';
  for (const [label, value] of rows) {
    if (!value) continue;
    const term = document.createElement('dt');
    term.textContent = label;
    const detail = document.createElement('dd');
    detail.textContent = value;
    list.append(term, detail);
  }
}

function drawDetail() {
  const scan = selected();
  el('detail').hidden = !scan;
  el('detail-empty').hidden = Boolean(scan);
  const fresh = state.shown !== (scan?.digest ?? null);
  if (fresh) {
    state.shown = scan?.digest ?? null;
    state.draft = {};
    split.reset();
  }
  pages.show(scan);
  if (!scan) return;
  el('detail-unfile').hidden = !scan.unfilable;
  drawFacts([
    ['File', scan.filename],
    ['Pages', `${scan.pages}${scan.pageSize ? ` · ${scan.pageSize}` : ''}`],
    ['Producer', scan.producer],
    ['Scanned', `${localDate(scan.scannedAt)} ${localTime(scan.scannedAt)} ${zoneOf(scan.scannedAt)}`],
    ['Filed', scan.ingestedAt ? localDate(scan.ingestedAt) : ''],
    ['SHA-256', `${scan.digest}…`],
  ]);
  if (!state.types.some((type) => type.name === scan.type)) {
    // A type this build does not register is kept as it is and shown, never
    // rewritten to something else.
    el('detail-type').append(new Option(`${scan.type} (unknown here)`, scan.type));
  }
  if (fresh) drawFields(true);
  else drawFields();
  const expiry = scan.expiry
    ? `Expires ${scan.expiry} — ${scan.state}.`
    : scan.expiryCleared
      ? 'Kept for good — the expiry was cleared by hand.'
      : `No expiry — ${scan.state}.`;
  el('detail-expiry').textContent = expiry;
  showError(null);
}

// showError puts an error in the status bar, or takes the last one away.
function showError(err) {
  statusBar.error(err);
}

async function save(extra = {}) {
  const scan = selected();
  if (!scan) return;
  const documents = state.draft.documents ?? scan.documents ?? [];
  // Pages in no split stop a save once, as they stop filing at intake.
  if (documents.length && !split.readyToFile()) return;
  let edit;
  if (documents.length) {
    // Each split carries its own fields; the file says only what it is.
    edit = {
      digest: scan.digest,
      type: 'unsorted',
      description: '',
      eventDate: '',
      eventZone: '',
      total: '',
      tags: [],
      documents: documents.map((document) => ({
        ...document,
        total: boxTotalForSave(document.total),
        eventZone: document.eventZone || (document.eventDate ? state.config.zone || '' : ''),
      })),
      ignoredPages: state.draft.ignoredPages ?? scan.ignoredPages ?? '',
      ...extra,
    };
  } else {
    edit = {
      digest: scan.digest,
      type: el('detail-type').value,
      description: el('detail-description').value,
      eventDate: el('detail-event-date').value,
      eventZone: el('detail-event-zone').value,
      expiresAt: el('detail-expires').value,
      total: boxTotalForSave(el('detail-total').value),
      tags: detailTags.get(),
      ...(state.draft.documents ? { documents: [], ignoredPages: '' } : {}),
      ...extra,
    };
  }
  try {
    const saved = await api.patch(edit);
    if (edit.total && edit.total !== scan.total) rememberBoxTotal(saved.total);
    (saved.documents || []).forEach((document, index) => {
      if (edit.documents?.[index]?.total !== scan.documents?.[index]?.total) rememberBoxTotal(document.total);
    });
  } catch (err) {
    showError(err);
    return;
  }
  // What was saved is what the detail shows next.
  state.shown = null;
  await reload();
}

function wireDetail() {
  // An event already happened; an expiry printed on a document may be years out.
  dateField(el('detail-event-date'), { notFuture: true, partial: true });
  dateField(el('detail-expires'));
  enterMovesOn(el('detail-event-date').closest('form'));
  // The detail has its own short Tab ring. The page itself is the browsing
  // position, where the document and split shortcuts work.
  el('detail-type').tabIndex = -1;
  const browseFocus = document.querySelector('.browse-main');
  browseFocus.tabIndex = -1;
  const fields = [
    el('detail-event-date').nextElementSibling.querySelector('input'),
    el('detail-event-zone'),
    el('detail-description'),
    el('detail-total'),
    detailTags.input,
  ];
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Tab' || !selected() || reader.isOpen()
        || event.metaKey || event.ctrlKey || event.altKey) return;
    const active = document.activeElement;
    const index = fields.findIndex((field) => field === active
      || (field === fields[0] && field.parentElement.contains(active)));
    if (index < 0 && active !== document.body && active !== browseFocus) return;
    if (event.isComposing || event.keyCode === 229 || (active === fields[1] && zones.isComposing())) {
      // Tab belongs to the input method until its composition is committed.
      event.preventDefault();
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (active === fields[1] && !event.shiftKey) {
      zones.commit().then((chosen) => {
        if ((chosen || !active.value.trim()) && document.activeElement === active) fields[2].focus();
      });
      return;
    }
    if (active === detailTags.input && active.value.trim()) detailTags.commit();
    const next = index < 0 ? (event.shiftKey ? fields.length - 1 : 0)
      : index + (event.shiftKey ? -1 : 1);
    if (next < 0 || next === fields.length) {
      // Keep the browsing state as an explicit focus target. Blurring a field
      // alone lets the browser continue its native Tab order on some engines.
      browseFocus.focus();
    } else {
      fields[next].focus();
      fields[next].select?.();
    }
  }, true);
  // A field typed into while a split is picked describes that split.
  for (const [id, name] of Object.entries(DETAIL)) {
    const input = el(id);
    input.addEventListener(input.tagName === 'SELECT' ? 'change' : 'input', () => {
      const document = split.editable();
      if (!document || id === 'detail-event-zone') return;
      document[name] = input.value;
    });
  }
  el('detail-event-zone').addEventListener('change', () => {
    const document = split.editable();
    if (document) document.eventZone = el('detail-event-zone').value.trim();
  });
  el('detail-event-zone').addEventListener('keydown', (event) => zones.key(event));
  el('split-mark').addEventListener('click', () => split.markOrClose());
  el('split-ignore').addEventListener('click', () => split.ignore());
  el('detail-read').addEventListener('click', () => read());
  el('split-type').addEventListener('click', () => split.openText());
  // The split keys work whenever the detail is open and nothing is being
  // typed; browsing is otherwise done with the mouse.
  document.addEventListener('keydown', (event) => {
    if (!selected() || event.metaKey || event.ctrlKey || event.altKey) return;
    // In the reader its own keys rule; the details beside it are for typing.
    if (reader.isOpen()) return;
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) return;
    // Enter on a focused button presses that button; anywhere else it reads.
    if (event.key === 'Enter' && document.activeElement?.tagName !== 'BUTTON') {
      event.preventDefault();
      read();
      return;
    }
    const actions = {
      ' ': () => split.markOrClose(),
      x: () => split.ignore(),
      ':': () => split.openText(),
      '-': () => split.remove(),
      ArrowLeft: () => pages.step(-1),
      ArrowRight: () => pages.step(1),
      ArrowUp: () => pages.step(-1),
      ArrowDown: () => pages.step(1),
      Escape: () => split.cancel(),
    };
    const action = actions[event.key];
    if (!action || (event.key !== 'Escape' && !split.applies())) return;
    event.preventDefault();
    action();
  });
  el('detail-redraw').addEventListener('click', async () => {
    const scan = selected();
    if (!scan) return;
    try {
      const result = await api.redraw(scan.digest);
      if (result.failed.length) {
        statusBar.showError(`Could not redraw: ${result.failed[0].detail}`);
      } else {
        statusBar.show('Thumbnail drawn again from the file.');
      }
    } catch (err) {
      showError(err);
      return;
    }
    state.shown = null;
    await reload();
  });
  el('detail-unfile').addEventListener('click', async () => {
    const scan = selected();
    if (!scan) return;
    // Nothing is deleted: the Box's copy goes to the trash, and the inbox
    // file it came from is waiting at intake again with this description.
    if (!window.confirm(`Take "${scan.description || scan.filename}" back to intake? Its copy in the Box moves to the trash; the inbox file is described again as it is now.`)) return;
    try {
      await api.unfile(scan.digest);
    } catch (err) {
      showError(err);
      return;
    }
    state.selected = null;
    await reload();
    await drawTrash();
  });
  el('detail-save').addEventListener('click', () => save());
  el('detail-confirm').addEventListener('click', () => save({ reviewed: true }));
  el('detail-keep').addEventListener('click', () => save({ expiryCleared: true, expiresAt: '' }));
  el('detail-trash').addEventListener('click', async () => {
    const scan = selected();
    if (!scan) return;
    // Nothing is deleted: this moves the file into the Box's trash, where it
    // stays until someone empties it by hand.
    if (!window.confirm(`Move "${scan.description || scan.filename}" to the trash?`)) return;
    try {
      await api.trash(scan.digest, 'discarded in browse');
    } catch (err) {
      showError(err);
      return;
    }
    state.selected = null;
    await reload();
  });
}

function wireFilters() {
  for (const id of [
    'filter-year',
    'filter-type',
    'filter-state',
    'filter-reviewed',
    'filter-currency',
    'filter-producer',
    'filter-zone',
    'filter-deferred',
  ]) {
    el(id).addEventListener('change', () => filtersChanged());
  }
  wireTagStrip();
  const text = el('filter-text');
  try {
    text.value = new URLSearchParams(location.search).get('q') || '';
  } catch {}
  text.addEventListener('input', () => filtersChanged());
  // Esc in the search empties it before it does anything else.
  text.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && text.value) {
      event.stopPropagation();
      text.value = '';
      filtersChanged();
    }
  });
  // Showing splits as documents is a way of looking, not a filter: Clear
  // leaves it, and the browser remembers it.
  const splits = el('view-splits');
  try {
    splits.checked = localStorage.getItem('box.browse.splits') === '1';
  } catch {}
  const sort = el('view-sort');
  try {
    sort.value = localStorage.getItem('box.browse.sort') || 'event';
  } catch {}
  if (!sort.value) sort.value = 'event';
  sort.addEventListener('change', () => {
    try {
      localStorage.setItem('box.browse.sort', sort.value);
    } catch {}
    draw();
  });
  const direction = el('view-direction');
  const layout = el('view-layout');
  try {
    direction.value = localStorage.getItem('box.browse.direction') || 'desc';
    layout.value = localStorage.getItem('box.browse.layout') || 'grid';
  } catch {}
  if (!direction.value) direction.value = 'desc';
  if (!layout.value) layout.value = 'grid';
  for (const [control, name] of [[direction, 'direction'], [layout, 'layout']]) {
    control.addEventListener('change', () => {
      try {
        localStorage.setItem(`box.browse.${name}`, control.value);
      } catch {}
      draw();
    });
  }
  wireListColumns();
  // A column heading sorts by that column; a second click turns the order.
  for (const th of document.querySelectorAll('#list th[data-sort]')) {
    th.addEventListener('click', () => {
      if (sort.value === th.dataset.sort) {
        direction.value = direction.value === 'asc' ? 'desc' : 'asc';
      } else {
        sort.value = th.dataset.sort;
        direction.value = newestFirst.has(sort.value) ? 'desc' : 'asc';
      }
      try {
        localStorage.setItem('box.browse.sort', sort.value);
        localStorage.setItem('box.browse.direction', direction.value);
      } catch {}
      draw();
    });
  }
  if (splits.checked || sort.value !== 'event' || direction.value !== 'desc' || layout.value !== 'grid') draw();
  splits.addEventListener('change', () => {
    try {
      localStorage.setItem('box.browse.splits', splits.checked ? '1' : '0');
    } catch {}
    state.selectedSplit = null;
    draw();
  });
  el('filters-clear').addEventListener('click', () => {
    for (const select of document.querySelectorAll('.filters select:not([id^="view-"])')) select.value = '';
    filterTags.set([]);
    el('filter-text').value = '';
    filtersChanged();
  });
}

// A filter that is narrowing the list is tinted, and Clear shows only then.
function markActiveFilters() {
  let any = false;
  for (const select of document.querySelectorAll('.filters select:not([id^="view-"])')) {
    const active = select.value !== '';
    select.closest('.filter').classList.toggle('filter-active', active);
    any ||= active;
  }
  const searched = el('filter-text').value.trim() !== '';
  el('filter-text').closest('.filter').classList.toggle('filter-active', searched);
  any ||= searched;
  const tagged = filterTags.get().length > 0;
  el('filter-tags').closest('.filter').classList.toggle('filter-active', tagged);
  el('filters-clear').hidden = !(any || tagged);
}

function filtersChanged() {
  markActiveFilters();
  writeTagsToURL();
  draw();
}

// The chosen tags are in the address, ?tag=japan-2019&tag=keep, so a view of
// the Box by tag can be kept as a bookmark and survives a reload.
function readTagsFromURL() {
  filterTags.set(new URLSearchParams(window.location.search).getAll('tag'));
  markActiveFilters();
}

function writeTagsToURL() {
  const params = new URLSearchParams(window.location.search);
  params.delete('tag');
  for (const tag of filterTags.get()) params.append('tag', tag);
  const words = el('filter-text').value.trim();
  if (words) params.set('q', words);
  else params.delete('q');
  const query = params.toString();
  history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`);
}

// countFacets is, for each tag, how many of the cards shown carry it. With the
// filter's AND, that is how many would be left if the tag were added.
function countFacets(shown) {
  state.facets = new Map();
  for (const entry of shown) {
    for (const tag of tagsOf(entry)) state.facets.set(tag, (state.facets.get(tag) ?? 0) + 1);
  }
}

// The tag strip is every tag in what is shown, most carried first, each a
// switch for the filter. A chosen tag stays in the strip, lit, so it can be
// switched off where it was switched on. It is open or closed as it was left.
const TAG_STRIP = 'box.browse.tagStrip';

function wireTagStrip() {
  let open = false;
  try {
    open = localStorage.getItem(TAG_STRIP) === '1';
  } catch {}
  const toggle = el('tag-strip-toggle');
  const apply = () => {
    el('tag-strip').hidden = !open;
    toggle.setAttribute('aria-expanded', String(open));
    toggle.textContent = open ? 'hide tags' : 'all tags';
  };
  apply();
  toggle.addEventListener('click', () => {
    open = !open;
    try {
      localStorage.setItem(TAG_STRIP, open ? '1' : '0');
    } catch {}
    apply();
  });
}

function drawTagStrip() {
  const strip = el('tag-strip');
  const chosen = filterTags.get();
  const uses = [...state.facets.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  strip.innerHTML = '';
  if (!uses.length) {
    const none = document.createElement('span');
    none.className = 'tag-strip-empty';
    none.textContent = state.tags.length ? 'No tags on the cards shown.' : 'No scan in the Box has a tag yet.';
    strip.append(none);
    return;
  }
  for (const [name, count] of uses) {
    const on = chosen.includes(name);
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `tag-facet${on ? ' tag-facet-on' : ''}`;
    button.setAttribute('aria-pressed', String(on));
    button.title = on ? `Stop filtering by ${name}` : `Show only the ${count} tagged ${name}`;
    const label = document.createElement('span');
    label.textContent = name;
    const figure = document.createElement('span');
    figure.className = 'tag-facet-count';
    figure.textContent = String(count);
    button.append(label, figure);
    button.addEventListener('click', () => filterTags.toggle(name));
    strip.append(button);
  }
}

// drawExceptionCount is the only trace of the exceptions on this page. They
// are rare, and a band shown every day is a band nobody reads, so browse only
// says how many there are and links to /check/, and says nothing when none.
async function drawExceptionCount() {
  const body = await api.exceptions();
  const count = body.exceptions.length;
  el('check-link').hidden = count === 0;
  el('check-count').textContent = String(count);
}

start().catch((err) => showError(err));

// The details are as wide as the person drags them, measured from the right
// edge. Both sides keep a minimum so neither gets squeezed past use. The width
// is a per-browser convenience, so it lives in localStorage.
const DETAIL_WIDTH = { key: 'dgs.box.detail.width', min: 320, fallback: 380, step: 24, leftMin: 420 };

function wireDetailResize() {
  const main = document.querySelector('.browse-main');
  const handle = el('detail-resize');
  const max = () => Math.max(DETAIL_WIDTH.min, main.clientWidth - DETAIL_WIDTH.leftMin - handle.offsetWidth);
  const apply = (width, keep) => {
    const clamped = Math.round(Math.min(Math.max(width, DETAIL_WIDTH.min), max()));
    main.style.setProperty('--detail-width', `${clamped}px`);
    handle.setAttribute('aria-valuenow', String(clamped));
    handle.setAttribute('aria-valuemin', String(DETAIL_WIDTH.min));
    handle.setAttribute('aria-valuemax', String(max()));
    if (keep) {
      try {
        localStorage.setItem(DETAIL_WIDTH.key, String(clamped));
      } catch {
        // Kept for this page load only.
      }
    }
    return clamped;
  };
  let saved = NaN;
  try {
    saved = Number(localStorage.getItem(DETAIL_WIDTH.key));
  } catch {
    // Nothing stored is the default width.
  }
  let width = apply(Number.isFinite(saved) && saved > 0 ? saved : DETAIL_WIDTH.fallback, false);

  handle.addEventListener('pointerdown', (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    handle.setPointerCapture(event.pointerId);
    const right = main.getBoundingClientRect().right;
    document.body.classList.add('resizing');
    const follow = (move) => {
      width = apply(right - move.clientX, false);
    };
    const finish = () => {
      handle.removeEventListener('pointermove', follow);
      document.body.classList.remove('resizing');
      width = apply(width, true);
    };
    handle.addEventListener('pointermove', follow);
    handle.addEventListener('pointerup', finish, { once: true });
    handle.addEventListener('pointercancel', finish, { once: true });
  });
  handle.addEventListener('dblclick', () => {
    width = apply(DETAIL_WIDTH.fallback, true);
  });
  handle.addEventListener('keydown', (event) => {
    const delta = { ArrowLeft: DETAIL_WIDTH.step, ArrowRight: -DETAIL_WIDTH.step }[event.key];
    if (!delta) return;
    event.preventDefault();
    event.stopPropagation();
    width = apply(width + delta, true);
  });
  window.addEventListener('resize', () => {
    width = apply(width, false);
  });
}
