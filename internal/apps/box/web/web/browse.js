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
  // marked is the set of digests a batch would be given to. Batch editing is
  // necessary — three hundred scans one at a time is an hour nobody finishes —
  // and it is also the only way to get three hundred records wrong at once,
  // which is what the log is for.
  marked: new Set(),
};

const el = (id) => document.getElementById(id);

const pages = pageView({
  strip: 'detail-page-strip',
  toggle: 'detail-pages-toggle',
  image: 'detail-image',
  label: 'detail-page-label',
});

async function start() {
  state.config = await api.config();
  el('sample-banner').hidden = !state.config.sample;
  el('paths').textContent = state.config.root || '';
  const types = await api.types();
  state.types = types.types;
  fillTypeOptions();
  await reload();
  wireFilters();
  wireDetail();
  wireMarked();
  el('verify').addEventListener('click', runVerify);
  await drawExceptions();
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
  const line = el('trash-line');
  if (!summary.count) {
    line.hidden = true;
    return;
  }
  line.hidden = false;
  const parts = [`${summary.count} in the trash`];
  if (summary.oldest) parts.push(`oldest thrown away ${summary.oldest}`);
  if (summary.keepDays && summary.overdue) {
    parts.push(
      `${summary.overdue} older than the ${summary.keepDays} days you keep — empty trash/ by hand when you are sure`,
    );
  }
  line.textContent = `${parts.join(' · ')}.`;
}

function fillTypeOptions() {
  for (const type of state.types) {
    el('filter-type').append(new Option(type.label, type.name));
    el('detail-type').append(new Option(`${type.label} (${type.name})`, type.name));
    el('marked-type').append(new Option(type.label, type.name));
    el('adopt-type').append(new Option(type.label, type.name));
  }
}

async function reload() {
  const body = await api.scans();
  state.scans = body.scans;
  state.totals = body.totals;
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

function draw() {
  const shown = state.scans.filter(matches);
  el('count').textContent = `${shown.length} of ${state.scans.length}`;
  el('grid-empty').hidden = shown.length > 0;
  drawTotals(shown);
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
  if (!sums.size) {
    el('totals').textContent = 'No amounts in this selection.';
    return;
  }
  el('totals').innerHTML = [...sums.entries()]
    .sort()
    .map(([code, line]) => {
      const amount = (line.minor / 10 ** line.decimals).toFixed(line.decimals);
      return `<span class="total"><strong>${code} ${amount}</strong> <span class="total-count">${line.count}</span></span>`;
    })
    .join('');
}

function card(scan) {
  const item = document.createElement('li');
  item.className = `card state-${scan.state}`;
  if (state.selected === scan.digest) item.classList.add('card-selected');
  if (state.marked.has(scan.digest)) item.classList.add('card-marked');
  const fellBack = !scan.eventDate;
  const badges = [];
  if (!scan.reviewed) badges.push('<span class="badge badge-guess">guess</span>');
  if (!scan.typeKnown) badges.push(`<span class="badge badge-warn">unknown type</span>`);
  if (scan.state === 'dead') badges.push('<span class="badge badge-dead">dead</span>');
  if (scan.needsSplit) badges.push('<span class="badge">split</span>');
  if (scan.group) badges.push(`<span class="badge">group</span>`);
  item.innerHTML =
    `<img class="card-thumb" loading="lazy" src="${api.image(scan.digest, 'thumb')}" alt="">` +
    `<div class="card-body">` +
    `<p class="card-title">${scan.description || scan.filename}</p>` +
    `<p class="card-meta">${scan.type} · ${sortDay(scan) || 'no date'}` +
    `${fellBack ? ' <span class="card-fallback">(scan date)</span>' : ''}</p>` +
    `<p class="card-meta">${scan.total || ''}</p>` +
    `<p class="card-badges">${badges.join('')}</p>` +
    `</div>`;
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
    draw();
  });
  return item;
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
  const line = el('marked-result');
  const failed = result.failed || [];
  const changed = (result.changed || []).length;
  line.hidden = false;
  line.textContent = failed.length
    ? `${changed} changed, ${failed.length} refused: ${failed[0].error}`
    : `${changed} changed. Every one of them is one line in the log.`;
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
    if (el('marked-group').value.trim()) edit.group = el('marked-group').value.trim();
    if (el('marked-tags').value.trim()) {
      edit.tags = el('marked-tags')
        .value.split(',')
        .map((tag) => tag.trim())
        .filter(Boolean);
    }
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

function drawDetail() {
  const scan = selected();
  el('detail').hidden = !scan;
  el('detail-empty').hidden = Boolean(scan);
  pages.show(scan);
  if (!scan) return;
  el('detail-facts').textContent = [
    scan.filename,
    scan.pages === 1 ? '1 page' : `${scan.pages} pages`,
    scan.pageSize,
    scan.producer,
    `scanned ${localDate(scan.scannedAt)} ${localTime(scan.scannedAt)} ${zoneOf(scan.scannedAt)}`,
    scan.ingestedAt ? `filed ${localDate(scan.ingestedAt)}` : '',
    `sha256:${scan.digest}…`,
  ]
    .filter(Boolean)
    .join(' · ');
  if (!state.types.some((type) => type.name === scan.type)) {
    // A type this build does not register is kept as it is and shown, never
    // rewritten to something else.
    el('detail-type').append(new Option(`${scan.type} (unknown here)`, scan.type));
  }
  el('detail-type').value = scan.type;
  el('detail-description').value = scan.description || '';
  el('detail-event-date').value = scan.eventDate || '';
  el('detail-event-zone').value = scan.eventZone || '';
  el('detail-expires').value = scan.expiresAt || '';
  el('detail-total').value = scan.total || '';
  el('detail-tags').value = (scan.tags || []).join(', ');
  const expiry = scan.expiry
    ? `Expires ${scan.expiry} — ${scan.state}.`
    : scan.expiryCleared
      ? 'Kept for good — the expiry was cleared by hand.'
      : `No expiry — ${scan.state}.`;
  el('detail-expiry').textContent = expiry;
  showError(null);
}

function showError(err) {
  const line = el('detail-error');
  line.hidden = !err;
  line.textContent = err ? err.message || String(err) : '';
}

async function save(extra = {}) {
  const scan = selected();
  if (!scan) return;
  const tags = el('detail-tags')
    .value.split(',')
    .map((tag) => tag.trim())
    .filter(Boolean);
  try {
    await api.patch({
      digest: scan.digest,
      type: el('detail-type').value,
      description: el('detail-description').value,
      eventDate: el('detail-event-date').value,
      eventZone: el('detail-event-zone').value,
      expiresAt: el('detail-expires').value,
      total: el('detail-total').value,
      tags,
      ...extra,
    });
  } catch (err) {
    showError(err);
    return;
  }
  await reload();
}

function wireDetail() {
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
    el(id).addEventListener('change', draw);
  }
  el('filters-clear').addEventListener('click', () => {
    for (const select of document.querySelectorAll('.filters select')) select.value = '';
    draw();
  });
}

async function drawExceptions() {
  const body = await api.exceptions();
  drawExceptionList(body.exceptions);
}

function drawExceptionList(exceptions) {
  const list = el('exceptions');
  list.innerHTML = '';
  el('exceptions-empty').hidden = exceptions.length > 0;
  for (const exception of exceptions) {
    const item = document.createElement('li');
    item.className = `exception exception-${exception.kind.replace(/\s+/g, '-')}`;
    item.innerHTML =
      `<span class="exception-kind">${exception.kind}</span>` +
      `<span class="exception-path">${exception.path}</span>` +
      `<span class="exception-detail">${exception.detail}</span>`;
    if (exception.adoptable) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'text-button';
      button.textContent = 'Adopt';
      button.addEventListener('click', () => adopt(exception.path, button));
      item.append(button);
    }
    list.append(item);
  }
}

// adopt takes in a scan that is already in the tree with no sidecar. It is
// described where it lies: its directory records an intake date that adopting
// does not get to rewrite, so nothing is copied and nothing is re-filed.
async function adopt(path, button) {
  const type = el('adopt-type').value;
  button.disabled = true;
  try {
    await api.adopt(path, type ? { type } : {});
  } catch (err) {
    showError(err);
    button.disabled = false;
    return;
  }
  await reload();
  await drawExceptions();
}

// runVerify reads every byte of every file in the Box. It is behind a button
// because it costs tens of gigabytes over a network filesystem, and it repairs
// nothing: a mismatch is reported with both digests and left exactly as it is.
async function runVerify() {
  const button = el('verify');
  button.disabled = true;
  el('verify-note').textContent = 'Reading every byte. This takes a while.';
  try {
    const body = await api.verify();
    const found = await api.exceptions();
    drawExceptionList([...found.exceptions, ...body.mismatches]);
    el('verify-note').textContent = body.count
      ? `${body.count} files no longer match their recorded digest. Nothing was rewritten.`
      : 'Every file still matches the digest recorded for it.';
  } catch (err) {
    el('verify-note').textContent = err.message || String(err);
  }
  button.disabled = false;
}

start().catch((err) => showError(err));
