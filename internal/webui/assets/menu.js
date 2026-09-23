// The shared context menu: the small list of commands a right click opens on
// the thing under the pointer. A page says what the commands are; where the
// menu goes, how it closes and how the keyboard walks it are the same on
// every page dgs serves.

let open = null; // the one menu on the page, or null

// openMenu shows a menu at a pointer event. sections is a list of sections,
// each a list of items, drawn with a rule between them: a section groups the
// commands that belong together. An item is
//   { label, title, onSelect, disabled, danger }
// An empty or all-empty section is left out, so a caller can build sections
// from what is there without counting first. Returns nothing; the menu closes
// on a choice, on a click elsewhere, on Escape, on scroll and on resize.
export function openMenu(event, sections) {
  event.preventDefault();
  event.stopPropagation();
  closeMenu();

  const groups = sections.map((items) => items.filter(Boolean)).filter((items) => items.length > 0);
  if (groups.length === 0) return;

  const menu = document.createElement("div");
  menu.className = "context-menu";
  menu.setAttribute("role", "menu");
  for (const items of groups) {
    const group = document.createElement("div");
    group.className = "menu-section";
    group.setAttribute("role", "group");
    for (const item of items) group.append(menuItem(menu, item));
    menu.append(group);
  }

  document.body.append(menu);
  place(menu, event.clientX, event.clientY);
  open = menu;
  listen(true);
  menu.querySelector(".menu-item:not([disabled])")?.focus();
}

// closeMenu takes the open menu off the page, if there is one.
export function closeMenu() {
  if (!open) return;
  open.remove();
  open = null;
  listen(false);
}

function menuItem(menu, item) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "menu-item" + (item.danger ? " danger" : "");
  button.setAttribute("role", "menuitem");
  button.textContent = item.label;
  if (item.title) button.title = item.title;
  if (item.disabled) button.disabled = true;
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    closeMenu();
    item.onSelect?.();
  });
  button.addEventListener("keydown", (event) => {
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    const items = [...menu.querySelectorAll(".menu-item:not([disabled])")];
    const next = items[(items.indexOf(button) + (event.key === "ArrowDown" ? 1 : items.length - 1)) % items.length];
    next?.focus();
  });
  return button;
}

// place puts the menu at the pointer, flipped back inside the window when it
// would otherwise run off the right edge or the bottom.
function place(menu, x, y) {
  const { width, height } = menu.getBoundingClientRect();
  const margin = 4;
  const left = Math.max(margin, Math.min(x, window.innerWidth - width - margin));
  const top = Math.max(margin, Math.min(y, window.innerHeight - height - margin));
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
}

function onPointerDown(event) {
  if (!open?.contains(event.target)) closeMenu();
}

function onKeyDown(event) {
  if (event.key === "Escape") {
    event.stopPropagation();
    closeMenu();
  }
}

function listen(on) {
  const method = on ? "addEventListener" : "removeEventListener";
  document[method]("pointerdown", onPointerDown, true);
  document[method]("keydown", onKeyDown, true);
  window[method]("resize", closeMenu);
  window[method]("scroll", closeMenu, true);
}
