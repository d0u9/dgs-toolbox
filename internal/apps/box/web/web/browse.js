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
  for (const scan of state.scans) {
    const year = sortDay(scan).slice(0, 4);
    if (year) years.add(year);
    if (scan.total) currencies.add(scan.total.split(' ')[0]);
    if (scan.producer) producers.add(scan.producer);
  }
  refill('filter-year', [...years].sort().reverse());
  refill('filter-currency', [...currencies].sort());
  refill('filter-producer', [...producers].sort());
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

function matches(scan) {
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
      total: document.total || '',
      tags: [...(scan.tags || []), ...(document.tags || [])],
      documents: [],
      split: { index, pages: document.pages, count: scan.documents.length },
    }));
  });
}

function draw() {
  const all = entries();
  const shown = all.filter(matches);
  el('count').textContent = `${shown.length} of ${all.length}`;
  el('grid-empty').hidden = shown.length > 0;
  drawTotals(shown);
  countFacets(shown);
  drawTagStrip();
  const grid = el('grid');
  grid.innerHTML = '';
  for (const scan of shown) {
    grid.append(card(scan));
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
  const chosen = new Set(filterTags.get());
  for (const tag of tagsOf(scan)) {
    const on = chosen.has(tag);
    badges.push(
      `<button type="button" class="badge badge-tag${on ? ' badge-tag-on' : ''}" data-tag="${escapeText(tag)}"` +
        ` title="${on ? 'Stop filtering by' : 'Show only cards tagged'} ${escapeText(tag)}">${escapeText(tag)}</button>`,
    );
  }
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
  return item;
}

const reader = readerOf();

// read opens the reader on the selected card. A split's card is read as its
// own document, only its pages; a scan's card reads the whole scan.
function read() {
  const scan = selected();
  if (!scan) return;
  const document = state.selectedSplit === null ? null : scan.documents?.[state.selectedSplit];
  if (!document?.pages) {
    reader.open(scan, { page: pages.page() });
    return;
  }
  const page = pages.page();
  reader.open(scan, {
    pages: document.pages,
    page: pagesOf(document.pages).has(page) ? page : null,
    title: `${document.description || scan.description || scan.filename} · p ${document.pages}`,
  });
}

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
      total: el('detail-total').value,
      tags: detailTags.get(),
      ...(state.draft.documents ? { documents: [], ignoredPages: '' } : {}),
      ...extra,
    };
  }
  try {
    await api.patch(edit);
  } catch (err) {
    showError(err);
    return;
  }
  // What was saved is what the detail shows next.
  state.shown = null;
  await reload();
}

function wireDetail() {
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
    'filter-deferred',
  ]) {
    el(id).addEventListener('change', () => filtersChanged());
  }
  wireTagStrip();
  // Showing splits as documents is a way of looking, not a filter: Clear
  // leaves it, and the browser remembers it.
  const splits = el('view-splits');
  try {
    splits.checked = localStorage.getItem('box.browse.splits') === '1';
  } catch {}
  if (splits.checked) draw();
  splits.addEventListener('change', () => {
    try {
      localStorage.setItem('box.browse.splits', splits.checked ? '1' : '0');
    } catch {}
    state.selectedSplit = null;
    draw();
  });
  el('filters-clear').addEventListener('click', () => {
    for (const select of document.querySelectorAll('.filters select')) select.value = '';
    filterTags.set([]);
    filtersChanged();
  });
}

// A filter that is narrowing the list is tinted, and Clear shows only then.
function markActiveFilters() {
  let any = false;
  for (const select of document.querySelectorAll('.filters select')) {
    const active = select.value !== '';
    select.closest('.filter').classList.toggle('filter-active', active);
    any ||= active;
  }
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
