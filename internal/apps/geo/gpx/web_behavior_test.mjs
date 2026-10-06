import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import * as format from './web/format.js';
import {TrackLayers} from './web/tracks.js';

const source = name => readFileSync(new URL(`./web/${name}.js`, import.meta.url), 'utf8');
const sharedSource = name => readFileSync(new URL(`../../../webui/assets/${name}.js`, import.meta.url), 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));

import {Element,documentStub} from '../../../webui/test_dom.mjs';
const deferred=()=>{let resolve;const promise=new Promise(yes=>{resolve=yes;});return {promise,resolve};};

function loadShared(name,context){
  const original=sharedSource(name),names=[...original.matchAll(/export (?:function|class|const) (\w+)/g)].map(match=>match[1]);
  const values=vm.runInContext(`(()=>{${original.replace(/^export /gm,'')} return {${names.join(',')}};})()`,context);
  Object.assign(context,values);return values;
}
function loadModule(name,context){
  if(name!=='trace' && source(name).includes('from "./trace.js"') && !context.addTrace)loadModule('trace',context);
  // Replace module linkage at the boundary, retaining every production method.
  vm.runInContext(source(name).replace(/^import .*;\s*$/gm,'').replace(/^export /gm,''),context,{filename:`${name}.js`});
}

function treeHarness(track){
  const calls=[];
  const entry={id:'a',color:'#0072b2',visible:false,hiddenParts:new Set(),track};
  const state={focus:'other.gpx',tracks:new Map([['current.gpx',entry]]),selection:new Set(),config:{canReveal:false}};
  const context=vm.createContext({
    tooltip(node,lines){node.tip=lines.filter(Boolean);},document:documentStub(),format,state,EYE:'eye',EYE_OFF:'hidden',UP:'up',DOWN:'down',DIAMOND:'ours',DIAMOND_OPEN:'recording',FOLDER:'folder',
    PART_GLYPH:{track:'track',route:'route',waypoint:'waypoint'},
    expandable:entry=>entry.track.parts.length>0,CHEVRON:"triangle",save(){},oursMark:()=>new Element('span'),filteredPaths:()=>['current.gpx'],
    bindRowDrag(){},copyTargets:()=>[],renderTracks(){calls.push(['render']);},
    select(path,options={}){calls.push(['select',path,plain(options)]);},
    selectionFor:()=>['current.gpx'],openMenu(event,groups){calls.push(['menu',groups.flat().filter(Boolean)]);},
    setVisible(path,visible){entry.visible=visible;calls.push(['visible',visible]);},
    setPartHidden(path,key,hidden){entry.hiddenParts[hidden?'add':'delete'](key);calls.push(['hidden',key,hidden]);},
    setFocus(path,options){state.focus=path;calls.push(['focus',plain(options)]);},
    fitBox(){calls.push(['fit']);},openWaypoint(){calls.push(['popup']);},
    map:{getZoom:()=>12,easeTo(){calls.push(['pan']);}},renameGPX(path){calls.push(['rename',path]);},
  });
  loadShared('selection',context);loadShared('controls',context);context.label=context.rowLabel;
  loadModule('drag',context);
  const app=source('app');
  vm.runInContext(app.slice(app.indexOf('const doubleRowClick'),app.indexOf('// manyMenu')),context);
  vm.runInContext(app.slice(app.indexOf('function partRow('),app.indexOf('// ---- dragging rows')),context);
  vm.runInContext(app.slice(app.indexOf('function showPart('),app.indexOf('\nfunction openWaypoint(')),context);
  // Run the application's actual capture listeners before startup's I/O.
  const captures=new Map();context.document.addEventListener=(type,fn,capture)=>{assert.equal(capture,true);captures.set(type,fn);};
  vm.runInContext(app.slice(app.indexOf('async function start() {'),app.indexOf('  const saved = load();'))+'}',context);
  context.start();
  return {context,entry,state,calls,captures};
}
function treeTrack(flags={}){
  return {name:'Trip',points:[[0,0]],parts:[],stops:[],stats:{distance:0},clean:{},...flags};
}

test('actual file-row listeners highlight on one click and fit only on the second rebuilt row',async()=>{
  const h=treeHarness(treeTrack({ours:true}));
  await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:100});
  assert.deepEqual(h.calls.filter(([kind])=>kind==='focus'),[['focus',{fit:false}]]);
  assert.equal(h.entry.visible,true);
  await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:200});
  assert.deepEqual(h.calls.filter(([kind])=>kind==='focus'),[['focus',{fit:false}],['focus',{fit:true}]]);
  for(const modifier of ['shiftKey','ctrlKey','metaKey']){
    h.calls.length=0;
    await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:300,[modifier]:true});
    assert.equal(h.calls.some(([kind])=>kind==='focus'),false);
  }
});

test('actual part-row listeners retain the camera on click and frame on double click',async()=>{
  for(const kind of ['track','route','waypoint']){
    const part={kind,key:'part-1',name:'Part',distance:0,points:[[0,0]],bounds:[[0,0],[1,1]]};
    const h=treeHarness(treeTrack({ours:true,parts:[part]}));
    h.entry.hiddenParts.add(part.key);
    await h.context.partRow('current.gpx',h.entry,part).fire('click',{timeStamp:100});
    assert.equal(h.entry.hiddenParts.has(part.key),false);
    assert.equal(h.calls.some(([action])=>['fit','pan'].includes(action)),false);
    assert.deepEqual(h.calls.find(([action])=>action==='focus'),['focus',{fit:false}]);
    await h.context.partRow('current.gpx',h.entry,part).fire('click',{timeStamp:200});
    assert.equal(h.calls.filter(([action])=>action===(kind==='waypoint'?'pan':'fit')).length,1);
  }
});

test('registered capture listeners interrupt a row pair on controls, outside clicks and context menus',async()=>{
  for(const interrupt of ['control','outside','contextmenu']){
    const h=treeHarness(treeTrack({ours:true}));
    await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:100});
    if(interrupt==='contextmenu')h.captures.get('contextmenu')();
    else h.captures.get('click')({target:{closest:selector=>selector.includes('.track-row')?(interrupt==='control'?{}:null):(interrupt==='control'?{}:null)}});
    await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:200});
    assert.deepEqual(h.calls.filter(([kind])=>kind==='focus').map(([,options])=>options.fit),[false,false]);
  }
});

test('file context menu offers Rename only for a draft or a dgs-created GPX',async()=>{
  for(const flags of [{ours:true},{draft:true},{}]){
    const h=treeHarness(treeTrack(flags));
    await h.context.trackRow('current.gpx',h.entry).fire('contextmenu');
    const rename=h.calls.find(([kind])=>kind==='menu')[1].find(item=>item.label==='Rename…');
    assert.equal(Boolean(rename),Boolean(flags.ours||flags.draft));
    if(rename){rename.onSelect();assert.deepEqual(h.calls.at(-1),['rename','current.gpx']);}
  }
});

function segmentHarness(pieces,{choice='separate',name='Named track',fail=false}={}){
  const root=new Element('div'),requests=[],dialogs=[],messages=[],refresh=[],shown=[];
  const ask=deferred(),choose=deferred(),nameShown=deferred();
  const track={path:'current.gpx',ours:true,pieces,cuts:pieces.slice(1).map(piece=>piece.first),cutCandidates:[],removed:[]};
  const entry={id:'a',track};
  const state={focus:track.path,tracks:new Map([[track.path,entry]])};
  const context=vm.createContext({
    tooltip(node,lines){node.tip=lines.filter(Boolean);},document:documentStub(),format,state,
    Option:class extends Element{constructor(text,value){super('option');this.textContent=text;this.value=value;}},
    status:{show:message=>messages.push(message),showError:message=>messages.push(message)},
    promptDialog:options=>{dialogs.push(['name',options]);nameShown.resolve(options);return ask.promise;},
    chooseDialog:options=>{dialogs.push(['choice',options]);return choose.promise;},
    saveFile:()=>assert.fail('Add opened a save dialog'),
    fetch:async(url,options)=>{requests.push({url,...options,body:JSON.parse(options.body)});return {ok:!fail,json:async()=>fail?{error:'Synthetic server failure'}:{path:track.path,tracks:JSON.parse(options.body).target.merge?1:JSON.parse(options.body).segments.length}};},
    reloadEntry:async path=>refresh.push(['reload',path]),expandable:()=>true,
    setFocus:(path,options)=>refresh.push(['focus',path,plain(options)]),renderTracks(){},save(){},
  });
  loadShared('selection',context);const controls=loadShared('controls',context);context.sharedTextButton=controls.textButton;
  loadModule('api',context);loadModule('cut',context);
  const app=source('app');
  vm.runInContext(app.slice(app.indexOf('async function writeSegments('),app.indexOf('// lassoDone')),context);
  const CutPanel=vm.runInContext('CutPanel',context);
  const panel=new CutPanel({root,onCuts(){},onHover(){},onShow:(piece,options)=>shown.push([piece.first,plain(options)]),onWrite:context.writeSegments});
  panel.show(track,{timeZone:'UTC',workspace:['another.gpx',track.path],folder:''});
  const add=()=>root.all(element=>element.classList.contains('cut-go'))[0];
  return {root,panel,track,state,requests,dialogs,messages,refresh,ask,choose,nameShown,add,choice,name,shown,context};
}
const pieces=[{first:0,last:2,name:'First',points:3,distance:20},{first:2,last:4,name:'Second',points:3,distance:20}];

test('Add button sends one named segment from the original source to the current GPX',async()=>{
  const h=segmentHarness(pieces);
  const checkboxes=h.root.all(element=>element.type==='checkbox');
  checkboxes.at(-1).checked=false;await checkboxes.at(-1).fire('change');
  assert.equal(h.add().textContent,'Add 1 segment');
  const pending=h.add().fire('click');
  assert.equal(h.dialogs[0][0],'name');assert.equal(h.requests.length,0);
  assert.equal(h.add().disabled,true);
  h.state.focus='another.gpx';
  h.panel.show({...h.track,path:'another.gpx'},{timeZone:'UTC',workspace:['current.gpx','another.gpx'],folder:''});
  h.ask.resolve('  Named track  ');await pending;
  assert.deepEqual(h.requests.map(request=>({url:request.url,method:request.method,body:request.body})),[{
    url:'api/segments/write',method:'POST',body:{path:'current.gpx',segments:[{first:0,last:2,name:'Named track'}],target:{mode:'add',path:'current.gpx'}},
  }]);
  assert.equal(h.requests[0].headers['Content-Type'],'application/json');
  assert.equal(h.track.pieces[0].name,'First');assert.equal(h.add().disabled,false);
  assert.deepEqual(h.refresh,[['reload','current.gpx']]);
});

test('multiple-segment Add button requires a choice and sends separate or named merged tracks',async()=>{
  for(const choice of ['separate','merge']){
    const h=segmentHarness(pieces);
    const pending=h.add().fire('click');
    assert.deepEqual(h.dialogs.map(([kind])=>kind),['choice']);assert.equal(h.requests.length,0);
    h.choose.resolve(choice);
    if(choice==='merge'){
      await h.nameShown.promise;
      assert.deepEqual(h.dialogs.map(([kind])=>kind),['choice','name']);assert.equal(h.requests.length,0);
      h.ask.resolve('  Merged walk  ');
    }
    await pending;
    const body=h.requests[0].body;
    assert.deepEqual(body,{path:'current.gpx',segments:pieces.map(({first,last,name})=>({first,last,name})),target:{mode:'add',path:'current.gpx',...(choice==='merge'?{merge:true,name:'Merged walk'}:{})}});
    assert.equal(h.requests.length,1);assert.equal(h.add().disabled,false);
    assert.deepEqual(h.refresh,[['reload','current.gpx'],['focus','current.gpx',{fit:false}]]);
  }
});

test('cancelling dialogs or an empty name emits no request and permits retry',async()=>{
  for(const cancel of ['choice','single name','merged name','blank merged name']){
    const h=segmentHarness(cancel==='single name'?pieces.slice(0,1):pieces);
    const pending=h.add().fire('click');
    if(cancel==='choice')h.choose.resolve(null);
    else{
      if(cancel!=='single name'){h.choose.resolve('merge');await h.nameShown.promise;}
      h.ask.resolve(cancel==='blank merged name'?'   ':null);
    }
    await pending;
    assert.equal(h.requests.length,0);assert.equal(h.add().disabled,false);
    assert.deepEqual(h.refresh,[]);
  }
});

test('failed segment request displays the server error and restores the Add action',async()=>{
  const h=segmentHarness(pieces.slice(0,1),{fail:true});
  const pending=h.add().fire('click');h.ask.resolve('Named');await pending;
  assert.equal(h.requests.length,1);assert.equal(h.messages.at(-1),'Synthetic server failure');
  assert.equal(h.add().disabled,false);assert.deepEqual(h.refresh,[]);
});

test('no selected segments disables Add and Save as actions',async()=>{
  const h=segmentHarness(pieces);
  const all=h.root.all(element=>element.id==='cut-all')[0];all.checked=false;await all.fire('change');
  assert.equal(h.add().disabled,true);
  assert.equal(h.root.all(element=>element.classList.contains('cut-save-as'))[0].disabled,true);
  assert.equal(h.requests.length,0);
});

function markerHarness(){
  const created=[];
  class Marker{
    constructor(options){this.options=options;created.push(this);}
    setLngLat(position){this.position=position;return this;}
    addTo(){return this;}
    getElement(){return this.options.element;}
    remove(){this.removed=true;}
  }
  class Popup{
    constructor(){this.handlers=[];}
    setLngLat(position){this.position=position;return this;}
    setDOMContent(body){this.body=body;return this;}
    addTo(){this.open=true;return this;}
    once(type,handler){assert.equal(type,'close');this.handlers.push(handler);}
    remove(){this.open=false;const handlers=this.handlers;this.handlers=[];handlers.forEach(fn=>fn());}
    isOpen(){return this.open;}
  }
  const context=vm.createContext({document:documentStub(),format,maplibregl:{Marker,Popup}});
  loadModule('stops',context);
  const StopMarkers=vm.runInContext('StopMarkers',context),selected=[];
  const markers=new StopMarkers({addSource(){},addLayer(){},getSource:()=>({setData(){}})},{onSelect:index=>selected.push(index),onRemove(){}});
  return {markers,created,selected};
}

test('stop markers and popups use retained point anchors through selection, hiding and refresh',async()=>{
  const h=markerHarness(),track={points:[[0,0],[0,0.0001],[0,0.0002]],stopParams:{distance:50},stops:[{first:0,last:1,anchor:1,center:[1,1],arrival:0,departure:600000,duration:600},{first:2,last:2,anchor:2,center:[2,2],arrival:600000,departure:1200000,duration:600}]};
  h.markers.show(track,'#0072b2',[[2,2]]);
  const [first,second]=h.created;
  assert.equal(first.position,track.points[1]);assert.equal(second.position,track.points[2]);
  assert.equal(first.options.anchor,'center');assert.deepEqual(plain(first.options.offset),[0,0]);assert.equal(first.options.subpixelPositioning,true);
  assert.equal(first.getElement().firstElementChild.tagName,'BUTTON');assert.equal(second.getElement().firstElementChild.hidden,true);
  const event=await first.getElement().firstElementChild.fire('click');assert.equal(event.stopped,true);assert.deepEqual(h.selected,[0]);
  h.markers.openPopup(0);assert.equal(h.markers.popup.position,track.points[1]);
  assert.equal(first.getElement().firstElementChild.classList.contains('selected'),true);
  assert.equal(first.getElement().classList.contains('selected'),false);
  h.markers.setOnMap(false);assert.equal(first.getElement().firstElementChild.classList.contains('off-map'),true);
  h.markers.setTimeZone('UTC');assert.equal(first.position,track.points[1]);assert.equal(h.markers.popup.position,track.points[1]);
  h.markers.show(track,'#0072b2');assert.equal(first.removed,true);assert.equal(second.removed,true);
  assert.equal(h.markers.markers[0].getElement().firstElementChild.classList.contains('off-map'),true);
  h.markers.clear();assert.equal(h.markers.markers.length,0);assert.equal(h.markers.popup.isOpen(),false);
});


test('unfocused tracks and routes remain legible, while hidden files stay fully hidden',()=>{
  const styles=new Map(),moves=[];
  const map={
    addSource(){},addLayer(layer){styles.set(layer.id,{paint:{...layer.paint},layout:{...layer.layout}});},
    setPaintProperty(id,key,value){styles.get(id).paint[key]=value;},
    setLayoutProperty(id,key,value){styles.get(id).layout[key]=value;},
    moveLayer(id,before){moves.push([id,before]);},
  };
  const layers=new TrackLayers(map),track={parts:[]};
  layers.add('a',track,'#0072b2');layers.add('b',track,'#cc79a7');
  layers.setFocus('a');
  for(const kind of ['line','route']){
    const inactive=styles.get(`track-b-${kind}`),active=styles.get(`track-a-${kind}`);
    assert.equal(inactive.layout.visibility,'visible');
    assert.ok(inactive.paint['line-opacity']>=0.8);
    assert.ok(inactive.paint['line-width']>=1.5);
    assert.ok(active.paint['line-width']>inactive.paint['line-width']);
  }
  assert.equal(styles.get('track-b-line-casing').layout.visibility,'visible');
  assert.ok(styles.get('track-b-line-casing').paint['line-opacity']>=0.6);
  assert.ok(styles.get('track-b-line-casing').paint['line-width']>styles.get('track-b-line').paint['line-width']);
  assert.deepEqual(moves.map(([id])=>id),['track-a-line-edge','track-a-line-casing','track-a-line','track-a-route-edge','track-a-route-casing','track-a-route','track-a-waypoint']);
  layers.setVisible('b',false);layers.setFocus('b');
  for(const kind of ['line-edge','line-casing','line','route-edge','route-casing','route','waypoint'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'none');
  layers.setVisible('b',true);layers.setFocus('a');
  for(const kind of ['line-edge','line-casing','line','route-edge','route-casing','route','waypoint'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'visible');
});


test('segment rows select one or toggle selection without fitting; a second rebuilt row click fits',async()=>{
  const h=segmentHarness(pieces);
  const rows=()=>h.root.all(element=>element.classList.contains('cut-row'));
  await rows()[0].fire('click',{timeStamp:100});
  assert.deepEqual([...h.panel.selected],[0]);assert.deepEqual(h.shown,[[0,{fit:false}]]);
  assert.equal(rows()[0].classList.contains('selected'),true);
  await rows()[0].fire('click',{timeStamp:200});
  assert.deepEqual(h.shown.at(-1),[0,{fit:true}]);
  await rows()[1].fire('click',{timeStamp:300,ctrlKey:true});
  assert.deepEqual([...h.panel.selected],[0,2]);assert.deepEqual(h.shown.at(-1),[2,{fit:false}]);
  await rows()[0].fire('click',{timeStamp:400,ctrlKey:true});assert.deepEqual([...h.panel.selected],[2]);
});

test('route waypoint selection never pans or removes; fit and removal are explicit menu actions',()=>{
  const calls=[],menus=[];
  const context=vm.createContext({format,document:documentStub(),openMenu:(event,sections)=>menus.push(sections),rowDoubleClick:()=>({reset(){}})});
  loadModule('route',context);
  const proto=vm.runInContext('RoutePlanner.prototype',context);
  const planner=Object.create(proto);
  Object.assign(planner,{waypoints:[[0,0],[0,1]],selected:null,doubleClick:{reset(){}},map:{easeTo:options=>calls.push(['pan',options.center])},display:point=>point,draw:()=>calls.push(['draw']),render:()=>calls.push(['render']),remove:index=>calls.push(['remove',index])});
  planner.selectWaypoint(1);assert.equal(planner.selected,1);assert.equal(calls.some(([kind])=>kind==='pan'||kind==='remove'),false);
  planner.waypointMenu({},1);assert.equal(calls.some(([kind])=>kind==='remove'),false);
  menus[0][0][0].onSelect();assert.deepEqual(calls.at(-3),['pan',planner.waypoints[1]]);
  menus[0][0][1].onSelect();assert.deepEqual(calls.at(-1),['remove',1]);
});

test('timeline candidates and cuts select on click; only right-click menu commands edit cuts',async()=>{
  const calls=[],menus=[],context=vm.createContext({format,document:documentStub(),openMenu:(event,sections)=>menus.push(sections)});
  loadModule('timeline',context);
  const timeline=Object.create(vm.runInContext('Timeline.prototype',context));
  Object.assign(timeline,{timeZone:'UTC',track:{time:[0,1000,2000],pieces:[]},cutting:{cuts:[2],candidates:[1],onAdd:index=>calls.push(['add',index]),onRemove:index=>calls.push(['remove',index]),onMove(){ }},fraction:()=>0.5,onScrub:index=>calls.push(['pick',index]),setIndex(){}});
  const elements=timeline.cutElements();
  for(const element of elements){
    await element.fire('click');await element.fire('dblclick');
    assert.equal(calls.some(([kind])=>kind==='add'||kind==='remove'),false);
    await element.fire('contextmenu');menus.at(-1)[0][0].onSelect();
    assert.equal(calls.at(-1)[0],element.classList.contains('timeline-candidate')?'add':'remove');
    calls.length=0;
  }
});


test('planning marker double-click survives rebuilding without selecting a list row as a double-click',async()=>{
  const calls=[];
  class Marker {
    constructor({element}){this.element=element;}
    setLngLat(){return this;} addTo(){return this;} on(){return this;} remove(){}
  }
  const context=vm.createContext({format,document:documentStub(),maplibregl:{Marker}});
  loadShared('selection',context);loadModule('route',context);
  const planner=Object.create(vm.runInContext('RoutePlanner.prototype',context));
  Object.assign(planner,{active:true,waypoints:[[0,0],[0,1]],legs:[],markers:[],handles:[],doubleClick:context.rowDoubleClick(),selected:null,display:point=>point,map:{getSource:()=>({setData(){}}),moveLayer(){},easeTo:options=>calls.push(options)},render(){}});
  planner.draw();await planner.markers[0].element.fire('click',{timeStamp:100});
  assert.equal(planner.selected,0);assert.equal(calls.length,0);
  await planner.markers[0].element.fire('click',{timeStamp:200});assert.equal(calls.length,1);
  assert.equal(planner.doubleClick('row:0',{timeStamp:300,clientX:10,clientY:10,detail:1}),false);
});

test('right-button presses cannot begin moving a timeline cut',async()=>{
  const context=vm.createContext({format,document:documentStub()});loadModule('timeline',context);
  const timeline=Object.create(vm.runInContext('Timeline.prototype',context));
  Object.assign(timeline,{timeZone:'UTC',track:{time:[0,1000],pieces:[]},cutting:{cuts:[1],candidates:[],onMove(){assert.fail('right-click must not move a cut');}},fraction:()=>0.5});
  const [cut]=timeline.cutElements();await cut.fire('pointerdown',{button:2});
  assert.equal(cut.listeners.has('pointermove'),false);assert.equal(cut.listeners.has('pointerup'),false);
});

test('map clicks select tracks or points; cut commands require their menu action',()=>{
  const listeners=new Map(),calls=[],menus=[];
  const entry={id:'track-a',track:{points:[[0,0],[0,1]]},hiddenParts:new Set()};
  const state={tracks:new Map([['synthetic.gpx',entry]]),focus:'synthetic.gpx',fill:{active:false}};
  let cutting=true;
  const context=vm.createContext({state,map:{on:(name,fn)=>listeners.set(name,fn),queryRenderedFeatures:()=>[{layer:{metadata:{track:'track-a'}}}]},layers:{nearest:()=>1,lineLayers:()=>['track-a-line']},focusedEntry:()=>entry,cutting:()=>cutting,waypointAt:()=>null,inspect:index=>calls.push(['inspect',index]),profile:{setIndex:index=>calls.push(['profile',index])},timeline:{setIndex:index=>calls.push(['timeline',index])},cutPanel:{addCut:index=>calls.push(['cut',index])},select:path=>calls.push(['select',path]),renderTracks(){},openMenu:(event,sections)=>menus.push(sections)});
  const app=source('app');vm.runInContext(app.slice(app.indexOf('function bindMap()'),app.indexOf('// ---- layout ----')),context);context.bindMap();
  const event={lngLat:{lng:0,lat:1},point:{x:10,y:10},originalEvent:{target:new Element('div')},preventDefault(){}};
  listeners.get('click')(event);assert.deepEqual(calls,[['inspect',1],['profile',1],['timeline',1]]);
  listeners.get('contextmenu')(event);assert.equal(calls.some(([kind])=>kind==='cut'),false);
  menus[0][1][0].onSelect();assert.deepEqual(calls.at(-1),['cut',1]);
  cutting=false;listeners.get('click')(event);assert.deepEqual(calls.at(-1),['select','synthetic.gpx']);
});


test('fill and removal rows select first, fit on double-click, and expose edits in context menus',async()=>{
  const calls=[],menus=[],context=vm.createContext({format,document:documentStub(),openMenu:(event,sections)=>menus.push(sections)});
  loadShared('selection',context);loadModule('clean',context);
  const panel=Object.create(vm.runInContext('CleanPanel.prototype',context));
  Object.assign(panel,{track:{time:[0,1000]},ways:[],timeZone:'UTC',doubleClick:context.rowDoubleClick(),render(){},onShowFill:(item,options)=>calls.push(['fill',options.fit]),onShowRange:(item,options)=>calls.push(['range',options.fit]),onRemoveFill:()=>calls.push(['remove']),restoreEdit:()=>calls.push(['restore'])});
  for(const [build,kind] of [[()=>panel.fillRow({first:0,last:1,profile:'straight',distance:10,points:2},0),'fill'],[()=>panel.editRow({kind:'lasso',points:[0,1]},0),'range']]){
    panel.doubleClick.reset();
    await build().children[0].fire('click',{timeStamp:100});assert.deepEqual(calls.at(-1),[kind,false]);
    await build().children[0].fire('click',{timeStamp:200});assert.deepEqual(calls.at(-1),[kind,true]);
    await build().fire('contextmenu');assert.deepEqual(calls.at(-1),[kind,false]);
    menus.at(-1)[0].at(-1).onSelect();assert.deepEqual(calls.at(-1),[kind==='fill'?'remove':'restore']);
  }
});


test('selecting a removed range or fill retains the camera until an explicit fit',()=>{
  const calls=[],context=vm.createContext({focusedEntry:()=>({track:{points:[[0,0],[0,1]]}}),inspect:index=>calls.push(['inspect',index]),profile:{setIndex(){}},timeline:{setIndex(){}},fitBox:()=>calls.push(['fit'])});
  const app=source('app');
  vm.runInContext(app.slice(app.indexOf('function showRange('),app.indexOf('// saveClean')),context);
  vm.runInContext(app.slice(app.indexOf('function showFill('),app.indexOf('// ---- opening files ----')),context);
  const range={kind:'range',first:0,last:1};
  context.showRange(range);context.showFill(range);assert.equal(calls.some(([kind])=>kind==='fit'),false);
  context.showRange(range,{fit:true});context.showFill(range,{fit:true});assert.equal(calls.filter(([kind])=>kind==='fit').length,2);
});


test('fine traces keep opaque opposing borders and distinguish every selection state',()=>{
  const layers=new Map(),context=vm.createContext({});loadModule('trace',context);
  const widths=vm.runInContext('TRACE_WIDTHS',context);
  const map={addLayer:layer=>layers.set(layer.id,layer)};
  for(const state of ['inactive','focused','segment','selected'])context.addTrace(map,{id:state,source:'test',color:'#0072b2',state,dashed:true});
  for(const state of ['inactive','focused','segment','selected']){
    const ids=context.traceLayers?context.traceLayers(state):vm.runInContext(`traceLayers('${state}')`,context);
    const [edge,casing,core]=[...ids].map(id=>layers.get(id).paint);
    assert.equal(edge['line-color'],'#17212b');assert.equal(casing['line-color'],'#ffffff');
    assert.ok(edge['line-width']>casing['line-width']);assert.ok(casing['line-width']>core['line-width']);
    assert.ok(edge['line-width']<5);assert.ok(core['line-width']<3);
    for(const paint of [edge,casing,core]){
      assert.equal(paint['line-opacity'],1);
      assert.ok(Math.abs(paint['line-width']*paint['line-dasharray'][0]-7)<1e-9);
      assert.ok(Math.abs(paint['line-width']*paint['line-dasharray'][1]-4)<1e-9);
    }
  }
  assert.ok(widths.selected[2]>widths.segment[2]);assert.ok(widths.focused[2]>widths.inactive[2]);
});

test('highlighted segments retain their colour and never bridge a recording break',()=>{
  const sources=new Map(),styles=new Map(),context=vm.createContext({format,document:documentStub()});loadModule('cut',context);
  const map={addSource:(id)=>sources.set(id,{}),addLayer:layer=>styles.set(layer.id,layer),getSource:id=>({setData:data=>sources.set(id,data)})};
  context.map=map;const overlay=vm.runInContext('new CutOverlay(map,"cursor")',context);
  const piece={first:0,last:5};const track={pieces:[{first:9},piece],points:[[0,0],[0,1],[0,2],[1,0],[1,1],[1,2]],removed:[false,false,false,false,false,false],segments:[0,0,0,1,1,1]};
  overlay.highlight(track,piece);const data=sources.get('cut-segment');
  assert.equal(data.geometry.type,'MultiLineString');assert.deepEqual(plain(data.geometry.coordinates),[track.points.slice(0,3),track.points.slice(3)]);
  assert.equal(data.properties.color,context.pieceColor?context.pieceColor(1):vm.runInContext('pieceColor(1)',context));
  assert.equal(styles.get('cut-segment').paint['line-opacity'],1);
  overlay.highlight(null,null);assert.equal(sources.get('cut-segment').features.length,0);
});

test('map location context menus preserve Add waypoint alongside cuts and pin edits',async()=>{
  const listeners=new Map(),menus=[],calls=[];let found=null,cut=false;
  const context=vm.createContext({state:{},map:{on:(name,fn)=>listeners.set(name,fn)},waypointAt:()=>found,focusedEntry:()=>({track:{}}),cutting:()=>cut,layers:{nearest:()=>2},inspect(){},openMenu:(event,sections)=>menus.push(sections.flat()),addWaypointAt:event=>calls.push(plain(event)),cutPanel:{addCut(){}},renamePart(){},startWaypointMove(){},deletePart(){}});
  const app=source('app');vm.runInContext(app.slice(app.indexOf('function bindMap()'),app.indexOf('// ---- layout ----')),context);context.bindMap();
  for(const mode of ['blank','cut','pin']){
    cut=mode==='cut';found=mode==='pin'?{path:'synthetic.gpx',entry:{track:{ours:true}},part:{}}:null;
    const event={point:{x:1,y:1},lngLat:{lng:12,lat:34},originalEvent:{},preventDefault(){}};
    listeners.get('contextmenu')(event);event.lngLat.lng=99;
    await menus.at(-1).find(item=>item.label==='Add waypoint…').onSelect();assert.deepEqual(calls.at(-1),{lngLat:{lng:12,lat:34}});
  }
});

test('adding a named waypoint uses the clicked WGS84 location and defaults to the focused writable GPX',async()=>{
  let answer={path:'current.gpx',name:'Synthetic point',description:'Test'};const calls=[];
  const state={focus:'current.gpx',tracks:new Map([['other.gpx',{track:{ours:true,name:'Other'}}],['recording.gpx',{track:{name:'Recording'}}],['current.gpx',{track:{draft:true,name:'Current'}}]])};
  const context=vm.createContext({state,planner:{fromDisplay:([lng,lat])=>[lng-1,lat-2]},waypointDialog:async(targets,position)=>{calls.push(['dialog',plain(targets),plain(position)]);return answer;},api:{addWaypoint:async(...args)=>calls.push(['add',...args])},reloadEntry:async path=>calls.push(['reload',path]),renderTracks(){},setFocus:(path,options)=>calls.push(['focus',path,plain(options)]),showMessage:message=>calls.push(['message',message])});
  const app=source('app');vm.runInContext(app.slice(app.indexOf('async function addWaypointAt('),app.indexOf('// fillEndAt')),context);
  const event={lngLat:{lng:12,lat:34}};await context.addWaypointAt(event);
  assert.deepEqual(calls[0],['dialog',[{path:'current.gpx',name:'Current'},{path:'other.gpx',name:'Other'}],[11,32]]);
  assert.deepEqual(calls[1],['add','current.gpx','Synthetic point','Test',11,32]);
  assert.deepEqual(calls.find(([kind])=>kind==='focus'),['focus','current.gpx',{fit:false}]);
  calls.length=0;answer=null;await context.addWaypointAt(event);assert.equal(calls.some(([kind])=>kind==='add'),false);
  calls.length=0;state.tracks=new Map([['recording.gpx',{track:{}}]]);await context.addWaypointAt(event);assert.equal(calls.some(([kind])=>kind==='dialog'||kind==='add'),false);
});


test('hiding parts and removing a GPX also updates every new outline layer',()=>{
  const added=[],filters=new Map(),removed=[],sources=[];
  const map={addSource(){},addLayer:layer=>added.push(layer.id),setFilter:(id,filter)=>filters.set(id,filter),removeLayer:id=>removed.push(id),removeSource:id=>sources.push(id)};
  const layers=new TrackLayers(map);layers.add('a',{parts:[]},'#0072b2');layers.setHiddenParts('a',new Set(['t0','r0']));
  const strokes=added.filter(id=>id.startsWith('track-a-'));
  assert.equal(strokes.length,7);
  for(const id of strokes){
    assert.ok(filters.has(id));assert.deepEqual(plain(filters.get(id).at(-1).at(-1).at(-1)),['literal',['t0','r0']]);
  }
  layers.remove('a');assert.deepEqual(removed,strokes);assert.deepEqual(sources,['track-a']);assert.equal(layers.ids.size,0);
});


test('workspace rows reserve the name and distance for scanning and keep full details in hover tips',()=>{
  const track=treeTrack({ours:true,unsaved:true,stats:{distance:1200},stops:[{},{}],clean:{sidecar:'synthetic.dgs.json'},parts:[{kind:'track'},{kind:'route'},{kind:'waypoint'}]});
  const h=treeHarness(track),row=h.context.trackRow('current.gpx',h.entry);
  assert.equal(row.all(e=>e.classList.contains('meta'))[0].textContent,'');
  assert.equal(row.all(e=>e.classList.contains('row-distance'))[0].textContent,format.distance(1200));
  assert.equal(row.all(e=>e.classList.contains('ours-mark')).length,1);
  assert.equal(row.all(e=>e.classList.contains('file-main')).length,1);
  assert.equal(row.all(e=>e.classList.contains('file-tools')).length,1);
  assert.ok(row.tip.includes('2 stops'));assert.ok(row.tip.includes('sidecar'));assert.ok(row.tip.includes('edited, not saved'));assert.ok(row.tip.includes('current.gpx'));
  assert.ok(row.classList.contains('unsaved'));
});

test('secondary workspace actions remain accessible in the context menu after removing inline buttons',async()=>{
  const h=treeHarness(treeTrack({ours:true}));h.state.config.canReveal=true;
  h.state.tracks.set('other.gpx',{...h.entry});h.context.filteredPaths=()=>['current.gpx','other.gpx'];
  h.context.api={reveal:async path=>h.calls.push(['reveal',path])};h.context.reorderFiles=(...args)=>h.calls.push(['order',...args]);
  await h.context.trackRow('current.gpx',h.entry).fire('contextmenu');
  const menu=h.calls.find(([kind])=>kind==='menu')[1];
  assert.equal(menu.find(item=>item.label==='Move up').disabled,true);assert.equal(menu.find(item=>item.label==='Move down').disabled,false);
  await menu.find(item=>item.label==='Show in file manager').onSelect();assert.deepEqual(h.calls.at(-1),['reveal','current.gpx']);
  menu.find(item=>item.label==='Move down').onSelect();assert.deepEqual(h.calls.at(-1),['order','current.gpx',null]);
});

test('segment rows keep plain numbers and names, move details into tips, and retain cut removal in the menu',async()=>{
  const h=segmentHarness(pieces),row=h.root.all(e=>e.classList.contains('cut-row'))[1];
  assert.equal(row.all(e=>e.classList.contains('cut-row-meta')).length,0);assert.equal(row.all(e=>e.classList.contains('cut-remove')).length,0);
  const number=row.all(e=>e.classList.contains('cut-number'))[0];assert.equal(number.textContent,'2');assert.equal(number.style.background,undefined);
  assert.ok(row.tip.some(line=>line.includes('3 points')));assert.equal(row.style.borderLeftColor,h.context.pieceColor?h.context.pieceColor(1):vm.runInContext('pieceColor(1)',h.context));
  const menus=[];h.context.openMenu=(event,sections)=>menus.push(sections.flat());
  await row.fire('contextmenu');assert.ok(menus[0].some(item=>item.label==='Remove cut before this segment'));
});

test('unnamed same-day segments show a short time span but retain the complete dates in their tip',()=>{
  const start=Date.UTC(2026,0,1,9),end=start+20*60*1000;
  const h=segmentHarness([{first:0,last:2,points:3,distance:20,start,end}]);h.panel.timeZone='UTC';h.panel.render();
  const row=h.root.all(e=>e.classList.contains('cut-row'))[0];assert.equal(row.all(e=>e.classList.contains('cut-name'))[0].textContent,'09:00–09:20');
  assert.ok(row.tip.some(line=>line.includes('2026-01-01')));assert.ok(row.tip.includes('20m'));
});


test('segment inspector uses plain tab counts, suppresses routine footer text and keeps errors visible',()=>{
  const context=vm.createContext({document:documentStub(),format});loadModule('clean',context);
  const panel=Object.create(vm.runInContext('CleanPanel.prototype',context));
  const params={edits:[]};for(const key of vm.runInContext('FILTERS.map(filter=>filter.key)',context))params[key]={enabled:false};
  Object.assign(panel,{root:new Element('div'),segmentsRoot:new Element('div'),tab:'segments',track:{name:'Synthetic trip',pieces:[{},{},{}],fills:[],clean:{params,counts:{}}}});
  panel.render();
  const tabs=panel.root.all(e=>e.attributes.role==='tab');assert.ok(tabs.some(tab=>tab.textContent==='Segments (3)'));
  assert.equal(panel.root.all(e=>e.classList.contains('tab-count')).length,0);
  assert.equal(panel.root.all(e=>e.classList.contains('clean-foot')).length,0);
  panel.track.clean.error='Synthetic state error';panel.render();
  assert.equal(panel.root.all(e=>e.classList.contains('error'))[0].textContent,'Sidecar not read: Synthetic state error');
});


test('file toolbar contains only origin, reveal and visibility; actions do not select or fit the row',async()=>{
  const h=treeHarness(treeTrack({ours:true}));h.state.config.canReveal=true;
  h.state.tracks.set('other.gpx',{...h.entry});h.context.filteredPaths=()=>['current.gpx','other.gpx'];
  h.context.api={reveal:async path=>h.calls.push(['reveal',path])};
  const row=h.context.trackRow('current.gpx',h.entry),tools=row.all(e=>e.classList.contains('file-tools'))[0];
  assert.deepEqual(tools.children.map(e=>e.className),['ours-mark ours','row-action','eye']);
  assert.equal(row.all(e=>e.type==='color')[0].hidden,true);
  assert.equal(row.children[0].className,'disclosure-space');
  assert.equal(row.children[1].className,'file-main');
  const reveal=await tools.all(e=>e.classList.contains('row-action'))[0].fire('click');assert.equal(reveal.stopped,true);assert.deepEqual(h.calls.at(-1),['reveal','current.gpx']);
  const eye=await tools.all(e=>e.classList.contains('eye'))[0].fire('click');assert.equal(eye.stopped,true);
  assert.equal(h.calls.some(([kind])=>kind==='focus'||kind==='fit'||kind==='select'),false);
  const color=row.all(e=>e.type==='color')[0];let opened=false;color.click=()=>{opened=true;};
  await row.fire('contextmenu');h.calls.find(([kind])=>kind==='menu')[1].find(item=>item.label==='Change colour…').onSelect();assert.equal(opened,true);
});

test('draft and recording origins remain distinct and remote pages omit the reveal button',()=>{
  for(const flags of [{draft:true},{ours:true},{}]){
    const h=treeHarness(treeTrack(flags)),row=h.context.trackRow('current.gpx',h.entry);
    const mark=row.all(e=>e.classList.contains('ours-mark'))[0];
    assert.equal(mark.classList.contains('ours'),Boolean(flags.draft||flags.ours));
    assert.ok(mark.attributes['aria-label']);assert.equal(row.all(e=>e.classList.contains('row-action')).length,0);
  }
});

test("file disclosure leads the row and expands without selecting or fitting", async()=>{
 const h=treeHarness(treeTrack({ours:true,parts:[{kind:"track"}]})),row=h.context.trackRow("current.gpx",h.entry);
 assert.ok(row.children[0].classList.contains("disclosure"));
 const event=await row.children[0].fire("click");
 assert.equal(event.stopped,true);assert.equal(h.entry.expanded,true);
 assert.equal(h.calls.some(([kind])=>kind==="focus"||kind==="fit"||kind==="select"),false);
});

test('clicking a child track visibly selects the rebuilt row and retains the camera',async()=>{
 const part={kind:'track',key:'t0',name:'Synthetic track',points:[[0,0]],bounds:[[0,0],[1,1]],distance:100};
 const h=treeHarness(treeTrack({ours:true,parts:[part]}));h.state.focus='current.gpx';
 await h.context.partRow('current.gpx',h.entry,part).fire('click',{timeStamp:100});
 assert.deepEqual(plain(h.state.partSelection),{path:'current.gpx',key:'t0'});
 assert.ok(h.context.partRow('current.gpx',h.entry,part).classList.contains('selected'));
 assert.equal(h.calls.some(([kind])=>kind==='fit'||kind==='pan'),false);
 const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
 assert.match(css,/\.list li\.part-row\.selected\s*\{[^}]*background: var\(--selected\)/);
});
