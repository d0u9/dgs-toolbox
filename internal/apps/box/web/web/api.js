// What the pages know about the server. Every call names a scan by its
// digest; none of them names a path, because a path in a request is a path out
// of the Box.

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
  intake: () => ask('/api/intake'),
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
  adopt: (path, edit) =>
    ask('/api/adopt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, edit }),
    }),
  // Verify reads every byte in the Box, so it is a POST and never something a
  // page reload starts.
  verify: () => ask('/api/verify', { method: 'POST' }),
  // page counts from 1 and is left off for the first page, which is the picture
  // every scan has.
  image: (digest, size, page) =>
    `/api/image?digest=${encodeURIComponent(digest)}&size=${size}` +
    (page && page > 1 ? `&page=${page}` : ''),
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
