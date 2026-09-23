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

function readPagesOpen() {
  try {
    return localStorage.getItem(PAGES_OPEN_KEY) === '1';
  } catch {
    return false;
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
function pageView(ids) {
  const strip = document.getElementById(ids.strip);
  const toggle = document.getElementById(ids.toggle);
  const image = document.getElementById(ids.image);
  const label = document.getElementById(ids.label);
  const view = { scan: null, page: 1, open: readPagesOpen() };

  // A PDF gets the strip whatever its length; a picture file is one page and
  // there is nothing to turn.
  const applies = (scan) => !!scan && scan.kind === 'pdf' && scan.pages >= 1;

  function drawLarge() {
    const scan = view.scan;
    image.src = api.image(scan.digest, 'preview', view.page);
    image.alt = scan.pages > 1 ? `${scan.filename}, page ${view.page}` : scan.filename;
    label.textContent = applies(scan) && scan.pages > 1 ? `page ${view.page} of ${scan.pages}` : '';
    for (const item of strip.children) {
      const current = Number(item.dataset.page) === view.page;
      item.classList.toggle('page-current', current);
      if (current) item.setAttribute('aria-current', 'page');
      else item.removeAttribute('aria-current');
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
    step: (delta) => go(view.page + delta),
    applies: () => applies(view.scan),
  };
}
