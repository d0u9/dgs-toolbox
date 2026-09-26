const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('OCR hover highlights without scrolling; explicit focus reveals only in PDF pane', () => {
  const moves = [];
  let highlighted = false;
  const span = {
    classList: { add() { highlighted = true; }, remove() { highlighted = false; } },
    getBoundingClientRect: () => ({ top: 700, bottom: 720, left: 100, right: 180 }),
    scrollIntoView() { assert.fail('must not scroll outer containers'); },
  };
  const pages = {
    clientTop: 0, clientLeft: 0, clientHeight: 400, clientWidth: 500,
    getBoundingClientRect: () => ({ top: 100, left: 0 }),
    querySelectorAll: () => [span], querySelector: () => span,
    scrollBy: (move) => moves.push(move),
  };
  const source = fs.readFileSync(`${__dirname}/web/common.js`, 'utf8');
  const start = source.indexOf('export function showSource(');
  const end = source.indexOf('\n}\n', start) + 2;
  const context = vm.createContext({ $: () => pages });
  vm.runInContext(source.slice(start, end).replace('export ', ''), context);
  for (let i = 0; i < 10; i++) context.showSource({ page: i % 2, line: 0 }, false);
  assert.equal(highlighted, true);
  assert.equal(moves.length, 0);
  context.showSource({ page: 1, line: 0 }, true);
  assert.equal(moves.length, 1);
  assert.equal(moves[0].top, 220);
  assert.equal(moves[0].left, 0);
  assert.equal(moves[0].behavior, 'instant');
  context.showSource(null);
  assert.equal(highlighted, false);
  assert.equal(moves.length, 1);
});
