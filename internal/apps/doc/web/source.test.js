const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('a country field with options is picked from them alone', () => {
  const source = fs.readFileSync(`${__dirname}/web/common.js`, 'utf8');
  const start = source.indexOf('export function inputFor(');
  const end = source.indexOf('\n}\n', start) + 3;
  const tags = [];
  let offered;
  const context = vm.createContext({
    el: (tag, props, ...children) => {
      tags.push(tag);
      const node = { tag, ...props, children,
        classList: { toggle() {} },
        addEventListener() {},
        append(...nodes) { this.children.push(...nodes); },
        setAttribute(key, value) { this[key] = value; },
        dispatchEvent() {},
      };
      return node;
    },
    Event, CARD_OPTIONS: 5, tall: () => '', fieldHead: () => [], countriesOf: () => ['USA'],
    suggest: (control, choices) => { offered = choices(); return control; },
  });
  vm.runInContext(source.slice(start, end).replace('export ', ''), context);
  const row = context.inputFor({ key: 'country', type: 'country', options: ['cn', 'aus'] }, 'cn', '', {});
  const [control, cards] = row.children;
  assert.equal(control.tag, 'select');
  assert.equal(control.hidden, true);
  assert.ok(!tags.includes('input'));
  assert.deepEqual(cards.children.map(button => button.children[0]), ['cn', 'aus']);
  cards.children[1].onclick();
  assert.equal(control.value, 'aus');
  context.inputFor({ key: 'country', type: 'country' }, '', '', {});
  assert.deepEqual(offered, ['USA']);
});

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

test('last file import replaces progress with success for Items and revisions', async () => {
  const source = fs.readFileSync(`${__dirname}/web/import.js`, 'utf8');
  const start = source.indexOf('$("import").onsubmit = async');
  const end = source.indexOf('\n};', start) + 3;
  for (const revision of [false, true]) {
    const nodes = new Map();
    const $ = id => {
      if (!nodes.has(id)) nodes.set(id, { value: '', hidden: false, checked: false });
      return nodes.get(id);
    };
    $('into').value = revision ? 'item-a' : '';
    const tags = { commit() {}, get: () => [] };
    const context = vm.createContext({
      $, tags, revisionTags: tags, noPDF: false, pages: [], dir: 'synthetic', selected: 'alice.pdf',
      outputFolder: '', selectedTemplate: 'passport', state: { capabilities: {} },
      files: [{ path: 'alice.pdf', item: 'item-a' }],
      fieldsOf: () => ({}), sharedWith: () => [], templateOf: () => ({}),
      say: (node, text) => { node.textContent = text; },
      post: async () => ({}), loadState: async () => ({ capabilities: {} }),
      markImported: () => {}, render: () => {},
      open: async () => assert.fail("import must not rescan source files"), pick: () => assert.fail('no unimported file remains'),
    });
    vm.runInContext(source.slice(start, end), context);
    await $('import').onsubmit({ preventDefault() {} });
    assert.equal($('import-message').textContent, 'Imported.');
    assert.equal($('list-message').textContent, 'Everything in this folder is in the tree.');
    assert.equal($('import-button').disabled, false);
    context.post = async () => { throw new Error('synthetic import failure'); };
    await $('import').onsubmit({ preventDefault() {} });
    assert.equal($('import-message').textContent, 'synthetic import failure');
    assert.equal($('import-button').disabled, false);
  }
});

test('multiple text fields restore individual comma-separated bubbles', async () => {
  const source = fs.readFileSync(`${__dirname}/web/common.js`, 'utf8');
  const start = source.indexOf('export function inputFor(');
  const end = source.indexOf('\n}\n', start) + 3;
  let options, restored;
  const context = vm.createContext({
    el: (tag, props, ...children) => ({ tag, ...props, children }),
    fieldHead: () => [], api: path => path, URLSearchParams,
    fetch: async () => ({ ok: true, json: async () => ['alice', 'bob'] }),
    tagField: (host, opts) => { options = opts; return { set: list => { restored = list; } }; },
  });
  vm.runInContext(source.slice(start, end).replace('export ', ''), context);
  context.inputFor({ key: 'people', type: 'text', multiple: true }, 'alice,bob, ,', '', {}, '', 'ticket');
  assert.deepEqual(Array.from(restored), ['alice', 'bob']);
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(Array.from(options.known(), entry => entry.name), ['alice', 'bob']);
});

test('read-ahead scans once and stops after three unimported files', () => {
  const source = fs.readFileSync(`${__dirname}/web/import.js`, 'utf8');
  const start = source.indexOf('function readAhead(path)');
  const end = source.indexOf('\n}\n', start) + 3;
  let inspected = 0, sent;
  const files = Array.from({length: 10000}, (_, index) => ({
    get path() { inspected++; return `scan-${index}`; },
    get item() { inspected++; return index < 5000 ? 'ITEM-1' : ''; },
  }));
  const context = vm.createContext({ files, AHEAD: 3, dir: 'synthetic', post: (endpoint, body) => { sent = body; return Promise.resolve(); } });
  vm.runInContext(source.slice(start, end), context);
  context.readAhead('scan-0');
  assert.deepEqual(Array.from(sent.paths), ['scan-5000', 'scan-5001', 'scan-5002']);
  assert.ok(inspected < 6000, `inspected ${inspected} fields`);
});


test('import updates pages and duplicate content without rescanning sources', () => {
 const source = fs.readFileSync(`${__dirname}/web/import.js`, 'utf8');
 const start = source.indexOf('function markImported(');
 const end = source.indexOf('\n}\n', start) + 3;
 const files = [
  {path:'a.png',digest:'image-a'}, {path:'b.png',digest:'image-b'},
  {path:'copy.png',digest:'image-a'}, {path:'other.pdf',digest:'other'},
 ];
 const context = vm.createContext({files, sourceRequest:0, sourceReading:null, say:()=>{}, $:()=>({})});
 vm.runInContext(source.slice(start,end),context);
 context.markImported({id:'ITEM-1',revisions:[{digest:'pdf',source_digests:['image-a','image-b']}]},['a.png','b.png']);
 assert.deepEqual(files.map(file=>file.item),['ITEM-1','ITEM-1','ITEM-1',undefined]);
});

test('Browse arrows select visible items without page scrolling or stealing editor keys', () => {
 const source = fs.readFileSync(`${__dirname}/web/browse.js`, 'utf8');
 const start = source.indexOf('function navigateItems('), end = source.indexOf('\n}\n', start) + 3;
 const items = [{id:'a'}, {id:'b'}, {id:'c'}];
 let opened, prevented = false;
 const pane = {focus() {}, querySelectorAll:()=>[], scrollTop:0};
 const context = vm.createContext({document:{body:{classList:{contains:()=>false}}}, selected:{id:'a'},
  shownItems:()=>items, open:item=>{opened=item.id;context.selected={id:item.id};}, $:()=>pane});
 vm.runInContext(source.slice(start,end),context);
 const event = key=>({key,target:{closest:()=>null},preventDefault(){prevented=true;}});
 context.navigateItems(event('ArrowDown'));assert.equal(opened,'b');assert.equal(prevented,true);
 context.navigateItems(event('ArrowUp'));assert.equal(opened,'a');
 context.navigateItems(event('ArrowUp'));assert.equal(opened,'a');
 opened=null;prevented=false;const typing=event('ArrowDown');typing.target.closest=()=>({});
 context.navigateItems(typing);assert.equal(opened,null);assert.equal(prevented,false);
});

test('time fields use 24-hour clock parts and retain seconds and time zone', () => {
 const source = fs.readFileSync(`${__dirname}/web/common.js`, 'utf8');
 const start=source.indexOf('export function inputFor('),end=source.indexOf('\n}\n',start)+3;
 const events=[];
 const context=vm.createContext({el:(tag,props,...children)=>({tag,...props,children,dispatchEvent:e=>events.push(e.type),focus(){this.focused=true;},select(){this.selected=true;}}),fieldHead:()=>[],Event,Intl,queueMicrotask:()=>{}});
 vm.runInContext(source.slice(start,end).replace('export ',''),context);
 const row=context.inputFor({key:'departure_time',type:'time',options:['Asia/Shanghai','Australia/Sydney']},'23:59:07 Asia/Shanghai','',{});
 const hidden=row.children[0], [clock,zone]=row.children[1].children;
 const [hour,,minute,,second]=clock.children;
 assert.equal(hour.value,'23');assert.equal(minute.value,'59');assert.equal(second.value,'07');
 assert.deepEqual(Array.from(zone.children, option => option.children[0]), ['Shanghai', 'Sydney']);
 assert.equal(hour.type,'number');assert.equal(hour.min,0);assert.equal(hour.max,23);
 assert.equal(minute.max,59);assert.equal(second.max,59);
 hour.value='0';zone.value='Australia/Sydney';hour.oninput();
 assert.equal(hidden.value,'00:59:07 Australia/Sydney');
 hour.value='24';hour.oninput();assert.equal(hour.value,'23');
 minute.value='60';minute.oninput();assert.equal(minute.value,'59');
 second.value='999';second.oninput();assert.equal(second.value,'59');
 minute.value='-1';minute.oninput();assert.equal(minute.value,'0');
 second.value='12.5';second.oninput();assert.equal(second.value,'12');
 assert.equal(hidden.value,'23:00:12 Australia/Sydney');

 hour.value='';hour.oninput();assert.equal(hidden.value,'');
 assert.deepEqual(events.slice(0,4),['input','change','input','change']);
 hour.value='2';hour.oninput({inputType:'insertText'});assert.equal(minute.focused,undefined);
 hour.value='20';hour.oninput({inputType:'insertText'});assert.equal(minute.focused,true);assert.equal(minute.selected,true);
 minute.value='55';minute.oninput({inputType:'insertText'});assert.equal(second.focused,true);assert.equal(second.selected,true);
 second.focused=false;minute.value='56';minute.oninput({inputType:'insertFromPaste'});assert.equal(second.focused,false);

 const blank=context.inputFor({key:'departure_time',type:'time',options:['Asia/Shanghai']},'','',{});
 const parts=blank.children[1].children[0].children.filter(node=>node.tag==='input');
 assert.deepEqual(Array.from(parts, part=>part.value),['','','']);
 assert.deepEqual(Array.from(parts, part=>part.placeholder),['--','--','--']);
 parts[1].value='12';parts[1].oninput();
 assert.deepEqual(Array.from(parts,part=>part.value),['00','12','00']);
 assert.equal(blank.children[0].value,'00:12:00 Asia/Shanghai');

 const existing=context.inputFor({key:'departure_time',type:'time',options:['Asia/Shanghai']},'00:00:00 Asia/Shanghai','',{});
 assert.deepEqual(Array.from(existing.children[1].children[0].children.filter(node=>node.tag==='input'),part=>part.value),['00','00','00']);

});


test('ambiguous time-zone city names retain their full paths', () => {
 const source=fs.readFileSync(`${__dirname}/web/common.js`,'utf8');
 const start=source.indexOf('export function inputFor('),end=source.indexOf('\n}\n',start)+3;
 const context=vm.createContext({el:(tag,props,...children)=>({tag,...props,children}),fieldHead:()=>[],Intl,queueMicrotask:()=>{}});
 vm.runInContext(source.slice(start,end).replace('export ',''),context);
 const row=context.inputFor({key:'time',type:'time',options:['America/Panama','Etc/GMT','Africa/Abidjan','Test/Abidjan']},'12:00 America/Panama','',{});
 const zone=row.children[1].children[1];
 assert.deepEqual(Array.from(zone.children, option => option.children[0]),['Panama','GMT','Africa/Abidjan','Test/Abidjan']);
 assert.deepEqual(Array.from(zone.children, option => option.value),['America/Panama','Etc/GMT','Africa/Abidjan','Test/Abidjan']);
});

test('Import exposes source listing before duplicate detection completes', async () => {
 const source=fs.readFileSync(`${__dirname}/web/import.js`,'utf8');
 const start=source.indexOf('let sourceRequest ='),end=source.indexOf('\n$("open").onclick',start);
 let finish, rendered=0;
 const slow=new Promise(resolve=>{finish=resolve;});
 const context=vm.createContext({sourceReading:null,sourceRequest:0,sourceChecking:false,AbortController,api:path=>path,
  say:()=>{},$:()=>({}),dir:'synthetic',files:[],state:{items:[]},selected:'',closed:new Set(),clearPreview:()=>{},remember:()=>{},render:()=>{rendered++;},
  fetch:async url=>url.includes('metadata=1')?{ok:true,json:async()=>({dir:'synthetic',files:[{path:'a.pdf'}]})}:slow});
 vm.runInContext(source.slice(start,end),context);
 await context.open('synthetic');
 assert.equal(rendered,2);assert.equal(context.files.length,1);
 context.state.items=[{id:'ITEM-1',revisions:[{digest:'a'}]}];
 finish({ok:true,json:async()=>({files:[{path:'a.pdf',digest:'a'}]})});
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(rendered,3);assert.equal(context.files[0].item,'ITEM-1');
});

test('Hide imported keeps unchecked files hidden until detection finishes', async () => {
 const source=fs.readFileSync(`${__dirname}/web/import.js`,'utf8');
 const start=source.indexOf('let sourceRequest ='),end=source.indexOf('\n$("open").onclick',start);
 const shown=source.match(/  const shown = .*;/)[0];
 let finish,ready=false;
 const slow=new Promise(resolve=>{finish=resolve;});
 const visible=[];
 const context=vm.createContext({AbortController,api:path=>path,sourceChecking:false,selected:'',sourceReading:null,
  say:()=>{},$:id=>({checked:id==='hide-kept'}),dir:'synthetic',files:[],state:{items:[]},closed:new Set(),clearPreview:()=>{},remember:()=>{},
  render:()=>{visible.push(vm.runInContext(`(()=>{${shown}return shown.length;})()`,context));},
  fetch:async url=>url.includes('metadata=1')?{ok:true,json:async()=>({dir:'synthetic',files:[{path:'a.pdf'},{path:'b.pdf'}]})}:slow});
 vm.runInContext(source.slice(start,end),context);
 const pending=context.open('synthetic').then(()=>{ready=true;});
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(ready,false);assert.deepEqual(visible,[0,0]);
 finish({ok:true,json:async()=>({files:[{path:'a.pdf',item:'ITEM-1'},{path:'b.pdf'}]})});
 await pending;
 assert.equal(ready,true);assert.deepEqual(visible,[0,0,1]);
});

test('time zone follows country defaults until explicitly chosen', () => {
 const source=fs.readFileSync(`${__dirname}/web/common.js`,'utf8');
 const start=source.indexOf('export function inputFor('),end=source.indexOf('\n}\n',start)+3;
 const pending=[];
 const context=vm.createContext({el:(tag,props,...children)=>({tag,...props,children,dispatchEvent(){},append(node){this.children.push(node);},get options(){return this.children;}}),fieldHead:()=>[],Intl,Event,queueMicrotask:fn=>pending.push(fn)});
 vm.runInContext(source.slice(start,end).replace('export ',''),context);
 const field={key:'session',type:'time',options:['Asia/Shanghai','Australia/Perth']};
 const row=context.inputFor(field,'','',{},undefined,undefined,'aus');
 const zone=row.children[1].children[1],clock=row.children[1].children[0];
 assert.equal(zone.value,'Australia/Sydney');
 const nation={name:'country',value:'cn'};
 let change;
 row.isConnected=true;
 row.parentElement={querySelector:()=>nation,addEventListener:(name,fn)=>{change=fn;}};
 pending.shift()();
 assert.equal(zone.value,'Asia/Shanghai');
 clock.children[0].value='12';clock.children[0].oninput();
 nation.value='aus';change({target:nation});
 assert.equal(row.children[0].value,'12:00:00 Australia/Sydney');
 zone.value='Australia/Perth';zone.onchange();
 nation.value='cn';change({target:nation});
 assert.equal(zone.value,'Australia/Perth');
 const saved=context.inputFor(field,'09:00:00 Australia/Perth','',{},undefined,undefined,'cn');
 saved.isConnected=true;saved.parentElement=row.parentElement;
 pending.shift()();
 assert.equal(saved.children[1].children[1].value,'Australia/Perth');
 const unknown=context.inputFor(field,'','',{},undefined,undefined,'other');
 assert.equal(unknown.children[1].children[1].value,'Asia/Shanghai');
});

test('Template filter selects only matches and Enter confirms the highlighted row', () => {
 const source=fs.readFileSync(`${__dirname}/web/import.js`,'utf8');
 const start=source.indexOf('  const words = $("template-filter")'),end=source.indexOf('  $("template").replaceChildren',start);
 const filter={hidden:false,value:'ticket'};
 const context=vm.createContext({$:()=>filter,state:{templates:[{type:'id_card'},{type:'event_ticket'},{type:'transport_ticket'}]},selectedTemplate:'id_card'});
 vm.runInContext(source.slice(start,end)+'\nglobalThis.matches = shown;',context);
 assert.deepEqual(Array.from(context.matches,t=>t.type),['event_ticket','transport_ticket']);
 assert.equal(context.selectedTemplate,'event_ticket');
 filter.value='nothing-matches';
 const empty=vm.createContext({$:()=>filter,state:context.state,selectedTemplate:'id_card'});
 vm.runInContext(source.slice(start,end)+'\nglobalThis.matches = shown;',empty);
 assert.equal(empty.matches.length,0);
 assert.equal(empty.selectedTemplate,'');
 let handler,picked;
 let active=0;
 const buttons=['event_ticket','transport_ticket'].map((type,index)=>({classList:{contains:()=>index===active},click:()=>{picked=type;},querySelector:()=>({textContent:type})}));
 const keyboard=vm.createContext({$:()=>({addEventListener:(event,fn)=>{handler=fn;}}),templateChoices:()=>buttons,typeOf:button=>button.querySelector().textContent,choose:type=>{active=buttons.findIndex(button=>button.querySelector().textContent===type);}});
 const keyStart=source.indexOf('$("template-filter").addEventListener("keydown"'),keyEnd=source.indexOf('\n$("template").addEventListener("keydown"',keyStart);
 vm.runInContext(source.slice(keyStart,keyEnd),keyboard);
 handler({key:'Enter',preventDefault(){}});
 assert.equal(picked,'event_ticket');
 handler({key:'ArrowDown',preventDefault(){}});
 handler({key:'Enter',preventDefault(){}});
 assert.equal(picked,'transport_ticket');
});

test('No matching Template disables import and blocks submission; clearing the filter restores it', async () => {
 const source=fs.readFileSync(`${__dirname}/web/import.js`,'utf8');
 const nodes=new Map();
 const $=id=>{
  if(!nodes.has(id)) nodes.set(id,{value:'',hidden:false,replaceChildren(...children){this.children=children;},querySelectorAll:()=>[]});
  return nodes.get(id);
 };
 const templates=Array.from({length:7},(_,i)=>({type:`type_${i}`,fields:[]}));
 const context=vm.createContext({$,state:{templates,items:[],root:''},selectedTemplate:'type_0',FILTER_FROM:6,
  templateOf:(state,type)=>state.templates.find(t=>t.type===type),suggestedType:'',dir:'',selected:'',noPDF:false,metadataItem:'',
  el:(tag,props,...children)=>({tag,...props,children,setAttribute(){}}),fieldInputs:()=>[],
  filterInto(){},suggest(){},drawReplaces(){},drawShared(){},
  tags:{commit(){assert.fail('blocked submission must not start');}},post(){assert.fail('blocked submission must not write');}});
 const start=source.indexOf('function drawFields()'),end=source.indexOf('\n}\n',start)+3;
 vm.runInContext(source.slice(start,end),context);
 $('template-filter').value='nothing-matches';
 context.drawFields();
 assert.equal(context.selectedTemplate,'');
 assert.equal($('template').children.length,0);
 assert.equal($('import-button').disabled,true);
 assert.equal($('fields').children[0].children[0],'No matching Templates.');
 const submitStart=source.indexOf('$("import").onsubmit = async'),submitEnd=source.indexOf('\n};',submitStart)+3;
 vm.runInContext(source.slice(submitStart,submitEnd),context);
 await $('import').onsubmit({preventDefault(){}});
 $('template-filter').value='';
 context.drawFields();
 assert.equal(context.selectedTemplate,'type_0');
 assert.equal($('template').children.length,7);
 assert.equal($('import-button').disabled,false);
});

test('Hide imported excludes the selected file after the final import', () => {
 const source=fs.readFileSync(`${__dirname}/web/import.js`,'utf8');
 const start=source.indexOf('  const shown = $("hide-kept")'),end=source.indexOf('  $("tree").replaceChildren',start);
 const toggle={checked:true};
 const context=vm.createContext({$:()=>toggle,sourceChecking:false,selected:'last.pdf',files:[{path:'last.pdf',item:'item-a'}]});
 const evaluate=()=>vm.runInContext('(function(){'+source.slice(start,end)+'return shown;})()',context);
 assert.equal(evaluate().length,0);
 context.files.push({path:'next.pdf'});
 assert.deepEqual(Array.from(evaluate(),f=>f.path),['next.pdf']);
 context.sourceChecking=true;
 assert.equal(evaluate().length,0);
 toggle.checked=false;
 assert.equal(evaluate().length,2);
});

test('File check uses shared columns and confirms only the chosen candidate', async () => {
  const source = fs.readFileSync(`${__dirname}/web/browse.js`, 'utf8');
  const nodes = new Map();
  const $ = (id) => {
    if (!nodes.has(id)) nodes.set(id, { hidden: false, disabled: false, showModal() {}, close() {} });
    return nodes.get(id);
  };
  const problem = { item: 'item-a', digest: 'digest-a', Path: 'old/document.pdf', Detail: 'Confirm its new path', candidates: ['new/document.pdf', 'other/document.pdf'] };
  const calls = [];
  let options, drawn, reject = false;
  const context = vm.createContext({
    $, state: { items: [{ id: 'item-a' }] }, label: () => 'Synthetic document',
    el: (tag, props, ...children) => ({ tag, ...props, children, append(...more) { this.children.push(...more); } }),
    listTable: (host, config) => { options = config; return { draw(rows) { drawn = rows; } }; },
    post: async (url, body) => {
      calls.push({ url, body });
      if (url === '/api/files/check') return { recorded: [], report: { Problems: [problem] } };
      if (reject) throw new Error('Synthetic confirmation failure');
      return {};
    },
    reload: async () => {}, say: (node, text) => { node.textContent = text; },
  });
  vm.runInContext(source.slice(source.indexOf('let fileProblems = [];')), context);
  await $('check-files').onclick();
  assert.deepEqual(Array.from(options.columns, c => c.id), ['item', 'path', 'detail', 'candidates']);
  assert.equal(drawn[0], problem);
  assert.equal($('check-files').disabled, false);
  const candidateColumn = options.columns.find(c => c.id === 'candidates');
  const cell = candidateColumn.cell(problem);
  assert.equal(cell.children[1].children[0].children[0], 'other/document.pdf');
  const button = cell.children[1].children[1];
  reject = true;
  await button.onclick();
  assert.equal(button.disabled, false);
  assert.equal(problem.confirmed, undefined);
  reject = false;
  await button.onclick();
  assert.deepEqual(JSON.parse(JSON.stringify(calls.at(-1))), {
    url: '/api/files/confirm', body: { item: 'item-a', digest: 'digest-a', previous: 'old/document.pdf', path: 'other/document.pdf' },
  });
  assert.equal(options.columns.find(c => c.id === 'path').cell(problem), 'other/document.pdf');
  assert.equal(options.columns.find(c => c.id === 'detail').cell(problem), 'Recorded');
  assert.equal(candidateColumn.cell(problem).children.length, 0);
});
