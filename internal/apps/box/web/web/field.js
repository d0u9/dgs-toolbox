'use strict';

// focusNextField moves the cursor to the field after `from` in its form, the
// way Enter walks a form top to bottom: date, zone, description, total, tags.
// The last field has nowhere to go, so it lets go instead, and a second Enter
// is then the page's.
function focusNextField(from) {
  const scope = from.closest('form, section, aside') || document.body;
  const next = [...scope.querySelectorAll('input, select, textarea')].find((each) =>
    each.type !== 'hidden' && !each.disabled && each.offsetParent !== null
    && !from.contains(each)
    && (from.compareDocumentPosition(each) & Node.DOCUMENT_POSITION_FOLLOWING));
  if (next) {
    next.focus();
    if (next.select) next.select();
  } else if (from.contains(document.activeElement)) {
    document.activeElement.blur();
  }
}

// enterMovesOn makes Enter in any plain text field of a form go to the next
// field. A field that answers Enter itself — a date box, an open zone list,
// the tag field — stops or prevents the event first, and is left alone.
function enterMovesOn(form) {
  form.addEventListener('keydown', (event) => {
    if (event.key !== 'Enter' || event.defaultPrevented) return;
    if (event.isComposing || event.keyCode === 229) return;
    const target = event.target;
    if (target.tagName !== 'INPUT' || target.type !== 'text') return;
    event.preventDefault();
    focusNextField(target);
  });
}
