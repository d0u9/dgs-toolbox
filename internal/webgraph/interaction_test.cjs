const {test} = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const {model, run, source} = require('./test_helpers.cjs');

const graph = {
  title:'Synthetic graph',
  groups:[{id:'host-a',label:'host-a',collapse:true},{id:'host-b',label:'host-b',collapse:true},
    {id:'apps',label:'apps',parent:'host-a',region:true,colorFilter:'lan'},
    {id:'edge',label:'edge',parent:'host-a',region:true,colorFilter:'edge'},
    {id:'proxy',label:'host-a/proxy',parent:'apps',summary:true},
    {id:'web',label:'host-a/web',parent:'apps',summary:true}],
  nodes:[{id:'http',label:'http',group:'proxy'},{id:'web-port',label:'web port',group:'web'},
    {id:'api',label:'api',group:'host-b'},{id:'isolated',label:'isolated service',group:'host-b'},
    {id:'declared',label:'declared host',contextOnly:true,tags:['lan']}],
  edges:[{from:'proxy',to:'web-port',label:'local',tags:['lan']},
    {from:'proxy',to:'api',label:'remote',tags:['wan']},
    {from:'web',to:'api',label:'untagged'},
    {from:'proxy',to:'edge',membership:true,tags:['edge']}],
  filters:[{id:'lan',label:'LAN'},{id:'wan',label:'WAN'},{id:'edge',label:'edge'}],
};

async function harness(input = graph) {
  const {Element} = await import('../webui/test_dom.mjs');
  const c = model(input);
  const ids = new Map();
  for (const id of ['search','results','declarations','error','graph','layout-state','selection',
    'view-state','empty','ring-help','filters','key','legend','title','counts','reload','fit',
    'relayout','zoom-in','zoom-out']) ids.set(id,new Element(id === 'search' ? 'input' : 'div'));
  ids.get('search').value = '';
  const levels = ['overview','services','medium','full'].map(level => {
    const button = new Element('button'); button.dataset.level = level; return button;
  });
  const views = ['network','ring','ring-services'].map(view => {
    const button = new Element('button'); button.dataset.view = view; return button;
  });
  const filters = (input.filters || []).map(filter => {
    const checkbox = new Element('input');checkbox.dataset.filter = filter.id;checkbox.checked = true;return checkbox;
  });
  const colorBy = new Element('input');colorBy.checked = false;
  ids.get('filters').querySelectorAll = () => filters;
  ids.get('filters').querySelector = () => colorBy;
  const document = new Element('document');
  document.getElementById = id => ids.get(id);
  document.createElement = tag => new Element(tag);
  document.querySelectorAll = selector => ({'[data-level]':levels,'[data-view]':views,
    'input[data-filter]':filters})[selector] || [];
  const styles = new Map(), classes = new Map(), calls = [], transforms = [];
    // Only SVG element creation is substituted; production render and listeners run unchanged.
  const root = new Element('g');
  const matches = (n,selector) => selector === '*' || (selector.startsWith('.')
    ? String(n.attributes.class || '').split(/\s+/).includes(selector.slice(1))
    : n.tagName.toLowerCase() === selector);
  function selection(nodes,parents=[],selector='') {
    let bindings = [];
    const apply = (n,value,index) => typeof value === 'function' ? value(n.__data__,index) : value;
    const api = {
      append(tag){return selection(nodes.map(parent=>{
        const n=new Element(tag);n.__data__=parent.__data__;parent.append(n);return n;
      }));},
      selectAll(query){return selection(nodes.flatMap(n=>n.all(child=>child!==n && matches(child,query))),nodes,query);},
      select(query){return selection(nodes.map(n=>n.all(child=>child!==n && matches(child,query))[0]).filter(Boolean));},
      data(value){bindings=parents.flatMap(parent=>apply(parent,value,0).map(datum=>({parent,datum})));return api;},
      join(tag){return selection(bindings.map(({parent,datum})=>{
        const n=new Element(tag);n.__data__=datum;parent.append(n);return n;
      }));},
      attr(name,value){nodes.forEach((n,i)=>n.setAttribute(name,apply(n,value,i)));return api;},
      classed(name,value){nodes.forEach((n,i)=>{
        const yes=apply(n,value,i);classes.set(`${selector}:${n.__data__.id}:${name}`,yes);
        const names=new Set(String(n.attributes.class || '').split(/\s+/).filter(Boolean));
        if(yes)names.add(name);else names.delete(name);n.setAttribute('class',[...names].join(' '));
      });return api;},
      style(name,value){nodes.forEach((n,i)=>{
        n.style[name]=apply(n,value,i);styles.set(`${selector}:${n.__data__?.id}:${name}`,n.style[name]);
      });return api;},
      text(value){nodes.forEach((n,i)=>{n.textContent=apply(n,value,i);});return api;},
      on(type,listener){nodes.forEach(n=>n.listeners.set(type,[event=>listener(event,n.__data__)]));return api;},
      remove(){nodes.forEach(n=>n.remove());return api;},
    };
    return api;
  }
  const world = selection([root]);
  const worldAttr=world.attr;
  world.attr=(name,value)=>{transforms.push([name,value]);return worldAttr(name,value);};
  const zoom = {transform(){},scaleBy(){},scaleExtent(){return this;},
    on(type,listener){this.listener=listener;return this;}};
  const svgElement = new Element('svg');svgElement.clientWidth=900;svgElement.clientHeight=600;
  const svg = {node:()=>svgElement,call(...args){calls.push(args);return this;},
    on(type,listener){svgElement.listeners.set(type,[listener]);return this;}};
  Object.assign(c,{document,world,svg});c.d3.zoom=()=>zoom;
  vm.runInContext(`let level="overview",scene=null,selected=null,pending=null,renderRequest=0;
    ${source.slice(source.indexOf('const zoom ='), source.indexOf('svg.call(zoom).on'))}
    ${source.slice(source.indexOf('async function render('), source.indexOf('function shorten('))}
    ${source.slice(source.indexOf('function shorten('), source.indexOf('new ResizeObserver('))}
  `,c);
  vm.runInContext(source.slice(source.indexOf('svg.call(zoom).on'),source.indexOf('function visibleTags')),c);
  vm.runInContext('const renderGraph=render; render=(fit=false)=>{pending=renderGraph(fit);return pending;};',c);
  vm.runInContext('showError(""); drawFilters(graph);',c);
  await vm.runInContext('render()',c);
  return {c,ids,levels,views,filters,colorBy,document,styles,classes,calls,transforms,zoom,svgElement,root,
    read:code=>vm.runInContext(code,c),
    async settle(){await vm.runInContext('pending',c);},
    async click(button){button.onclick();await this.settle();},
  };
}

test('view buttons choose projections, lock ring detail controls and restore Network detail', async () => {
  const h = await harness();
  await h.click(h.levels[1]);
  assert.equal(h.read('scopeNow()'),'services');
  assert.equal(h.levels[1].attributes['aria-pressed'],true);
  for (const [index,scope] of [[1,'overview'],[2,'services']]) {
    await h.click(h.views[index]);
    assert.equal(h.read('scopeNow()'),scope);
    assert.ok(h.levels.every(button=>button.disabled));
    assert.equal(h.views[index].attributes['aria-pressed'],true);
  }
  await h.click(h.views[0]);
  assert.equal(h.read('scopeNow()'),'services');
  assert.ok(h.levels.every(button=>!button.disabled));
  await h.click(h.levels[2]);assert.equal(h.read('scopeNow()'),'medium');
  await h.click(h.levels[3]);assert.equal(h.read('scopeNow()'),'full');
});

test('zoom, Fit and keyboard navigation never change the selected view or detail', async () => {
  const h = await harness();await h.click(h.levels[1]);
  h.zoom.listener({transform:h.c.d3.zoomIdentity.translate(20,30).scale(2)});
  assert.equal(h.transforms.at(-1)[0],'transform');
  h.ids.get('zoom-in').onclick();h.ids.get('zoom-out').onclick();h.ids.get('fit').onclick();
  assert.equal(h.calls.at(-3)[1],1.4);
  assert.equal(h.calls.at(-2)[1],1/1.4);
  assert.ok(Number.isFinite(h.calls.at(-1)[1].k));
  const before = h.calls.length;
  await h.document.fire('keydown',{target:h.ids.get('search'),key:'f'});
  assert.equal(h.calls.length,before,'typing in search must not fit the graph');
  await h.document.fire('keydown',{target:h.svgElement,key:'f'});
  assert.equal(h.calls.length,before+1);
  assert.equal(h.read('view'),'network');assert.equal(h.read('level'),'services');
});

test('selection keeps direct neighbors and joined regions visible; Escape and background clear it', async () => {
  const h = await harness();await h.click(h.levels[1]);
  const proxy = h.root.all(n=>n.attributes.class==='node' && n.__data__?.id==='proxy')[0];
  await proxy.fire('click');
  assert.equal(h.classes.get('.node:proxy:selected'),true);
  assert.equal(h.classes.get('.node:web:faded'),false);
  assert.equal(h.classes.get('.node:api:faded'),false);
  assert.equal(h.classes.get('.node:isolated:faded'),true);
  assert.equal(h.styles.get('.membership-region:apps:opacity'),1);
  assert.equal(h.styles.get('.membership-region:edge:opacity'),1);
  assert.match(h.ids.get('selection').textContent,/Network: apps/);
  assert.match(h.ids.get('selection').textContent,/Network: edge/);
  assert.equal(h.ids.get('selection').hidden,false);
  await h.document.fire('keydown',{target:h.svgElement,key:'Escape'});
  assert.equal(h.read('selected'),null);assert.equal(h.ids.get('selection').hidden,true);
  await proxy.fire('keydown',{key:'Enter'});
  await h.svgElement.fire('click');
  assert.equal(h.read('selected'),null);
  assert.equal(h.styles.get('.membership-region:edge:opacity'),1,'clearing selection keeps networks visible');
});

test('filter and declaration listeners rebuild the scene while preserving untagged connections', async () => {
  const h = await harness();await h.click(h.levels[1]);
  h.filters.find(n=>n.dataset.filter==='wan').checked=false;
  await h.filters.find(n=>n.dataset.filter==='wan').fire('change');await h.settle();
  assert.equal(h.read('scene.edges.some(e=>e.label==="remote")'),false);
  assert.equal(h.read('scene.edges.some(e=>e.label==="untagged")'),true);
  assert.equal(h.read('scene.nodes.some(n=>n.id==="declared")'),false);
  h.ids.get('declarations').checked=true;
  h.ids.get('declarations').onchange({target:h.ids.get('declarations')});await h.settle();
  assert.equal(h.read('scene.nodes.some(n=>n.id==="declared")'),true);
  h.ids.get('declarations').checked=false;
  h.ids.get('declarations').onchange({target:h.ids.get('declarations')});await h.settle();
  assert.equal(h.read('scene.nodes.some(n=>n.id==="declared")'),false);
  const previous = h.read('scene');
  h.colorBy.checked=true;await h.colorBy.fire('change');await h.settle();
  assert.equal(h.read('scene'),previous,'recoloring must keep the same geometry');
  assert.equal(h.read('edgeColour("hop",["lan"])'),h.read('darken(filterColour.get("lan"))'));
});

test('relayout rebuilds geometry, preserves selection and keeps the service view', async () => {
  const h = await harness();await h.click(h.levels[1]);
  h.read('select(scene.nodes.find(n=>n.id==="proxy"))');
  const previous=h.read('scene');await h.click(h.ids.get('relayout'));
  assert.notEqual(h.read('scene'),previous);
  assert.equal(h.read('selected.id'),'proxy');
  assert.equal(h.read('level'),'services');
  assert.equal(h.read('scene.regions.length'),2);
});

test('search includes declarations and reveals filtered results without leaving Services', async () => {
  const h = await harness();await h.click(h.levels[1]);
  h.filters[0].checked=false;await h.filters[0].fire('change');await h.settle();
  h.ids.get('search').value='declared';await h.ids.get('search').fire('input');
  await h.ids.get('results').children[0].onclick();
  assert.equal(h.read('selected.id'),'declared');
  assert.equal(h.ids.get('declarations').checked,true);
  assert.equal(h.filters[0].checked,true);
  assert.equal(h.read('level'),'services');
  h.ids.get('search').value='not-a-node';await h.ids.get('search').fire('input');
  assert.equal(h.ids.get('results').textContent,'No matches');
  h.ids.get('search').value='';await h.ids.get('search').fire('input');
  assert.equal(h.ids.get('results').hidden,true);
});

test('reload publishes the fresh graph and failure retains the last usable scene', async () => {
  const h = await harness();await h.click(h.levels[1]);
  h.read('select(scene.nodes.find(n=>n.id==="proxy"))');
  const previous=h.read('scene');
  h.c.fetch=async()=>({ok:false,json:async()=>({error:'Synthetic load failure'})});
  await h.ids.get('reload').onclick();
  assert.equal(h.read('scene'),previous);
  assert.match(h.ids.get('error').textContent,/Synthetic load failure/);
  h.c.fetch=async()=>({ok:true,json:async()=>({...graph,title:'Fresh graph',nodes:[...graph.nodes,{id:'new',label:'new service',group:'host-b'}]})});
  await h.ids.get('reload').onclick();
  assert.equal(h.ids.get('title').textContent,'Fresh graph');
  assert.equal(h.read('scene.nodes.some(n=>n.id==="new")'),true);
  assert.equal(h.read('selected'),null);
  assert.equal(h.read('level'),'services');
  assert.equal(h.ids.get('error').hidden,true);
});

test('layout errors clear busy state and stale layout completion cannot replace the scene', async () => {
  const h = await harness();const previous=h.read('scene');
  vm.runInContext(source.slice(source.indexOf('async function render('),source.indexOf('function shorten(')),h.c);
  h.read('geometry=()=>Promise.reject(new Error("Synthetic layout failure"))');
  assert.equal(await h.read('render()'),false);
  assert.equal(h.ids.get('graph').attributes['aria-busy'],'false');
  assert.equal(h.ids.get('layout-state').hidden,true);
  assert.match(h.ids.get('error').textContent,/Layout failed: Synthetic layout failure/);
  assert.equal(h.read('scene'),previous);
  let finish;h.c.deferred=new Promise(resolve=>{finish=resolve;});
  h.read('geometry=()=>deferred');const stale=h.read('render()');
  h.read('geometry=()=>Promise.reject(new Error("Newer layout failure"))');
  assert.equal(await h.read('render()'),false);
  finish({nodes:[],edges:[]});
  assert.equal(await stale,false);
  assert.equal(h.read('scene'),previous);
  assert.match(h.ids.get('error').textContent,/Newer layout failure/);
  assert.equal(h.ids.get('graph').attributes['aria-busy'],'false');
});

test('Services renders short names, memberships and visible color-matched network areas', async () => {
  const h=await harness();await h.click(h.levels[1]);
  const proxy=h.root.all(n=>n.attributes.class==='node' && n.__data__?.id==='proxy')[0];
  assert.equal(proxy.children.find(n=>n.tagName==='TEXT').textContent,'proxy');
  assert.equal(proxy.children.find(n=>n.attributes.class==='node-detail').textContent,'apps · edge');
  const areas=h.root.all(n=>n.attributes.class==='membership-region');
  assert.equal(areas.length,2);
  for(const area of areas){
    assert.notEqual(area.style.display,'none');
    assert.equal(area.style.opacity,1);
    assert.equal(area.children.find(n=>n.tagName==='PATH').attributes.stroke,
      h.read(`filterColour.get(${JSON.stringify(area.__data__.colorFilter)})`));
    assert.ok(area.children.some(n=>n.tagName==='TEXT' && n.textContent===area.__data__.label));
  }
  const host=h.root.all(n=>n.attributes.class==='group' && n.__data__?.id==='host-a')[0];
  await host.fire('dblclick');
  assert.equal(h.read('level'),'services');assert.equal(h.read('view'),'network');
  assert.ok(Number.isFinite(h.calls.at(-1)[1].k));
});

test('empty inventories render the empty state and finish loading without errors', async () => {
  const h=await harness({title:'Empty',groups:[],nodes:[],edges:[]});
  assert.equal(h.ids.get('empty').hidden,false);
  assert.equal(h.ids.get('graph').attributes['aria-busy'],'false');
  assert.equal(h.ids.get('layout-state').hidden,true);
  assert.equal(h.ids.get('error').hidden,true);
  assert.equal(h.root.all(n=>String(n.attributes.class).split(/\s+/).includes('node')).length,0);
});
