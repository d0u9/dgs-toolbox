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

const pages = pageView({
  strip: 'page-strip',
  toggle: 'pages-toggle',
  image: 'page-image',
  label: 'page-label',
});

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

  pages.show(scan);
  if (!empty) {
    el('facts').textContent = facts(scan).join(' · ');
    el('flags').innerHTML = flags(scan)
      .map((flag) => `<span class="flag flag-${flag.kind}">${flag.text}</span>`)
      .join('');
    el('event-date').value = state.draft.eventDate ?? scan.eventDate ?? '';
    el('event-zone').value =
      state.draft.eventZone ?? scan.eventZone ?? state.config.zone ?? '';
    el('description').value = state.draft.description ?? scan.description ?? '';
    el('total').value = state.draft.total ?? scan.total ?? '';
    el('tags').value = (state.draft.tags ?? scan.tags ?? []).join(', ');
  }
  markType();
  drawExpiry();
  drawQueue();
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
  if (scan.needsSplit) out.push({ kind: 'note', text: 'several documents — filed whole' });
  if (scan.needsRender) out.push({ kind: 'note', text: 'no image to extract' });
  if (scan.group) out.push({ kind: 'note', text: `group ${scan.group}` });
  return out;
}

function markType() {
  const chosen = state.draft.type ?? current()?.type;
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
  const name = state.draft.type ?? scan.type;
  const type = state.types.find((candidate) => candidate.name === name);
  if (state.draft.expiryCleared) {
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
    item.innerHTML =
      `<span class="queue-time">${localTime(scan.scannedAt)}</span>` +
      `<span class="queue-name">${scan.filename}</span>`;
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
  });
}

function chooseType(name) {
  state.draft.type = name;
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

// fileCurrent takes the selection into the Box. A run gets one type and shared
// tags; a date, total and description still belong to one document.
async function fileCurrent() {
  const run = selection();
  if (!run.length) return;
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
      await api.file({
        digest: run[0].digest,
        type,
        description: el('description').value,
        eventDate: el('event-date').value,
        eventZone: el('event-zone').value,
        total: el('total').value,
        tags,
        expiryCleared: state.draft.expiryCleared ?? false,
        group: state.draft.group ?? '',
      });
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
}

// sameAsPrevious binds this scan to the one before it: a contract scanned in
// three passes, bound while the parts are still on screen together.
function sameAsPrevious() {
  const previous = state.pending[state.cursor - 1];
  if (!previous) return;
  state.draft.group = previous.group || previous.digest.slice(0, 6);
  draw();
}

function wireBatch() {
  el('batch-clear').addEventListener('click', () => {
    state.anchor = null;
    draw();
  });
}

function wireFields() {
  for (const id of ['event-date', 'event-zone', 'description', 'total', 'tags']) {
    el(id).addEventListener('input', () => {
      if (id === 'tags') state.draft.tags = parseTags(el(id).value);
      else state.draft[
        { 'event-date': 'eventDate', 'event-zone': 'eventZone', description: 'description', total: 'total' }[id]
      ] = el(id).value;
      drawExpiry();
    });
    el(id).addEventListener('keydown', (event) => {
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
    [['g'], 'same document'],
    [['Backspace'], 'reject'],
    [['\u2193', '\u2191'], 'move'],
    [['p'], 'pages'],
    [['\u2190', '\u2192'], 'turn page'],
    [['Shift', '\u2193'], 'select a run'],
    [['Tab'], 'fields'],
    [['Esc'], 'back to keys'],
  ],
  bindings: {
    ArrowDown: (event) => move(1, event.shiftKey),
    ArrowUp: (event) => move(-1, event.shiftKey),
    Backspace: () => rejectCurrent(),
    ArrowLeft: () => pages.step(-1),
    ArrowRight: () => pages.step(1),
    Tab: () => el('event-date').focus(),
    p: () => pages.toggle(),
    k: () => toggleKeep(),
    g: () => sameAsPrevious(),
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
  drawExpiry();
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

start().catch((err) => showError(err));
