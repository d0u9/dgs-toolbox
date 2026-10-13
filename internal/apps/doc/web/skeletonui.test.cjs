const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('Doc 2 reuses styled trees and splitter while retaining pins', async () => {
  for (const kind of ['rules', 'outlines']) {
    const nodes = new Map();
    function el(tag, props = {}, ...children) {
      const node = { tag, ...props, children: children.filter(Boolean),
        append(...xs) { this.children.push(...xs.filter(Boolean)); },
        replaceChildren(...xs) { this.children = xs; },
        setAttribute(key, value) { this[key] = value; },
        get lastChild() { return this.children.at(-1); },
      };
      return node;
    }
    const $ = id => {
      if (!nodes.has(id)) nodes.set(id, el('div'));
      return nodes.get(id);
    };
    const file = { item: 'item-a', path: 'Media/01-alice.pdf', digest: 'synthetic' };
    const group = { files: [file], root: { path: '', files: [], children: [
      { path: 'Media', files: [file], count: 1, children: [] },
      { path: 'Empty', files: [], count: 0, children: [] },
    ] }, unplaced: [], plans: {}, clashes: [] };
    let treeOptions, names, splitter, saved;
    const context = vm.createContext({
      $, el, api: x => x, label: () => 'alice', frame() {}, say() {}, exportOutline() {},
      iconSvg: name => name, nameTree: (_host, entries, options) => { names = { entries, options }; },
      Folded: class extends Set { load(key) { this.key = key; } },
      resizable: (_pane, key) => { splitter = key; },
      fileTree: (_files, options) => { treeOptions = options; return el('tree'); },
      document: { querySelector: () => el('section') },
      history: { replaceState() {} }, location: { hash: '#current', pathname: '/', search: '' },
      confirm: () => true, setTimeout: () => 1, clearTimeout() {},
      fetch: async () => ({ json: async () => ({
        rules: [{ name: 'current', folders: [{ name: 'First', folders: [{ name: 'Child' }] }, { name: 'Second' }] }],
        outlines: [{ name: 'current', mount: [], defaults: {}, pins: {} }], snapshots: [],
      }) }),
      post: async (path, body) => {
        if (path === '/api/outlines/group') return group;
        if (path === '/api/defs/outlines') saved = JSON.parse(JSON.stringify(body.outline));
        return [];
      },
    });
    const source = fs.readFileSync(`${__dirname}/web/skeletonui.js`, 'utf8')
      .replace(/^import .*;\n/gm, '').replace('export async function', 'async function');
    vm.runInContext(source, context);
    await context.startSkeleton({ root: 'synthetic', items: [{ id: 'item-a' }] }, kind);
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(splitter, `dgs-doc-${kind}-tree`);
    assert.equal(names.options.selected, 'current');
    if (kind === 'rules') {
      const section = $('form').children[1];
      const folders = section.children[1];
      assert.equal(folders.className, 'vf-stack rule-folders');
      const first = folders.children[0];
      const heading = first.children[0];
      assert.equal(heading.tag, 'summary');
      assert.equal(heading.children[1].className, 'folder-actions');
      const actions = heading.children[1].children;
      assert.equal(actions[0].disabled, true);
      assert.equal(first.children[1].children[1].children[0].children[0].children[0].value, 'Child');
      assert.equal(first.children[1].className, 'folder-branch');
      const fields = first.children[1].children[0];
      assert.equal(first.open, true);
      assert.equal(fields.className, 'folder-fields');
      assert.deepEqual(fields.children.slice(0, 6).map(node => node.children[0].children[0]),
        ['Take types', 'If', 'Each', 'Order', 'Start at', 'File name']);
      assert.equal(fields.children[6].children.length, 2);
      heading.children[0].value = 'Renamed';
      heading.children[0].oninput();
      assert.equal(heading.children[0].value, 'Renamed');
      assert.equal(heading.children[1].children.length, 3);
      let prevented = false, stopped = false;
      actions[1].onclick({ preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } });
      assert.equal(prevented && stopped, true);
      assert.equal($('form').children[1].children[1].children[0].children[0].children[0].value, 'Second');
    }

    assert.deepEqual(Array.from(treeOptions.folders), ['Media/', 'Empty/']);
    assert.equal(treeOptions.folderExtra('Media/').children[0], '1');
    const extras = treeOptions.fileExtra(file);
    assert.equal(extras.className, 'ft-extras');
    assert.equal(extras.children[0].className, 'ft-reveal');
    if (kind === 'outlines') {
      const row = el('div');
      treeOptions.decorate(row, { kind: 'file', file });
      assert.equal(row.draggable, true);
      const folder = el('div');
      treeOptions.decorate(folder, { kind: 'folder', path: 'Empty/' });
      await folder.ondrop({ preventDefault() {}, stopPropagation() {}, dataTransfer: { getData: () => 'item-a' } });
      const number = extras.children.at(-1).lastChild;
      number.value = '7'; number.oninput();
      await $('form').onsubmit({ preventDefault() {} });
      assert.deepEqual(saved.pins.Empty.items, ['item-a']);
      assert.equal(saved.pins.Media.numbers['item-a'], 7);
    }
  }
});
