// The intake page. One scan, or one run of scans, fills the screen; a single
// key sets the type; two fields take the date and the description; Enter files
// it and moves on. Nothing here needs the mouse, because this is several
// hundred repetitions and a hand leaving the keyboard is a real cost.

const state = {
  config: {},
  types: [],
  pending: [],
  cursor: 0,
  // anchor is where a Shift+Arrow run started. A run is how a batch of scans
  // from one sitting is given one type and, optionally, the same tags at once.
  anchor: null,
  draft: {},
};

const el = (id) => document.getElementById(id);

const pages = pageView(
  {
    strip: 'page-strip',
    toggle: 'pages-toggle',
    image: 'page-image',
    label: 'page-label',
  },
  (item, page) => split.decorate(item, page),
  // Turning to a page picks the split it is in, so the desk follows the eye.
  (page) => {
    const before = split.active();
    split.follow(page);
    if (split.active() !== before) drawDesk();
    else split.drawHead();
  },
);

const split = splitter({
  pages,
  scan: () => current(),
  draft: () => state.draft,
  showError,
  say,
  changed: () => {
    draw();
    saveSoon();
  },
});

// FIELDS maps the desk's inputs to the names a scan and a split share.
const FIELDS = {
  'event-date': 'eventDate',
  'event-zone': 'eventZone',
  description: 'description',
  total: 'total',
  tags: 'tags',
};

// value reads one field of whatever the desk is describing: the split that
// is picked, or the scan as a whole. A split is described as if it were a PDF
// of its own, so it never shows the scan's values.
function value(name) {
  const document = split.active();
  if (document) {
    if (name === 'eventZone') return document.eventZone || state.config.zone || '';
    return document[name] ?? (name === 'tags' ? [] : '');
  }
  const scan = current();
  if (name === 'eventZone') return state.draft.eventZone ?? scan?.eventZone ?? state.config.zone ?? '';
  return state.draft[name] ?? scan?.[name] ?? (name === 'tags' ? [] : '');
}

function setValue(name, text) {
  const document = split.editable();
  const target = document ?? state.draft;
  target[name] = name === 'tags' ? parseTags(text) : text;
}

async function start() {
  state.config = await api.config();
  el('sample-banner').hidden = !state.config.sample;
  el('paths').textContent = [state.config.inbox, '→', state.config.root]
    .filter(Boolean)
    .join(' ');
  el('currency-hint').textContent = state.config.currency
    ? `default ${state.config.currency}`
    : 'name the currency';
  const types = await api.types();
  state.types = types.types;
  assertTypeKeysAreFree();
  wireQueueResize();
  drawTypes();
  await reload();
  wireKeys();
  wireFields();
  wireBatch();
  wireDuplicates();
  await drawHeldBack();
  await drawRejected();
}

// showQueueTab switches the sidebar between the inbox and what was rejected.
// Rejected is rarely wanted, so it waits behind a tab instead of taking room
// under the queue.
function showQueueTab(name) {
  el('tab-inbox').setAttribute('aria-selected', String(name === 'inbox'));
  el('tab-rejected').setAttribute('aria-selected', String(name === 'rejected'));
  el('inbox-panel').hidden = name !== 'inbox';
  el('rejected').hidden = name !== 'rejected';
}

// drawRejected lists what was turned away, so a Backspace pressed by mistake
// is one click to undo. Rejecting never touched the file, so restoring cannot
// lose anything either.
async function drawRejected() {
  let body;
  try {
    body = await api.rejected();
  } catch (err) {
    showError(err);
    return;
  }
  el('rejected-empty').hidden = body.count !== 0;
  el('rejected-count').textContent = String(body.count);
  const list = el('rejected-list');
  list.innerHTML = '';
  for (const item of body.rejected) {
    const entry = document.createElement('li');
    entry.className = 'rejected-item';
    // The file opens in the browser's own viewer: every page of it, as it is
    // in the inbox, without putting it back in the list first.
    const view = document.createElement('a');
    view.className = 'rejected-view';
    view.href = api.rejectedFile(item.digest);
    view.target = '_blank';
    view.rel = 'noopener';
    view.title = `Open ${item.filename}`;
    const name = document.createElement('span');
    name.className = 'rejected-name';
    name.textContent = item.filename;
    const detail = document.createElement('span');
    detail.className = 'rejected-detail';
    detail.textContent = [item.reason, localDate(item.at)].filter(Boolean).join(' · ');
    view.append(name, detail);
    const restore = document.createElement('button');
    restore.type = 'button';
    restore.className = 'rejected-restore';
    restore.textContent = 'Restore';
    restore.title = 'Put it back in the inbox list';
    restore.addEventListener('click', async () => {
      try {
        await api.restore(item.digest);
      } catch (err) {
        showError(err);
        return;
      }
      await reload();
      const back = state.pending.findIndex((scan) => scan.digest === item.digest);
      if (back >= 0) {
        state.cursor = back;
        draw();
      }
      await drawRejected();
    });
    entry.append(view, restore);
    list.append(entry);
  }
}

// drawHeldBack reports what was read and not taken in: truncated files, bad
// copies, damaged old backups. A skip nobody is told about means believing the
// inbox is empty when it is not, so this area is drawn even when it is empty
// only in the sense of being hidden.
async function drawHeldBack() {
  let body;
  try {
    body = await api.incomplete();
  } catch (err) {
    showError(err);
    return;
  }
  el('held-back').hidden = body.count === 0;
  el('held-back-count').textContent = String(body.count);
  const list = el('held-back-list');
  list.innerHTML = '';
  for (const item of body.incomplete) {
    const entry = document.createElement('li');
    entry.className = 'exception exception-incomplete';
    entry.innerHTML =
      `<span class="exception-path">${item.path}</span>` +
      `<span class="exception-detail">${item.detail}</span>`;
    list.append(entry);
  }
}

// reload re-reads the inbox. `at` is where the cursor should land — the place
// the scans just dealt with came from, so the next one to look at is the one
// that moved up into that slot rather than whatever happened to be under the
// cursor.
async function reload(at) {
  const body = await api.intake();
  state.pending = body.pending;
  const wanted = at ?? state.cursor;
  state.cursor = Math.min(Math.max(0, wanted), Math.max(0, state.pending.length - 1));
  state.anchor = null;
  state.draft = {};
  draw();
}

function current() {
  return state.pending[state.cursor];
}

// selection is the run a type would be given to: the cursor alone, or the
// stretch between the anchor and the cursor.
function selection() {
  if (state.anchor === null) return current() ? [current()] : [];
  const from = Math.min(state.anchor, state.cursor);
  const to = Math.max(state.anchor, state.cursor);
  return state.pending.slice(from, to + 1);
}

function drawTypes() {
  const list = el('types');
  list.innerHTML = '';
  for (const type of state.types) {
    const item = document.createElement('button');
    item.type = 'button';
    item.dataset.name = type.name;
    item.className = `type type-${type.nature}`;
    item.setAttribute('role', 'option');
    item.title = `${type.covers} — ${lifetimeSentence(type)}`;
    item.innerHTML =
      `<kbd>${type.key}</kbd>` +
      `<span class="type-text">` +
      `<span class="type-label">${type.label}</span>` +
      `<span class="type-life">${lifetimeText(type)}</span>` +
      `</span>`;
    item.addEventListener('click', () => chooseType(type.name));
    list.append(item);
  }
}

// lifetimeText is what the chip carries under the label, so it is the short
// form: the sentence version belongs in the expiry line, which says the date.
function lifetimeText(type) {
  if (type.lifetime === null || type.lifetime === undefined) return 'no expiry';
  if (type.lifetime === 0) return 'event day';
  if (type.lifetime % 365 === 0) return `+${type.lifetime / 365}y`;
  return `+${type.lifetime}d`;
}

// lifetimeSentence is the hover text's long form of lifetimeText.
function lifetimeSentence(type) {
  if (type.expiryExpected && (type.lifetime === null || type.lifetime === undefined)) {
    return 'no default expiry; enter the date printed on it';
  }
  if (type.lifetime === null || type.lifetime === undefined) return 'kept for good, no expiry';
  if (type.lifetime === 0) return 'expires on the event date';
  if (type.lifetime % 365 === 0) {
    const years = type.lifetime / 365;
    return `expires ${years} year${years === 1 ? '' : 's'} after the event date`;
  }
  return `expires ${type.lifetime} days after the event date`;
}

function draw() {
  const scan = current();
  const empty = !scan;
  el('page-empty').hidden = !empty;
  el('page-image').hidden = empty;
  el('progress-text').textContent = empty
    ? 'The inbox is empty. Nothing is waiting.'
    : `${state.cursor + 1} of ${state.pending.length} waiting`;
  drawDuplicates();
  const run = selection();
  const batch = run.length > 1;
  el('batch').hidden = !batch;
  el('batch-count').textContent = String(run.length);
  // Filing a run gives one type and shared tags to all of them. Fields that
  // describe one document are put out of reach rather than silently dropped.
  el('fields').classList.toggle('run-selected', batch);
  for (const id of ['event-date', 'event-zone', 'description', 'total']) {
    el(id).disabled = batch;
  }

  // Another scan on the desk starts with no mark and its first split.
  if (state.shown !== scan?.digest) {
    state.shown = scan?.digest;
    split.reset();
    el('save-state').textContent = '';
  }
  pages.show(scan);
  if (!empty) {
    el('facts').textContent = facts(scan).join(' · ');
    el('flags').innerHTML = flags(scan)
      .map((flag) => `<span class="flag flag-${flag.kind}">${flag.text}</span>`)
      .join('');
  }
  drawDesk();
  drawQueue();
}

// drawDesk fills the fields from whatever they describe, and says above them
// which pages that is.
function drawDesk() {
  split.drawHead();
  if (current()) {
    for (const [id, name] of Object.entries(FIELDS)) {
      const shown = value(name);
      el(id).value = name === 'tags' ? shown.join(', ') : shown;
    }
  }
  markType();
  drawExpiry();
}

// facts are what the file says about itself, which is everything the tool
// knows before a word is typed.
function facts(scan) {
  const out = [scan.filename];
  if (scan.pages) out.push(scan.pages === 1 ? '1 page' : `${scan.pages} pages`);
  if (scan.pageSize) out.push(scan.pageSize);
  if (scan.colour) out.push(scan.colour);
  if (scan.dpi) out.push(`${scan.dpi} dpi`);
  if (scan.producer) out.push(scan.producer);
  const day = localDate(scan.scannedAt);
  if (day) out.push(`scanned ${day} ${localTime(scan.scannedAt)} ${zoneOf(scan.scannedAt)}`);
  return out;
}

function flags(scan) {
  const out = [];
  if (scan.incomplete)
    out.push({ kind: 'stop', text: 'incomplete — no %%EOF, not filed' });
  if (scan.duplicateOf && scan.trashedAt)
    out.push({
      kind: 'warn',
      text: `already thrown away on ${localDate(scan.trashedAt)}`,
    });
  else if (scan.duplicateOf)
    out.push({ kind: 'warn', text: `duplicate of ${scan.duplicateOf}` });
  const documents = state.draft.documents ?? scan.documents ?? [];
  if (documents.length) {
    out.push({ kind: 'note', text: `${documents.length} split${documents.length === 1 ? '' : 's'}` });
  } else if (scan.needsSplit) out.push({ kind: 'note', text: 'several documents — filed whole' });
  if (scan.needsRender) out.push({ kind: 'note', text: 'no image to extract' });
  return out;
}

function markType() {
  const document = split.active();
  const chosen = document ? document.type : state.draft.type ?? current()?.type;
  for (const item of el('types').children) {
    item.classList.toggle('type-chosen', item.dataset.name === chosen);
  }
}

// drawExpiry shows what the type and the date would make of the expiry, so a
// wrong type is visible before it is filed rather than in a filter months
// later.
function drawExpiry() {
  const scan = current();
  if (!scan) {
    el('expiry-line').textContent = '';
    return;
  }
  const document = split.active();
  const name = document ? document.type : state.draft.type ?? scan.type;
  const type = state.types.find((candidate) => candidate.name === name);
  if (!document && state.draft.expiryCleared) {
    el('expiry-line').textContent = 'Kept for good — no expiry.';
    return;
  }
  if (!type || type.lifetime === null || type.lifetime === undefined) {
    el('expiry-line').textContent = type?.expiryExpected
      ? 'Expiry is printed on the document — fill it in later in Browse.'
      : 'No expiry: this is kept because you want it.';
    return;
  }
  const date = el('event-date').value.trim();
  if (!date) {
    el('expiry-line').textContent = 'Expires once there is an event date.';
    return;
  }
  el('expiry-line').textContent = `Expires ${addDays(date, type.lifetime)}.`;
}

function addDays(iso, days) {
  const parsed = new Date(`${iso}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return '—';
  parsed.setUTCDate(parsed.getUTCDate() + days);
  return parsed.toISOString().slice(0, 10);
}

function drawQueue() {
  const list = el('queue-list');
  list.innerHTML = '';
  const run = new Set(selection().map((scan) => scan.digest));
  state.pending.forEach((scan, index) => {
    const item = document.createElement('li');
    item.className = 'queue-item';
    if (index === state.cursor) item.classList.add('queue-current');
    if (run.has(scan.digest) && run.size > 1) item.classList.add('queue-run');
    // The name gets the whole width; when it was scanned sits under it.
    const name = document.createElement('span');
    name.className = 'queue-name';
    name.textContent = scan.filename;
    name.title = scan.filename;
    const time = document.createElement('span');
    time.className = 'queue-time';
    time.textContent = scan.scannedAt
      ? `Scanned ${localDate(scan.scannedAt)} ${localTime(scan.scannedAt)}`
      : '';
    // The scan on the desk shows its draft, so a split is seen here before
    // the save lands.
    const shown = index === state.cursor ? { ...scan, ...state.draft } : scan;
    const typeName = shown.type && shown.type !== 'unsorted' ? shown.type : '';
    const marks = document.createElement('span');
    marks.className = 'queue-marks';
    marks.innerHTML =
      (typeName ? `<span class="badge">${typeName}</span>` : '') + badgeHTML(handling(shown));
    item.append(name, time, marks);
    item.addEventListener('click', () => {
      state.cursor = index;
      state.anchor = null;
      state.draft = {};
      draw();
    });
    list.append(item);
  });
}

// duplicates is every scan in the inbox the Box already holds — matched on the
// whole file or on the image streams alone, including against the trash.
function duplicates() {
  return state.pending.filter((scan) => scan.duplicateOf);
}

// drawDuplicates offers the group as one decision. Dealing with them one at a
// time is the tedium this tool exists to remove, and a match against the trash
// says the judgement has already been made once.
function drawDuplicates() {
  const found = duplicates();
  el('duplicates').hidden = found.length === 0;
  if (!found.length) return;
  const thrown = found.filter((scan) => scan.trashedAt).length;
  el('duplicates-text').textContent =
    `${found.length} of these are already in the Box` +
    (thrown ? `, ${thrown} of them in its trash` : '') +
    '.';
}

function wireDuplicates() {
  for (const name of ['inbox', 'rejected']) {
    el('tab-' + name).addEventListener('click', () => showQueueTab(name));
  }
  el('duplicates-reject').addEventListener('click', async () => {
    const found = duplicates();
    if (!found.length) return;
    const from = state.cursor;
    try {
      await api.trashMany(
        found.map((scan) => scan.digest),
        'duplicate of something already in the Box',
      );
    } catch (err) {
      showError(err);
      return;
    }
    await reload(from);
    await drawRejected();
  });
}

function chooseType(name) {
  const document = split.editable();
  if (document) document.type = name;
  else state.draft.type = name;
  saveSoon();
  markType();
  drawExpiry();
}

function move(step, extend) {
  if (!state.pending.length) return;
  if (extend && state.anchor === null) state.anchor = state.cursor;
  if (!extend) state.anchor = null;
  const next = state.cursor + step;
  if (next < 0 || next >= state.pending.length) return;
  state.cursor = next;
  if (!extend) state.draft = {};
  draw();
}

function showError(err) {
  const line = el('error');
  if (!err) {
    line.hidden = true;
    line.textContent = '';
    return;
  }
  line.hidden = false;
  line.textContent = err.message || String(err);
}

// describe is everything said about one scan on the desk, as one edit: what
// filing sends, and what a draft saves.
function describe(scan) {
  const documents = state.draft.documents ?? scan.documents ?? [];
  if (documents.length) {
    // Each split is its own document with its own fields, so the file itself
    // says nothing beyond what it is. A split with a date and no zone is read
    // in the configured one, as the desk showed it.
    return {
      digest: scan.digest,
      type: 'unsorted',
      description: '',
      eventDate: '',
      eventZone: '',
      total: '',
      tags: [],
      expiryCleared: state.draft.expiryCleared ?? false,
      documents: documents.map((document) => ({
        ...document,
        eventZone: document.eventZone || (document.eventDate ? state.config.zone || '' : ''),
      })),
      ignoredPages: state.draft.ignoredPages ?? scan.ignoredPages ?? '',
    };
  }
  return {
    digest: scan.digest,
    type: state.draft.type ?? scan.type,
    description: el('description').value,
    eventDate: el('event-date').value,
    eventZone: el('event-zone').value,
    total: el('total').value,
    tags: parseTags(el('tags').value),
    expiryCleared: state.draft.expiryCleared ?? false,
    ...(state.draft.documents ? { documents: [], ignoredPages: '' } : {}),
  };
}

// saveSoon keeps what has been typed about a scan without filing it. The
// server holds it in the inbox's state file — never in the Box — so a reload
// or a closed tab halfway through a long PDF loses nothing. The edit is taken
// now, while the scan is still on the desk, and sent a moment later so a
// burst of keystrokes is one write.
const SAVE = { timer: null, edit: null };

function saveSoon() {
  const scan = current();
  if (!scan || selection().length > 1 || !Object.keys(state.draft).length) return;
  if (SAVE.edit && SAVE.edit.digest !== scan.digest) saveNow();
  SAVE.edit = describe(scan);
  el('save-state').textContent = 'unsaved';
  clearTimeout(SAVE.timer);
  SAVE.timer = setTimeout(saveNow, 500);
}

async function saveNow() {
  clearTimeout(SAVE.timer);
  const edit = SAVE.edit;
  SAVE.edit = null;
  if (!edit) return;
  try {
    const saved = await api.patch(edit);
    // The inbox list keeps what was saved, so coming back to this scan shows
    // it even though the desk's own draft is gone by then.
    const scan = state.pending.find((candidate) => candidate.digest === edit.digest);
    if (scan) {
      for (const name of ['type', 'description', 'eventDate', 'eventZone', 'total', 'tags',
        'expiryCleared', 'documents', 'ignoredPages']) {
        scan[name] = saved[name];
      }
    }
    if (!SAVE.edit) el('save-state').textContent = 'draft saved';
  } catch (err) {
    // A half-typed date is refused; the draft is simply saved on the next
    // keystroke that makes it whole.
    el('save-state').textContent = `not saved: ${err.message || err}`;
  }
}

// fileCurrent takes the selection into the Box. A run gets one type and shared
// tags; a date, total and description still belong to one document.
async function fileCurrent() {
  const run = selection();
  if (!run.length) return;
  // Pages in no split stop filing once, so a forgotten page is a decision.
  if (run.length === 1 && !split.readyToFile()) return;
  const from = state.anchor === null ? state.cursor : Math.min(state.anchor, state.cursor);
  const type = state.draft.type ?? current().type;
  const tags = parseTags(el('tags').value);
  showError(null);
  try {
    if (run.length > 1) {
      // Filing copies bytes, so a run is still one call per scan: each copy is
      // verified against its own digest before it is published, and a failure
      // must stop that scan rather than the other two hundred.
      for (const scan of run) {
        await api.file({ digest: scan.digest, type, tags });
      }
    } else {
      await api.file(describe(run[0]));
    }
  } catch (err) {
    showError(err);
    return;
  }
  await reload(from);
}

async function rejectCurrent() {
  const scan = current();
  if (!scan) return;
  const from = state.cursor;
  try {
    await api.trash(scan.digest, 'rejected at intake');
  } catch (err) {
    showError(err);
    return;
  }
  await reload(from);
  await drawRejected();
}

// say puts a short line on the desk for a key whose effect is otherwise easy
// to miss. It clears itself; the next say replaces it.
const NOTICE = { timer: null };
function say(text) {
  const line = el('notice');
  line.textContent = text;
  line.hidden = false;
  clearTimeout(NOTICE.timer);
  NOTICE.timer = setTimeout(() => {
    line.hidden = true;
  }, 5000);
}

function wireBatch() {
  el('batch-clear').addEventListener('click', () => {
    state.anchor = null;
    draw();
  });
}

const zones = zonePicker(el('event-zone'), el('zone-options'), (name) => {
  setValue('eventZone', name);
  saveSoon();
});

function wireFields() {
  el('event-zone').addEventListener('change', () => {
    setValue('eventZone', el('event-zone').value.trim());
    saveSoon();
  });
  for (const id of ['event-date', 'event-zone', 'description', 'total', 'tags']) {
    el(id).addEventListener('input', () => {
      // Letters typed into the zone are a search, not a zone: only a picked
      // one, or what is left when the field is let go, is kept.
      if (id === 'event-zone') return;
      setValue(FIELDS[id], el(id).value);
      drawExpiry();
      saveSoon();
    });
    el(id).addEventListener('keydown', (event) => {
      if (id === 'event-zone' && zones.key(event)) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        el(id).blur();
      }
      if (event.key === 'Enter') {
        event.preventDefault();
        fileCurrent();
      }
    });
  }
}

function parseTags(value) {
  const seen = new Set();
  return value
    .split(',')
    .map((tag) => tag.trim())
    .filter((tag) => {
      if (!tag || seen.has(tag)) return false;
      seen.add(tag);
      return true;
    });
}

// The inbox column is as wide as the person drags it, and no narrower than a
// time and a few characters of a name: below that every row is an ellipsis.
// The width is a per-browser convenience, so it is kept in localStorage and a
// page that cannot store it simply starts at the default.
const QUEUE_WIDTH = { key: 'dgs.box.queue.width', min: 220, fallback: 280, step: 24 };

function wireQueueResize() {
  const main = document.querySelector('.intake-main');
  const handle = el('queue-resize');
  const max = () => Math.max(QUEUE_WIDTH.min, Math.floor(main.clientWidth * 0.5));
  const apply = (width, keep) => {
    const clamped = Math.round(Math.min(Math.max(width, QUEUE_WIDTH.min), max()));
    main.style.setProperty('--queue-width', `${clamped}px`);
    handle.setAttribute('aria-valuenow', String(clamped));
    handle.setAttribute('aria-valuemin', String(QUEUE_WIDTH.min));
    handle.setAttribute('aria-valuemax', String(max()));
    if (keep) {
      try {
        localStorage.setItem(QUEUE_WIDTH.key, String(clamped));
      } catch {
        // Kept for this page load only.
      }
    }
    return clamped;
  };
  let saved = NaN;
  try {
    saved = Number(localStorage.getItem(QUEUE_WIDTH.key));
  } catch {
    // Nothing stored is the default width.
  }
  let width = apply(Number.isFinite(saved) && saved > 0 ? saved : QUEUE_WIDTH.fallback, false);

  handle.addEventListener('pointerdown', (event) => {
    if (event.button !== 0) return;
    event.preventDefault();
    handle.setPointerCapture(event.pointerId);
    const left = main.getBoundingClientRect().left;
    document.body.classList.add('resizing');
    const follow = (move) => {
      width = apply(move.clientX - left, false);
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
    width = apply(QUEUE_WIDTH.fallback, true);
  });
  // The separator can be moved without a mouse too. The page's own keys are
  // not reached from here: a focused separator swallows its arrows.
  handle.addEventListener('keydown', (event) => {
    const delta = { ArrowLeft: -QUEUE_WIDTH.step, ArrowRight: QUEUE_WIDTH.step }[event.key];
    if (!delta) return;
    event.preventDefault();
    event.stopPropagation();
    width = apply(width + delta, true);
  });
  // A narrower window may leave the saved width over half of it.
  window.addEventListener('resize', () => {
    width = apply(width, false);
  });
}

// One key map, arrows and Enter, so nothing has to be learnt. It leaves the
// type keys alone: a type is one letter, and that reflex is never relearnt.
const KEYS = {
  hints: [
    [['Enter'], 'file and next'],
    [['k'], 'keep for good'],
    [['Backspace'], 'reject'],
    [['\u2193', '\u2191'], 'move'],
    [['p'], 'pages'],
    [['Space'], 'split pages'],
    [['x'], 'ignore page'],
    [[':'], 'type splits'],
    [['-'], 'remove split'],
    [['\u2190', '\u2192'], 'turn page'],
    [['Shift', '\u2193'], 'select a run'],
    [['Tab'], 'fields'],
    [['Esc'], 'back to keys'],
  ],
  bindings: {
    // While a split is being marked the arrows move along its pages — the
    // strip runs top to bottom — rather than off to another scan.
    ArrowDown: (event) => (split.marking() ? pages.step(1) : move(1, event.shiftKey)),
    ArrowUp: (event) => (split.marking() ? pages.step(-1) : move(-1, event.shiftKey)),
    Backspace: () => rejectCurrent(),
    ArrowLeft: () => pages.step(-1),
    ArrowRight: () => pages.step(1),
    Tab: () => el('event-date').focus(),
    p: () => pages.toggle(),
    ' ': () => split.markOrClose(),
    x: () => split.ignore(),
    ':': () => split.openText(),
    // The full-width colon, so a Chinese or Japanese input method in its
    // own mode still opens the splits.
    '\uff1a': () => split.openText(),
    '-': () => split.remove(),
    k: () => toggleKeep(),
  },
};

// A shifted command still owns its lowercase letter as a human shortcut. This
// catches a catalogue or key map change at startup instead of silently making
// a type unreachable.
function assertTypeKeysAreFree() {
  const commands = new Set(
    Object.keys(KEYS.bindings)
      .filter((key) => key.length === 1)
      .map((key) => key.toLowerCase()),
  );
  const collision = state.types.find((type) => commands.has(type.key.toLowerCase()));
  if (collision) {
    throw new Error(`Type key ${collision.key} for ${collision.name} conflicts with an intake command`);
  }
}

function toggleKeep() {
  state.draft.expiryCleared = !state.draft.expiryCleared;
  say(state.draft.expiryCleared
    ? 'Kept for good — no expiry. k again goes back to the type default.'
    : 'Expiry back to the type default.');
  drawExpiry();
  saveSoon();
}

// drawKeys writes the strip along the floor of the desk from the key map, so
// what is shown and what is bound cannot drift apart.
function drawKeys() {
  const list = el('keys');
  list.innerHTML = '';
  for (const [keys, meaning] of KEYS.hints) {
    const item = document.createElement('li');
    item.innerHTML =
      keys.map((key) => `<kbd>${key}</kbd>`).join('') +
      `<span>${meaning}</span>`;
    list.append(item);
  }
}

function wireKeys() {
  drawKeys();
  document.addEventListener('keydown', (event) => {
    const typing = ['INPUT', 'TEXTAREA'].includes(document.activeElement?.tagName);
    if (typing) return;
    if (event.metaKey || event.ctrlKey || event.altKey) return;

    if (event.key === 'Enter') {
      event.preventDefault();
      fileCurrent();
      return;
    }
    if (event.key === 'Escape') {
      // Esc first lets go of a mark being made, then of everything unsaved.
      if (split.cancel()) return;
      state.anchor = null;
      state.draft = {};
      draw();
      return;
    }

    const binding = KEYS.bindings[event.key];
    if (binding) {
      event.preventDefault();
      binding(event);
      return;
    }

    // A type is the last thing tried, so a command letter keeps its key.
    const type = state.types.find((candidate) => candidate.key === event.key);
    if (type) {
      event.preventDefault();
      chooseType(type.name);
    }
  });
}

// A tab closed within the half second a save waits still keeps its draft.
window.addEventListener('pagehide', () => {
  if (!SAVE.edit) return;
  fetch('/api/scan', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(SAVE.edit),
    keepalive: true,
  });
});

start().catch((err) => showError(err));
