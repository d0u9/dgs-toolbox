'use strict';

// The exceptions page. Browse only links here, and only when there is
// something to see: exceptions shown every day are exceptions nobody reads.
// Each kind has its own group because each has its own answer.

const el = (id) => document.getElementById(id);

const GROUPS = {
  'no sidecar': 'group-adopt',
  'digest mismatch': 'group-mismatch',
  'no scan': 'group-orphan',
};

const state = { types: [], mismatches: [] };

// splitPath shows the name first and the directory quietly after it: the name
// is what a person recognises, the directory only says where to look.
function splitPath(path) {
  const cut = path.lastIndexOf('/');
  return cut < 0 ? { name: path, dir: '' } : { name: path.slice(cut + 1), dir: path.slice(0, cut + 1) };
}

function row(exception) {
  const item = document.createElement('li');
  item.className = 'check-row';
  const { name, dir } = splitPath(exception.path);
  const text = document.createElement('div');
  text.className = 'check-text';
  const nameLine = document.createElement('div');
  nameLine.className = 'check-name';
  nameLine.textContent = name;
  text.append(nameLine);
  if (dir) {
    const dirLine = document.createElement('div');
    dirLine.className = 'check-dir';
    dirLine.textContent = dir;
    text.append(dirLine);
  }
  if (exception.detail) {
    const detail = document.createElement('div');
    detail.className = 'check-detail';
    detail.textContent = exception.detail;
    text.append(detail);
  }
  item.append(text);
  if (exception.adoptable) item.append(adoptControls(exception.path));
  return item;
}

function adoptControls(path) {
  const box = document.createElement('div');
  box.className = 'check-actions';
  const select = document.createElement('select');
  select.setAttribute('aria-label', 'Type to adopt as');
  select.append(new Option('unsorted', ''));
  for (const type of state.types) select.append(new Option(type.label, type.name));
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'button primary';
  button.textContent = 'Adopt';
  button.addEventListener('click', () => adopt(path, select.value, button));
  box.append(select, button);
  return box;
}

function draw(exceptions) {
  const byGroup = {};
  for (const exception of exceptions) {
    const group = GROUPS[exception.kind] || 'group-other';
    (byGroup[group] ||= []).push(exception);
  }
  for (const group of ['group-adopt', 'group-mismatch', 'group-orphan', 'group-other']) {
    const section = el(group);
    const found = byGroup[group] || [];
    section.hidden = found.length === 0;
    section.querySelector('.check-count').textContent = String(found.length);
    const list = section.querySelector('.check-list');
    list.innerHTML = '';
    for (const exception of found) list.append(row(exception));
  }
  el('check-clear').hidden = exceptions.length > 0;
  statusBar.setSummary(exceptions.length ? `${exceptions.length} to look at` : 'nothing to look at');
}

async function refresh() {
  const body = await api.exceptions();
  draw([...body.exceptions, ...state.mismatches]);
}

// adopt takes in a scan that is already in the tree with no sidecar. Its
// directory records an intake date adopting does not get to rewrite, so
// nothing is copied and nothing is re-filed.
async function adopt(path, type, button) {
  button.disabled = true;
  try {
    await api.adopt(path, type ? { type } : {});
  } catch (err) {
    statusBar.error(err);
    button.disabled = false;
    return;
  }
  statusBar.show(`Adopted ${splitPath(path).name}.`);
  await refresh();
}

// runVerify reads every byte of every file in the Box. It repairs nothing: a
// mismatch is reported with both digests and left exactly as it is.
async function runVerify() {
  const button = el('verify');
  button.disabled = true;
  el('verify-note').textContent = 'Reading every byte. This takes a while.';
  try {
    const body = await api.verify();
    state.mismatches = body.mismatches;
    await refresh();
    el('verify-note').textContent = body.count
      ? `${body.count} files no longer match their recorded digest. Nothing was rewritten.`
      : `Every file still matches its recorded digest (checked ${new Date().toLocaleString()}).`;
  } catch (err) {
    el('verify-note').textContent = '';
    statusBar.error(err);
  }
  button.disabled = false;
}

// runRedraw draws every filed scan's thumbnail again. It touches only the
// cache on this machine.
async function runRedraw() {
  const button = el('redraw');
  button.disabled = true;
  el('redraw-note').textContent = 'Drawing every thumbnail again. This takes a while.';
  try {
    const body = await api.redraw();
    el('redraw-note').textContent = body.failed.length
      ? `${body.redrawn} redrawn, ${body.failed.length} could not be: ${body.failed.map((f) => `${f.path} (${f.detail})`).join('; ')}`
      : `${body.redrawn} redrawn.`;
  } catch (err) {
    el('redraw-note').textContent = '';
    statusBar.error(err);
  }
  button.disabled = false;
}

async function start() {
  const config = await api.config();
  el('sample-banner').hidden = !config.sample;
  el('paths').textContent = config.root || '';
  state.types = (await api.types()).types;
  el('verify').addEventListener('click', runVerify);
  el('redraw').addEventListener('click', runRedraw);
  await refresh();
}

start().catch((err) => statusBar.error(err));
