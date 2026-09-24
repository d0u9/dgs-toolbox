// The reader: one scan, or one split of it, read the way a PDF viewer reads —
// every page down one column, a strip of pages beside it. It only reads.
// Marking splits stays in the details, where the fields they describe are.
//
// It is part of Browse, not a page of its own: it takes the place of the
// filters and the grid, and the top bar and the status bar stay, saying where
// the person is. The grid is left underneath, scrolled where it was.
//
// A split is read as its own document: only its pages, numbered within it,
// so what is on screen is what the split will be taken to be.
//
// A page is drawn only while it is near the window. Each page has its place
// held from the start, sized to a sheet of paper, so the scrollbar is right
// before anything has loaded; a page scrolled far away gives its picture up
// again, so a hundred-page scan never holds a hundred decoded images.

const READER_WIDTH_KEY = 'dgs.box.reader.width';
// READER_WHOLE_PAGE is the stored width meaning "a whole page in the window".
const READER_WHOLE_PAGE = -1;
const READER_RAIL = { key: 'dgs.box.reader.rail', min: 96, max: 480, fallback: 180, step: 24 };

function readReaderWidth() {
  try {
    const width = Number(localStorage.getItem(READER_WIDTH_KEY));
    if (width === READER_WHOLE_PAGE) return width;
    return width >= 300 && width <= 3200 ? width : 0;
  } catch {
    return 0;
  }
}

function writeReaderWidth(width) {
  try {
    localStorage.setItem(READER_WIDTH_KEY, String(width));
  } catch {
    // A private window keeps the width for this page load only.
  }
}

// readerOf builds the reader once, on the <section id="reader"> in the page.
function readerOf() {
  const section = document.getElementById('reader');
  const crumb = document.getElementById('reader-crumb');
  const body = document.getElementById('reader-body');
  const handle = document.getElementById('reader-resize');
  const title = document.getElementById('reader-title');
  const label = document.getElementById('reader-page');
  const rail = document.getElementById('reader-rail');
  const column = document.getElementById('reader-column');
  const scroller = document.getElementById('reader-scroll');
  // Zero is "fit the width of the window", READER_WHOLE_PAGE is "fit a whole
  // page in the window"; anything else is pixels.
  const view = { scan: null, pages: [], current: 0, width: readReaderWidth() };

  // A sheet of A-series paper, until the page's own picture says otherwise.
  const paper = 1 / Math.SQRT2;

  // near draws the pages within two screens of the window and lets go of
  // those further out than six. The gap between the two stops a page at the
  // edge from being loaded and dropped with every small scroll.
  const near = new IntersectionObserver((seen) => {
    for (const entry of seen) {
      if (entry.isIntersecting) load(entry.target);
    }
  }, { root: scroller, rootMargin: '200% 0px' });
  const far = new IntersectionObserver((seen) => {
    for (const entry of seen) {
      if (!entry.isIntersecting) unload(entry.target);
    }
  }, { root: scroller, rootMargin: '600% 0px' });
  // shown follows which page fills the middle of the window.
  const shown = new IntersectionObserver((seen) => {
    for (const entry of seen) {
      if (entry.isIntersecting) mark(Number(entry.target.dataset.index));
    }
  }, { root: scroller, rootMargin: '-50% 0px -50% 0px' });

  function load(sheet) {
    const picture = sheet.firstElementChild;
    if (picture.getAttribute('src')) return;
    picture.src = api.image(view.scan.digest, 'preview', Number(sheet.dataset.page));
  }

  function unload(sheet) {
    const picture = sheet.firstElementChild;
    if (!picture.getAttribute('src')) return;
    picture.removeAttribute('src');
    sheet.classList.remove('reader-drawn');
  }

  function mark(index) {
    if (index === view.current && label.textContent) return;
    view.current = index;
    const page = view.pages[index];
    const count = view.pages.length;
    label.textContent = view.split
      ? `${index + 1} / ${count} · page ${page} of the scan`
      : `${index + 1} / ${count}`;
    statusBar.setSummary(`Page ${label.textContent}`);
    for (const item of rail.children) {
      const current = Number(item.dataset.index) === index;
      item.classList.toggle('page-current', current);
      if (current) {
        item.setAttribute('aria-current', 'page');
        item.scrollIntoView({ block: 'nearest' });
      } else item.removeAttribute('aria-current');
    }
  }

  function applyWidth() {
    const whole = view.width === READER_WHOLE_PAGE;
    column.style.width = view.width > 0 ? `${view.width}px` : '';
    column.classList.toggle('reader-fit', view.width === 0);
    column.classList.toggle('reader-whole', whole);
    document.getElementById('reader-fit').setAttribute('aria-pressed', String(view.width === 0));
    document.getElementById('reader-whole').setAttribute('aria-pressed', String(whole));
    if (whole) sizeWhole();
  }

  // sizeWhole makes a page exactly as tall as the window. Each page keeps its
  // own shape, so a receipt is narrow and a landscape page wide, and every one
  // is seen whole without scrolling inside it.
  function sizeWhole() {
    const pad = parseFloat(getComputedStyle(column).paddingTop) || 0;
    column.style.setProperty('--whole-height', `${Math.max(100, scroller.clientHeight - 2 * pad)}px`);
  }
  new ResizeObserver(() => {
    if (view.width === READER_WHOLE_PAGE) sizeWhole();
  }).observe(scroller);

  // zoom sizes the pages by factor, keeping the point at (x, y) in the
  // window — the cursor, or the middle — over the same spot of the page.
  function zoom(factor, x, y) {
    const box = scroller.getBoundingClientRect();
    const px = x ?? box.width / 2;
    const py = y ?? box.height / 2;
    const fx = (scroller.scrollLeft + px) / scroller.scrollWidth;
    const fy = (scroller.scrollTop + py) / scroller.scrollHeight;
    // From a fitted mode, zooming starts at the size the page is drawn now.
    const sheet = sheets()[view.current];
    const pad = parseFloat(getComputedStyle(column).paddingLeft) || 0;
    const now = view.width > 0 ? view.width
      : sheet ? sheet.getBoundingClientRect().width + 2 * pad
      : column.getBoundingClientRect().width;
    view.width = Math.round(Math.min(3200, Math.max(300, now * factor)));
    writeReaderWidth(view.width);
    applyWidth();
    scroller.scrollLeft = fx * scroller.scrollWidth - px;
    scroller.scrollTop = fy * scroller.scrollHeight - py;
  }

  function fit(mode = 0) {
    view.width = mode;
    writeReaderWidth(mode);
    applyWidth();
    sheets()[view.current]?.scrollIntoView({ block: 'start' });
  }

  const sheets = () => column.children;

  // go turns to a page and says so at once, rather than when the scroll has
  // been noticed, so a key held down turns one page per press.
  function go(index) {
    const next = Math.min(Math.max(0, index), view.pages.length - 1);
    sheets()[next]?.scrollIntoView({ block: 'start' });
    mark(next);
  }

  function clear() {
    near.disconnect();
    far.disconnect();
    shown.disconnect();
    // Dropping the pictures before the elements lets the browser free them now
    // rather than whenever it collects the detached nodes.
    for (const sheet of sheets()) sheet.firstElementChild.removeAttribute('src');
    column.innerHTML = '';
    rail.innerHTML = '';
  }

  function build() {
    const { scan, pages } = view;
    const sheetFragment = document.createDocumentFragment();
    const railFragment = document.createDocumentFragment();
    pages.forEach((page, index) => {
      const sheet = document.createElement('div');
      sheet.className = 'reader-sheet';
      sheet.dataset.page = String(page);
      sheet.dataset.index = String(index);
      sheet.style.aspectRatio = String(paper);
      const picture = document.createElement('img');
      picture.alt = `Page ${page}`;
      picture.decoding = 'async';
      picture.draggable = false;
      picture.addEventListener('load', () => {
        // The page's own shape replaces the guess, so a landscape page or a
        // receipt is not letterboxed in an A4 box.
        if (picture.naturalWidth && picture.naturalHeight) {
          sheet.style.aspectRatio = `${picture.naturalWidth} / ${picture.naturalHeight}`;
        }
        sheet.classList.add('reader-drawn');
      });
      picture.addEventListener('error', () => {
        if (picture.getAttribute('src')) sheet.classList.add('page-missing');
      });
      sheet.append(picture);
      sheetFragment.append(sheet);

      const item = document.createElement('li');
      item.className = 'page-item';
      item.dataset.index = String(index);
      item.title = `Page ${page}`;
      const thumb = document.createElement('img');
      thumb.loading = 'lazy';
      thumb.alt = `Page ${page}`;
      thumb.src = api.image(scan.digest, 'thumb', page);
      thumb.addEventListener('error', () => item.classList.add('page-missing'), { once: true });
      const number = document.createElement('span');
      number.className = 'page-number';
      number.textContent = String(page);
      item.append(thumb, number);
      item.addEventListener('click', () => go(index));
      railFragment.append(item);
    });
    column.append(sheetFragment);
    rail.append(railFragment);
    for (const sheet of sheets()) {
      near.observe(sheet);
      far.observe(sheet);
      shown.observe(sheet);
    }
  }

  // The status bar's hints belong to Browse; the reader borrows the row and
  // gives it back as it was.
  let hints = null;
  const hintsSpan = () => document.querySelector('.statusbar .status-hints');

  function close() {
    if (section.hidden) return;
    section.hidden = true;
    crumb.hidden = true;
    crumb.textContent = '';
    clear();
    view.scan = null;
    statusBar.setSummary('');
    if (hints !== null) statusBar.setHints(hints, { html: true });
    hints = null;
    if (view.closed) view.closed();
  }

  // place fits the reader between the top bar and the status bar, whatever
  // banner sits above the one or however tall the other is drawn.
  function place() {
    const top = document.querySelector('.topbar')?.getBoundingClientRect().bottom ?? 0;
    const bar = document.querySelector('.statusbar');
    const bottom = bar ? window.innerHeight - bar.getBoundingClientRect().top : 0;
    section.style.setProperty('--reader-top', `${Math.max(0, top)}px`);
    section.style.setProperty('--reader-bottom', `${Math.max(0, bottom)}px`);
  }
  window.addEventListener('resize', () => {
    if (!section.hidden) place();
  });

  // The reader's keys are its own while it is open: the details' split keys
  // must not act on a scan while it is being read, so nothing reaches Browse.
  document.addEventListener('keydown', (event) => {
    if (section.hidden) return;
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    if (event.target === handle) return;
    // Typing in the details beside the pages is typing, not reading.
    if (event.target.closest?.('input, textarea, select, [contenteditable], .detail')) return;
    const actions = {
      j: () => go(view.current + 1),
      k: () => go(view.current - 1),
      ArrowRight: () => go(view.current + 1),
      ArrowLeft: () => go(view.current - 1),
      PageDown: () => go(view.current + 1),
      PageUp: () => go(view.current - 1),
      g: () => go(0),
      Home: () => go(0),
      G: () => go(view.pages.length - 1),
      End: () => go(view.pages.length - 1),
      '+': () => zoom(1.25),
      '=': () => zoom(1.25),
      '-': () => zoom(0.8),
      '0': () => fit(0),
      w: () => fit(0),
      p: () => fit(READER_WHOLE_PAGE),
      Escape: close,
      q: close,
    };
    const action = actions[event.key];
    event.stopImmediatePropagation();
    if (!action) return;
    event.preventDefault();
    action();
  }, true);

  // Cmd or Ctrl with the wheel zooms about the cursor, as a PDF viewer does; a
  // trackpad pinch arrives the same way. The plain wheel still scrolls.
  scroller.addEventListener('wheel', (event) => {
    if (!event.metaKey && !event.ctrlKey) return;
    event.preventDefault();
    const box = scroller.getBoundingClientRect();
    zoom(Math.exp(-event.deltaY * 0.002), event.clientX - box.left, event.clientY - box.top);
  }, { passive: false });

  wireRail();

  // wireRail makes the strip of pages as wide as the person drags it, and
  // remembers it.
  function wireRail() {
    const apply = (width, keep) => {
      const clamped = Math.round(Math.min(Math.max(width, READER_RAIL.min), READER_RAIL.max));
      body.style.setProperty('--rail-width', `${clamped}px`);
      handle.setAttribute('aria-valuenow', String(clamped));
      handle.setAttribute('aria-valuemin', String(READER_RAIL.min));
      handle.setAttribute('aria-valuemax', String(READER_RAIL.max));
      if (keep) {
        try {
          localStorage.setItem(READER_RAIL.key, String(clamped));
        } catch {
          // Kept for this page load only.
        }
      }
      return clamped;
    };
    let saved = NaN;
    try {
      saved = Number(localStorage.getItem(READER_RAIL.key));
    } catch {
      // Nothing stored is the default width.
    }
    let width = apply(Number.isFinite(saved) && saved > 0 ? saved : READER_RAIL.fallback, false);
    handle.addEventListener('pointerdown', (event) => {
      if (event.button !== 0) return;
      event.preventDefault();
      handle.setPointerCapture(event.pointerId);
      const left = body.getBoundingClientRect().left;
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
      width = apply(READER_RAIL.fallback, true);
    });
    handle.addEventListener('keydown', (event) => {
      const delta = { ArrowLeft: -READER_RAIL.step, ArrowRight: READER_RAIL.step }[event.key];
      if (!delta) return;
      event.preventDefault();
      width = apply(width + delta, true);
    });
  }

  document.getElementById('reader-close').addEventListener('click', close);
  document.getElementById('reader-in').addEventListener('click', () => zoom(1.25));
  document.getElementById('reader-out').addEventListener('click', () => zoom(0.8));
  document.getElementById('reader-fit').addEventListener('click', () => fit(0));
  document.getElementById('reader-whole').addEventListener('click', () => fit(READER_WHOLE_PAGE));

  return {
    // open reads scan from its first page, or, given a split's page range, only
    // those pages, starting at the one given.
    open(scan, options = {}) {
      if (!scan) return;
      clear();
      const all = Array.from({ length: Math.max(1, scan.pages || 1) }, (_, i) => i + 1);
      const pages = options.pages ? [...pagesOf(options.pages)].sort((a, b) => a - b) : all;
      view.scan = scan;
      view.pages = pages.length ? pages : all;
      view.split = Boolean(options.pages && pages.length);
      view.current = 0;
      label.textContent = '';
      title.textContent = options.title || scan.description || scan.filename;
      title.title = scan.filename;
      crumb.textContent = title.textContent;
      crumb.hidden = false;
      view.closed = options.closed;
      if (section.hidden) {
        hints = hintsSpan()?.innerHTML ?? null;
        statusBar.setHints('<kbd>j</kbd>/<kbd>k</kbd> page · <kbd>⌘</kbd>+wheel zoom · <kbd>w</kbd>/<kbd>p</kbd> width/page · <kbd>Esc</kbd> back', { html: true });
      }
      place();
      section.hidden = false;
      applyWidth();
      build();
      scroller.scrollTop = 0;
      const start = options.page ? view.pages.indexOf(options.page) : 0;
      mark(Math.max(0, start));
      if (start > 0) go(start);
      scroller.focus();
    },
    close,
    isOpen: () => !section.hidden,
  };
}
