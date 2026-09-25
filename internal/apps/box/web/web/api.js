// What the pages know about the server. Every call names a scan by its
// digest; none of them names a path in the Box, because a path in a request is
// a path out of it. The one path sent is a folder to read as the inbox, which
// the server refuses inside the Box.

// pictureVersion changes when pictures are redrawn. The server lets the browser
// keep a picture for an hour, so a redrawn one is asked for under a new URL.
let pictureVersion = 0;

async function ask(path, options) {
  const response = await fetch(path, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || `${response.status} ${response.statusText}`);
  }
  return body;
}

const api = {
  config: () => ask('/api/config'),
  types: () => ask('/api/types'),
  tags: () => ask('/api/tags'),
  intake: () => ask('/api/intake'),
  inbox: (dir) =>
    ask('/api/inbox', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir }),
    }),
  scans: () => ask('/api/scans'),
  exceptions: () => ask('/api/exceptions'),
  patch: (edit) =>
    ask('/api/scan', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(edit),
    }),
  file: (edit) =>
    ask('/api/intake/file', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(edit),
    }),
  trash: (digest, reason) =>
    ask('/api/trash', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digest, reason }),
    }),
  patchMany: (digests, edit) =>
    ask('/api/scans', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digests, edit }),
    }),
  trashMany: (digests, reason) =>
    ask('/api/trash/batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digests, reason }),
    }),
  trashSummary: () => ask('/api/trash'),
  incomplete: () => ask('/api/incomplete'),
  rejected: () => ask('/api/rejected'),
  rejectedFile: (digest) => `/api/rejected/file?digest=${encodeURIComponent(digest)}`,
  reveal: (digest) =>
    ask('/api/reveal', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digest }),
    }),
  unfile: (digest) =>
    ask('/api/unfile', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digest }),
    }),
  restore: (digest) =>
    ask('/api/rejected/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digest }),
    }),
  adopt: (path, edit) =>
    ask('/api/adopt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, edit }),
    }),
  // Verify reads every byte in the Box, so it is a POST and never something a
  // page reload starts.
  verify: () => ask('/api/verify', { method: 'POST' }),
  // redraw drops a filed scan's stored pictures and draws them again; with no
  // digest it redraws every filed scan, which reads every file.
  redraw: async (digest) => {
    const body = await ask('/api/redraw', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ digest: digest || '' }),
    });
    pictureVersion = Date.now();
    return body;
  },
  // page counts from 1 and is left off for the first page, which is the picture
  // every scan has.
  image: (digest, size, page) =>
    `/api/image?digest=${encodeURIComponent(digest)}&size=${size}` +
    (page && page > 1 ? `&page=${page}` : '') +
    (pictureVersion ? `&v=${pictureVersion}` : ''),
};

// localDate reads a scan's own timestamp in the offset it was recorded with,
// never converted: the person remembers the local time, so that is what is
// shown.
function localDate(stamp) {
  if (!stamp) return '';
  return stamp.slice(0, 10);
}

function localTime(stamp) {
  if (!stamp || stamp.length < 16) return '';
  return stamp.slice(11, 16);
}

// zoneOf is the offset a timestamp carries, as written.
function zoneOf(stamp) {
  if (!stamp) return '';
  const match = stamp.match(/([+-]\d{2}:\d{2}|Z)$/);
  return match ? match[1] : '';
}

// handling is what has been done to a scan beyond giving it a type — split,
// pages ignored, kept for good — as badges both lists draw the same way.
function handling(scan) {
  const out = [];
  const documents = scan.documents ?? [];
  if (documents.length) {
    out.push({
      text: `${documents.length} split${documents.length === 1 ? '' : 's'}`,
      title: `Split into p ${documents.map((document) => document.pages).join(' · p ')}`,
    });
  }
  if (scan.ignoredPages) out.push({ text: `p ${scan.ignoredPages} ignored`, title: 'Pages left out of every split' });
  if (scan.expiryCleared) out.push({ text: 'kept', title: 'Kept for good — no expiry' });
  return out;
}

// badgeHTML draws handling's badges; the text is the page's own, so only the
// hover title needs escaping.
function badgeHTML(badges) {
  return badges
    .map((badge) => {
      const title = badge.title.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
      return `<span class="badge" title="${title}">${badge.text}</span>`;
    })
    .join('');
}
