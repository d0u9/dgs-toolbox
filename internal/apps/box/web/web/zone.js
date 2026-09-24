// The zone field's search. A zone is an IANA name nobody types from memory,
// so a few letters — "syd", "aus", "chi" — list the zones they could mean,
// by city, by country or by region, and the arrows and Enter pick one. The
// matching is the server's; this only draws the answer and moves through it.

function zonePicker(input, list, picked) {
  const view = { zones: [], index: -1, timer: null, asked: '', composing: false };

  function close() {
    view.zones = [];
    view.index = -1;
    list.hidden = true;
    list.innerHTML = '';
    input.setAttribute('aria-expanded', 'false');
  }

  function draw() {
    list.innerHTML = '';
    if (!view.zones.length) {
      close();
      return;
    }
    view.zones.forEach((zone, index) => {
      const item = document.createElement('li');
      item.className = `zone-option${index === view.index ? ' zone-active' : ''}`;
      item.setAttribute('role', 'option');
      item.setAttribute('aria-selected', String(index === view.index));
      const city = zone.zone.split('/').pop().replace(/_/g, ' ');
      item.innerHTML =
        `<span class="zone-city">${city}</span>` +
        `<span class="zone-where">${[zone.country, zone.comment].filter(Boolean).join(' · ')}</span>` +
        `<span class="zone-name">${zone.zone}</span>`;
      // mousedown, not click: a click lands after the field has lost focus.
      item.addEventListener('mousedown', (event) => {
        event.preventDefault();
        pick(index);
      });
      list.append(item);
    });
    list.hidden = false;
    input.setAttribute('aria-expanded', 'true');
    list.querySelector('.zone-active')?.scrollIntoView({ block: 'nearest' });
  }

  function pick(index) {
    pickFrom(view.zones, index);
  }

  function pickFrom(zones, index) {
    const zone = zones[index];
    if (!zone) return;
    clearTimeout(view.timer);
    view.asked = zone.zone;
    input.value = zone.zone;
    close();
    picked(zone.zone);
  }

  async function search() {
    if (view.composing) return;
    const query = input.value.trim();
    view.asked = query;
    if (!query) {
      close();
      return;
    }
    let body;
    try {
      body = await ask(`/api/zones?q=${encodeURIComponent(query)}`);
    } catch {
      return;
    }
    // An answer to an older query is dropped: typing outruns the network.
    if (view.asked !== query || document.activeElement !== input) return;
    view.zones = body.zones;
    // Already a whole zone name: nothing to choose between.
    if (view.zones.length === 1 && view.zones[0].zone === query) {
      close();
      return;
    }
    view.index = view.zones.length ? 0 : -1;
    draw();
  }

  input.setAttribute('role', 'combobox');
  input.setAttribute('aria-autocomplete', 'list');
  input.setAttribute('aria-expanded', 'false');
  input.addEventListener('input', () => {
    clearTimeout(view.timer);
    // Results for the previous text must never be chosen for the new text.
    view.asked = '';
    close();
    if (view.composing) return;
    view.timer = setTimeout(search, 120);
  });
  input.addEventListener('compositionstart', () => {
    view.composing = true;
    clearTimeout(view.timer);
    view.asked = '';
    close();
  });
  input.addEventListener('compositionend', () => {
    view.composing = false;
    clearTimeout(view.timer);
    view.timer = setTimeout(search, 120);
  });
  input.addEventListener('focus', () => input.select());
  input.addEventListener('blur', () => close());

  return {
    isComposing: () => view.composing,
    // Tab uses the current text, even if the debounced search has not run yet.
    async commit() {
      const query = input.value.trim();
      if (!query || view.composing) return false;
      let zones = view.asked === query && view.zones.length ? view.zones : null;
      if (!zones) {
        try {
          zones = (await ask(`/api/zones?q=${encodeURIComponent(query)}`)).zones;
        } catch {
          return false;
        }
      }
      if (input.value.trim() !== query || document.activeElement !== input) return false;
      if (zones.length) pickFrom(zones, zones === view.zones ? Math.max(0, view.index) : 0);
      return Boolean(zones.length);
    },
    // key handles a keydown in the field while the list is open, and reports
    // whether it did — so Enter picks a zone rather than filing the scan.
    key(event) {
      if (view.composing || event.isComposing || event.keyCode === 229) return false;
      if (list.hidden || !view.zones.length) return false;
      const step = { ArrowDown: 1, ArrowUp: -1 }[event.key];
      if (step) {
        view.index = (view.index + step + view.zones.length) % view.zones.length;
        draw();
      } else if (event.key === 'Enter' || event.key === 'Tab') {
        if (event.key === 'Tab' && view.index < 0) return false;
        pick(Math.max(0, view.index));
        // Enter that picks a zone also moves on, as Enter does in every field.
        if (event.key === 'Enter') focusNextField(input);
      } else if (event.key === 'Escape') {
        close();
      } else {
        return false;
      }
      event.preventDefault();
      event.stopPropagation();
      return true;
    },
  };
}
