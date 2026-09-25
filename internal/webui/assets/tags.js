// The tag field: each tag a bubble, typed into one at a time, with the tags the
// current tree already uses offered as the letters come in.
//
// Tags are free-form, and free-form drifts: "Japan", "japan 2019" and
// "japan-2019" are three tags to a filter. So a tag is spelled one way —
// lower case, spaces joined by a hyphen, as internal/tag spells it — and
// the spelling that already exists is the one offered first. A tag the tree has
// never seen is still allowed, and its bubble is drawn dashed so it is noticed.
//
// Keys, in the text box:
//   Enter, Tab   take the highlighted suggestion (or the text, when none)
//   ,            take the text exactly as typed
//   Backspace    on an empty box, pick the last bubble; again, remove it
//   ↑ ↓          move through the suggestions
//   Esc          close the suggestions, then leave the field

function normalizeTag(text) {
  return String(text).toLowerCase().trim().split(/\s+/).filter(Boolean).join('-');
}

// tagList normalizes and drops the empty and the repeated, keeping order.
function tagList(tags) {
  const seen = new Set();
  const out = [];
  for (const raw of tags || []) {
    const tag = normalizeTag(raw);
    if (!tag || seen.has(tag)) continue;
    seen.add(tag);
    out.push(tag);
  }
  return out;
}

// tagField turns host, an empty element, into a tag field.
//
// options:
//   known()      [{name, count}] the current tree already uses, most used first
//   count(name)  the number drawn beside a suggestion; defaults to known's
//   onChange(tags)
//   placeholder
//   only         only tags from known() can be taken — a filter's field
//   limit        suggestions shown at most (8)
function tagField(host, options) {
  const opts = { limit: 8, placeholder: '', only: false, ...options };
  const view = { tags: [], armed: false, items: [], index: -1, disabled: false };

  host.classList.add('tag-field');
  host.innerHTML = '';
  const bubbles = document.createElement('span');
  bubbles.className = 'tag-bubbles';
  const input = document.createElement('input');
  input.type = 'text';
  input.className = 'tag-input';
  input.spellcheck = false;
  input.autocomplete = 'off';
  input.setAttribute('role', 'combobox');
  input.setAttribute('aria-autocomplete', 'list');
  input.setAttribute('aria-expanded', 'false');
  if (host.dataset.label) input.setAttribute('aria-label', host.dataset.label);
  const list = document.createElement('ul');
  list.className = 'tag-options';
  list.setAttribute('role', 'listbox');
  list.hidden = true;
  host.append(bubbles, input, list);

  const known = () => (opts.known ? opts.known() : []);
  const countOf = (name) => {
    if (opts.count) return opts.count(name);
    return known().find((use) => use.name === name)?.count ?? 0;
  };
  const isKnown = (name) => known().some((use) => use.name === name);

  function changed() {
    drawBubbles();
    opts.onChange?.(view.tags.slice());
  }

  function drawBubbles() {
    bubbles.innerHTML = '';
    view.tags.forEach((tag, index) => {
      const bubble = document.createElement('span');
      bubble.className = 'tag-bubble';
      if (!isKnown(tag)) {
        bubble.classList.add('tag-new');
        bubble.title = 'New: nothing here has this tag yet';
      }
      if (view.armed && index === view.tags.length - 1) bubble.classList.add('tag-armed');
      const name = document.createElement('span');
      name.textContent = tag;
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'tag-remove';
      remove.tabIndex = -1;
      remove.setAttribute('aria-label', `Remove ${tag}`);
      remove.textContent = '×';
      remove.disabled = view.disabled;
      // mousedown, not click: the text box keeps its focus.
      remove.addEventListener('mousedown', (event) => {
        event.preventDefault();
        if (view.disabled) return;
        view.tags.splice(index, 1);
        view.armed = false;
        changed();
        suggest();
      });
      bubble.append(name, remove);
      bubbles.append(bubble);
    });
    input.placeholder = view.tags.length ? '' : opts.placeholder;
  }

  function add(raw) {
    const tag = normalizeTag(raw);
    input.value = '';
    view.armed = false;
    if (!tag || view.tags.includes(tag)) return false;
    if (opts.only && !isKnown(tag)) return false;
    view.tags.push(tag);
    changed();
    return true;
  }

  // suggest lists what the text could mean: tags starting with it first, then
  // tags containing it, each most used first. With nothing typed it offers the
  // most used. A text that is no existing tag is offered last, as new.
  function suggest() {
    if (document.activeElement !== input || view.disabled) {
      close();
      return;
    }
    const text = normalizeTag(input.value);
    const free = known().filter((use) => !view.tags.includes(use.name));
    let items;
    if (!text) {
      items = free.slice(0, opts.limit);
    } else {
      const starts = free.filter((use) => use.name.startsWith(text));
      const within = free.filter((use) => !use.name.startsWith(text) && use.name.includes(text));
      items = [...starts, ...within].slice(0, opts.limit);
    }
    view.items = items.map((use) => ({ name: use.name, fresh: false }));
    if (text && !opts.only && !isKnown(text) && !view.tags.includes(text)) {
      view.items.push({ name: text, fresh: true });
    }
    view.index = view.items.length ? 0 : -1;
    // With nothing typed, the list is an offer, not a choice: Enter leaves.
    if (!text) view.index = -1;
    draw();
  }

  function draw() {
    list.innerHTML = '';
    if (!view.items.length) {
      close();
      return;
    }
    view.items.forEach((item, index) => {
      const row = document.createElement('li');
      row.className = `tag-option${index === view.index ? ' tag-active' : ''}`;
      row.setAttribute('role', 'option');
      row.setAttribute('aria-selected', String(index === view.index));
      const name = document.createElement('span');
      name.className = 'tag-option-name';
      name.textContent = item.name;
      const note = document.createElement('span');
      note.className = 'tag-option-count';
      note.textContent = item.fresh ? 'new tag' : String(countOf(item.name));
      row.append(name, note);
      row.addEventListener('mousedown', (event) => {
        event.preventDefault();
        add(item.name);
        suggest();
      });
      list.append(row);
    });
    list.hidden = false;
    input.setAttribute('aria-expanded', 'true');
  }

  function close() {
    view.items = [];
    view.index = -1;
    list.hidden = true;
    list.innerHTML = '';
    input.setAttribute('aria-expanded', 'false');
  }

  function move(step) {
    if (!view.items.length) return;
    const count = view.items.length;
    view.index = view.index < 0 ? (step > 0 ? 0 : count - 1) : (view.index + step + count) % count;
    draw();
  }

  // take is Enter and Tab: the highlighted suggestion, else the text itself.
  function take() {
    const item = view.items[view.index];
    if (item) return add(item.name);
    if (input.value.trim()) return add(input.value);
    return false;
  }

  input.addEventListener('keydown', (event) => {
    // Enter that commits an input method's composition is the input method's.
    if (event.isComposing || event.keyCode === 229) return;
    switch (event.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        if (!view.items.length) return;
        event.preventDefault();
        move(event.key === 'ArrowDown' ? 1 : -1);
        return;
      case 'Enter':
        event.preventDefault();
        event.stopPropagation();
        if (take()) suggest();
        // Enter on an empty field only leaves it, as in every other field.
        else if (!input.value.trim()) input.blur();
        return;
      case 'Tab':
        if (!input.value.trim()) {
          // Tab out of an empty tag field goes on as Enter does: to the next
          // field, or, from the last one, back to the page's keys — not to
          // whatever the browser would focus next.
          if (event.shiftKey) return;
          if (!opts.next) return;
          event.preventDefault();
          opts.next(input);
          return;
        }
        event.preventDefault();
        take();
        suggest();
        return;
      case ',':
        event.preventDefault();
        if (opts.only) take();
        else add(input.value);
        suggest();
        return;
      case 'Backspace':
        if (input.value || !view.tags.length) {
          view.armed = false;
          return;
        }
        event.preventDefault();
        if (view.armed) {
          view.tags.pop();
          view.armed = false;
          changed();
        } else {
          view.armed = true;
          drawBubbles();
        }
        suggest();
        return;
      case 'Escape':
        event.preventDefault();
        event.stopPropagation();
        if (view.items.length && input.value) {
          input.value = '';
          suggest();
        } else if (!list.hidden) close();
        else input.blur();
        return;
      default:
        if (view.armed) {
          view.armed = false;
          drawBubbles();
        }
    }
  });
  input.addEventListener('input', () => {
    // A pasted "a, b, c" is three tags; the last is still being typed.
    if (input.value.includes(',')) {
      const parts = input.value.split(',');
      const rest = parts.pop();
      for (const part of parts) add(part);
      input.value = rest;
    }
    suggest();
  });
  input.addEventListener('focus', () => {
    host.classList.add('tag-focus');
    suggest();
  });
  input.addEventListener('blur', () => {
    host.classList.remove('tag-focus');
    // What was typed and not taken is kept, so leaving the field never loses it.
    if (input.value.trim() && !opts.only) add(input.value);
    input.value = '';
    view.armed = false;
    drawBubbles();
    close();
  });
  // A click anywhere in the field is a click in its text box.
  host.addEventListener('mousedown', (event) => {
    if (event.target === host || event.target === bubbles) {
      event.preventDefault();
      input.focus();
    }
  });

  drawBubbles();

  return {
    get: () => view.tags.slice(),
    set(tags) {
      view.tags = tagList(tags);
      view.armed = false;
      drawBubbles();
    },
    // add takes one tag from outside, as a click on a tag elsewhere does.
    add(tag) {
      if (view.tags.includes(normalizeTag(tag))) return;
      add(tag);
    },
    remove(tag) {
      const at = view.tags.indexOf(normalizeTag(tag));
      if (at < 0) return;
      view.tags.splice(at, 1);
      changed();
    },
    toggle(tag) {
      if (view.tags.includes(normalizeTag(tag))) this.remove(tag);
      else this.add(tag);
    },
    disable(disabled) {
      view.disabled = disabled;
      input.disabled = disabled;
      host.classList.toggle('tag-disabled', disabled);
      drawBubbles();
    },
    focus: () => input.focus(),
    commit: () => { if (take()) suggest(); },
    // redraw is for when known() has changed, so a bubble's "new" is current.
    redraw: drawBubbles,
    input,
  };
}
window.tagField = tagField;
