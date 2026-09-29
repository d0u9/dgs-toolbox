// A rule's if drawn as bubbles: each comparison a bubble of key, operator
// and values chosen from lists, joined by && or ||, with brackets as boxes
// that nest. The text box it wraps stays the value; a Text switch shows it
// for typing, and the server's reading of the text redraws the bubbles.
import { el, post } from "/common.js";

// The operators a bubble offers; a leading ! negates.
const OPS = [["==", "=="], ["!=", "!="], ["in", "in"], ["!in", "not in"], ["~", "~ contains"], ["!~", "not ~"], ["has", "has"], ["!has", "not has"]];
const bare = (s) => /^[^\s()[\],"&|!=~]+$/.test(s) && s !== "in" && s !== "has";
const quote = (s) => bare(s) ? s : '"' + s + '"';

// format writes a node as text: the reverse of expr.Tree.
export function format(n, top = true) {
  if (!n) return "";
  if (n.op) {
    const items = n.items.filter((i) => i.op ? i.items.length : i.key);
    if (!items.length) return "";
    const inner = items.map((i) => format(i, false)).join(" " + n.op + " ");
    return n.not ? "!(" + inner + ")" : top || items.length === 1 ? inner : "(" + inner + ")";
  }
  if (!n.key) return "";
  const vals = n.values || [];
  switch (n.cmp) {
    case "has": return (n.not ? "!" : "") + "has(" + n.key + ")";
    case "in": return (n.not ? "!(" : "") + n.key + " in [" + vals.map(quote).join(", ") + "]" + (n.not ? ")" : "");
    case "~": return (n.not ? "!(" : "") + n.key + " ~ " + quote(vals[0] || "") + (n.not ? ")" : "");
    default: return n.key + (n.not ? " != " : " == ") + quote(vals[0] || "");
  }
}

let dragging = null; // the bubble being dragged: { list, n }
let lists = 0;

// condition wraps input, in place when it is in the page, a text box holding an if. keys() lists the keys
// to offer; values(key) the values known for one. onEdit is called after
// the bubbles change the text.
export function condition(input, { keys, values, onEdit }) {
  let tree = { op: "&&", items: [] };
  let text = false;
  const box = el("div", { className: "cond" });
  const toggle = el("button", { type: "button", className: "button cond-text", textContent: "Text", title: "Type the condition as text" });
  const wrap = el("div", { className: "cond-wrap" });
  if (input.parentNode) input.replaceWith(wrap);
  wrap.append(box, input, toggle);
  const show = () => { input.hidden = !text; box.hidden = text; toggle.classList.toggle("on", text); };
  toggle.onclick = () => { text = !text; show(); if (text) input.focus(); };
  show();

  const edited = () => {
    input.value = format(tree);
    input.dispatchEvent(new Event("input"));
    onEdit?.();
    draw();
  };
  const act = (title, t, onclick) => el("button", { type: "button", className: "cond-x", title, textContent: t, onclick });
  const listId = () => "cond-list-" + (++lists);

  const bubble = (n, list) => {
    const keyIn = el("input", { className: "cond-key mono", value: n.key || "", placeholder: "key", spellcheck: false, autocomplete: "off" });
    const kid = listId();
    keyIn.setAttribute("list", kid);
    const keyList = el("datalist", { id: kid }, ...keys().map((k) => el("option", { value: k })));
    keyIn.onchange = () => { n.key = keyIn.value.trim(); edited(); };
    const opSel = el("select", { className: "cond-op mono" }, ...OPS.map(([v, t]) => el("option", { value: v, textContent: t })));
    opSel.value = (n.not && n.cmp !== "==" ? "!" : "") + (n.cmp === "==" && n.not ? "!=" : n.cmp || "==");
    opSel.onchange = () => {
      const v = opSel.value;
      n.not = v.startsWith("!");
      n.cmp = v === "!=" ? "==" : v.replace(/^!/, "");
      if (n.cmp === "has") n.values = [];
      else if (n.cmp !== "in") n.values = (n.values || []).slice(0, 1);
      edited();
    };
    const vid = listId();
    const known = n.key ? values(n.key) : [];
    const valList = el("datalist", { id: vid }, ...known.map((v) => el("option", { value: v })));
    const valueIn = (v, i) => {
      const input = el("input", { className: "cond-value", value: v, placeholder: "value", spellcheck: false, autocomplete: "off" });
      input.setAttribute("list", vid);
      input.size = Math.max(4, [...v].length + 1);
      input.onchange = () => {
        const x = input.value.trim();
        n.values = n.values || [];
        if (x) n.values[i] = x; else n.values.splice(i, 1);
        edited();
      };
      return input;
    };
    let vals = [];
    if (n.cmp === "in") {
      vals = [el("span", { className: "cond-bracket" }, "["),
        ...(n.values || []).map((v, i) => el("span", { className: "cond-chip" }, valueIn(v, i), act("Remove the value", "×", () => { n.values.splice(i, 1); edited(); }))),
        (() => { const add = valueIn("", (n.values || []).length); add.placeholder = "+"; add.classList.add("cond-more"); add.title = "Add a value"; return add; })(),
        el("span", { className: "cond-bracket" }, "]")];
    } else if (n.cmp !== "has") {
      vals = [valueIn((n.values || [])[0] || "", 0)];
    }
    const node = el("span", { className: "cond-bubble" + (n.key ? "" : " empty") + (n.not ? " negated" : "") }, keyIn, keyList, opSel, ...vals, valList,
      act("Remove the condition", "×", () => { list.splice(list.indexOf(n), 1); edited(); }));
    return node;
  };

  // group draws a list joined by one operator; nested, it is a bracket box.
  const group = (g, parent) => {
    const joiner = () => el("button", { type: "button", className: "cond-join mono", textContent: g.op, title: "Switch && and ||",
      onclick: () => { g.op = g.op === "&&" ? "||" : "&&"; edited(); } });
    const parts = [];
    g.items.forEach((n, i) => {
      if (i) parts.push(joiner());
      const part = n.op ? group(n, g.items) : bubble(n, g.items);
      draggable(part, n, g.items);
      parts.push(part);
    });
    const adds = el("span", { className: "cond-adds" },
      act("Add a condition", "+", () => { g.items.push({ key: "", cmp: "==", values: [] }); edited(); }),
      act("Add brackets: conditions joined the other way", "(+)", () => { g.items.push({ op: g.op === "&&" ? "||" : "&&", items: [{ key: "", cmp: "==", values: [] }] }); edited(); }));
    if (!parent) return el("div", { className: "cond-group top" }, ...parts, adds);
    const neg = el("button", { type: "button", className: "cond-not mono" + (g.not ? " on" : ""), textContent: "!", title: "Negate the brackets",
      onclick: () => { g.not = !g.not; edited(); } });
    return el("span", { className: "cond-group" + (g.not ? " negated" : "") }, neg, ...parts, adds, act("Remove the brackets and what is in them", "×", () => { parent.splice(parent.indexOf(g), 1); edited(); }));
  };

  // A bubble or bracket box dragged onto another drops before it, in its
  // list; never into itself.
  const inside = (n, list) => !!n.items && (n.items === list || n.items.some((c) => inside(c, list)));
  const draggable = (node, n, list) => {
    node.draggable = true;
    node.addEventListener("dragstart", (event) => {
      if (event.target !== node) return;
      event.stopPropagation(); dragging = { list, n }; event.dataTransfer.effectAllowed = "move"; node.classList.add("dragging");
    });
    node.addEventListener("dragend", () => node.classList.remove("dragging"));
    const fits = () => dragging && dragging.n !== n && !inside(dragging.n, list);
    node.addEventListener("dragover", (event) => { if (!fits()) return; event.preventDefault(); event.stopPropagation(); node.classList.add("drop"); });
    node.addEventListener("dragleave", () => node.classList.remove("drop"));
    node.addEventListener("drop", (event) => {
      node.classList.remove("drop");
      if (!fits()) return;
      event.preventDefault(); event.stopPropagation();
      const from = dragging; dragging = null;
      from.list.splice(from.list.indexOf(from.n), 1);
      list.splice(list.indexOf(n), 0, from.n);
      edited();
    });
  };

  const draw = () => box.replaceChildren(group(tree, null));

  // read has the server read the text, and redraws the bubbles from it;
  // text it cannot read keeps the Text box open.
  let asked = 0;
  const read = async () => {
    const mine = ++asked, s = input.value.trim();
    let next = { op: "&&", items: [] }, bad = false;
    if (s) {
      try {
        const t = (await post("/api/rules/if", { if: s })).tree;
        next = t.op && !t.not ? t : { op: "&&", items: [t] };
      } catch {
        bad = true;
      }
    }
    if (mine !== asked) return;
    if (bad) { text = true; show(); return; }
    tree = next;
    draw();
  };
  input.addEventListener("change", read);
  draw();
  return { el: wrap, read };
}
