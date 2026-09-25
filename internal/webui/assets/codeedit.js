// A small YAML editor for every page dgs serves: line numbers, colour and
// folding over a plain textarea, so typing, undo and the clipboard stay the
// browser's own. Folded lines leave the textarea and come back in value.

// codeEditor fills host and returns { value, focus }. onChange is called
// after each edit, onSave on Ctrl/Cmd+S.
export function codeEditor(host, { onChange = () => {}, onSave = null } = {}) {
  host.classList.add("code-edit");
  const gutter = document.createElement("div");
  gutter.className = "code-gutter";
  const body = document.createElement("div");
  body.className = "code-body";
  const shown = document.createElement("pre");
  shown.className = "code-shown";
  shown.setAttribute("aria-hidden", "true");
  const area = document.createElement("textarea");
  area.className = "code-area";
  area.spellcheck = false;
  area.autocomplete = "off";
  area.wrap = "off";
  body.append(shown, area);
  host.replaceChildren(gutter, body);

  // folds: each is a visible line index and the lines hidden after it.
  let folds = [];
  let lines = [""];

  const full = () => {
    const out = [];
    lines.forEach((line, i) => {
      out.push(line);
      for (const f of folds) if (f.at === i) out.push(...f.hidden);
    });
    return out.join("\n");
  };

  const draw = () => {
    lines = area.value.split("\n");
    const at = new Map(folds.map((f) => [f.at, f]));
    shown.replaceChildren(...lines.map((line, i) => {
      const row = document.createElement("div");
      row.className = "code-line";
      row.append(...highlight(line));
      const f = at.get(i);
      if (f) row.append(span("code-folded", ` ⋯ ${f.hidden.length} lines`));
      if (!line && !f) row.append("​");
      return row;
    }));
    let number = 1;
    gutter.replaceChildren(...lines.map((line, i) => {
      const row = document.createElement("div");
      row.className = "code-number";
      const f = at.get(i);
      const toggle = document.createElement("span");
      toggle.className = "code-fold";
      if (f) {
        toggle.textContent = "▸";
        toggle.classList.add("folded");
        toggle.title = "Unfold";
        toggle.onclick = () => unfold(f);
      } else if (foldEnd(lines, i) > i) {
        toggle.textContent = "▾";
        toggle.title = "Fold";
        toggle.onclick = () => fold(i);
      }
      row.append(String(number), toggle);
      number += 1 + (f ? f.hidden.length : 0);
      return row;
    }));
    scroll();
  };

  const scroll = () => {
    shown.style.transform = `translate(${-area.scrollLeft}px, ${-area.scrollTop}px)`;
    gutter.style.transform = `translateY(${-area.scrollTop}px)`;
  };

  // replace sets the textarea, keeping the caret where it was when it can.
  const replace = (text) => {
    const caret = area.selectionStart;
    area.value = text;
    area.setSelectionRange(Math.min(caret, text.length), Math.min(caret, text.length));
  };

  const fold = (i) => {
    const end = foldEnd(lines, i);
    const hidden = lines.slice(i + 1, end + 1);
    // Folds inside the new one go into it, unfolded, so none is lost.
    const inner = folds.filter((f) => f.at > i && f.at <= end);
    const expanded = [];
    hidden.forEach((line, k) => {
      expanded.push(line);
      for (const f of inner) if (f.at === i + 1 + k) expanded.push(...f.hidden);
    });
    folds = folds.filter((f) => !inner.includes(f)).map((f) => f.at > end ? { ...f, at: f.at - hidden.length } : f);
    folds.push({ at: i, hidden: expanded });
    replace([...lines.slice(0, i + 1), ...lines.slice(end + 1)].join("\n"));
    draw();
  };

  const unfold = (f) => {
    folds = folds.filter((x) => x !== f).map((x) => x.at > f.at ? { ...x, at: x.at + f.hidden.length } : x);
    replace([...lines.slice(0, f.at + 1), ...f.hidden, ...lines.slice(f.at + 1)].join("\n"));
    draw();
  };

  // After an edit, folds before it stay, folds after it move with the lines,
  // and a fold whose own line changed opens in place.
  area.addEventListener("input", () => {
    const next = area.value.split("\n");
    let head = 0;
    while (head < lines.length && head < next.length && lines[head] === next[head]) head++;
    let tail = 0;
    while (tail < lines.length - head && tail < next.length - head &&
      lines[lines.length - 1 - tail] === next[next.length - 1 - tail]) tail++;
    const shift = next.length - lines.length;
    const opened = [];
    folds = folds.flatMap((f) => {
      if (f.at < head) return [f];
      if (f.at >= lines.length - tail) return [{ ...f, at: f.at + shift }];
      opened.push(f);
      return [];
    });
    if (opened.length) {
      // Put the hidden lines back after the edit, where the fold was.
      let text = next;
      for (const f of opened.sort((a, b) => b.at - a.at)) {
        const at = Math.min(f.at + 1, text.length);
        text = [...text.slice(0, at), ...f.hidden, ...text.slice(at)];
        folds = folds.map((x) => x.at >= at ? { ...x, at: x.at + f.hidden.length } : x);
      }
      replace(text.join("\n"));
    }
    draw();
    onChange();
  });
  area.addEventListener("scroll", scroll);
  area.addEventListener("keydown", (event) => {
    if (event.key === "Tab" && !event.shiftKey) {
      event.preventDefault();
      document.execCommand("insertText", false, "  ");
    } else if (event.key === "s" && (event.metaKey || event.ctrlKey) && onSave) {
      event.preventDefault();
      onSave();
    }
  });

  return {
    get value() { return full(); },
    set value(text) {
      folds = [];
      area.value = text;
      area.scrollTop = 0;
      draw();
    },
    focus: () => area.focus(),
  };
}

// foldEnd is the last line of the block line i opens, or i when it opens
// none: the lines after it indented deeper, and a list at its own indent
// under a key that ends in a colon.
export function foldEnd(lines, i) {
  const own = indent(lines[i]);
  if (!lines[i].trim() || lines[i].trim().startsWith("#")) return i;
  const key = /:\s*(#.*)?$/.test(lines[i]) && !lines[i].trim().startsWith("- ");
  let end = i;
  for (let k = i + 1; k < lines.length; k++) {
    const line = lines[k];
    if (!line.trim()) continue;
    const at = indent(line);
    if (at > own || (key && at === own && line.trim().startsWith("- "))) end = k;
    else break;
  }
  return end;
}

const indent = (line) => line.length - line.trimStart().length;

function span(className, text) {
  const s = document.createElement("span");
  s.className = className;
  s.textContent = text;
  return s;
}

// highlight is one YAML line as spans: comments, keys, list dashes, quoted
// strings, numbers, booleans and null. Anything else is plain text.
export function highlight(line) {
  const out = [];
  let rest = line;
  const lead = /^(\s*)(- )?/.exec(rest);
  if (lead[1]) out.push(lead[1]);
  if (lead[2]) out.push(span("code-dash", "- "));
  rest = rest.slice(lead[0].length);
  if (rest.startsWith("#")) return [...out, span("code-comment", rest)];
  const key = /^([^\s#'"][^:#]*?|'[^']*'|"[^"]*"):(?=\s|$)/.exec(rest);
  if (key) {
    out.push(span("code-key", key[1]), ":");
    rest = rest.slice(key[0].length);
  }
  // The value, up to a comment that is not inside quotes.
  let value = rest;
  let comment = "";
  let quote = "";
  for (let k = 0; k < rest.length; k++) {
    const c = rest[k];
    if (quote) { if (c === quote) quote = ""; continue; }
    if (c === "'" || c === '"') quote = c;
    else if (c === "#" && (k === 0 || /\s/.test(rest[k - 1]))) { value = rest.slice(0, k); comment = rest.slice(k); break; }
  }
  const word = value.trim();
  const pad = value.slice(0, value.length - value.trimStart().length);
  const after = value.slice(pad.length + word.length);
  if (!word) out.push(value);
  else {
    let kind = "code-text";
    if (/^(['"]).*\1$/.test(word)) kind = "code-string";
    else if (/^-?\d+(\.\d+)?$/.test(word)) kind = "code-num";
    else if (/^(true|false|null|~)$/i.test(word)) kind = "code-bool";
    else if (/^[[{]/.test(word)) kind = "code-flow";
    out.push(pad, span(kind, word), after);
  }
  if (comment) out.push(span("code-comment", comment));
  return out;
}
