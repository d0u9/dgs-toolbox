const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function setup() {
  const listeners = {};
  const document = {
    addEventListener: (name, fn) => { listeners[name] = fn; },
    querySelectorAll: () => [dialog],
  };
  const window = { innerWidth: 800, innerHeight: 600,
    addEventListener: (name, fn) => { listeners[name] = fn; } };
  const dialog = { open: true, isConnected: true, style: {}, dataset: {},
    getBoundingClientRect: () => ({ left: parseFloat(dialog.style.left) || 200,
      top: parseFloat(dialog.style.top) || 150, width: 300, height: 200 }) };
  let captured = false;
  const handle = {
    closest: () => dialog, classList: { add() {}, remove() {} },
    setPointerCapture: () => { captured = true; },
    hasPointerCapture: () => captured,
    releasePointerCapture: () => { captured = false; },
  };
  vm.runInNewContext(fs.readFileSync(`${__dirname}/assets/dialog.js`, 'utf8'), { document, window });
  const down = (control = false) => listeners.pointerdown({ button: 0, pointerId: 1,
    clientX: 220, clientY: 160, preventDefault() {},
    target: { closest: (selector) => selector.startsWith('button') ? (control ? {} : null) : handle } });
  return { listeners, dialog, window, down };
}

test('heading drag follows its pointer, clamps to viewport and ends on cancellation', () => {
  const { listeners, dialog, down } = setup();
  down();
  listeners.pointermove({ pointerId: 2, clientX: 400, clientY: 300 });
  assert.equal(dialog.style.left, '200px');
  listeners.pointermove({ pointerId: 1, clientX: 320, clientY: 210 });
  assert.equal(dialog.style.left, '300px');
  assert.equal(dialog.style.top, '200px');
  listeners.pointermove({ pointerId: 1, clientX: 2000, clientY: -1000 });
  assert.equal(dialog.style.left, '500px');
  assert.equal(dialog.style.top, '0px');
  listeners.pointercancel({ pointerId: 1 });
  listeners.pointermove({ pointerId: 1, clientX: 250, clientY: 250 });
  assert.equal(dialog.style.left, '500px');
});

test('controls do not drag; shrinking viewport keeps moved windows reachable', () => {
  const { listeners, dialog, window, down } = setup();
  down(true);
  assert.equal(dialog.dataset.moved, undefined);
  down();
  window.innerWidth = 400;
  window.innerHeight = 250;
  listeners.resize();
  assert.equal(dialog.style.left, '100px');
  assert.equal(dialog.style.top, '50px');
});
