// The zone field's search. A zone is an IANA name nobody types from memory,
// so a few letters — "syd", "aus", "chi" — list the zones they could mean,
// by city, by country or by region, and the arrows and Enter pick one. The
// matching is the server's; this only draws the answer and moves through it.

function zonePicker(input, list, picked) {
  const view = { zones: [], index: -1, timer: null, asked: '' };

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
    const zone = view.zones[index];
    if (!zone) return;
    input.value = zone.zone;
    close();
    picked(zone.zone);
  }

  async function search() {
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
    view.timer = setTimeout(search, 120);
  });
  input.addEventListener('focus', () => input.select());
  input.addEventListener('blur', () => close());

  return {
    // key handles a keydown in the field while the list is open, and reports
    // whether it did — so Enter picks a zone rather than filing the scan.
    key(event) {
      if (list.hidden || !view.zones.length) return false;
      const step = { ArrowDown: 1, ArrowUp: -1 }[event.key];
      if (step) {
        view.index = (view.index + step + view.zones.length) % view.zones.length;
        draw();
      } else if (event.key === 'Enter' || event.key === 'Tab') {
        if (event.key === 'Tab' && view.index < 0) return false;
        pick(Math.max(0, view.index));
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
