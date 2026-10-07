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

// Explicit hit areas for arrows inside a number input.
export function numberStepper(input) {
  const group = document.createElement("span");
  group.className = "ui-number-stepper";
  if (input.parentNode) input.replaceWith(group);
  group.append(input);
  for (const [direction, glyph, title] of [[-1, "▼", "Decrease"], [1, "▲", "Increase"]]) {
    const control = button("icon-button", glyph, title, () => {
      if (input.disabled || input.readOnly) return;
      const step = input.step;
      if (step === "any") input.step = "1";
      try { direction > 0 ? input.stepUp() : input.stepDown(); }
      finally { input.step = step; }
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });
    group.append(control);
  }
  return group;
}
