// The last successfully saved currency follows the desk between Intake and
// Browse in this browser. Sidecars still store only canonical ISO 4217 codes.
const LAST_CURRENCY = 'box.lastCurrency';

function boxTotalForSave(text) {
  const raw = String(text || '').trim();
  if (!/^[+-]?\d+(?:\.\d*)?$/.test(raw)) return text;
  const remembered = localStorage.getItem(LAST_CURRENCY);
  return remembered ? `${remembered} ${raw}` : text;
}

function rememberBoxTotal(total) {
  const code = /^([A-Z]{3})\s/.exec(total || '')?.[1];
  if (code) localStorage.setItem(LAST_CURRENCY, code);
}
