const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function viewer() {
  const elements = {};
  let callback;
  const context = vm.createContext({
    document: { getElementById: (id) => elements[id] },
    localStorage: { getItem: () => 'page' },
    devicePixelRatio: 2,
    IntersectionObserver: class {
      constructor(fn) { callback = fn; }
      observe() {}
      disconnect() {}
    },
  });
  vm.runInContext(fs.readFileSync(`${__dirname}/web/viewer.js`, 'utf8')
    .replaceAll('export function', 'function'), context);
  const control = () => ({ classList: { toggle() {} } });
  Object.assign(elements, {
    pages: { clientWidth: 932, clientHeight: 1500, scrollWidth: 932,
      scrollHeight: 3000, scrollLeft: 0, scrollTop: 750 },
    'zoom-level': {}, 'fit-width': control(), 'fit-page': control(), 'page-at': {},
  });
  return { elements, context, notify: (entries) => callback(entries) };
}

test('fit-page does not reload a relative high-resolution URL on image load', () => {
  const { context } = viewer();
  let src = '/api/page?size=page', loads = 0;
  const img = {
    dataset: { large: '/api/page?size=large' },
    getAttribute: () => src,
    get src() { return `http://localhost${src}`; },
    set src(value) { src = value; loads++; },
  };
  context.sheet = { dataset: { ratio: 1.414 }, style: {}, querySelector: () => img };
  vm.runInContext('sheets = [sheet]; apply(); apply(); apply();', context);
  assert.equal(loads, 1);
});

test('page tracking scrolls only the thumbnail rail and settles when visible', () => {
  const { elements, context, notify } = viewer();
  const rail = { hidden: false, clientTop: 0, clientHeight: 200, scrollTop: 0,
    getBoundingClientRect: () => ({ top: 100 }) };
  const thumb = { classList: { toggle() {} },
    getBoundingClientRect: () => ({ top: 350 - rail.scrollTop, bottom: 450 - rail.scrollTop }),
    scrollIntoView() { assert.fail('must not scroll ancestors'); } };
  rail.children = [thumb];
  elements.rail = rail;
  context.sheet = { dataset: { page: 0 } };
  vm.runInContext('sheets = [sheet]; watch();', context);
  const entries = [{ target: context.sheet, intersectionRatio: 0.5 }];
  notify(entries);
  assert.equal(rail.scrollTop, 150);
  notify(entries);
  assert.equal(rail.scrollTop, 150);
  assert.equal(elements.pages.scrollTop, 750);
});
