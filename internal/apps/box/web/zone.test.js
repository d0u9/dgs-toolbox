const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('IME Tab keeps Zone focused and a later Tab selects the current query', async () => {
  const handlers = {};
  const input = {
    value: '',
    setAttribute() {},
    addEventListener(name, handler) { handlers[name] = handler; },
    select() {},
  };
  const list = { hidden: true, innerHTML: '' };
  const chosen = [];
  const document = {
    activeElement: input,
    createElement() {
      return { className: '', setAttribute() {}, addEventListener() {} };
    },
  };
  const context = vm.createContext({ document, setTimeout, clearTimeout,
    ask: async (url) => ({ zones: url.endsWith('syd')
      ? [{ zone: 'Australia/Sydney', country: 'Australia' }]
      : [{ zone: 'America/Dawson', country: 'Canada' }] }),
  });
  vm.runInContext(fs.readFileSync(`${__dirname}/web/zone.js`, 'utf8'), context);
  const picker = vm.runInContext('zonePicker', context)(input, list, (zone) => chosen.push(zone));

  handlers.compositionstart();
  input.value = 'syd';
  handlers.input();
  assert.equal(picker.isComposing(), true);
  assert.equal(picker.key({ key: 'Tab', isComposing: true }), false);
  assert.equal(await picker.commit(), false);
  assert.equal(input.value, 'syd');

  handlers.compositionend();
  assert.equal(await picker.commit(), true);
  assert.equal(input.value, 'Australia/Sydney');
  assert.deepEqual(chosen, ['Australia/Sydney']);
});
