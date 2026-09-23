// The page view of a PDF: a strip of one thumbnail per page beside the scan,
// shown and hidden by the person, and the page picked in it drawn large.
//
// Both pages use it, so it owns only what they share — which page is shown,
// whether the strip is open, and drawing the strip. Where the strip sits and
// how big the large picture is stay each page's own CSS.
//
// A page is asked for only when its thumbnail scrolls into view, because a
// fifty-page document is fifty reads of the scan the first time it is opened.

const PAGES_OPEN_KEY = 'dgs.box.pages.open';

// The strip is open unless the person closed it: most scans worth a second
// look have more than one page, and a closed strip hides that they do.
function readPagesOpen() {
  try {
    return localStorage.getItem(PAGES_OPEN_KEY) !== '0';
  } catch {
    return true;
  }
}

function writePagesOpen(open) {
  try {
    localStorage.setItem(PAGES_OPEN_KEY, open ? '1' : '0');
  } catch {
    // A private window keeps the choice for this page load only.
  }
}

// pageView wires one strip. ids names its elements: strip (an <ol>), toggle
// (a button), image (the large <img>) and label (where "page 2 of 5" goes).
// decorate, when given, is called with each strip item and its page number
// after every redraw, so a page can mark pages without owning the strip.
// turned, when given, is told the new page whenever the person turns to one.
function pageView(ids, decorate, turned) {
  const strip = document.getElementById(ids.strip);
  const toggle = document.getElementById(ids.toggle);
  const image = document.getElementById(ids.image);
  const label = document.getElementById(ids.label);
  const view = { scan: null, page: 1, open: readPagesOpen() };

  // A PDF gets the strip whatever its length; a picture file is one page and
  // there is nothing to turn.
  const applies = (scan) => !!scan && scan.kind === 'pdf' && scan.pages >= 1;

  // A page with no embedded picture — a vector or text PDF — cannot be drawn:
  // the tool extracts pictures and has no PDF rasteriser. The frame says so
  // instead of showing a broken image with its alt text spilling out.
  const frame = image.parentElement;
  image.addEventListener('load', () => frame.classList.remove('no-picture'));
  image.addEventListener('error', () => frame.classList.add('no-picture'));

  function drawLarge() {
    const scan = view.scan;
    frame.classList.remove('no-picture');
    image.src = api.image(scan.digest, 'preview', view.page);
    preload(scan);
    image.alt = scan.pages > 1 ? `${scan.filename}, page ${view.page}` : scan.filename;
    label.textContent = applies(scan) && scan.pages > 1 ? `page ${view.page} of ${scan.pages}` : '';
    for (const item of strip.children) {
      const current = Number(item.dataset.page) === view.page;
      item.classList.toggle('page-current', current);
      if (current) item.setAttribute('aria-current', 'page');
      else item.removeAttribute('aria-current');
      if (decorate) decorate(item, Number(item.dataset.page));
    }
  }

  // preload asks for the pages either side of the one shown, so the next
  // turn finds its picture already drawn and in the browser's cache.
  const preloaded = [];
  function preload(scan) {
    preloaded.length = 0;
    if (!applies(scan)) return;
    for (const page of [view.page + 1, view.page - 1, view.page + 2]) {
      if (page < 1 || page > scan.pages) continue;
      const picture = new Image();
      picture.src = api.image(scan.digest, 'preview', page);
      preloaded.push(picture);
    }
  }

  function drawStrip() {
    const scan = view.scan;
    const usable = applies(scan);
    toggle.hidden = !usable;
    toggle.setAttribute('aria-pressed', String(usable && view.open));
    strip.hidden = !(usable && view.open);
    strip.innerHTML = '';
    if (!usable || !view.open) return;
    for (let page = 1; page <= scan.pages; page++) {
      const item = document.createElement('li');
      item.className = 'page-item';
      item.dataset.page = String(page);
      item.title = `Page ${page}`;
      const picture = document.createElement('img');
      picture.loading = 'lazy';
      picture.alt = `Page ${page}`;
      picture.src = api.image(scan.digest, 'thumb', page);
      // A page with no picture of its own — a vector page, a blank one — still
      // has its place in the strip, so the numbering never lies.
      picture.addEventListener('error', () => item.classList.add('page-missing'), { once: true });
      const number = document.createElement('span');
      number.className = 'page-number';
      number.textContent = String(page);
      item.append(picture, number);
      item.addEventListener('click', () => go(page));
      strip.append(item);
    }
  }

  function go(page) {
    const scan = view.scan;
    if (!scan) return;
    const last = Math.max(1, scan.pages || 1);
    const next = Math.min(Math.max(1, page), last);
    if (next === view.page) return;
    view.page = next;
    if (turned) turned(next);
    drawLarge();
    strip.querySelector('.page-current')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  toggle.addEventListener('click', () => togglePages());

  function togglePages() {
    if (!applies(view.scan)) return;
    view.open = !view.open;
    writePagesOpen(view.open);
    drawStrip();
    drawLarge();
  }

  // open shows the strip without remembering it as the person's choice: a
  // split needs the pages, and closing them again is still one key.
  function open() {
    if (!applies(view.scan) || view.open) return;
    view.open = true;
    drawStrip();
    drawLarge();
  }

  return {
    // show draws a scan from its first page, or redraws the same scan where
    // the person left it.
    show(scan) {
      if (!scan) {
        view.scan = null;
        toggle.hidden = true;
        strip.hidden = true;
        strip.innerHTML = '';
        label.textContent = '';
        return;
      }
      const same = view.scan && view.scan.digest === scan.digest;
      view.scan = scan;
      if (!same) {
        view.page = 1;
        drawStrip();
      }
      drawLarge();
    },
    toggle: togglePages,
    open,
    step: (delta) => go(view.page + delta),
    go,
    page: () => view.page,
    applies: () => applies(view.scan),
    // redraw reapplies decorate without reloading a picture.
    redraw: () => view.scan && drawLarge(),
  };
}
