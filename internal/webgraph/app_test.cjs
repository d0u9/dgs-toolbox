const {test} = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const {model, run, source} = require('./test_helpers.cjs');
const graph = {
  groups: [
    {id:'owner',label:'owner'}, {id:'host-a',label:'host-a',parent:'owner',collapse:true},
    {id:'bridge',label:'bridge',parent:'host-a'}, {id:'proxy',label:'proxy',parent:'bridge',summary:true},
    {id:'host-b',label:'host-b',collapse:true}, {id:'web',label:'web',parent:'host-b',summary:true},
    {id:'db',label:'db',parent:'host-b',summary:true}, {id:'declarations',label:'declarations'}
  ],
  nodes: [
    {id:'a',label:'alpha',group:'proxy'}, {id:'b',label:'beta',group:'web'},
    {id:'b-alt',label:'beta-alt',group:'web'}, {id:'c',label:'gamma',group:'db'},
    {id:'standalone',label:'standalone service',group:'host-b'},
    {id:'declared',label:'declared host',group:'declarations',contextOnly:true}
  ],
  edges: [
    {from:'proxy',to:'b',label:'one',tags:['lan']}, {from:'proxy',to:'b-alt',label:'two',tags:['wan']},
    {from:'web',to:'a',label:'reverse',tags:['lan']}, {from:'web',to:'c',label:'internal'}
  ]
};
test('overview keeps owners and aggregates directional crossing edges', async () => {
  const c = model(graph);
  const result = await run('project("overview")', c);
  assert.deepEqual(Array.from(result.nodes, n => n.id), ['owner','host-a','host-b']);
  assert.equal(result.edges.length, 2);
  assert.equal(result.edges[0].weight, 2);
  assert.deepEqual(Array.from(result.edges[0].tags), ['lan','wan']);
  assert.equal(result.edges[1].source, 'host-b');
});
test('detailed geometry stays inside parents and is cached independently', async () => {
  const c = model(graph);
  const result = await run('geometry("full")', c);
  const nodes = new Map(result.nodes.map(n => [n.id,n]));
  for (const n of result.nodes) {
    assert.ok(Number.isFinite(n.x) && Number.isFinite(n.y));
    const parent = nodes.get(n.parent);
    if (parent) {
      assert.ok(Math.abs(n.x-parent.x)+n.w/2 < parent.w/2);
      assert.ok(Math.abs(n.y-parent.y)+n.h/2 < parent.h/2);
    }
  }
  assert.equal(await run('geometry("full") === geometry("full")', c), true);
  assert.equal(await run('geometry("full") === geometry("overview")', c), false);
  assert.equal(result.edges.length, 4);
});
test('radial layout is deterministic after relayout and input reordering', async () => {
  const positions = result => Array.from(result.nodes, n => [n.id,n.x,n.y]).sort((a,b) => a[0].localeCompare(b[0]));
  const c = model(graph, 'ring');
  const first = await run('geometry("overview")', c);
  const snapshot = positions(first);
  const rebuilt = await run('layouts.clear(); geometry("overview")', c);
  assert.deepEqual(positions(rebuilt), snapshot);
  const reversed = model({...graph, groups:[...graph.groups].reverse(), nodes:[...graph.nodes].reverse()}, 'ring');
  assert.deepEqual(positions(await run('geometry("overview")', reversed)), snapshot);
  assert.ok(first.treeLinks.length > 0);
  const leaves = first.hierarchy.leaves();
  assert.ok(leaves.every(n => n.y === leaves[0].y));
});
test('service ring collapses ports, retains same-host service edges and boxes hosts', async () => {
  const c = model(graph, 'ring-services');
  const result = await run('geometry("services")', c);
  const leaves = result.nodes.filter(n => !n.isGroup);
  assert.deepEqual(Array.from(leaves, n => n.id), ['proxy','web','db','standalone']);
  assert.equal(result.edges.length, 3);
  assert.equal(result.edges[0].weight, 2);
  assert.equal(result.edges[2].source.id, 'web');
  assert.equal(result.edges[2].target.id, 'db');
  assert.deepEqual(Array.from(result.sectors, n => n.id), ['host-a','host-b']);
  for (const sector of result.sectors) {
    assert.ok(sector.path && Number.isFinite(sector.x));
    const host = result.nodes.find(n => n.id === sector.id);
    for (const leaf of host.hierarchy.leaves()) {
      assert.ok(leaf.x > sector.from && leaf.x < sector.to);
      assert.ok(leaf.y > sector.inner && leaf.y < sector.outer);
    }
  }
});
test('declared hosts are opt-in while isolated runtime components stay visible', async () => {
  const c = model(graph);
  assert.equal(await run('project("full").nodes.some(n => n.id === "declared")', c), false);
  assert.equal(await run('project("full").nodes.some(n => n.id === "declarations")', c), false);
  assert.equal(await run('project("full").nodes.some(n => n.id === "standalone")', c), true);
  assert.equal(await run('includeDeclarations = true; project("full").nodes.some(n => n.id === "declared")', c), true);
});
test('filters change aggregate counts and retain untagged connections', async () => {
  const c = model(graph);
  const result = await run('filterOn = new Set(["lan"]); project("overview")', c);
  assert.equal(result.edges[0].weight, 1);
  assert.equal(result.edges[0].label, 'one');
  assert.equal(await run('visibleTags({tags:["wan","lan"]})', c), true);
  assert.equal(await run('visibleTags({tags:["wan"]})', c), false);
  assert.equal(await run('visibleTags({tags:[]})', c), true);
  assert.equal(await run('project("services").edges.some(e => e.label === "internal")', c), true);
});
test('parallel, reverse and self-loop routes remain separate and finite', async () => {
  const input = {nodes:[{id:'a',label:'a'},{id:'b',label:'b'}], edges:[
    {from:'a',to:'b',label:'first'}, {from:'a',to:'b',label:'second'}, {from:'b',to:'a'},
    {from:'a',to:'a',label:'loop-one'}, {from:'a',to:'a',label:'loop-two'}
  ]};
  for (const view of ['network','ring-services']) {
    const c = model(input, view);
    const routes = await run(`geometry("full").edges.map(e => ${view === 'network' ? 'networkRoute(e).path' : 'radialRoute(e)'})`, c);
    assert.equal(new Set(routes).size, input.edges.length);
    assert.ok(routes.every(route => route && !/NaN|Infinity/.test(route)));
  }
});
test('empty and single-node inventories have finite radial geometry', async () => {
  for (const input of [{}, {nodes:[{id:'one',label:'one'}]}]) {
    const c = model(input, 'ring');
    const result = await run('geometry("overview")', c);
    assert.ok(result.nodes.every(n => Number.isFinite(n.x) && Number.isFinite(n.y)));
  }
});

test('orthogonal routes avoid unrelated nodes and round their bends', async () => {
  const c = model(graph);
  const scene = await run('geometry("full")', c);
  for (const edge of scene.edges) {
    assert.ok(edge.points.length >= 2);
    for (const [point, node] of [[edge.points[0], edge.source], [edge.points.at(-1), edge.target]]) {
      const dx = Math.abs(point.x-node.x), dy = Math.abs(point.y-node.y);
      assert.ok(dx <= node.w/2+.001 && dy <= node.h/2+.001, 'attachment must lie on its actual node');
      assert.ok(Math.abs(dx-node.w/2) < .001 || Math.abs(dy-node.h/2) < .001, 'attachment must be on the border');
    }
    const byID = new Map(scene.nodes.map(n => [n.id, n]));
    const related = new Set();
    for (const end of [edge.source, edge.target]) for (let n = end; n; n = byID.get(n.parent)) related.add(n.id);
    for (let i = 1; i < edge.points.length; i++) {
      const a = edge.points[i-1], b = edge.points[i];
      assert.ok(a.x === b.x || a.y === b.y, 'each segment follows a routing channel');
      for (const node of scene.nodes.filter(n => !related.has(n.id))) {
        const x0 = node.x-node.w/2, x1 = node.x+node.w/2;
        const y0 = node.y-node.h/2, y1 = node.y+node.h/2;
        const crosses = a.x === b.x
          ? a.x > x0 && a.x < x1 && Math.max(a.y,b.y) > y0 && Math.min(a.y,b.y) < y1
          : a.y > y0 && a.y < y1 && Math.max(a.x,b.x) > x0 && Math.min(a.x,b.x) < x1;
        assert.equal(crosses, false, 'route must not cut through a node');
      }
    }
  }
  c.points = [{x:0,y:0},{x:100,y:0},{x:100,y:100}];
  const rounded = await run('networkRoute({points})', c);
  assert.match(rounded.path, /L90,0Q100,0 100,10/);
});

test('Network services retains every additional container membership without duplicating the process', async () => {
  const input = {
    groups: [
      {id:'host-a',label:'host-a',collapse:true},
      {id:'apps',label:'apps',parent:'host-a',region:true},
      {id:'internal',label:'internal',parent:'host-a',region:true},
      {id:'edge',label:'edge',parent:'host-a',region:true},
      {id:'proxy',label:'host-a/proxy',parent:'apps',summary:true,detail:'docker · internal · edge'},
    ],
    nodes: [{id:'http',label:'http',group:'proxy'}, {id:'https',label:'https',group:'proxy'}, {id:'other',label:'other',group:'apps'}],
    edges: [
      {from:'proxy',to:'internal',kind:'joins',membership:true,tags:['internal']},
      {from:'proxy',to:'edge',kind:'joins',membership:true,tags:['edge']},
    ],
  };
  const c = model(input);
  const scene = await run('geometry("services")', c);
  assert.equal(scene.nodes.filter(n => n.id === 'proxy').length, 1);
  const proxy = scene.nodes.find(n => n.id === 'proxy');
  assert.equal(proxy.parent, 'host-a');
  assert.equal(proxy.displayLabel, 'proxy');
  assert.deepEqual(Array.from(proxy.regions), ['apps','internal','edge']);
  assert.equal(scene.nodes.filter(n => n.reference).length, 0);
  assert.equal(scene.regions.length, 3);
  assert.equal(scene.edges.length, 0);
  c.scene = scene;
  assert.equal(await run('scene.regions.every(r => r.contour.coordinates.some(p => d3.polygonContains(p[0], [(scene.nodes.find(n => n.id === "proxy").x-r.grid.x)/r.grid.step, (scene.nodes.find(n => n.id === "proxy").y-r.grid.y)/r.grid.step])))', c), true);
  assert.equal(await run('scene.regions.filter(r => r.id !== "apps").every(r => !r.contour.coordinates.some(p => d3.polygonContains(p[0], [(scene.nodes.find(n => n.id === "other").x-r.grid.x)/r.grid.step, (scene.nodes.find(n => n.id === "other").y-r.grid.y)/r.grid.step]) && !p.slice(1).some(h => d3.polygonContains(h, [(scene.nodes.find(n => n.id === "other").x-r.grid.x)/r.grid.step, (scene.nodes.find(n => n.id === "other").y-r.grid.y)/r.grid.step]))))', c), true);
  const filtered = await run('filterOn = new Set(["internal"]); project("services")', c);
  assert.deepEqual(Array.from(filtered.nodes.find(n => n.id === 'proxy').regions), ['apps','internal']);
  const ring = model(input, 'ring-services');
  assert.equal((await run('project("services")', ring)).edges.length, 0);
  assert.equal((await run('project("full")', c)).nodes.filter(n => n.reference).length, 1);
});

test('service membership buckets preserve local route coordinates and separate network areas', async () => {
  const c = model({groups:[
    {id:'host-a',label:'host-a',collapse:true},
    {id:'apps',label:'apps',parent:'host-a',region:true},
    {id:'edge',label:'edge',parent:'host-a',region:true},
  ],nodes:[{id:'a',label:'a',group:'apps'},{id:'b',label:'b',group:'apps'},
    {id:'c',label:'c',group:'edge'}],edges:[{from:'a',to:'b'}]});
  const scene = await run('geometry("services")', c);
  assert.equal(scene.membershipBuckets.length, 2);
  const edge = scene.edges[0];
  for (const [id,point] of [['a',edge.points[0]],['b',edge.points.at(-1)]]) {
    const n = scene.nodes.find(n => n.id === id);
    assert.ok(Math.abs(point.x-n.x) <= n.w/2+1);
    assert.ok(Math.abs(point.y-n.y) <= n.h/2+1);
  }
  const [a,b] = scene.membershipBuckets;
  assert.ok(Math.abs(a.x-b.x) > (a.w+b.w)/2 || Math.abs(a.y-b.y) > (a.h+b.h)/2);
});

test('network regions share filter colors across hosts and stable fallback colors', async () => {
 const input = {groups:[
  {id:'host-a',label:'host-a',collapse:true},{id:'host-b',label:'host-b',collapse:true},
  {id:'apps-a',label:'apps',parent:'host-a',region:true,colorFilter:'apps'},
  {id:'apps-b',label:'apps',parent:'host-b',region:true,colorFilter:'apps'},
  {id:'edge-a',label:'edge',parent:'host-a',region:true},
  {id:'edge-b',label:'edge',parent:'host-b',region:true},
 ],nodes:[{id:'a',label:'a',group:'apps-a'},{id:'b',label:'b',group:'apps-b'},
 {id:'c',label:'c',group:'edge-a'},{id:'d',label:'d',group:'edge-b'}]};
 const c = model(input);
 await run('filterColour = new Map([["internet","#0072B2"],["apps","#009E73"]]); true',c);
 const scene = await run('geometry("services")', c);
 const regions = new Map(scene.regions.map(r=>[r.id,r]));
 assert.equal(regions.get('apps-a').regionColour, '#009E73');
 assert.equal(regions.get('apps-b').regionColour, '#009E73');
 assert.equal(regions.get('edge-a').regionColour, regions.get('edge-b').regionColour);
});

test('search preserves Services and Ring views for visible results and reveals hidden ports', async () => {
 const {Element} = await import('../webui/test_dom.mjs');
 for (const mode of ['network','ring-services']) {
  const c = model(graph,mode);
  const searchInput = new Element('input'); searchInput.value = 'proxy';
  const results = new Element('div'), declarations = new Element('input');
  c.document = {getElementById:id=>({search:searchInput,results,declarations})[id],
    createElement:tag=>new Element(tag),querySelectorAll:()=>[]};
  vm.runInContext(`let level="services", scene=null, selected=null, framed=null, renders=0;
    function select(n){selected=n;} function frame(nodes){framed=nodes;}
    async function render(){renders++;scene=await geometry(scopeNow());return true;}
    ${source.slice(source.indexOf('function search()'), source.indexOf('document.getElementById("search").addEventListener'))}`, c);
  await vm.runInContext('render()', c);
  vm.runInContext('search()',c);
  await results.children[0].onclick();
  assert.equal(vm.runInContext('view',c),mode);
  assert.equal(vm.runInContext('level',c),'services');
  assert.equal(vm.runInContext('selected.id',c),'proxy');
  assert.equal(vm.runInContext('renders',c),1);
  searchInput.value = 'alpha';
  vm.runInContext('search()',c);
  await results.children[0].onclick();
  assert.equal(vm.runInContext('view',c),'network');
  assert.equal(vm.runInContext('level',c),'full');
  assert.equal(vm.runInContext('selected.id',c),'a');
 }
});
