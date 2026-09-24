'use strict';

// dateField turns a YYYY-MM-DD text input into three boxes — year, month,
// day — filled one at a time. Enter checks the box it is pressed in and, only
// when that part is a real one, moves on; a wrong part is said at once and
// stays put, so 2038 or month 13 never gets as far as the server.
//
// The original input stays the one the page reads and writes: it becomes a
// hidden input holding the whole date, or nothing while the date is empty or
// incomplete. Setting its value or its disabled flag redraws the boxes, and a
// finished (or cleared) date fires an input event on it, so the page's own
// handlers carry on as before.
//
// options.latest is the last year a date may fall in; options.notFuture also
// refuses any day after today. options.partial lets the date stop after the
// year or the month — Enter on an empty month or day box — for an event known
// only that far; the value is then YYYY or YYYY-MM. options.done runs after Enter on a good day,
// once focus has moved on.
function dateField(input, options = {}) {
  const earliest = options.earliest ?? 1900;
  const latest = () => options.latest ?? new Date().getFullYear() + 100;

  const wrap = document.createElement('span');
  wrap.className = 'datefield';
  const parts = [
    { name: 'year', width: 4, placeholder: 'YYYY' },
    { name: 'month', width: 2, placeholder: 'MM' },
    { name: 'day', width: 2, placeholder: 'DD' },
  ].map((part, index) => {
    const box = document.createElement('input');
    box.type = 'text';
    box.inputMode = 'numeric';
    box.maxLength = part.width;
    box.placeholder = part.placeholder;
    box.spellcheck = false;
    box.autocomplete = 'off';
    box.className = `datefield-${part.name}`;
    box.setAttribute('aria-label', part.name);
    if (index > 0) {
      const dash = document.createElement('span');
      dash.className = 'datefield-dash';
      dash.textContent = '-';
      wrap.append(dash);
    }
    wrap.append(box);
    return box;
  });
  const [yearBox, monthBox, dayBox] = parts;
  const message = document.createElement('span');
  message.className = 'datefield-message';
  message.setAttribute('role', 'alert');
  wrap.append(message);

  input.after(wrap);
  input.type = 'hidden';

  // The hidden input's own value and disabled accessors still do the work;
  // these only redraw the boxes when the page changes either.
  const proto = HTMLInputElement.prototype;
  const valueOf = Object.getOwnPropertyDescriptor(proto, 'value');
  const disabledOf = Object.getOwnPropertyDescriptor(proto, 'disabled');
  Object.defineProperty(input, 'value', {
    get() { return valueOf.get.call(input); },
    set(text) { valueOf.set.call(input, text); draw(); },
  });
  Object.defineProperty(input, 'disabled', {
    get() { return disabledOf.get.call(input); },
    set(flag) {
      disabledOf.set.call(input, flag);
      for (const box of parts) box.disabled = flag;
    },
  });

  function draw() {
    const match = /^(\d{4})(?:-(\d{2}))?(?:-(\d{2}))?$/.exec(valueOf.get.call(input));
    yearBox.value = match ? match[1] : '';
    monthBox.value = match ? match[2] || '' : '';
    dayBox.value = match ? match[3] || '' : '';
    say('');
  }

  function say(text, box) {
    message.textContent = text;
    wrap.classList.toggle('invalid', Boolean(text));
    for (const each of parts) each.classList.toggle('invalid', each === box);
  }

  // check answers what is wrong with one box given the boxes before it, or ''.
  function check(box) {
    const text = box.value.trim();
    if (!/^\d+$/.test(text)) return 'digits only';
    const number = Number(text);
    if (box === yearBox) {
      if (text.length !== 4) return 'year has 4 digits';
      if (number < earliest || number > latest()) return `year ${earliest}–${latest()}`;
      if (options.notFuture && number > new Date().getFullYear()) return 'not in the future';
      return '';
    }
    if (box === monthBox) {
      if (number < 1 || number > 12) return 'month 1–12';
      if (options.notFuture && isFuture(Number(yearBox.value), number, 1)) return 'not in the future';
      return '';
    }
    const year = Number(yearBox.value);
    const month = Number(monthBox.value);
    const last = new Date(Date.UTC(year, month, 0)).getUTCDate();
    if (number < 1 || number > last) return `day 1–${last}`;
    if (options.notFuture && isFuture(year, month, number)) return 'not in the future';
    return '';
  }

  function isFuture(year, month, day) {
    const today = new Date();
    const asked = year * 10000 + month * 100 + day;
    return asked > today.getFullYear() * 10000 + (today.getMonth() + 1) * 100 + today.getDate();
  }

  // filled is the boxes that make up the date: all three, or for a partial
  // date the year, or the year and month, with every box after them empty.
  // It is null when that is not the shape of what is typed.
  function filled() {
    const count = parts.findIndex((box) => !box.value.trim());
    if (count === -1) return parts;
    if (parts.slice(count).some((box) => box.value.trim())) return null;
    if (count > 0 && !options.partial) return null;
    return parts.slice(0, count);
  }

  // commit writes the date as far as it is known, or clears it when every box
  // is empty.
  function commit() {
    const used = filled();
    if (!used || used.some((box) => check(box))) return;
    const next = used.map((box, index) => (index ? box.value.padStart(2, '0') : box.value)).join('-');
    if (next === valueOf.get.call(input)) return;
    valueOf.set.call(input, next);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  }

  parts.forEach((box, index) => {
    box.addEventListener('keydown', (event) => {
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === 'Enter') {
        event.preventDefault();
        // An empty date is a real answer: Enter on nothing at all clears it.
        if (parts.every((each) => !each.value.trim())) {
          say('');
          commit();
          leave();
          return;
        }
        // With partial dates, Enter on an empty month or day stops the date
        // there: the event is known to the year, or to the month.
        const stop = options.partial && index > 0 && !box.value.trim();
        for (const earlier of parts.slice(0, stop ? index : index + 1)) {
          const wrong = check(earlier);
          if (wrong) {
            say(wrong, earlier);
            earlier.focus();
            earlier.select();
            return;
          }
        }
        say('');
        if (stop) {
          // A day typed after an emptied month is not a date of any shape.
          if (!filled()) {
            say('month first', box);
            return;
          }
          commit();
          leave();
          return;
        }
        if (box !== dayBox) {
          if (box === monthBox) box.value = box.value.padStart(2, '0');
          parts[index + 1].focus();
          parts[index + 1].select();
          return;
        }
        box.value = box.value.padStart(2, '0');
        commit();
        leave();
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        draw();
        box.blur();
        return;
      }
      // Backspace in an empty box goes back one, as it would in one field.
      if (event.key === 'Backspace' && !box.value && index > 0) {
        event.preventDefault();
        parts[index - 1].focus();
      }
    });
    box.addEventListener('input', () => {
      box.value = box.value.replace(/\D/g, '');
      if (box.classList.contains('invalid')) say('');
      // Emptying every box is clearing the date; say so straight away.
      if (parts.every((each) => !each.value)) commit();
    });
  });

  // leave moves on to the next field after the date, as Enter moved from
  // year to month.
  function leave() {
    focusNextField(wrap);
    if (options.done) options.done();
  }

  // Leaving the field with Tab or the mouse keeps a finished date, and an
  // unfinished one goes back to what was there.
  wrap.addEventListener('focusout', (event) => {
    if (wrap.contains(event.relatedTarget)) return;
    const used = filled();
    const complete = used && used.length && used.every((box) => !check(box));
    if (complete) commit();
    else if (parts.some((box) => box.value.trim())) draw();
  });

  draw();
  return { focus: () => yearBox.focus() };
}
