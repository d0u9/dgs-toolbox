// Splitting one PDF into the documents it holds. Nothing is cut: the split is
// a list of page ranges that goes into the sidecar, and each range is
// described as if it were a PDF of its own — the same type keys and the same
// fields the desk always has, under a heading that says which pages they are
// for. Ranges may overlap, and a page that is deliberately no document is
// marked ignored, so a forgotten page can be told apart from a blank one.
//
// Space marks where a split starts and, pressed again, where it ends; x
// ignores; ":" takes "1-3, 2-5, 6, !7-10" for someone who already knows the
// page numbers.

// parseSpec reads the text form. "!" before a range ignores it. It answers
// { documents: ["1-3", ...], ignored: Set } or throws.
function parseSpec(text, last) {
  const documents = [];
  const ignored = new Set();
  for (const raw of text.split(',')) {
    let part = raw.trim();
    if (!part) continue;
    const ignore = part.startsWith('!');
    if (ignore) part = part.slice(1).trim();
    const match = part.match(/^(\d+)(?:\s*-\s*(\d+))?$/);
    if (!match) throw new Error(`"${raw.trim()}" is not a page or a range`);
    const from = Number(match[1]);
    const to = match[2] ? Number(match[2]) : from;
    if (from < 1 || to < from) throw new Error(`"${raw.trim()}" runs backwards`);
    if (to > last) throw new Error(`"${raw.trim()}" is past the last page, ${last}`);
    if (ignore) for (let page = from; page <= to; page++) ignored.add(page);
    else documents.push(rangeText(from, to));
  }
  return { documents, ignored };
}

function rangeText(from, to) {
  return from === to ? `${from}` : `${from}-${to}`;
}

// pagesOf is every page a range text names, as numbers.
function pagesOf(text) {
  const out = new Set();
  for (const part of (text || '').split(',')) {
    const match = part.trim().match(/^(\d+)(?:-(\d+))?$/);
    if (!match) continue;
    const from = Number(match[1]);
    const to = match[2] ? Number(match[2]) : from;
    for (let page = from; page <= to; page++) out.add(page);
  }
  return out;
}

// compact writes a set of pages as the fewest ranges: 1,2,3,7 is "1-3,7".
function compact(pages) {
  const sorted = [...pages].sort((a, b) => a - b);
  const ranges = [];
  for (const page of sorted) {
    const last = ranges[ranges.length - 1];
    if (last && page === last[1] + 1) last[1] = page;
    else ranges.push([page, page]);
  }
  return ranges.map(([from, to]) => rangeText(from, to)).join(',');
}

// emptyDocument is a split nobody has described yet.
function emptyDocument(pages) {
  return { pages, type: '', description: '', tags: [], eventDate: '', eventZone: '', total: '' };
}

// splitter owns the split of the scan on the desk. host gives it the page
// view, the scan and its draft, and a way to redraw the desk; it never draws
// the fields itself, because a split is described with the desk's own.
function splitter(host) {
  const el = (id) => document.getElementById(id);
  const view = { mark: null, selected: 0, confirming: false };

  const scan = () => host.scan();
  const documents = () => host.draft().documents ?? scan()?.documents ?? [];
  const ignored = () => pagesOf(host.draft().ignoredPages ?? scan()?.ignoredPages ?? '');
  const applies = () => !!scan() && scan().kind === 'pdf' && scan().pages > 1;
  const marking = () => view.mark !== null;

  // own copies the scan's split into the draft the first time it changes, so
  // what is filed is what is on screen and the scan itself is untouched.
  function own() {
    const draft = host.draft();
    if (!draft.documents) {
      draft.documents = (scan()?.documents ?? []).map((document) => ({
        ...emptyDocument(document.pages),
        ...document,
        tags: [...(document.tags || [])],
      }));
    }
    if (draft.ignoredPages === undefined) draft.ignoredPages = scan()?.ignoredPages ?? '';
    return draft;
  }

  function held() {
    const out = new Set();
    for (const document of documents()) for (const page of pagesOf(document.pages)) out.add(page);
    return out;
  }

  function unassigned() {
    if (!applies() || !documents().length) return new Set();
    const used = held();
    for (const page of ignored()) used.add(page);
    const out = new Set();
    for (let page = 1; page <= scan().pages; page++) if (!used.has(page)) out.add(page);
    return out;
  }

  // follow picks the split the page being looked at belongs to, so turning
  // to a page is how its split is chosen. A page in two splits keeps the one
  // already picked if it is one of them.
  function follow(page) {
    const list = documents();
    if (!list.length) return;
    if (pagesOf(list[view.selected]?.pages).has(page)) return;
    const found = list.findIndex((document) => pagesOf(document.pages).has(page));
    if (found >= 0) view.selected = found;
  }

  function decorate(item, page) {
    let badge = item.querySelector('.page-docs');
    const list = documents();
    const marking = view.mark !== null;
    if (!list.length && !marking && !ignored().size) {
      badge?.remove();
      item.classList.remove('page-ignored', 'page-mark', 'page-free');
      return;
    }
    if (!badge) {
      badge = document.createElement('span');
      badge.className = 'page-docs';
      item.append(badge);
    }
    const members = [];
    list.forEach((document, index) => {
      if (pagesOf(document.pages).has(page)) members.push(index);
    });
    badge.innerHTML = members
      .map((index) => `<span class="doc-chip doc-${index % 6}${index === view.selected ? ' doc-selected' : ''}">${index + 1}</span>`)
      .join('');
    // The page is framed in its split's colour, so a run of pages reads as
    // one piece down the strip; a page in two splits is framed in the first
    // and carries both numbers.
    item.classList.remove(...[0, 1, 2, 3, 4, 5].map((n) => `page-doc-${n}`));
    if (members.length) item.classList.add(`page-doc-${members[0] % 6}`);
    item.classList.toggle('page-in-split', members.length > 0);
    item.classList.toggle('page-in-selected', members.includes(view.selected));
    const skip = ignored().has(page);
    item.classList.toggle('page-ignored', skip);
    item.classList.toggle('page-free', list.length > 0 && members.length === 0 && !skip);
    const current = host.pages.page();
    item.classList.toggle(
      'page-mark',
      marking && page >= Math.min(view.mark, current) && page <= Math.max(view.mark, current),
    );
  }

  // drawHead is the one line above the desk that says what the fields are
  // describing: a split and its pages, or the whole file.
  function drawHead() {
    const head = el('split-head');
    const list = documents();
    const current = host.pages.page();
    // Marking is shown where the eye is — on the page itself — as well as
    // here, so a Space pressed a moment ago is never a mode nobody can see.
    const banner = el('mark-banner');
    const marking = view.mark !== null && applies();
    banner.hidden = !marking;
    if (marking) {
      const from = Math.min(view.mark, current);
      const to = Math.max(view.mark, current);
      banner.textContent = `Marking a split: p ${rangeText(from, to)} · ↑↓ extend · Space ends it here · Esc cancels`;
    }
    head.classList.toggle('split-marking', marking);
    el('split-remove').hidden = !(list.length && !marking);
    const show = applies() && (list.length > 0 || view.mark !== null || ignored().size > 0);
    head.hidden = !show;
    if (!show) return;
    let title;
    if (view.mark !== null) {
      title = `Marking p ${rangeText(Math.min(view.mark, current), Math.max(view.mark, current))} — Space ends the split, x ignores these pages`;
    } else if (list.length) {
      const document = list[view.selected];
      title = `Split ${view.selected + 1} of ${list.length} · p ${document.pages}`;
    } else {
      title = 'No splits yet — Space on the first page of one';
    }
    el('split-title').textContent = title;
    const chips = el('split-chips');
    chips.innerHTML = '';
    list.forEach((document, index) => {
      const chip = window.document.createElement('button');
      chip.type = 'button';
      chip.className = `doc-chip doc-${index % 6}${index === view.selected ? ' doc-selected' : ''}`;
      chip.textContent = String(index + 1);
      chip.title = `Split ${index + 1}: p ${document.pages}`;
      chip.addEventListener('click', () => pick(index));
      chips.append(chip);
    });
    const notes = [];
    if (ignored().size) notes.push(`ignored p ${compact(ignored())}`);
    const free = unassigned();
    if (free.size) notes.push(`not in any split: p ${compact(free)}`);
    el('split-notes').textContent = notes.join(' · ');
    el('split-notes').classList.toggle('split-warn', free.size > 0);
    el('split-confirm').hidden = !(view.confirming && free.size);
    el('split-confirm').textContent =
      `p ${compact(free)} ${free.size === 1 ? 'is' : 'are'} in no split. ${host.again || 'Enter'} ignores ${free.size === 1 ? 'it' : 'them'}; Esc goes back.`;
  }

  function redraw() {
    host.pages.redraw();
    drawHead();
  }

  // pick shows a split in the desk and its first page in the viewer.
  function pick(index) {
    const list = documents();
    if (!list[index]) return;
    view.selected = index;
    const first = Math.min(...pagesOf(list[index].pages));
    host.pages.go(first);
    host.changed();
  }

  function markOrClose() {
    if (!applies()) return;
    const current = host.pages.page();
    if (view.mark === null) {
      view.mark = current;
      host.pages.open();
      redraw();
      return;
    }
    const from = Math.min(view.mark, current);
    const to = Math.max(view.mark, current);
    view.mark = null;
    const draft = own();
    const blocked = [...ignored()].filter((page) => page >= from && page <= to);
    if (blocked.length) {
      host.showError(new Error(`p ${compact(blocked)} ${blocked.length === 1 ? 'is' : 'are'} ignored; press x there first to stop ignoring`));
      redraw();
      return;
    }
    draft.documents.push(emptyDocument(rangeText(from, to)));
    view.selected = draft.documents.length - 1;
    view.confirming = false;
    host.showError(null);
    host.changed();
    // The next split usually starts on the page after this one ends. The
    // desk stays on the split just made until the page is turned onto it.
    if (to < scan().pages) host.pages.go(to + 1);
  }

  function ignore() {
    if (!applies()) return;
    const current = host.pages.page();
    const from = view.mark === null ? current : Math.min(view.mark, current);
    const to = view.mark === null ? current : Math.max(view.mark, current);
    view.mark = null;
    const inSplit = held();
    const blocked = [];
    for (let page = from; page <= to; page++) if (inSplit.has(page)) blocked.push(page);
    if (blocked.length) {
      host.showError(new Error(`p ${compact(blocked)} ${blocked.length === 1 ? 'is' : 'are'} in a split; remove the split (-) to ignore ${blocked.length === 1 ? 'it' : 'them'}`));
      redraw();
      return;
    }
    const draft = own();
    const set = ignored();
    const all = [...Array(to - from + 1).keys()].every((offset) => set.has(from + offset));
    for (let page = from; page <= to; page++) {
      if (all) set.delete(page);
      else set.add(page);
    }
    draft.ignoredPages = compact(set);
    view.confirming = false;
    host.showError(null);
    host.changed();
  }

  function remove() {
    const list = documents();
    if (!list.length) return;
    const draft = own();
    draft.documents.splice(view.selected, 1);
    view.selected = Math.max(0, Math.min(view.selected, draft.documents.length - 1));
    // Pages are ignored only beside splits: with none left the file is one
    // document again, every page of it.
    if (!draft.documents.length) draft.ignoredPages = '';
    host.changed();
  }

  function openText() {
    if (!applies()) return;
    const form = el('split-text-form');
    const input = el('split-text');
    const parts = documents().map((document) => document.pages);
    if (ignored().size) parts.push(...compact(ignored()).split(',').map((range) => `!${range}`));
    input.value = parts.join(', ');
    el('split-head').hidden = false;
    form.hidden = false;
    input.focus();
    input.select();
  }

  function wireText() {
    const form = el('split-text-form');
    const input = el('split-text');
    form.addEventListener('submit', (event) => event.preventDefault());
    input.addEventListener('keydown', (event) => {
      // Stopped here, so the page does not read this Enter as "file".
      event.stopPropagation();
      if (event.key === 'Escape') {
        event.preventDefault();
        form.hidden = true;
        input.blur();
        drawHead();
        return;
      }
      if (event.key !== 'Enter') return;
      event.preventDefault();
      let parsed;
      try {
        parsed = parseSpec(input.value, scan().pages);
      } catch (err) {
        host.showError(err);
        return;
      }
      const inSplit = new Set();
      for (const pages of parsed.documents) for (const page of pagesOf(pages)) inSplit.add(page);
      const clash = [...parsed.ignored].filter((page) => inSplit.has(page));
      if (clash.length) {
        host.showError(new Error(`p ${compact(clash)} cannot be both ignored and in a split`));
        return;
      }
      host.showError(null);
      // What was said about a split is kept when its pages are unchanged, so
      // retyping the list does not throw descriptions away.
      const draft = own();
      const before = new Map(draft.documents.map((document) => [document.pages, document]));
      draft.documents = parsed.documents.map((pages) => before.get(pages) ?? emptyDocument(pages));
      draft.ignoredPages = parsed.documents.length ? compact(parsed.ignored) : '';
      view.selected = 0;
      view.confirming = false;
      form.hidden = true;
      input.blur();
      host.changed();
    });
  }

  wireText();
  el('split-remove').addEventListener('click', () => remove());

  return {
    decorate,
    drawHead,
    follow,
    applies,
    markOrClose,
    ignore,
    remove,
    openText,
    marking,
    // active is the split the desk is describing, or null for the whole file.
    active: () => documents()[view.selected] ?? null,
    // editable is that split in the draft, copied there on first change.
    editable: () => (documents().length ? own().documents[view.selected] : null),
    cancel() {
      if (view.mark === null && !view.confirming) return false;
      view.mark = null;
      view.confirming = false;
      redraw();
      return true;
    },
    // readyToFile stops filing once when some page is in no split; the next
    // call ignores those pages and lets it through.
    readyToFile() {
      const free = unassigned();
      if (!free.size) return true;
      if (!view.confirming) {
        view.confirming = true;
        drawHead();
        return false;
      }
      const draft = own();
      const set = ignored();
      for (const page of free) set.add(page);
      draft.ignoredPages = compact(set);
      view.confirming = false;
      return true;
    },
    // reset forgets the view state when another scan comes onto the desk.
    reset() {
      view.mark = null;
      view.selected = 0;
      view.confirming = false;
      el('split-text-form').hidden = true;
    },
  };
}
