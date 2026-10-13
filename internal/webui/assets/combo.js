// A text box with a list of choices under it: a ▾ beside the box opens the
// whole list, typing narrows it. ↑/↓ move, Enter or a click picks, Esc
// closes. What is typed stays allowed; the list only offers.

const el = (tag, props, ...children) => {
  const node = Object.assign(document.createElement(tag), props || {});
  node.append(...children);
  return node;
};

const menu = el("ul", { className: "combo-menu", hidden: true });
document.body.append(menu);
let menuFor = null;
const closeMenu = () => { menu.hidden = true; menuFor = null; };
// The menu is placed by the box's place on screen, so a scroll closes it.
addEventListener("scroll", (event) => { if (menuFor && !menu.contains(event.target)) closeMenu(); }, true);
addEventListener("resize", closeMenu);

// suggest offers choices() under input. With arrow set, the box is wrapped
// with a ▾ button that opens the list, and the wrapper is returned. With
// search set, typing narrows by search(choice) rather than by the choice
// shown, so a choice is found by text it does not show.
export function suggest(input, choices, { arrow = false, search = null } = {}) {
  let active = -1, shown = [], all = false, picking = false;
  // A pick tells the page the box changed; the box itself must not take that
  // input for typing and open the list again. The input ime.js sends once a
  // word is composed is typing, though it is not the browser's own.
  const pick = (v) => {
    input.value = v; closeMenu();
    picking = true;
    try { input.dispatchEvent(new Event("input", { bubbles: true })); } finally { picking = false; }
    input.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const close = () => { if (menuFor === input) closeMenu(); };
  const mark = (c, q) => {
    const at = q ? c.toLowerCase().indexOf(q) : -1;
    return at < 0 ? [c] : [c.slice(0, at), el("mark", {}, c.slice(at, at + q.length)), c.slice(at + q.length)];
  };
  const draw = async () => {
    const q = all ? "" : input.value.trim().toLowerCase();
    const list = await choices();
    if (document.activeElement !== input) return;
    const exact = q && list.some((c) => c.toLowerCase() === q);
    const text = (c) => (search ? search(c) : c).toLowerCase();
    shown = (exact ? list : list.filter((c) => text(c).includes(q))).slice(0, 200);
    if (!shown.length) {
      if (!all) { close(); return; }
      menu.replaceChildren(el("li", { className: "empty" }, "Nothing known yet"));
    } else {
      active = Math.min(active, shown.length - 1);
      menu.replaceChildren(...shown.map((c, i) => el("li", {
        className: (i === active ? "active" : "") + (c === input.value ? " chosen" : ""), title: c,
        onmousedown: (event) => { event.preventDefault(); pick(c); } }, ...mark(c, exact ? "" : q))));
    }
    const r = (input.closest(".combo") || input).getBoundingClientRect();
    // Fit the choices, expanding beyond the box up to the window width.
    menu.style.minWidth = Math.min(r.width, innerWidth - 16) + "px";
    menu.style.left = "0px";
    menu.hidden = false;
    menu.style.left = Math.max(8, Math.min(r.left, innerWidth - 8 - menu.offsetWidth)) + "px";
    const below = innerHeight - r.bottom;
    if (below < 200 && r.top > below) { menu.style.top = ""; menu.style.bottom = innerHeight - r.top + 4 + "px"; }
    else { menu.style.bottom = ""; menu.style.top = r.bottom + 4 + "px"; }
    menu.hidden = false;
    menuFor = input;
    menu.children[active]?.scrollIntoView({ block: "nearest" });
  };
  input.addEventListener("focus", () => { active = -1; all = true; draw(); });
  input.addEventListener("input", () => { if (picking) return; active = -1; all = false; draw(); });
  input.addEventListener("blur", close);
  input.addEventListener("keydown", (event) => {
    if (menuFor !== input) {
      if (event.key === "ArrowDown") { event.preventDefault(); all = true; draw(); }
      return;
    }
    if ((event.key === "ArrowDown" || event.key === "ArrowUp") && shown.length) {
      event.preventDefault();
      active = (active + (event.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length;
      draw();
    } else if (event.key === "Enter" && active >= 0 && shown[active] !== undefined) {
      event.preventDefault(); pick(shown[active]);
    } else if (event.key === "Escape") {
      event.stopPropagation(); close();
    }
  });
  if (!arrow) return input;
  const button = el("button", { type: "button", className: "combo-arrow", tabIndex: -1, title: "Show the choices", "aria-label": "Show the choices",
    onmousedown: (event) => {
      event.preventDefault();
      if (menuFor === input) { close(); return; }
      if (document.activeElement === input) { active = -1; all = true; draw(); } else input.focus();
    } });
  const wrap = el("span", { className: "combo" });
  if (input.parentNode) input.replaceWith(wrap);
  wrap.append(input, button);
  return wrap;
}
