import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import * as format from './web/format.js';
import {TrackLayers} from './web/tracks.js';
import {sharpen} from './web/sharpen.js';

const source = name => readFileSync(new URL(`./web/${name}.js`, import.meta.url), 'utf8');
const sharedSource = name => readFileSync(new URL(`../../../webui/assets/${name}.js`, import.meta.url), 'utf8');
const plain = value => JSON.parse(JSON.stringify(value));

import {Element,documentStub} from '../../../webui/test_dom.mjs';
const deferred=()=>{let resolve;const promise=new Promise(yes=>{resolve=yes;});return {promise,resolve};};

function loadShared(name,context){
  if(name === "dialog" && !context.window)context.window={addEventListener(){}};
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
  const entry={id:'a',color:'#0072b2',visible:true,hiddenParts:new Set(),track};
  const state={focus:'other.gpx',tracks:new Map([['current.gpx',entry]]),partPicks:new Set(),selection:new Set(),config:{canReveal:false}};
  const context=vm.createContext({
    tooltip(node,lines){node.tip=lines.filter(Boolean);},document:documentStub(),format,state,EYE:'eye',EYE_OFF:'hidden',UP:'up',DOWN:'down',DIAMOND:'ours',DIAMOND_OPEN:'recording',FOLDER:'folder',
    iconSvg:name=>`<svg data-icon="${name}"></svg>`,PART_GLYPH:{track:'track',route:'route',waypoint:'waypoint'},
    expandable:entry=>entry.track.parts.length>0,CHEVRON:"triangle",save(){},oursMark:()=>new Element('span'),filteredPaths:()=>['current.gpx'],
    bindRowDrag(){},copyTargets:()=>[],renderTracks(){calls.push(['render']);},
    select(path,options={}){calls.push(['select',path,plain(options)]);},
    selectionFor:()=>['current.gpx'],openMenu(event,groups){calls.push(['menu',groups.flat().filter(Boolean)]);},
    setVisible(path,visible){entry.visible=visible;calls.push(['visible',visible]);},
    setPartHidden(path,key,hidden){entry.hiddenParts[hidden?'add':'delete'](key);calls.push(['hidden',key,hidden]);},
    setFocus(path,options){state.focus=path;calls.push(['focus',plain(options)]);},
    setMode:mode=>calls.push(["mode",mode]),applyCutPanel(){calls.push(['cut-panel']);},applyCleanPanel(){},showCharts(){},fitBox(){calls.push(['fit']);},openWaypoint(){calls.push(['popup']);},
    map:{getZoom:()=>12,easeTo(){calls.push(['pan']);}},renameGPX(path){calls.push(['rename',path]);},renameName(path){calls.push(['name',path]);},restoreName(path){calls.push(['restore',path]);},
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
    assert.equal(h.entry.hiddenParts.has(part.key),true);
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

test('file context menu renames the GPX name, and the file only for a draft or a dgs-created GPX',async()=>{
  for(const flags of [{ours:true},{draft:true},{}]){
    const h=treeHarness(treeTrack(flags));
    await h.context.trackRow('current.gpx',h.entry).fire('contextmenu');
    const items=h.calls.find(([kind])=>kind==='menu')[1].flat();
    const rename=items.find(item=>item.label==='Rename…'),file=items.find(item=>item.label==='Rename file…');
    rename.onSelect();
    assert.deepEqual(h.calls.at(-1),[flags.draft?'rename':'name','current.gpx']);
    assert.equal(Boolean(file),!flags.draft);
    if(file){file.onSelect();assert.deepEqual(h.calls.at(-1),['rename','current.gpx']);}
    assert.equal(items.some(item=>item.label==='Restore the name'),false);
  }
  const h=treeHarness(treeTrack({renamed:true}));
  await h.context.trackRow('current.gpx',h.entry).fire('contextmenu');
  h.calls.find(([kind])=>kind==='menu')[1].flat().find(item=>item.label==='Restore the name').onSelect();
  assert.deepEqual(h.calls.at(-1),['restore','current.gpx']);
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
  context.formDialog=async options=>{
    const fields=options.fields.flatMap(field=>field.children);
    const target=fields.find(field=>field.attributes['aria-label']==='Save to');
    const mode=fields.find(field=>field.attributes['aria-label']==='Add as');
    const input=fields.find(field=>field.attributes['aria-label']==='Track name');
    if(mode){
      dialogs.push(['choice',{options:[{value:'separate'},{value:'merge'}]}]);
      const value=await choose.promise;if(value==null)return null;mode.value=value;await mode.fire('change');
    }
    if(!mode||mode.value==='merge'){
      const spec={value:input.value};dialogs.push(['name',spec]);nameShown.resolve(spec);
      const value=await ask.promise;if(value==null)return null;input.value=value;
    }
    return options.read();
  };
  loadModule('dialog',context);loadModule('api',context);loadModule('cut',context);
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
  assert.equal(styles.get('track-b-line-edge').layout.visibility,'visible');
  assert.ok(styles.get('track-b-line-edge').paint['line-opacity']>=0.6);
  assert.ok(styles.get('track-b-line-edge').paint['line-width']>styles.get('track-b-line').paint['line-width']);
  assert.deepEqual(moves.map(([id])=>id),['track-a-line-edge','track-a-line','track-a-route-edge','track-a-route','track-a-points','track-a-direction','track-a-gap','track-a-gap-direction','track-a-waypoint','track-a-photo']);
  layers.setVisible('b',false);layers.setFocus('b');
  for(const kind of ['line-edge','line','route-edge','route','waypoint','photo'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'none');
  layers.setVisible('b',true);layers.setFocus('a');
  for(const kind of ['line-edge','line','route-edge','route','waypoint'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'visible');
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
  const entry={id:'track-a',visible:true,track:{name:'A',points:[[0,0],[0,1]],parts:[{kind:'track',key:'t0',name:'Walk'}]},hiddenParts:new Set()};
  const state={tracks:new Map([['synthetic.gpx',entry]]),focus:'synthetic.gpx',fill:{active:false},cleanOpen:true};
  let cutting=true,features=[{layer:{metadata:{track:'track-a'}},properties:{part:'t0'}}];
  const context=vm.createContext({state,pickHidden:entry=>entry.hiddenParts,showCharts(){},map:{on:(name,fn)=>listeners.set(name,fn),queryRenderedFeatures:()=>features},layers:{nearest:()=>1,lineLayers:()=>['track-a-line']},focusedEntry:()=>entry,editPart:()=>entry.track.parts[0],setTool:tool=>calls.push(['tool',tool]),cutting:()=>cutting,waypointAt:()=>null,inspect:index=>calls.push(['inspect',index]),profile:{setIndex:index=>calls.push(['profile',index])},timeline:{setIndex:index=>calls.push(['timeline',index])},cutPanel:{addCut:index=>calls.push(['cut',index])},setMode:mode=>calls.push(["mode",mode]),select:path=>calls.push(['select',path]),showPart:(path,part)=>{state.partSelection={path,key:part.key};calls.push(['select',path]);calls.push(['mode','edit']);},renderTracks(){},applyCutPanel(){},applyCleanPanel(){},openMenu:(event,sections)=>menus.push(sections)});
  const app=source('app');vm.runInContext(app.slice(app.indexOf('function bindMap()'),app.indexOf('// ---- layout ----')),context);context.bindMap();
  const event={lngLat:{lng:0,lat:1},point:{x:10,y:10},originalEvent:{target:new Element('div')},preventDefault(){}};
  listeners.get('click')(event);assert.deepEqual(calls,[['inspect',1],['profile',1],['timeline',1]]);
  listeners.get('contextmenu')(event);assert.equal(calls.some(([kind])=>kind==='cut'),false);
  menus[0][1][0].onSelect();assert.deepEqual(calls.at(-1),['cut',1]);
  cutting=false;listeners.get('click')(event);assert.deepEqual(calls.at(-2),['select','synthetic.gpx']);assert.deepEqual(calls.at(-1),['mode','edit']);
  assert.deepEqual(plain(state.partSelection),{path:'synthetic.gpx',key:'t0'},'a click on a line selects that track, so Edit stays open on it');
  assert.equal(entry.expanded,true,'the clicked track opens its GPX in the workspace');
  const other={id:'track-b',visible:true,track:{name:'B',points:[[0,0]],parts:[{kind:'track',key:'t0',name:'Ride'}]},hiddenParts:new Set()};
  state.tracks.set('other.gpx',other);
  features=[...features,{layer:{metadata:{track:'track-b'}},properties:{part:'t0'}}];
  menus.length=0;listeners.get('click')(event);
  assert.deepEqual(plain(menus[0][0].map(item=>item.label)),['Walk — A','Ride — B'],'overlapping tracks are offered as a list');
  menus[0][0][1].onSelect();
  assert.equal(other.expanded,true);assert.deepEqual(plain(state.partSelection),{path:'other.gpx',key:'t0'});
  other.visible=false;menus.length=0;calls.length=0;listeners.get('click')(event);
  assert.equal(menus.length,0,'a hidden GPX is not offered');
  menus.length=0;listeners.get('contextmenu')(event);
  menus[0].flat().find(item=>item.label==='Cut here').onSelect();
  assert.deepEqual(calls.slice(-2),[['tool','cut'],['cut',1]],'Cut here outside Segments opens it, then cuts');
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


test('traces keep an opaque inverse-colour outline and distinguish every selection state',()=>{
  const layers=new Map(),context=vm.createContext({});loadModule('trace',context);
  const widths=vm.runInContext('TRACE_WIDTHS',context);
  const map={addLayer:layer=>layers.set(layer.id,layer)};
  for(const state of ['inactive','focused','segment','selected'])context.addTrace(map,{id:state,source:'test',color:'#0072b2',state,dashed:true});
  for(const state of ['inactive','focused','segment','selected']){
    const ids=context.traceLayers?context.traceLayers(state):vm.runInContext(`traceLayers('${state}')`,context);
    const [edge,core]=[...ids].map(id=>layers.get(id).paint);
    assert.equal(edge['line-color'],'#ff8d4d');assert.equal(core['line-color'],'#0072b2');
    assert.ok(edge['line-width']>core['line-width']);
    assert.ok(edge['line-width']<=6);assert.ok(core['line-width']>=3.6);
    assert.ok(core['line-width']/edge['line-width']>=0.6);
    for(const paint of [edge,core]){
      assert.equal(paint['line-opacity'],1);
      assert.ok(Math.abs(paint['line-width']*paint['line-dasharray'][0]-7)<1e-9);
      assert.ok(Math.abs(paint['line-width']*paint['line-dasharray'][1]-4)<1e-9);
    }
  }
  assert.ok(widths.selected[1]>widths.segment[1]);assert.ok(widths.focused[1]>widths.inactive[1]);
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
  const context=vm.createContext({state:{},pickHidden:entry=>entry.hiddenParts,map:{on:(name,fn)=>listeners.set(name,fn)},waypointAt:()=>found,focusedEntry:()=>({track:{}}),cutting:()=>cut,layers:{nearest:()=>2},inspect(){},openMenu:(event,sections)=>menus.push(sections.flat()),addWaypointAt:event=>calls.push(plain(event)),cutPanel:{addCut(){}},renamePart(){},startWaypointMove(){},deletePart(){}});
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
  assert.equal(strokes.length,10);
  for(const id of strokes){
    assert.ok(filters.has(id));assert.deepEqual(plain(filters.get(id).at(-1).at(-1).at(-1)),['literal',['t0','r0']]);
    if(id==='track-a-photo')assert.deepEqual(plain(filters.get(id)[2]),['boolean',['get','photos'],false]);
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
  assert.ok(row.classList.contains('ui-file-row'));
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
  assert.deepEqual(tools.children.map(e=>e.className.split(' ')[0]),['ours-mark','row-action','eye']);
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
 assert.ok(h.context.partRow('current.gpx',h.entry,part).classList.contains('ui-file-row'));
 assert.equal(h.calls.some(([kind])=>kind==='fit'||kind==='pan'),false);
 const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
 assert.match(css,/\.list li\.part-row\.selected\s*\{[^}]*background: var\(--selected\)/);
});

test('triangle grips isolate resize gestures and fold or restore the same panel',async()=>{
 const nodes=new Map(),calls=[];
 const context=vm.createContext({TRIANGLE_RIGHT:'triangle',state:{hidden:{}},$:id=>{if(!nodes.has(id))nodes.set(id,new Element('button'));return nodes.get(id);},hidePanel:(...args)=>calls.push(args),applyPanels(){},mapOnly:()=>false,setMapOnly(){}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('function bindPanels('),app.indexOf('// ViewControl')),context);context.bindPanels(null);
 for(const [panel,name,a,b] of [['sidebar','workspace','left','right'],['inspector','inspector','right','left'],['profile','profile','down','up']]){
  assert.equal(nodes.get('hide-'+name).innerHTML,'triangle');assert.equal(nodes.get('show-'+name).innerHTML,'triangle');
  for(const type of ['pointerdown','dblclick','keydown']){const event=await nodes.get('hide-'+name).fire(type);assert.equal(event.stopped,true);}
  await nodes.get('hide-'+name).fire('click');await nodes.get('show-'+name).fire('click');
  assert.deepEqual(calls.slice(-2),[[panel,true],[panel,false]]);
 }
});

test('GPX static buttons use shared action or setting controls and ordinary fields have no page skin',()=>{
 const html=readFileSync(new URL('./web/index.html',import.meta.url),'utf8');
 for(const match of html.matchAll(/<button\b([^>]*)>/g)){
  const classes=match[1].match(/class="([^"]*)"/)?.[1].split(/\s+/)||[];
  assert.ok(classes.some(name=>['button','icon-button','text-button','chip','action','link-button'].includes(name)),match[0]);
 }
 const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
 for(const match of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)){
  if(/\.basemap select|\.track-tools \.filter|\.clean-field input|\.stop-params input|\.route-name|\.range-join/.test(match[1])) assert.ok(!/(?:height|padding|border|background|font-size)\s*:/.test(match[2]),match[1]);
 }
});

test('Cut action buttons retain the standard secondary style through the actual button wrapper',()=>{
 const h=segmentHarness(pieces);
 for(const text of ['Cut at every stop','Clear cuts']){
  const button=h.root.all(e=>e.tagName==='BUTTON'&&e.textContent===text)[0];
  assert.ok(button,text);assert.ok(button.classList.contains('secondary'),text);assert.equal(button.classList.contains('strong-outline'),false,text);
 }
});

function dragHarness(){
 class Target extends Element {
  removeEventListener(type,listener){this.listeners.set(type,(this.listeners.get(type)||[]).filter(fn=>fn!==listener));}
  appendChild(child){this.append(child);}
  cloneNode(){return new Target('li');}
  getBoundingClientRect(){return {left:0,top:0,width:100,height:28};}
  setPointerCapture(id){this.captured=id;}
  releasePointerCapture(){this.captured=null;}
 }
 const document=new Target('document');document.body=new Target('body');document.createElement=tag=>new Target(tag);
 const window=new Target('window'),list=new Target('ul'),row=new Target('li');list.append(row);
 const timers=new Map();let next=0;const calls=[];
 const context=vm.createContext({document,window,setTimeout:fn=>{timers.set(++next,fn);return next;},clearTimeout:id=>timers.delete(id)});
 loadModule('drag',context);
 const dispose=context.longPressDrag(row,{onStart:()=>calls.push('start'),onMove:()=>calls.push('move'),onDrop:()=>calls.push('drop'),onCancel:()=>calls.push('cancel')});
 const down=()=>row.fire('pointerdown',{button:0,pointerType:'mouse',pointerId:1});
 return {document,window,list,row,calls,dispose,down};
}

test('row drag can finish outside its source and repeated gestures leave no ghost or listeners',async()=>{
 const h=dragHarness();
 for(let i=0;i<20;i++){
  await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30});await h.document.fire('pointerup',{pointerId:1});
  assert.equal(h.list.children.length,1);
  for(const type of ['pointermove','pointerup','pointercancel','keydown'])assert.equal((h.document.listeners.get(type)||[]).length,0,type);
 }
 assert.equal(h.calls.filter(value=>value==='drop').length,20);
});

test('lost capture, blur and list replacement cancel row drags and allow the next drag',async()=>{
 for(const reason of ['lostpointercapture','blur','replacement']){
  const h=dragHarness();await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30});
  if(reason==='lostpointercapture')await h.row.fire(reason,{pointerId:1});
  else if(reason==='blur')await h.window.fire(reason);else h.dispose();
  assert.equal(h.list.children.length,1,reason);assert.equal(h.calls.at(-1),'cancel',reason);
  await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30});await h.document.fire('pointerup',{pointerId:2});
  assert.equal(h.calls.at(-1),'move');await h.document.fire('pointerup',{pointerId:1});assert.equal(h.calls.at(-1),'drop');
 }
});

test('a missed release and a detached source cancel instead of continuing a stale drag',async()=>{
 for(const reason of ['release','detached']){
  const h=dragHarness();await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30,buttons:1});
  if(reason==='detached')h.row.removed=true;
  await h.document.fire('pointermove',{pointerId:1,clientX:40,buttons:reason==='release'?0:1});
  assert.equal(h.calls.at(-1),'cancel');assert.equal(h.list.children.length,1);
  await h.document.fire('pointerup',{pointerId:1});assert.equal(h.calls.includes('drop'),false);
 }
});

test('inverse outlines follow feature colours and file colour edits',()=>{
 const context=vm.createContext({});loadModule('trace',context);
 assert.equal(context.inverseColor('#123456'),'#edcba9');
 assert.deepEqual(plain(context.inverseColor(['get','color'])),['rgb',...[0,1,2].map(i=>['-',255,['at',i,['to-rgba',['get','color']]]])]);
 const paints=[];context.setTraceColor({setPaintProperty:(...args)=>paints.push(args)},'track','#123456');
 assert.deepEqual(paints,[['track-edge','line-color','#edcba9'],['track','line-color','#123456']]);
});

test('mixed-file visibility omits All and uses shared row icons without selecting the file',async()=>{
 const h=treeHarness(treeTrack({parts:[{kind:'track',key:'t0'},{kind:'route',key:'r0'}]}));
 const app=source('app');vm.runInContext(app.slice(app.indexOf('function partsToolbar('),app.indexOf('function setPartHidden(')),h.context);
 h.context.setPartsHidden=(...args)=>h.calls.push(['parts',...args]);
 const row=h.context.partsToolbar('current.gpx',h.entry),button=row.all(e=>e.tagName==='BUTTON')[0];
 assert.ok(!row.all(e=>e.textContent==='All').length);
 h.entry.track.parts=[{kind:'track',key:'t0'},{kind:'track',key:'t1'}];assert.equal(h.context.partsToolbar('current.gpx',h.entry).children.length,0);
 assert.ok(row.classList.contains('ui-file-row'));assert.ok(button.classList.contains('icon-button'));assert.ok(button.classList.contains('ui-row-icon'));
 const event=await button.fire('click');assert.equal(event.stopped,true);assert.equal(h.calls.at(-1)[0],'parts');
});

test('hidden child tracks disappear from segments, cut points and existing selection without bridging gaps',()=>{
 const moves=[],styles=new Map(),sources=new Map(),context=vm.createContext({format,document:documentStub()});loadModule('cut',context);
 const map={addSource:id=>sources.set(id,{}),addLayer:layer=>styles.set(layer.id,layer),getLayer:id=>styles.has(id)||id==='cursor',moveLayer:(id,before)=>moves.push([id,before]),getSource:id=>({setData:data=>sources.set(id,data)})};context.map=map;
 const overlay=vm.runInContext('new CutOverlay(map,"cursor")',context);
 const piece={first:0,last:7},points=Array.from({length:8},(_,i)=>[i,0]);
 const track={pieces:[piece],points,removed:Array(8).fill(false),segments:Array(8).fill(0),cuts:[1,3,6]};
 overlay.show(track);overlay.highlight(track,piece);overlay.show(track,[[2,5]]);
 const order=moves.map(([id])=>id);
 assert.ok(order.lastIndexOf('cut-pieces-direction')>order.lastIndexOf('cut-segment'));
 assert.ok(moves.every(([,before])=>before==='cursor'));
 assert.ok(!order.includes('cursor'));
 assert.equal(styles.get('cut-pieces-direction').source,'cut-pieces');
 assert.equal(styles.get('cut-pieces-direction').layout['icon-size'],3.8/12);
 assert.deepEqual(plain(styles.get('cut-pieces-direction').paint['icon-color']),plain(context.directionColor(['get','color'])));
 const expected=[points.slice(0,2),points.slice(6)];
 assert.deepEqual(plain(sources.get('cut-pieces').features[0].geometry.coordinates),expected);
 assert.deepEqual(plain(sources.get('cut-segment').geometry.coordinates),expected);
 assert.deepEqual(plain(sources.get('cut-points').features.map(f=>f.geometry.coordinates)),[points[1],points[6]]);
 overlay.show(track,[[0,7]]);assert.equal(sources.get('cut-segment').geometry.coordinates.length,0);
 overlay.show(track);assert.deepEqual(plain(sources.get('cut-segment').geometry.coordinates),[points]);
});

test('single clicks on hidden files and child items preserve visibility and never open or fit them',async()=>{
 for(const visible of [false,true])for(const kind of ['track','route','waypoint']){
  const part={kind,key:'p0',name:'Synthetic part',points:[[0,0]],distance:0,bounds:[[0,0],[1,1]]};
  const h=treeHarness(treeTrack({ours:true,parts:[part]}));h.entry.visible=visible;h.entry.hiddenParts.add(part.key);
  await h.context.partRow('current.gpx',h.entry,part).fire('click',{timeStamp:100});
  assert.equal(h.entry.visible,visible);assert.ok(h.entry.hiddenParts.has(part.key));
  assert.equal(h.calls.some(([kind])=>['visible','hidden','popup','fit','pan'].includes(kind)),false);
  assert.deepEqual(plain(h.state.partSelection),{path:'current.gpx',key:part.key});
 }
 const h=treeHarness(treeTrack({ours:true}));h.entry.visible=false;
 await h.context.trackRow('current.gpx',h.entry).fire('click',{timeStamp:100});
 assert.equal(h.entry.visible,false);assert.ok(h.calls.some(([kind])=>kind==='select'));
 assert.equal(h.calls.some(([kind])=>['visible','fit','focus'].includes(kind)),false);
});


test('complete unchanged track supplies the segment display, rename dialog and export default without saving an override', async()=>{
  const piece={first:0,last:2,points:3,distance:20,start:Date.UTC(2026,0,1,9),end:Date.UTC(2026,0,1,10)};
  const h=segmentHarness([piece]);
  h.panel.track.parts=[{kind:'track',first:0,last:2,name:'Synthetic walk'}];h.panel.render();
  const row=h.root.all(e=>e.classList.contains('cut-row'))[0];
  assert.equal(row.all(e=>e.classList.contains('cut-name'))[0].textContent,'Synthetic walk');
  assert.deepEqual(plain(h.panel.names()),[]);
  const menus=[];h.context.openMenu=(event,sections)=>menus.push(sections.flat());
  await row.fire('contextmenu');
  const rename=menus[0].find(item=>item.label==='Rename…').onSelect();
  assert.equal(h.dialogs.at(-1)[1].value,'Synthetic walk');h.ask.resolve(null);await rename;
  h.panel.track.parts[0].name='Renamed walk';h.panel.render();
  const adding=h.add().fire('click');await new Promise(resolve=>setImmediate(resolve));
  assert.equal(h.dialogs.at(-1)[1].value,'Renamed walk');
  h.ask.resolve(null);await adding;
});

test('track name fallback rejects cuts, changed geometry, removals, fills and multiple tracks; explicit names win',()=>{
  const piece={first:0,last:2,points:3,distance:20,start:Date.UTC(2026,0,1,9),end:Date.UTC(2026,0,1,10)};
  const h=segmentHarness([piece]);
  const base={parts:[{kind:'track',first:0,last:2,name:'Synthetic walk'}],cuts:[],removed:[],points:[[0,0],[1,1],[2,2]],original:null,fills:[]};
  for(const change of [{cuts:[1]},{removed:['','manual','']},{original:[[0,0],[1,2],[2,2]]},{fills:[{first:0,last:2}]},{parts:[{kind:'track',first:0,last:1,name:'A'},{kind:'track',first:2,last:2,name:'B'}]},{parts:[{kind:'track',first:0,last:2,name:' '}]}]){
    Object.assign(h.panel.track,base,change);h.panel.render();
    assert.equal(h.root.all(e=>e.classList.contains('cut-name'))[0].textContent,'09:00–10:00');
  }
  Object.assign(h.panel.track,base);piece.name='Custom segment';h.panel.render();
  assert.equal(h.root.all(e=>e.classList.contains('cut-name'))[0].textContent,'Custom segment');
  assert.deepEqual(plain(h.panel.names()),[{start:0,name:'Custom segment'}]);
  piece.name='';piece.start=null;piece.end=null;h.panel.track.parts=[];h.panel.render();
  assert.equal(h.root.all(e=>e.classList.contains('cut-name'))[0].textContent,'Segment 1');
});


test('child track selection refreshes segments even when its GPX is already focused and preserves visibility',()=>{
  const part={key:'t1',kind:'track',first:3,last:5,name:'Walk'};
  const h=treeHarness(treeTrack({parts:[part]}));h.state.focus='current.gpx';h.entry.hiddenParts.add('t1');
  h.context.showPart('current.gpx',part);
  assert.ok(h.calls.some(([action])=>action==='cut-panel'));
  assert.equal(h.entry.hiddenParts.has('t1'),true);
  assert.equal(h.calls.some(([action])=>['fit','pan','visible','hidden'].includes(action)),false);
});

test('selected track scopes segment rows and writes while preserving other track names and cuts',()=>{
  const all=[{first:0,last:1,name:'Ride A',points:2,distance:100},{first:1,last:2,name:'Ride B',points:2,distance:100},{first:3,last:4,name:'Walk A',points:2,distance:10},{first:4,last:5,name:'Walk B',points:2,distance:10}];
  const h=segmentHarness(all);h.track.cuts=[1,4];h.track.cutCandidates=[1,4];
  h.track.parts=[{key:'t0',kind:'track',first:0,last:2,name:'Ride'},{key:'t1',kind:'track',first:3,last:5,name:'Walk'}];
  const changes=[];h.panel.onCuts=(cuts,names)=>changes.push(plain({cuts,names}));
  const show=part=>h.panel.show(h.track,{part,timeZone:'UTC',workspace:[h.track.path],folder:''});
  show(h.track.parts[1]);
  h.panel.moveCut(4,1);assert.equal(changes.length,0);
  assert.deepEqual(plain(h.panel.track.pieces.map(p=>p.distance)),[10,10]);
  assert.equal(h.root.all(e=>e.classList.contains('cut-row')).length,2);
  h.panel.rename(all[2],'New walk');
  assert.deepEqual(changes.at(-1).cuts,[1,4]);assert.ok(changes.at(-1).names.some(n=>n.name==='Ride A'));
  h.root.all(e=>e.tagName==='BUTTON'&&e.textContent==='Clear cuts')[0].fire('click');
  assert.deepEqual(changes.at(-1).cuts,[1]);assert.ok(changes.at(-1).names.some(n=>n.name==='Ride B'));
  h.panel.selected=new Set([4]);show(h.track.parts[0]);assert.deepEqual([...h.panel.selected],[0,1]);
  show(h.track.parts[1]);assert.deepEqual([...h.panel.selected],[4]);
  show(null);assert.equal(h.panel.track.pieces.length,4);
});

test('application passes the selected track to the real segment panel and its map overlay',()=>{
  const h=segmentHarness([{first:0,last:2,points:3,distance:100},{first:3,last:5,points:3,distance:10}]);
  h.track.parts=[{key:'t0',kind:'track',first:0,last:2,name:'Ride'},{key:'t1',kind:'track',first:3,last:5,name:'Walk'}];
  h.state.focus=h.track.path;h.state.partSelection={path:h.track.path,key:'t1'};
  const overlays=[],timelines=[];Object.assign(h.context,{focusedEntry:()=>({track:h.track}),cutting:()=>true,cutPanel:h.panel,cutOverlay:{show:track=>overlays.push(track)},hiddenRanges:()=>[],timeline:{setCutting:value=>timelines.push(value)},$:()=>({value:'UTC'})});h.state.config={};
  const app=source('app');vm.runInContext(app.slice(app.indexOf('function applyCutPanel()'),app.indexOf('async function writeSegments(')),h.context);
  h.context.applyCutPanel();assert.deepEqual(plain(h.panel.track.pieces.map(p=>p.distance)),[10]);assert.equal(overlays.at(-1).pieces.length,1);
  h.state.partSelection=null;h.context.applyCutPanel();assert.equal(h.panel.track.pieces.length,2);
});

function twoTrackSegments(){
 const all=[{first:0,last:4,points:5,distance:200,name:'Ride'},{first:5,last:9,points:5,distance:20,name:'Walk'}];
 const h=segmentHarness(all);
 h.track.parts=[{key:'t0',kind:'track',first:0,last:4,name:'Ride'},{key:'t1',kind:'track',first:5,last:9,name:'Walk'}];
 h.track.cuts=[];h.track.cutCandidates=[2,7];h.track.removed=Array(10).fill('');
 h.showPart=part=>h.panel.show(h.track,{part,timeZone:'UTC',workspace:[h.track.path],folder:''});
 return h;
}

test('adding a selected child captures only its original point range even if another child is selected during naming',async()=>{
 const h=twoTrackSegments();h.showPart(h.track.parts[1]);
 const pending=h.add().fire('click');assert.equal(h.dialogs[0][1].value,'Walk');assert.equal(h.requests.length,0);
 h.showPart(h.track.parts[0]);h.ask.resolve('Named walk');await pending;
 assert.deepEqual(h.requests[0].body,{path:'current.gpx',segments:[{first:5,last:9,name:'Named walk'}],target:{mode:'add',path:'current.gpx'}});
 assert.equal(h.track.pieces[1].name,'Walk');
});

test('cut at every stop touches only the selected child and retains sibling names and existing cuts',async()=>{
 const h=twoTrackSegments(),changes=[];h.track.cuts=[2];h.panel.onCuts=(cuts,names)=>changes.push(plain({cuts,names}));
 h.showPart(h.track.parts[1]);
 await h.root.all(e=>e.tagName==='BUTTON'&&e.textContent==='Cut at every stop')[0].fire('click');
 assert.deepEqual(changes,[{cuts:[2,7],names:[{start:0,name:'Ride'},{start:5,name:'Walk'}]}]);
});

test('timeline cut additions reject sibling points, track endpoints and removed-only tracks',()=>{
 const h=twoTrackSegments(),changes=[];h.panel.onCuts=(cuts,names)=>changes.push(plain({cuts,names}));h.showPart(h.track.parts[1]);
 for(const index of [0,2,4,5,9])h.panel.addCut(index);
 assert.equal(changes.length,0);h.panel.addCut(7);assert.deepEqual(changes[0].cuts,[7]);
 h.track.removed=Array(10).fill('manual');h.showPart(h.track.parts[1]);h.panel.addCut(7);assert.equal(changes.length,1);
});

test('renaming a GPX retains independent child choices and destinations across panel hide and restore',()=>{
 const h=twoTrackSegments();h.showPart(h.track.parts[0]);h.panel.selected.clear();
 h.showPart(h.track.parts[1]);h.panel.addPath='current.gpx';h.panel.renamePath('current.gpx','renamed.gpx');h.track.path='renamed.gpx';
 h.panel.hide();h.showPart(h.track.parts[0]);assert.equal(h.panel.selected.size,0);
 h.showPart(h.track.parts[1]);assert.deepEqual([...h.panel.selected],[5]);assert.equal(h.panel.addPath,'renamed.gpx');
 assert.equal([...h.panel.choices.keys()].some(key=>key.startsWith('current.gpx\0')),false);
});

test('cuts and geometry edits in a sibling track do not replace an unchanged child track name',()=>{
 const h=twoTrackSegments();for(const piece of h.track.pieces)piece.name='';
 h.track.cuts=[2];h.track.removed[2]='manual';h.track.fills=[{first:0,last:4}];h.showPart(h.track.parts[1]);
 const row=h.root.all(e=>e.classList.contains('cut-row'))[0];assert.equal(row.all(e=>e.classList.contains('cut-name'))[0].textContent,'Walk');
 assert.deepEqual(plain(h.panel.names()),[]);
});

test('scoped cut overlay excludes sibling paths and cut markers and stays hidden when its child is hidden',()=>{
 const h=twoTrackSegments();h.track.cuts=[2,7];h.track.points=Array.from({length:10},(_,i)=>[i,0]);h.track.segments=[0,0,0,0,0,1,1,1,1,1];h.showPart(h.track.parts[1]);
 const sources=new Map(),map={addSource:id=>sources.set(id,{}),addLayer(){},getLayer:()=>false,getSource:id=>({setData:data=>sources.set(id,data)})};
 h.context.map=map;const overlay=vm.runInContext('new CutOverlay(map,"cursor")',h.context);
 overlay.show(h.panel.track);overlay.highlight(h.panel.track,h.panel.track.pieces[0]);
 assert.deepEqual(plain(sources.get('cut-points').features.map(f=>f.geometry.coordinates)),[[7,0]]);
 assert.deepEqual(plain(sources.get('cut-pieces').features[0].geometry.coordinates),[h.track.points.slice(5)]);
 overlay.show(h.panel.track,[[5,9]]);
 assert.equal(sources.get('cut-points').features.length,0);assert.equal(sources.get('cut-segment').geometry.coordinates.length,0);
 overlay.show(h.panel.track);assert.deepEqual(plain(sources.get('cut-segment').geometry.coordinates),[h.track.points.slice(5)]);
});

test('Escape and pointer cancellation remove drag ghosts, release capture and permit a fresh drag',async()=>{
 for(const type of ['keydown','pointercancel']){
  const h=dragHarness();await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30,buttons:1});
  await h.document.fire(type,{pointerId:1,key:'Escape'});
  assert.equal(h.calls.at(-1),'cancel');assert.equal(h.list.children.length,1);assert.equal(h.row.captured,null);
  for(const listener of ['pointermove','pointerup','pointercancel','keydown'])assert.equal((h.document.listeners.get(listener)||[]).length,0);
  await h.down();await h.document.fire('pointermove',{pointerId:1,clientX:30,buttons:1});await h.document.fire('pointerup',{pointerId:1});
  assert.equal(h.calls.at(-1),'drop');assert.equal(h.list.children.length,1);
 }
});

test('segment checkboxes immediately update all map paths and preserve shared checked boundaries',async()=>{
 const h=segmentHarness([{first:0,last:2,points:3,distance:20},{first:2,last:4,points:3,distance:20}]);
 Object.assign(h.track,{points:[[0,0],[1,0],[2,0],[3,0],[4,0]],segments:[0,0,0,0,0],removed:['','','','',''],parts:[{key:'t0',kind:'track',first:0,last:4,name:'Trip'}]});
 const data=new Map(),map={addSource:(id,options)=>data.set(id,options.data),addLayer(){},getSource:id=>({setData:value=>data.set(id,value)})};
 const layers=new TrackLayers(map);layers.add('a',h.track,'#0072b2');
 const overlayData=[];Object.assign(h.context,{cutting:()=>true,focusedEntry:()=>h.state.tracks.get(h.state.focus),cutPanel:h.panel,layers,hiddenRanges:()=>[],cutOverlay:{show:(track,hidden)=>overlayData.push(plain(hidden))},cleanOverlay:{show(){},showFills(){}},stopMarkers:{show(){}}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('let segmentVisibilityEntry'),app.indexOf('function applyCutPanel()')),h.context);
 h.panel.onSelection=h.context.applySegmentVisibility;h.panel.show(h.track,{timeZone:'UTC',workspace:[h.track.path],folder:''});
 const second=h.root.all(e=>e.classList.contains('cut-check')).at(-1);second.checked=false;await second.fire('change');
 const line=()=>plain(data.get('track-a').features[0].geometry.coordinates);
 assert.deepEqual(line(),[h.track.points.slice(0,3)]);assert.deepEqual(overlayData.at(-1),[[3,4]]);
 const all=h.root.all(e=>e.id==='cut-all')[0];all.checked=false;await all.fire('change');assert.deepEqual(line(),[]);
 const again=h.root.all(e=>e.id==='cut-all')[0];again.checked=true;await again.fire('change');assert.deepEqual(line(),[h.track.points]);
 const off=h.root.all(e=>e.classList.contains('cut-check')).at(-1);off.checked=false;await off.fire('change');
 h.context.cutting=()=>false;h.panel.hide();assert.deepEqual(line(),[h.track.points]);
});

test('the lasso keeps unchecked segments hidden after leaving the Segments tab',async()=>{
 const h=segmentHarness([{first:0,last:2,points:3,distance:20},{first:2,last:4,points:3,distance:20}]);
 Object.assign(h.track,{points:[[0,0],[1,0],[2,0],[3,0],[4,0]],segments:[0,0,0,0,0],removed:['','','','',''],parts:[{key:'t0',kind:'track',first:0,last:4,name:'Trip'}]});
 const data=new Map(),map={addSource:(id,options)=>data.set(id,options.data),addLayer(){},getSource:id=>({setData:value=>data.set(id,value)})};
 const layers=new TrackLayers(map);layers.add('a',h.track,'#0072b2');
 Object.assign(h.context,{cutting:()=>true,focusedEntry:()=>h.state.tracks.get(h.state.focus),cutPanel:h.panel,layers,hiddenRanges:()=>[],cutOverlay:{show(){}},cleanOverlay:{show(){},showFills(){}},stopMarkers:{show(){}}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('let segmentVisibilityEntry'),app.indexOf('function applyCutPanel()')),h.context);
 h.panel.onSelection=h.context.applySegmentVisibility;h.panel.show(h.track,{timeZone:'UTC',workspace:[h.track.path],folder:''});
 const second=h.root.all(e=>e.classList.contains('cut-check')).at(-1);second.checked=false;await second.fire('change');
 const line=()=>plain(data.get('track-a').features[0].geometry.coordinates);
 // Picking the lasso leaves the Segments tab: the unchecked segment stays hidden.
 h.state.lasso=true;h.context.cutting=()=>false;h.panel.hide();
 assert.deepEqual(line(),[h.track.points.slice(0,3)]);
 assert.deepEqual(plain(h.context.uncheckedRanges()),[[3,4]]);
 // Turning the lasso off shows the whole track again.
 h.state.lasso=false;h.context.applySegmentVisibility();
 assert.deepEqual(line(),[h.track.points]);
});

test('hidden ranges break base and fill paths instead of bridging an unchecked segment',()=>{
 const data=new Map(),map={addSource:(id,options)=>data.set(id,options.data),addLayer(){},getSource:id=>({setData:value=>data.set(id,value)})};
 const track={points:Array.from({length:7},(_,i)=>[i,0]),segments:Array(7).fill(0),removed:Array(7).fill(''),parts:[{key:'t0',kind:'track',first:0,last:6,name:'Trip'}],fills:[{first:0,last:6,points:5}]};
 const layers=new TrackLayers(map);layers.add('a',track,'#0072b2');layers.update('a',track,[[2,4]]);
 assert.deepEqual(plain(data.get('track-a').features[0].geometry.coordinates),[track.points.slice(0,2),track.points.slice(5)]);
 const context=vm.createContext({document:documentStub(),format});loadModule('clean',context);
 const overlay=Object.create(vm.runInContext('CleanOverlay.prototype',context));overlay.map=map;overlay.showFills(track,[[2,4]]);
 assert.deepEqual(plain([...data.values()].at(-1).geometry.coordinates),[track.points.slice(0,2),track.points.slice(5)]);
});

test('one Add modal selects Save to and validates naming and separate or merged tracks',async()=>{
 for(const count of [1,2]){
  const document=documentStub(),context=vm.createContext({document});loadShared('dialog',context);loadModule('dialog',context);
  const pending=context.segmentSaveDialog({targets:['current.gpx','other.gpx'],target:'current.gpx',segments:Array.from({length:count},()=>({name:'Walk'}))});
  const dialog=document.body.children.at(-1),form=dialog.all(e=>e.tagName==='FORM')[0];
  const target=dialog.all(e=>e.attributes['aria-label']==='Save to')[0],name=dialog.all(e=>e.attributes['aria-label']==='Track name')[0];
  assert.equal(target.value,'current.gpx');target.value='other.gpx';
  if(count===2){const mode=dialog.all(e=>e.attributes['aria-label']==='Add as')[0];assert.equal(name.parentElement.hidden,true);mode.value='merge';await mode.fire('change');assert.equal(name.parentElement.hidden,false);}
  name.value=' ';const rejected=await form.fire('submit');assert.equal(rejected.defaultPrevented,true);assert.equal(dialog.open,true);
  name.value='  Named walk  ';await form.fire('submit');dialog.close();assert.deepEqual(plain(await pending),{path:'other.gpx',merge:count===2,name:'Named walk'});
 }
});

test('separate Add keeps individual names; cancelled target selection writes nothing and target control is absent from the panel',async()=>{
 const h=segmentHarness(pieces);assert.equal(h.root.all(e=>e.tagName==='SELECT').length,0);
 h.context.segmentSaveDialog=async()=>null;await h.add().fire('click');assert.equal(h.requests.length,0);assert.equal(h.add().disabled,false);
 h.context.segmentSaveDialog=async()=>({path:'another.gpx',merge:false,name:''});await h.add().fire('click');
 assert.equal(h.requests[0].body.target.path,'another.gpx');assert.deepEqual(h.requests[0].body.segments.map(p=>p.name),['First','Second']);
 assert.equal(h.panel.addPath,'another.gpx');
});


test('toggling Compare preserves unchecked segment visibility',()=>{
 const h=twoTrackSegments();h.showPart(h.track.parts[1]);h.panel.selected.clear();
 const shown=[];Object.assign(h.context,{cutPanel:h.panel,focusedEntry:()=>h.state.tracks.get(h.state.focus),cutting:()=>true,hiddenRanges:()=>[],cleanOverlay:{show:(track,hidden)=>shown.push([track,plain(hidden)])},applyEditTools(){},save(){}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('function setCompare('),app.indexOf('// applyEditTools')),h.context);
 vm.runInContext(app.slice(app.indexOf('// uncheckedRanges'),app.indexOf('function applySegmentVisibility')),h.context);
 h.context.setCompare(false);assert.equal(shown[0][0],null);h.context.setCompare(true);assert.deepEqual(shown.at(-1)[1],[[5,9]]);
});

test('number step buttons call native stepping and notify existing change handlers',async()=>{
 const context=vm.createContext({document:documentStub(),Event});loadShared('controls',context);
 const input=new Element('input'),calls=[];
 input.step='any';input.stepUp=()=>calls.push(['up',input.step]);input.stepDown=()=>calls.push(['down',input.step]);
 input.dispatchEvent=event=>calls.push([event.type,event.bubbles]);
 const group=context.numberStepper(input),[down,up]=group.children.slice(1);
 assert.equal(group.children[0],input);assert.equal(up.type,'button');
 await up.fire('click');assert.deepEqual(calls,[['up','1'],['change',true]]);assert.equal(input.step,'any');
 calls.length=0;input.step='5';await down.fire('click');assert.deepEqual(calls,[['down','5'],['change',true]]);
 calls.length=0;input.disabled=true;await up.fire('click');assert.deepEqual(calls,[]);
 input.disabled=false;input.readOnly=true;await down.fire('click');assert.deepEqual(calls,[]);
 input.readOnly=false;input.step='any';input.stepUp=()=>{throw Error('native stepping failed');};
 await assert.rejects(up.fire('click'),/native stepping failed/);assert.equal(input.step,'any');
});

test('Alt, Cmd and Ctrl dragging copy tracks and waypoints without moving the source',async()=>{
 const app=source('app');
 for(const kind of ['track','waypoint'])for(const modifier of ['altKey','metaKey','ctrlKey']){
  const calls=[],part={key:'p0',kind,name:'Synthetic part'},row={dataset:{path:'target.gpx'}};
  const context=vm.createContext({dragRow:{path:'source.gpx',key:'p0',kind},dragGap:{at:()=>({row,above:true})},
   $:()=>({getBoundingClientRect:()=>({left:0,right:100})}),state:{focus:null,partPicks:new Set(),tracks:new Map([['source.gpx',{track:{name:'Source',parts:[part]}}],['target.gpx',{track:{name:'Target',ours:true,parts:[]}}]])},
   api:{copyPart:async(...args)=>calls.push(args)},reloadEntry:async path=>calls.push(['reload',path]),renderTracks(){},showMessage(){},basename:path=>path});
  vm.runInContext(app.slice(app.indexOf('const copyModifier'),app.indexOf('// afterFile')),context);
  const target=context.dropAt(50,50,{[modifier]:true});assert.equal(target.kind,'into');assert.equal(target.copy,true);
  assert.equal(context.dropAt(50,50,{}).kind,'refused');
  vm.runInContext(app.slice(app.indexOf('async function dropIntoFile('),app.indexOf('// renamePart')),context);
  await context.dropIntoFile(context.dragRow,target);
  assert.deepEqual(calls,[['source.gpx','p0','target.gpx',false],['reload','target.gpx']]);
  context.state.tracks.get('target.gpx').track.ours=false;
  assert.equal(context.dropAt(50,50,{[modifier]:true}).kind,'refused');
 }
});

test('picked parts drag together: copied in listed order, moved out last first, mixed kinds never reorder',async()=>{
 const app=source('app'),calls=[];
 const parts=[{key:'w0',kind:'waypoint',name:'A'},{key:'w1',kind:'waypoint',name:'B'},{key:'t0',kind:'track',name:'T'}];
 const listed=['w0','w1','t0'].map(key=>({dataset:{path:'source.gpx',key}}));
 const state={focus:null,partPicks:new Set(),tracks:new Map([['source.gpx',{track:{name:'Source',ours:true,parts}}],['target.gpx',{track:{name:'Target',ours:true,parts:[]}}]])};
 let row={dataset:{path:'target.gpx'}};
 const context=vm.createContext({state,gap:{at:()=>({row,above:true})},
  $:()=>({children:listed,getBoundingClientRect:()=>({left:0,right:100})}),
  api:{copyPart:async(...args)=>calls.push(['copy',...args]),deletePart:async(...args)=>calls.push(['delete',...args])},
  reloadEntry:async()=>{},renderTracks(){},showMessage(){},basename:path=>path});
 vm.runInContext(app.slice(app.indexOf('const partId'),app.indexOf('// afterFile')),context);
 vm.runInContext(app.slice(app.indexOf('async function dropIntoFile('),app.indexOf('// renamePart')),context);
 state.partPicks=new Set(['source.gpx\nw1','source.gpx\nw0']);
 const held={kind:'waypoint',path:'source.gpx',key:'w1'};held.group=context.heldGroup(held);
 assert.equal(held.group.length,2);
 context.held=held;vm.runInContext('dragRow=held;dragGap=gap',context);
 const target=context.dropAt(50,50,{});assert.equal(target.kind,'into');
 await context.dropIntoFile(held,target);
 assert.deepEqual(plain(calls),[['copy','source.gpx','w0','target.gpx',false],['copy','source.gpx','w1','target.gpx',false],['delete','source.gpx','w1'],['delete','source.gpx','w0']]);
 state.partPicks=new Set(['source.gpx\nw0','source.gpx\nt0']);
 const mixed={kind:'waypoint',path:'source.gpx',key:'w0'};mixed.group=context.heldGroup(mixed);context.held=mixed;vm.runInContext('dragRow=held',context);
 row={dataset:{path:'source.gpx',kind:'waypoint',key:'w1'}};
 assert.equal(context.dropAt(50,50,{}),null,'a track and a waypoint picked together do not reorder');
});

test('moving reordered added waypoints preserves unselected siblings and deletes numeric keys high first',async()=>{
 const app=source('app');
 for(const picked of [[1,0],[10,2]]){
  const remaining=Array.from({length:Math.max(...picked)+2},(_,i)=>`Waypoint ${i}`);
  const expected=remaining.filter((_,i)=>!picked.includes(i)),copied=[],deleted=[];
  const parts=remaining.map((name,i)=>({name,key:`w${i}`,kind:'waypoint'}));
  const state={focus:null,partPicks:new Set(picked.map(i=>`source.gpx\nw${i}`)),tracks:new Map([
   ['source.gpx',{track:{parts}}],['target.gpx',{track:{name:'Target',parts:[]}}],
  ])};
  const context=vm.createContext({state,partId:(path,key)=>`${path}\n${key}`,
   $:()=>({children:[...picked.map(i=>parts[i]),...parts.filter((_,i)=>!picked.includes(i))].map(part=>({dataset:{path:'source.gpx',key:part.key}}))}),
   api:{copyPart:async(path,key)=>copied.push(remaining[Number(key.slice(1))]),
    deletePart:async(path,key)=>{deleted.push(key);remaining.splice(Number(key.slice(1)),1);}},
   reloadEntry:async()=>{},renderTracks(){},showMessage(message,error){assert.equal(error,undefined,message);},basename:path=>path,
  });
  vm.runInContext(app.slice(app.indexOf('async function dropIntoFile('),app.indexOf('// editPhotos')),context);
  await context.dropIntoFile({path:'source.gpx',group:picked.map(i=>({path:'source.gpx',key:`w${i}`}))},{path:'target.gpx',copy:false});
  assert.deepEqual(copied,picked.map(i=>`Waypoint ${i}`),'copy order follows the list');
  assert.deepEqual(deleted,[...picked].sort((a,b)=>b-a).map(i=>`w${i}`));
  assert.deepEqual(remaining,expected,'unselected waypoints survive');
 }
});

test('partial transfers refresh changed files, report progress and clear the old batch before retry',async()=>{
 const app=source('app');
 for(const stage of ['copy','delete'])for(const copy of stage==='copy'?[true,false]:[false]){
  const parts=[{key:'w0',kind:'waypoint',name:'A'},{key:'w1',kind:'waypoint',name:'B'}];
  const state={focus:null,partPicks:new Set(['source.gpx\nw0','source.gpx\nw1']),partAnchor:'source.gpx\nw0',partSelection:{path:'source.gpx',key:'w0'},tracks:new Map([
   ['source.gpx',{track:{parts,unsaved:false}}],['target.gpx',{track:{name:'Target',parts:[],unsaved:false}}],
  ])};
  let copies=0,deletes=0;
  const reloaded=[],messages=[];
  const context=vm.createContext({state,
   $:()=>({children:parts.map(part=>({dataset:{path:'source.gpx',key:part.key}}))}),
   api:{copyPart:async()=>{if(stage==='copy'&&copies===1)throw Error('Copy refused');copies++;},
    deletePart:async()=>{if(deletes===1)throw Error('Delete refused');deletes++;}},
   reloadEntry:async path=>{reloaded.push(path);state.tracks.get(path).track.unsaved=path==='target.gpx'?copies>0:deletes>0;},
   renderTracks(){},showMessage:(...args)=>messages.push(args),basename:path=>path,
  });
  vm.runInContext(app.slice(app.indexOf('const partId'),app.indexOf('// ---- dragging rows')),context);
  vm.runInContext(app.slice(app.indexOf('async function dropIntoFile('),app.indexOf('// editPhotos')),context);
  const held={kind:'waypoint',path:'source.gpx',key:'w0'};held.group=context.heldGroup(held);
  await context.dropIntoFile(held,{path:'target.gpx',copy});
  assert.deepEqual(reloaded,copy?['target.gpx']:['target.gpx','source.gpx']);
  assert.equal(state.tracks.get('target.gpx').track.unsaved,true,'accepted copies appear as unsaved');
  assert.equal(state.tracks.get('source.gpx').track.unsaved,stage==='delete');
  assert.equal(state.partPicks.size,0);assert.equal(state.partAnchor,null);assert.equal(state.partSelection,null);
  assert.deepEqual(plain(context.heldGroup({kind:'waypoint',path:'source.gpx',key:'w1'})),[{kind:'waypoint',path:'source.gpx',key:'w1'}],'a new drag cannot replay the old batch');
  assert.equal(messages.at(-1)[1],true);
  assert.match(messages.at(-1)[0],new RegExp(`Copied ${stage==='copy'?1:2} of 2`));
  if(!copy)assert.match(messages.at(-1)[0],new RegExp(`removed ${deletes} of 2 from sources`));
  assert.match(messages.at(-1)[0],stage==='copy'?/Copy refused/:/Delete refused/);
 }
});

test('a waypoint with photos is marked on the map, carries a camera in its row and opens its photos on click',async()=>{
  // Map: the feature says it has photos, and the camera layer draws only those.
  const added=[],sources=new Map();
  const map={addSource(id,source){sources.set(id,source);},addLayer:layer=>added.push(layer)};
  const layers=new TrackLayers(map);
  layers.add('a',{parts:[
    {kind:'waypoint',key:'w0',name:'Summit',points:[[0,0]],photos:[{name:'aaaaaaaaaaaa.webp'}]},
    {kind:'waypoint',key:'w1',name:'Hut',points:[[1,1]]},
  ]},'#0072b2');
  const features=sources.get('track-a').data.features;
  assert.deepEqual(features.map(f=>[f.properties.part,f.properties.photos]),[['w0',true],['w1',false]]);
  const camera=added.find(layer=>layer.id==='track-a-photo');
  assert.equal(camera.type,'symbol');assert.equal(camera.layout['icon-image'],'waypoint-camera');
  assert.deepEqual(plain(camera.filter[2]),['boolean',['get','photos'],false]);

  // Row: a waypoint with photos shows the camera glyph instead of the pin.
  const photoPart={kind:'waypoint',key:'w0',name:'Summit',points:[[0,0]],bounds:[[0,0],[0,0]],photos:[{name:'aaaaaaaaaaaa.webp'}]};
  const plainPart={kind:'waypoint',key:'w1',name:'Hut',points:[[1,1]],bounds:[[1,1],[1,1]]};
  const h=treeHarness(treeTrack({ours:true,parts:[photoPart,plainPart]}));
  h.context.CAMERA='camera';
  assert.equal(h.context.partRow('current.gpx',h.entry,photoPart).children[0].innerHTML,'camera');
  assert.equal(h.context.partRow('current.gpx',h.entry,plainPart).children[0].innerHTML,'waypoint');

  // Click: one click on a waypoint on the map selects it and opens its popup.
  const listeners=new Map(),calls=[];
  const entry={id:'track-a',track:{points:[]},hiddenParts:new Set()};
  const context=vm.createContext({state:{tracks:new Map([['synthetic.gpx',entry]]),fill:{active:false}},map:{on:(name,fn)=>listeners.set(name,fn)},
    layers:{},focusedEntry:()=>null,cutting:()=>false,waypointAt:()=>({path:'synthetic.gpx',entry,part:photoPart}),
    showPart:(path,part)=>calls.push(['select',path,part.key]),openWaypoint:(path,e,part)=>calls.push(['popup',path,part.key])});
  const app=source('app');vm.runInContext(app.slice(app.indexOf('function bindMap()'),app.indexOf('// ---- layout ----')),context);context.bindMap();
  listeners.get('click')({point:{x:0,y:0},originalEvent:{target:new Element('div')}});
  assert.deepEqual(calls,[['select','synthetic.gpx','w0'],['popup','synthetic.gpx','w0']]);
});

test('the crosshair belongs to the bare map: map icons and the timeline use the arrow',()=>{
  const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
  const rule=selector=>css.match(new RegExp(`(^|\\n)${selector.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')}\\s*\\{([^}]*)\\}`))?.[2]||'';
  for(const selector of ['.timeline-bar','.stop-marker'])assert.match(rule(selector),/cursor:\s*default/,selector);
  assert.match(rule('.maplibregl-canvas-container.maplibregl-interactive canvas'),/cursor:\s*crosshair/);
  const app=source('app'),hover=app.slice(app.indexOf('map.getCanvas().style.cursor = found ?'));
  assert.match(hover,/^map\.getCanvas\(\)\.style\.cursor = found \? "default"/);
});

test('sharpening leaves flat areas and alpha alone and steepens an edge',()=>{
  const width=4,height=1,pixels=new Uint8ClampedArray([100,100,100,255, 100,100,100,200, 200,200,200,255, 200,200,200,255]);
  sharpen(pixels,width,height,0.4);
  assert.deepEqual([...pixels.filter((_,i)=>i%4===0)],[100,90,210,200]);
  assert.deepEqual([...pixels.filter((_,i)=>i%4===3)],[255,200,255,255]);
  const flat=new Uint8ClampedArray(4*9).fill(77);sharpen(flat,3,3);assert.ok(flat.every(v=>v===77));
});

test('map hover between two sparse track points answers the nearer end', () => {
  // A flat projection: one degree is one pixel.
  const map = {project: ([x, y]) => ({x, y}), unproject: ([x, y]) => ({lng: x, lat: y})};
  const layers = Object.create(TrackLayers.prototype);
  layers.map = map;
  const track = {points: [[0, 0], [200, 0], [400, 0], [600, 0]], removed: [], segments: [0, 0, 1, 1], parts: [{kind: 'track', key: 't0', first: 0, last: 3}]};
  assert.equal(layers.nearest(track, {x: 60, y: 5}), 0);
  assert.equal(layers.nearest(track, {x: 150, y: 5}), 1);
  assert.equal(layers.nearest(track, {x: 300, y: 5}), -1, 'a recording break is not a step');
  assert.equal(layers.nearest(track, {x: 100, y: 40}), -1);
  assert.equal(layers.nearest(track, {x: 150, y: 5}, 24, new Set(['t0'])), -1);
});

// gestureHarness loads ViewGestures against a map stub that records what the view does.
function gestureHarness(){
  const listeners=new Map(),outer=new Map(),windowListeners=new Map(),classes=new Set(),pans=[],flags={dragPan:true,doubleClickZoom:true,boxZoom:true};
  const handler=name=>({enable:()=>flags[name]=true,disable:()=>flags[name]=false});
  const container={classList:{toggle:(name,on)=>on?classes.add(name):classes.delete(name),add:name=>classes.add(name),remove:name=>classes.delete(name)},
    contains:target=>target!==null,setPointerCapture(){},addEventListener:(name,fn)=>listeners.set(name,fn),removeEventListener:name=>listeners.delete(name)};
  const outerElement={addEventListener:(name,fn,options)=>outer.set(name,[...(outer.get(name)||[]),[fn,options]]),
    removeEventListener:(name,fn)=>outer.set(name,(outer.get(name)||[]).filter(([other])=>other!==fn))};
  const map={getCanvasContainer:()=>container,getContainer:()=>outerElement,panBy:(offset,options)=>pans.push([offset,options]),
    dragPan:handler('dragPan'),doubleClickZoom:handler('doubleClickZoom'),boxZoom:handler('boxZoom')};
  const timers=[];
  const context=vm.createContext({setTimeout:fn=>timers.push(fn),window:{addEventListener:(name,fn)=>windowListeners.set(name,fn)}});
  vm.runInContext(source('gestures').replace('export class','class')+';this.ViewGestures=ViewGestures;',context);
  const gestures=new context.ViewGestures(map);
  const key=(code,down,target={})=>{const event={code,target,prevented:false,preventDefault(){this.prevented=true;}};windowListeners.get(down?'keydown':'keyup')(event);return event;};
  const press=(extra={})=>{const event={button:0,target:{},clientX:50,clientY:50,pointerId:1,prevented:false,stopped:false,
    preventDefault(){this.prevented=true;},stopImmediatePropagation(){this.stopped=true;},...extra};outer.get('pointerdown')[0][0](event);return event;};
  return {gestures,listeners,outer,classes,pans,flags,timers,key,press};
}

test('view gestures are the same under every tool: no double-click or box zoom, Space+drag always pans',()=>{
  const h=gestureHarness();
  assert.equal(h.flags.doubleClickZoom,false,'double-click belongs to the tools');
  assert.equal(h.flags.boxZoom,false,'Shift+drag belongs to the tools');
  // A tool that does not claim the drag leaves plain drags to pan the map.
  h.gestures.setToolDrag(false);assert.equal(h.flags.dragPan,true);
  // A tool that claims it takes plain drags; Space+drag still pans.
  h.gestures.setToolDrag(true);assert.equal(h.flags.dragPan,false);
  const plain=h.press();assert.equal(plain.prevented,false);assert.equal(plain.stopped,false);assert.equal(h.pans.length,0);
  // Space away from the map, or while typing, keeps its own meaning.
  assert.equal(h.key('Space',true).prevented,false);assert.equal(h.classes.has('view-pan-ready'),false);
  h.listeners.get('pointerenter')();
  assert.equal(h.key('Space',true,{closest:()=>({})}).prevented,false);
  // Over the map, Space shows the open hand and a drag moves the map with a closed one.
  assert.equal(h.key('Space',true,{closest:()=>null}).prevented,true);assert.ok(h.classes.has('view-pan-ready'));
  assert.equal(h.press({button:2}).stopped,false,'the right button still rotates and tilts');
  const pan=h.press();
  assert.equal(pan.prevented,true);assert.equal(pan.stopped,true,'no tool sees a Space press');assert.ok(h.classes.has('view-panning'));
  h.listeners.get('pointermove')({clientX:40,clientY:45});h.listeners.get('pointermove')({clientX:30,clientY:45});h.listeners.get('pointerup')();
  assert.equal(JSON.stringify(h.pans),JSON.stringify([[[10,5],{duration:0}],[[10,0],{duration:0}]]));
  assert.equal(h.classes.has('view-panning'),false);
  // The click that ends the pan is not a tool's click.
  const [swallow]=h.outer.get('click').at(-1);const click={stopped:false,stopImmediatePropagation(){this.stopped=true;}};swallow(click);assert.equal(click.stopped,true);
  h.key('Space',false);assert.equal(h.classes.has('view-pan-ready'),false);
  // With Space released, a press goes to the tool again.
  assert.equal(h.press().stopped,false);
});

test('lasso clicks build a polygon while drags remain map panning; finish and cancel clean up',()=>{
 const handlers=new Map(),attributes=new Map(),done=[];
 const node=()=>({classList:{add(){},toggle(){}},setAttribute:(key,value)=>attributes.set(key,value),removeAttribute:key=>attributes.delete(key),append(){}});
 const context=vm.createContext({document:{createElementNS:node},window:{addEventListener(){},removeEventListener(){}}});
 const clean=source('clean');vm.runInContext(clean.slice(clean.indexOf('export class Lasso'),clean.indexOf('// insidePolygon')).replace('export class','class')+';this.Lasso=Lasso;',context);
 let offset=0;
 const map={getCanvasContainer:()=>({append(){}}),project:([x,y])=>({x:x+offset,y}),on:(name,fn)=>handlers.set(name,fn),off:name=>handlers.delete(name)};
 const lasso=new context.Lasso(map,{onDone:(...args)=>done.push(plain(args)),onCancel:()=>lasso.setActive(false)});
 lasso.setActive(true);assert.equal(handlers.has('pointerdown'),false);
 for(const [lng,lat] of [[0,0],[10,0],[10,10]])handlers.get('click')({lngLat:{lng,lat},originalEvent:{altKey:true,detail:1}});
 handlers.get('click')({lngLat:{lng:10,lat:10},originalEvent:{detail:2}});
 offset=5;handlers.get('move')();assert.match(attributes.get('d'),/^M5,0/);
 handlers.get('dblclick')({preventDefault(){}});assert.deepEqual(done,[[[[5,0],[15,0],[15,10]],{restore:true}]]);
 assert.equal(attributes.has('d'),false);lasso.onKey({key:'Escape'});assert.equal(handlers.size,0);
 const app=source('app');assert.match(app,/gestures\?\.setToolDrag\(false\)/);
});

// editHarness loads Edit's track scope, the edit panel and saving against one
// file of two tracks.
function editHarness(){
  const app=source('app');
  const part=(key,first,last,stops)=>({key,kind:'track',name:key==='t0'?'Morning':'Evening',first,last,clean:{spikes:{enabled:false},drift:{enabled:false},smooth:{enabled:false},stops:{enabled:stops}}});
  const track={parts:[part('t0',0,4,false),part('t1',5,9,true),{key:'w0',kind:'waypoint',first:0,last:0}],
    points:Array.from({length:10},(_,i)=>[i,0]),removed:['manual','','','','','stop','stop','','manual',''],
    fills:[{first:6,last:8,points:1}],pieces:[{first:0,last:4},{first:5,last:9}],
    clean:{counts:{},params:{stops:{enabled:false},edits:[{kind:'lasso',points:[0],join:true},{kind:'range',first:8,last:8,join:false}]}}};
  const state={focus:'a.gpx',tracks:new Map([['a.gpx',{track}]]),partSelection:null,cleanOpen:true,routeOpen:false,hidden:{},rangeTool:false,fill:{}};
  const elements=new Map(),el=id=>{if(!elements.has(id))elements.set(id,{hidden:false,disabled:false,title:'',attrs:{},children:[],replaceChildren(...c){this.children=c;},setAttribute(k,v){this.attrs[k]=v;}});return elements.get(id);};
  const saved=[],shown=[],editing=[];
  const context=vm.createContext({state,$:el,structuredClone,document:{createElement:()=>({})},WeakMap,Math,
    focusedEntry:()=>state.tracks.get(state.focus),timeline:{setEditing:value=>editing.push(value)},
    cleanPanel:{show:view=>shown.push(view),hide(){},addRange(){}},map:null,planner:null,cutting:()=>false,
    setLasso(){},setRangeTool(){},setFillTool(){},applyEditTools(){},
    api:{saveClean:(path,first,clean)=>{saved.push({path,first,clean});return Promise.resolve();}},
    changeFocused:async change=>change(state.focus)});
  vm.runInContext(app.slice(app.indexOf('function applyCleanPanel()'),app.indexOf('// setMode switches')),context);
  vm.runInContext(app.slice(app.indexOf('// saveClean writes'),app.indexOf('// changeFocused runs')),context);
  return {context,state,track,el,saved,shown,editing};
}

test('Edit works on a selected track only; the file, a route or a waypoint leaves the panel empty',()=>{
  const h=editHarness();
  h.context.applyCleanPanel();
  assert.equal(h.state.cleanOpen,true,'selecting the file stays in Edit');
  assert.equal(h.el('clean-panel').hidden,false);assert.equal(h.el('clean-panel').children.length,1,'an empty panel with a hint');
  assert.equal(h.shown.length,0);assert.equal(h.el('mode-edit').disabled,false);
  h.state.partSelection={path:'a.gpx',key:'w0'};h.context.applyCleanPanel();
  assert.equal(h.shown.length,0,'a waypoint is not a track');
  h.state.cleanOpen=false;h.context.applyCleanPanel();
  assert.equal(h.el('mode-edit').disabled,true,'Edit cannot be entered without a track');
  h.state.partSelection={path:'a.gpx',key:'t1'};h.context.applyCleanPanel();
  assert.equal(h.el('mode-edit').disabled,false);
  h.state.cleanOpen=true;h.context.applyCleanPanel();
  assert.equal(h.shown.length,1);assert.equal(h.el('clean-panel').hidden,false);
});

test('the edit panel shows one track: its filters, edits, fills, segments and counts',()=>{
  const h=editHarness();
  h.state.partSelection={path:'a.gpx',key:'t1'};h.context.applyCleanPanel();
  const view=h.shown.at(-1);
  assert.equal(view.name,'Evening');
  assert.equal(view.clean.params.stops.enabled,true,'the track\'s own filters');
  assert.equal(JSON.stringify(view.clean.params.edits),JSON.stringify([{kind:'range',first:8,last:8,join:false}]));
  assert.equal(view.clean.counts.manual,1);assert.equal(view.clean.counts.stop,2);assert.equal(view.clean.counts.filled,1);
  assert.equal(view.fills.length,1);assert.equal(view.pieces.length,1);
  assert.equal(h.editing.at(-1).edits,view.clean.params.edits,'the timeline indexes the same edits as the panel');
  h.context.applyCleanPanel();assert.equal(h.shown.at(-1),view,'the panel keeps its view while nothing changed');
});

test('saving an edit sends the track\'s filters and keeps the other tracks\' edits',async()=>{
  const h=editHarness();
  h.state.partSelection={path:'a.gpx',key:'t1'};h.context.applyCleanPanel();
  const params=structuredClone(h.shown.at(-1).clean.params);
  params.stops.enabled=false;params.edits=[];
  await h.context.saveClean(params);
  const [sent]=h.saved;
  assert.equal(sent.first,5);assert.equal(sent.clean.stops.enabled,false);
  assert.equal(JSON.stringify(sent.clean.edits),JSON.stringify([{kind:'lasso',points:[0],join:true}]),'the first track\'s edit stays');
});

test('in Edit, tool clicks pick points of the edited track only',()=>{
  const app=source('app');
  const entry={hiddenParts:new Set(['t2']),track:{parts:[{key:'t0',kind:'track'},{key:'t1',kind:'track'},{key:'t2',kind:'track'},{key:'w0',kind:'waypoint'}]}};
  const state={cleanOpen:true};
  const context=vm.createContext({state,focusedEntry:()=>entry,editPart:()=>entry.track.parts[1]});
  vm.runInContext(app.slice(app.indexOf('// pickHidden are'),app.indexOf('// inPart reports')),context);
  assert.deepEqual([...context.pickHidden(entry)].sort(),['t0','t2']);
  state.cleanOpen=false;assert.equal(context.pickHidden(entry),entry.hiddenParts);
  for(const call of ['layers.nearest(focused.track, event.point, 24, pickHidden(focused))','layers.nearest(track, event.point, 24, pickHidden(entry))','layers.nearest(cutEntry.track, event.point, 24, pickHidden(cutEntry))'])assert.ok(app.includes(call),call);
});

test('in Edit, a timeline range is cut to the edited track and the lasso takes its points only',()=>{
  const h=editHarness();
  h.state.partSelection={path:'a.gpx',key:'t1'};
  const ranges=[];h.context.cleanPanel.addRange=(first,last)=>ranges.push([first,last]);
  h.context.applyCleanPanel();
  const {onRange}=h.editing.at(-1);
  onRange(2,7);onRange(0,3);onRange(8,12);
  assert.deepEqual(ranges,[[5,7],[8,9]],'clamped to 5..9; a range wholly on another track adds nothing');
  // The lasso: every point lies inside the shape, only the edited track's are taken.
  const app=source('app');const edits=[];
  Object.assign(h.context,{map:{project:([x,y])=>({x,y})},insidePolygon:()=>true,hiddenRanges:()=>[],uncheckedRanges:()=>[],
    cleanPanel:{track:h.shown.at(-1),addEdit:edit=>edits.push(edit)},editPart:()=>h.track.parts[1]});
  vm.runInContext(app.slice(app.indexOf('// pushEdit records'),app.indexOf('function setLasso(')),h.context);
  vm.runInContext(app.slice(app.indexOf('// lassoDone removes'),app.indexOf('// reportFocus tells')),h.context);
  h.context.lassoDone([],{restore:false});
  assert.equal(JSON.stringify(edits),JSON.stringify([{kind:'lasso',points:[7,9],join:true}]),'kept points 7 and 9 of track t1 only');
});

test('empty GPX rows carry a file icon aligned with the disclosure; children are indented further',()=>{
 const h=treeHarness(treeTrack({parts:[]})),row=h.context.trackRow('current.gpx',h.entry);
 assert.ok(row.children[0].classList.contains('disclosure-space'));
 assert.match(row.children[0].innerHTML,/data-icon="file"/);
 assert.equal(row.children[0].attributes['aria-hidden'],'true');
 const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
 assert.match(css,/\.disclosure-space\s*\{[^}]*width: 18px/);
 assert.match(css,/\.list li\.part-row\s*\{[^}]*margin-left: 28px/);
});

test('manually removed samples and bands can be hidden without hiding automatic removals or changing data',()=>{
 const sources=new Map(),layout=new Map(),filters=new Map(),context=vm.createContext({document:documentStub(),format});loadModule('clean',context);
 const map={addSource(){},addLayer(){},setLayoutProperty:(id,key,value)=>layout.set(id,value),setFilter:(id,value)=>filters.set(id,plain(value)),getSource:id=>({setData:value=>sources.set(id,plain(value))})};context.map=map;
 const overlay=vm.runInContext('new CleanOverlay(map,"cursor")',context);
 const track={points:[[0,0],[1,0],[2,0]],removed:['manual','manual','spikes'],segments:[0,0,0]};
 overlay.show(track);const before=JSON.stringify([...sources]);
 overlay.setManualVisible(false);assert.equal(layout.get('clean-manual'),'none');assert.equal(layout.get('clean-manual-band'),'none');
 assert.deepEqual(filters.get('clean-removed'),['!=',['get','rule'],'manual']);
 overlay.setManualVisible(true);assert.equal(layout.get('clean-manual'),'visible');assert.equal(filters.get('clean-removed'),null);
 assert.equal(JSON.stringify([...sources]),before);assert.deepEqual(track.removed,['manual','manual','spikes']);
});

test('New GPX preserves the source track segment choices through focus changes',async()=>{
 const selectedPieces=[...pieces,{first:4,last:6,name:'Third',points:3,distance:20}],h=segmentHarness(selectedPieces);
 const part={kind:'track',key:'t0',first:0,last:6};h.track.parts=[part];
 const options={part,timeZone:'UTC',workspace:['current.gpx'],folder:''};h.panel.show(h.track,options);
 const rows=()=>h.root.all(e=>e.classList.contains('cut-row'));
 await rows()[1].fire('click',{timeStamp:100});await rows()[2].fire('click',{timeStamp:200,ctrlKey:true});
 assert.deepEqual([...h.panel.selected],[2,4]);
 Object.assign(h.context,{cutPanel:h.panel,promptDialog:async()=> 'Synthetic draft',api:{newDraft:async()=>({path:'draft:new'})},
  addTrack:async()=>{h.panel.hide();h.state.focus='draft:new';},showMessage(){}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('async function newGPX()'),app.indexOf('// discardSidecar')),h.context);
 await h.context.newGPX();h.state.focus='current.gpx';h.panel.show(h.track,{...options,workspace:['current.gpx','draft:new']});
 assert.deepEqual([...h.panel.selected],[2,4]);h.panel.hide();h.panel.hide();h.panel.show(h.track,options);assert.deepEqual([...h.panel.selected],[2,4]);
});

test('workspace drags reorder siblings in writable GPX files while refusing recordings and mixed kinds',async()=>{
 const parts=[{key:'t0',kind:'track'},{key:'t1',kind:'track'},{key:'w0',kind:'waypoint'}],row={dataset:{path:'source.gpx',kind:'track',key:'t1'}},calls=[];
 const context=vm.createContext({dragRow:{path:'source.gpx',kind:'track',key:'t0'},dragGap:{at:()=>({row,above:false})},
  $:()=>({getBoundingClientRect:()=>({left:0,right:100})}),state:{focus:null,tracks:new Map([['source.gpx',{track:{ours:true,parts}}]])},
  api:{orderParts:async(...args)=>calls.push(plain(args))},reloadEntry:async()=>{},renderTracks(){},showMessage(){}});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('const copyModifier'),app.indexOf('// markDrop')),context);
 vm.runInContext(app.slice(app.indexOf('async function applyDrop'),app.indexOf('// reorderFiles')),context);
 vm.runInContext(app.slice(app.indexOf('async function reorderParts'),app.indexOf('// dropIntoFile')),context);
 const target=context.dropAt(50,50,{});assert.equal(target.kind,'parts');await context.applyDrop(context.dragRow,target);
 assert.deepEqual(calls,[['source.gpx','track',['t1','t0']]]);
 context.state.tracks.get('source.gpx').track.ours=false;assert.equal(context.dropAt(50,50,{}),null);
 context.state.tracks.get('source.gpx').track.ours=true;row.dataset.kind='waypoint';assert.equal(context.dropAt(50,50,{}),null);
});

test('clicking a visible track enters Edit; hidden tracks, routes and waypoints do not',async()=>{
 for(const kind of ['track','route','waypoint'])for(const visible of [true,false])for(const hidden of [true,false]){
  const part={kind,key:'p0',points:[[0,0]],bounds:[[0,0],[1,1]]},h=treeHarness(treeTrack({parts:[part]}));
  h.entry.visible=visible;if(hidden)h.entry.hiddenParts.add(part.key);
  h.context.showPart('current.gpx',part);
  assert.equal(h.calls.some(([action,mode])=>action==='mode'&&mode==='edit'),kind==='track'&&visible&&!hidden);
  assert.equal(h.calls.some(([action])=>action==='fit'||action==='pan'),false);
 }
});

test('map hover follows the selected track and synchronizes time, elevation and speed cursors',()=>{
 const handlers=new Map(),frames=[],positions=[],elevation=[],speed=[],readout=new Element('span');
 const track={points:[[0,0],[10,0],[20,0],[30,0],[10,0],[20,0]],segments:[0,0,0,0,1,1],removed:[],
  parts:[{kind:'track',key:'t0',first:0,last:3},{kind:'track',key:'t1',first:4,last:5}],distance:[0,100,200,300,400,500],time:[0,1000,2000,3000,4000,5000],elevation:[10,20,30,40,50,60],speed:[1,2,3,4,5,6]};
 const entry={id:'a',visible:true,track,color:'#0072b2',hiddenParts:new Set()},state={focus:'source.gpx',partSelection:{path:'source.gpx',key:'t0'},tracks:new Map([['source.gpx',entry]]),fill:{active:false}};
 let moving=false;
 const map={on:(name,fn)=>handlers.set(name,fn),isMoving:()=>moving,project:([x,y])=>({x,y}),unproject:([x,y])=>({lng:x,lat:y}),getCanvas:()=>({style:{}})};
 const layers=Object.create(TrackLayers.prototype);Object.assign(layers,{map,setCursor:point=>positions.push(point),setHover(){}});
 const context=vm.createContext({format,state,map,layers,requestAnimationFrame:fn=>frames.push(fn),waypointAt:()=>null,showPhotoHover(){},focusedEntry:()=>entry,cutting:()=>false,uncheckedRanges:()=>[]});
 loadModule('profile',context);loadModule('timeline',context);
 const profile=vm.runInContext('Object.create(Profile.prototype)',context),timeline=vm.runInContext('Object.create(Timeline.prototype)',context);
 const chart=values=>({over:{clientHeight:20},valToPos:value=>value,setCursor:cursor=>values.push(cursor.left)});
 Object.assign(profile,{track,charts:[chart(elevation),chart(speed)],labels:{readout},timeZone:'UTC'});
 Object.assign(timeline,{track,range:[0,5000],cursor:{style:{}},percent:time=>`${time/50}%`});Object.assign(context,{profile,timeline});
 const app=source('app');vm.runInContext(app.slice(app.indexOf('function inspect(index)'),app.indexOf('// ---- the timeline')),context);
 vm.runInContext(app.slice(app.indexOf('function bindMap()'),app.indexOf('// ---- layout ----')),context);context.bindMap();
 for(let i=0;i<4;i++){
  handlers.get('mousemove')({point:{x:i*10,y:0},originalEvent:{buttons:0}});frames.shift()();
  assert.equal(positions.at(-1),track.points[i]);assert.equal(timeline.cursor.style.left,`${i*20}%`);
  assert.equal(elevation.at(-1),i/10);assert.equal(speed.at(-1),i/10);
  assert.ok(readout.textContent.includes(`${track.elevation[i]} m`));
 }
 moving=true;handlers.get('mousemove')({point:{x:20,y:0},originalEvent:{buttons:0}});assert.equal(frames.length,0);
 handlers.get('mouseout')();assert.equal(timeline.cursor.hidden,true);assert.equal(readout.textContent,'');
});

test('nearest hover keeps its previous branch at equal-distance crossings and excludes unchecked ranges',()=>{
 const layers=Object.create(TrackLayers.prototype);layers.map={project:([x,y])=>({x,y}),unproject:([x,y])=>({lng:x,lat:y})};
 const track={points:[[0,0],[10,0],[20,0],[10,0],[10,10]],removed:[],segments:[0,0,0,0,0],parts:[{kind:'track',key:'t0',first:0,last:4}]};
 assert.equal(layers.nearest(track,{x:10,y:0},24,new Set(),{preferred:1}),1);
 assert.equal(layers.nearest(track,{x:10,y:0},24,new Set(),{preferred:3}),3);
 assert.equal(layers.nearest(track,{x:10,y:0},1,new Set(),{hiddenRanges:[[1,3]]}),-1);
});

test('profile axes and canvas margins inspect samples even outside the narrow uPlot overlay',async()=>{
 const context=vm.createContext({document:documentStub(),format,uiPx:value=>value,getComputedStyle:()=>({getPropertyValue:()=>'',fontFamily:'sans-serif'})});
 const calls=[];let chart;
 context.uPlot=class{
  constructor(options,data){this.root=new Element('div');this.over=new Element('div');this.over.getBoundingClientRect=()=>({left:100,width:200});this.over.contains=target=>target===this.over;this.data=data;chart=this;}
  posToIdx(x){return Math.round(x/100);}
 };
 loadModule('profile',context);
 const profile=vm.runInContext('Object.create(Profile.prototype)',context);
 Object.assign(profile,{charts:[],offset:0,setIndex:index=>calls.push(['cursor',index]),onHover:index=>calls.push(['hover',index]),showReadout:index=>calls.push(['readout',index])});
 const container=new Element('div');container.clientWidth=300;container.clientHeight=100;
 profile.addChart(container,[0,1,2],[10,20,30],'#0072b2',false,'empty',String);
 await chart.root.fire('pointermove',{clientX:200,buttons:0,target:container});assert.deepEqual(calls,[['cursor',1],['hover',1]]);
 calls.length=0;await chart.root.fire('pointermove',{clientX:200,buttons:0,target:chart.over});assert.deepEqual(calls,[]);
 await chart.root.fire('pointermove',{clientX:200,buttons:1,target:container});assert.deepEqual(calls,[]);
 await chart.root.fire('mouseleave');assert.deepEqual(calls,[['readout',null],['hover',null]]);
 const app=source('app'),inspect=app.slice(app.indexOf('function inspect(index)'),app.indexOf('// ---- the timeline'));
 assert.match(inspect,/timeline\.setIndex\(index\)/);assert.match(inspect,/profile\.setIndex\(index\)/);
 const css=readFileSync(new URL('./web/app.css',import.meta.url),'utf8');
 assert.match(css,/\.timeline-cursor\s*\{[^}]*z-index: 3/);
});


test('map reset zoom frames the selected visible track or visible GPX files', async () => {
  const bounds=[[1,2],[3,4]], part={key:'track-1',bounds};
  const entry={visible:true,hiddenParts:new Set(),track:{parts:[part]}};
  const hidden={visible:false,track:{parts:[]}};
  const calls=[];
  const document=documentStub();
  const create=document.createElement;
  document.createElement=tag=>{const node=create(tag);node.appendChild=child=>node.append(child);return node;};
  const context=vm.createContext({document,state:{tracks:new Map([['source.gpx',entry],['hidden.gpx',hidden]])},
    focusedEntry:()=>entry,editPart:()=>part,fitBox:box=>calls.push(['box',plain(box)]),fit:tracks=>calls.push(['files',tracks]),
    MAXIMIZE:'fit',CHEVRON_UP:'up',CHEVRON_DOWN:'down',ROTATE_CCW:'left',ROTATE_CW:'right',clearInterval,setInterval});
  const app=source('app');
  vm.runInContext(app.slice(app.indexOf('const TILT_STEP'),app.indexOf('// bindTerrain'))+';globalThis.ViewControl=ViewControl;',context);
  const control=new context.ViewControl().onAdd({});
  const reset=control.children[0];
  assert.match(reset.attributes['aria-label'],/Reset zoom/);
  await reset.fire('click');
  assert.deepEqual(calls.pop(),['box',bounds]);
  entry.hiddenParts.add(part.key);
  await reset.fire('click',{detail:0});
  assert.deepEqual(Array.from(calls.pop()[1]),[entry.track]);
  entry.visible=false;
  await reset.fire('click');
  assert.equal(calls.pop()[1].length,0);
});
