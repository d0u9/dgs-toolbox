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
  // Replace module linkage at the boundary, retaining every production method.
  vm.runInContext(source(name).replace(/^import .*;\s*$/gm,'').replace(/^export /gm,''),context,{filename:`${name}.js`});
}

function treeHarness(track){
  const calls=[];
  const entry={id:'a',color:'#0072b2',visible:false,hiddenParts:new Set(),track};
  const state={focus:'other.gpx',tracks:new Map([['current.gpx',entry]]),selection:new Set(),config:{canReveal:false}};
  const context=vm.createContext({
    tooltip(){},document:documentStub(),format,state,EYE:'eye',EYE_OFF:'hidden',UP:'up',DOWN:'down',
    PART_GLYPH:{track:'track',route:'route',waypoint:'waypoint'},
    expandable:()=>false,oursMark:()=>new Element('span'),filteredPaths:()=>['current.gpx'],
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
  vm.runInContext(app.slice(app.indexOf('const doubleRowClick'),app.indexOf('// oursMark')),context);
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
    document:documentStub(),format,state,
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
    assert.ok(inactive.paint['line-width']>=2);
    assert.ok(active.paint['line-width']>inactive.paint['line-width']);
  }
  assert.equal(styles.get('track-b-casing').layout.visibility,'visible');
  assert.ok(styles.get('track-b-casing').paint['line-opacity']>=0.6);
  assert.ok(styles.get('track-b-casing').paint['line-width']>styles.get('track-b-line').paint['line-width']);
  assert.deepEqual(moves.map(([id])=>id),['track-a-casing','track-a-route','track-a-line','track-a-waypoint']);
  layers.setVisible('b',false);layers.setFocus('b');
  for(const kind of ['line','route','casing','waypoint'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'none');
  layers.setVisible('b',true);layers.setFocus('a');
  for(const kind of ['line','route','casing','waypoint'])assert.equal(styles.get(`track-b-${kind}`).layout.visibility,'visible');
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
  const event={point:{x:10,y:10},originalEvent:{target:new Element('div')},preventDefault(){}};
  listeners.get('click')(event);assert.deepEqual(calls,[['inspect',1],['profile',1],['timeline',1]]);
  listeners.get('contextmenu')(event);assert.equal(calls.some(([kind])=>kind==='cut'),false);
  menus[0][0][0].onSelect();assert.deepEqual(calls.at(-1),['cut',1]);
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
