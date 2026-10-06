// Shared dragging for static and dynamically created dialogs. Event delegation
// also covers nested confirmations without per-dialog listeners or observers.
let drag = null;
const interactive = "button, input, select, textarea, a, [contenteditable], [role=button]";

function place(dialog, left, top) {
  const rect = dialog.getBoundingClientRect();
  dialog.style.position = "fixed";
  dialog.style.margin = "0";
  dialog.style.right = "auto";
  dialog.style.bottom = "auto";
  dialog.style.left = `${Math.max(0, Math.min(left, window.innerWidth - rect.width))}px`;
  dialog.style.top = `${Math.max(0, Math.min(top, window.innerHeight - rect.height))}px`;
  dialog.dataset.moved = "true";
}

function finish(event) {
  if (!drag || (event?.pointerId !== undefined && event.pointerId !== drag.id)) return;
  const { handle, id } = drag;
  drag = null;
  handle.classList.remove("dialog-dragging");
  if (handle.hasPointerCapture(id)) handle.releasePointerCapture(id);
}

document.addEventListener("pointerdown", (event) => {
  if (event.button !== 0 || event.isPrimary === false || event.target.closest(interactive)) return;
  const handle = event.target.closest("h1, h2, h3, [data-dialog-handle]");
  const dialog = handle?.closest("dialog[open]");
  if (!dialog || handle.closest("dialog") !== dialog) return;
  finish();
  const rect = dialog.getBoundingClientRect();
  // Freeze the used size before replacing the browser's centred positioning.
  dialog.style.width = `${rect.width}px`;
  dialog.style.boxSizing = "border-box";
  place(dialog, rect.left, rect.top);
  drag = { dialog, handle, id: event.pointerId, x: event.clientX, y: event.clientY,
    left: rect.left, top: rect.top };
  handle.classList.add("dialog-dragging");
  handle.setPointerCapture(event.pointerId);
  event.preventDefault();
});

document.addEventListener("pointermove", (event) => {
  if (!drag || event.pointerId !== drag.id) return;
  if (!drag.dialog.open || !drag.dialog.isConnected) { finish(); return; }
  place(drag.dialog, drag.left + event.clientX - drag.x, drag.top + event.clientY - drag.y);
});
for (const type of ["pointerup", "pointercancel", "lostpointercapture"]) {
  document.addEventListener(type, finish);
}
document.addEventListener("close", (event) => {
  if (drag?.dialog === event.target) finish();
}, true);
window.addEventListener("blur", () => finish());
window.addEventListener("resize", () => {
  finish();
  for (const dialog of document.querySelectorAll('dialog[open][data-moved]')) {
    const rect = dialog.getBoundingClientRect();
    place(dialog, rect.left, rect.top);
  }
});

// Shared modal lifecycle. Pages build domain-specific fields; the library
// owns heading, actions, cancellation, keyboard focus and completion.
export function formDialog({ title, message, fields = [], read, confirm = "OK", cancel = "Cancel", danger = false, focus, focusCancel = false }) {
  return new Promise(resolve => {
    const opener = document.activeElement;
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    dialog.setAttribute("aria-label", title);
    const heading = document.createElement("h3"); heading.textContent = title;
    const text = document.createElement("p"); text.textContent = message || "";
    const form = document.createElement("form"); form.method = "dialog";
    const buttons = document.createElement("div"); buttons.className = "dialog-buttons";
    const no = document.createElement("button"); no.type = "button"; no.className = "button"; no.textContent = cancel;
    const yes = document.createElement("button"); yes.type = "submit"; yes.className = danger ? "button danger" : "action"; yes.textContent = confirm;
    buttons.append(no, yes); form.append(...fields, buttons); dialog.append(heading, text, form);
    let answer = null;
    no.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", event => {
      answer = read();
      if (answer == null) { event.preventDefault(); (focus || fields[0])?.focus(); }
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      if (opener?.isConnected) opener.focus();
      resolve(answer);
    });
    document.body.append(dialog); dialog.showModal();
    const initial = focusCancel ? no : focus || fields[0] || no;
    initial.focus(); initial.select?.();
  });
}
export function promptDialog({ title, message, value = "", confirm = "OK", cancel = "Cancel" }) {
  const input = document.createElement("input"); input.className = "field prompt-input"; input.value = value;
  input.setAttribute("aria-label", title);
  return formDialog({ title, message, fields: [input], confirm, cancel, read: () => input.value.trim() || null });
}
export function chooseDialog({ title, message, options, confirm = "OK", cancel = "Cancel" }) {
  const choice = document.createElement("select"); choice.className = "field prompt-input"; choice.setAttribute("aria-label", title);
  for (const option of options) {
    const item = document.createElement("option"); item.value = option.value; item.textContent = option.label; item.title = option.title || option.value; choice.append(item);
  }
  return formDialog({ title, message, fields: [choice], confirm, cancel, read: () => choice.value || null });
}
export async function confirmDialog({ title, message, detail, confirm = "Continue", cancel = "Cancel", danger = false }) {
  const fields = [];
  if (detail) { const text = document.createElement("code"); text.className = "dialog-detail"; text.textContent = detail; fields.push(text); }
  return await formDialog({ title, message, fields, confirm, cancel, danger, focusCancel: true, read: () => true }) === true;
}
