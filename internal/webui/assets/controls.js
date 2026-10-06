// DOM builders for controls and two-line rows. Pages supply the action;
// shared controls supply button semantics and stop a row click bubbling.
export function button(className, text, title, onClick) {
  const element = document.createElement("button");
  element.type = "button";
  element.className = className;
  element.textContent = text;
  if (title) { element.title = title; element.setAttribute("aria-label", title); }
  element.addEventListener("click", event => { event.stopPropagation(); return onClick?.(event); });
  return element;
}
export function iconButton(className, icon, title, onClick) {
  const element = button(className, "", title, onClick);
  element.innerHTML = icon;
  return element;
}
export function textButton(text, disabled, onClick, className = "text-button") {
  const element = button(className, text, "", onClick);
  element.disabled = disabled;
  return element;
}
export function rowLabel(name, details = []) {
  const box = document.createElement("span"); box.className = "label ui-row-label";
  const title = document.createElement("span"); title.className = "name"; title.textContent = name;
  const meta = document.createElement("span"); meta.className = "meta"; meta.textContent = details.filter(Boolean).join(" · ");
  box.append(title, meta);
  return box;
}
