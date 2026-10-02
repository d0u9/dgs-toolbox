// The shared list table, as in Finder's list view: a header per column,
// a click on one sorts by it and again the other way, its right edge drags
// it wider or narrower (a double-click puts it back), a header dragged onto
// another moves it there, and a right click on the header, or its +,
// chooses the columns shown. Order, widths and sort are remembered in this
// browser under a key. The page says what the columns are and what a row
// is; drawing, sorting and remembering are the same on every page.
import { openMenu } from "/ui/menu.js";

const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return node;
};
const natural = (a, b) => String(a).localeCompare(String(b), undefined, { numeric: true });
const FILLER = 36;

// listTable draws into host. Options:
//   key      where order, widths and sort are kept; none keeps nothing.
//   columns  every column there may be, or a function answering them, each
//            { id, text, title, width = 130, cell(row), value(row),
//              fixed, sortable = true, resizable = true, down, group }.
//            cell answers a string, a node or a whole <td>; value is what
//            sorts and defaults to cell's text. A fixed column is always
//            shown, first, in the order given, and cannot be moved or
//            hidden. down sorts it newest or largest first on its first
//            click. group puts it in that section of the column menu.
//   className  added to the table's, such as "striped".
//   chooser  false shows every column in the order given, with no menu.
//   movable  false keeps headers where they are.
//   sorting  { get(), set(sort) } when the page owns the order, such as a
//            Sort control beside the list: the rows come sorted, a click
//            on a header calls set with { id, down } and the page draws.
//   head(th, column)  dresses a header, such as with a tooltip.
//   shown    the ids of the other columns shown at first, in order.
//   sort     the first sort, { id, down }; "" or no id is the first column.
//   lead     a column drawn before all, neither sorted nor sized:
//            { head(), cell(row), width = 32 }, such as a tick box.
//   row(data, cells)  the <tr> for one row around its cells; plain without.
//   empty    the text drawn when there are no rows.
// It answers { draw(rows), get rows(), get table() }; draw remembers the
// rows, so a sort or a column change draws them again.
export function listTable(host, { key = "", columns, shown = [], sort = {}, lead = null, row = null, empty = "Nothing to show.",
  className = "", chooser = true, movable = true, sorting = null, head: dress = null } = {}) {
  const all = () => (typeof columns === "function" ? columns() : columns);
  let kept = {};
  try { kept = (key && JSON.parse(localStorage.getItem(key) || "null")) || {}; } catch { kept = {}; }
  let order = kept.shown || [...shown];
  let sortBy = kept.sort || { id: sort.id || "", down: !!sort.down };
  let widths = kept.widths || {};
  const current = () => (sorting ? sorting.get() : sortBy);
  const keep = () => { if (key) try { localStorage.setItem(key, JSON.stringify({ shown: order, sort: sortBy, widths })); } catch { /* this page only */ } };
  let rows = [];
  let table = null;
  let moving = "";

  const text = (v) => (v instanceof Node ? v.textContent : v ?? "");
  const valueOf = (c, r) => (c.value ? c.value(r) : text(c.cell(r)));
  const visible = () => {
    const list = all();
    if (!chooser) return list;
    const byId = new Map(list.map((c) => [c.id, c]));
    return [...list.filter((c) => c.fixed), ...order.map((id) => byId.get(id)).filter((c) => c && !c.fixed)];
  };

  function menu(event) {
    const groups = new Map();
    for (const c of all().filter((c) => !c.fixed)) {
      const g = c.group || "";
      if (!groups.has(g)) groups.set(g, []);
      groups.get(g).push({ label: (order.includes(c.id) ? "✓ " : " ") + c.text, onSelect: () => {
        order = order.includes(c.id) ? order.filter((id) => id !== c.id) : [...order, c.id];
        keep();
        draw();
      } });
    }
    openMenu(event, [...groups.values()]);
  }

  // fit gives each column the width it was dragged to and the room left to
  // the empty column after the last, so the rows still reach the side and a
  // narrower column leaves room at the right rather than widening another.
  function fit() {
    if (!table) return;
    const cols = [...table.querySelectorAll("col")];
    const total = cols.slice(0, -1).reduce((n, c) => n + parseFloat(c.style.width), 0);
    const room = Math.max(FILLER, host.clientWidth - total - 2);
    cols.at(-1).style.width = room + "px";
    table.style.width = total + room + "px";
  }

  function head(c, list) {
    const first = c === list[0];
    const s = current();
    const sortable = c.sortable !== false;
    const on = sortable && (s.id === c.id || (!s.id && first && !sorting));
    const th = el("th", { className: "lt-head" + (on ? " sorted" : "") + (sortable ? " sortable" : ""),
      title: c.title || [sortable && "Click to sort", movable && !c.fixed && "drag to move", chooser && "right-click for columns"].filter(Boolean).join("; ") },
      el("span", { className: "lt-text" }, c.text, on ? el("span", { className: "lt-arrow" }, s.down ? " ▼" : " ▲") : null));
    if (sortable) {
      th.setAttribute("aria-sort", on ? (s.down ? "descending" : "ascending") : "none");
      th.addEventListener("click", () => {
        const next = on ? { id: c.id, down: !s.down } : { id: c.id, down: !!c.down };
        if (sorting) return sorting.set(next);
        sortBy = next;
        keep();
        draw();
      });
    }
    if (dress) dress(th, c);
    if (c.resizable === false) return th;
    const grip = el("span", { className: "lt-grip", title: "Drag to size; double-click to fit" });
    grip.addEventListener("click", (event) => event.stopPropagation());
    // A double-click fits the column to its widest cell, the header too: a
    // cell does not wrap, so its scrollWidth is its content at full length.
    grip.addEventListener("dblclick", (event) => {
      event.stopPropagation();
      const n = [...th.parentElement.children].indexOf(th);
      let widest = th.scrollWidth;
      for (const tr of table.tBodies[0].rows) if (tr.cells[n]) widest = Math.max(widest, tr.cells[n].scrollWidth);
      widths[c.id] = Math.max(48, widest + 2);
      keep();
      draw();
    });
    grip.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      event.stopPropagation();
      const col = table.querySelector(`col[data-id="${CSS.escape(c.id)}"]`);
      const from = event.clientX, start = parseFloat(col.style.width);
      grip.setPointerCapture(event.pointerId);
      const move = (e) => { widths[c.id] = Math.max(48, Math.round(start + e.clientX - from)); col.style.width = widths[c.id] + "px"; fit(); };
      const up = () => { grip.removeEventListener("pointermove", move); grip.removeEventListener("pointerup", up); keep(); };
      grip.addEventListener("pointermove", move);
      grip.addEventListener("pointerup", up);
    });
    th.append(grip);
    if (movable && chooser && !c.fixed) {
      th.draggable = true;
      th.addEventListener("dragstart", (event) => { moving = c.id; event.dataTransfer.setData("text/plain", c.id); event.dataTransfer.effectAllowed = "move"; });
      th.addEventListener("dragend", () => { moving = ""; });
      th.addEventListener("dragover", (event) => { if (moving && moving !== c.id) { event.preventDefault(); th.classList.add("drop"); } });
      th.addEventListener("dragleave", () => th.classList.remove("drop"));
      th.addEventListener("drop", (event) => {
        event.preventDefault();
        const from = event.dataTransfer.getData("text/plain") || moving;
        if (!from || from === c.id) return;
        const rest = order.filter((id) => id !== from);
        rest.splice(rest.indexOf(c.id) + (order.indexOf(from) < order.indexOf(c.id) ? 1 : 0), 0, from);
        order = rest;
        keep();
        draw();
      });
    }
    return th;
  }

  function draw() {
    const list = visible();
    if (!rows.length) { table = null; host.replaceChildren(...(empty ? [el("p", { className: "muted lt-empty" }, empty)] : [])); return; }
    const by = list.find((c) => c.id === sortBy.id && c.sortable !== false) || list.find((c) => c.sortable !== false);
    const sorted = sorting || !by ? rows : [...rows].sort((a, b) =>
      (natural(valueOf(by, a), valueOf(by, b)) || natural(valueOf(list[0], a), valueOf(list[0], b))) * (sortBy.down ? -1 : 1));
    const col = (id, width) => { const c = el("col"); c.style.width = width + "px"; c.dataset.id = id; return c; };
    table = el("table", { className: ("list-table " + className).trim() },
      el("colgroup", {}, lead ? col("", lead.width || 32) : null,
        ...list.map((c) => col(c.id, widths[c.id] || c.width || 130)), col("", FILLER)),
      el("thead", chooser ? { oncontextmenu: menu } : {}, el("tr", {},
        lead ? el("th", { className: "lt-lead" }, lead.head ? lead.head() : "") : null,
        ...list.map((c) => head(c, list)),
        chooser ? el("th", { className: "lt-plus", title: "Choose columns", onclick: (event) => { event.stopPropagation(); menu(event); } }, "+") : el("th"))),
      el("tbody", {}, ...sorted.map((r) => {
        const cells = [
          lead ? el("td", { className: "lt-lead" }, lead.cell(r)) : null,
          ...list.map((c) => { const v = c.cell(r); return v instanceof HTMLTableCellElement ? v : el("td", { title: text(v) }, v ?? ""); }),
          el("td"),
        ].filter(Boolean);
        return row ? row(r, cells) : el("tr", {}, ...cells);
      })));
    host.replaceChildren(table);
    fit();
  }

  new ResizeObserver(fit).observe(host);
  return {
    draw(next = rows) { rows = next; draw(); },
    get rows() { return rows; },
    get table() { return table; },
  };
}
