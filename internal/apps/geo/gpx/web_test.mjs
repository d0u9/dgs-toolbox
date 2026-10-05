import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { rowDoubleClick } from './web/drag.js';
import { TrackLayers } from './web/tracks.js';
const click = (timeStamp, extras = {}) => ({timeStamp, clientX: 10, clientY: 10, detail: 1, ...extras});
test('double click survives replacement rows, resets after fit and rejects other gestures', () => {
 const double = rowDoubleClick();
 assert.equal(double('file-a', click(100)), false);
 assert.equal(double('file-a', click(200)), true);
 assert.equal(double('file-a', click(300)), false);
 assert.equal(double('file-b', click(400)), false);
 assert.equal(double('file-b', click(1000)), false);
 assert.equal(double('file-b', click(1100, {shiftKey:true})), false);
 assert.equal(double('file-b', click(1200)), false);
 assert.equal(double('file-b', click(1300, {clientX:30})), false);
 assert.equal(double('file-b', click(1400, {detail:0})), false);
 assert.equal(double('file-b', click(1500)), false);
});
test('single part click preserves camera; double click frames tracks and routes', () => {
 const source = readFileSync(new URL('./web/app.js', import.meta.url), 'utf8');
 const code = source.slice(source.indexOf('function showPart('), source.indexOf('\nfunction openWaypoint('));
 for (const kind of ['track', 'route', 'waypoint']) {
  const calls=[];
  const entry={visible:false,hiddenParts:new Set(['part-a'])};
  const context=vm.createContext({state:{focus:'other',tracks:new Map([['file-a',entry]])},setVisible:(...args)=>calls.push(['visible',...args]),setPartHidden:(...args)=>calls.push(['hidden',...args]),setFocus:(...args)=>calls.push(['focus',...args]),fitBox:()=>calls.push(['fit']),openWaypoint:()=>calls.push(['popup']),map:{getZoom:()=>12,easeTo:()=>calls.push(['pan'])}});
  vm.runInContext(code,context);
  const part={kind,key:'part-a',points:[[0,0]],bounds:[[0,0],[1,1]]};
  context.showPart('file-a',part);
  assert.equal(calls.some(([action])=>['fit','pan'].includes(action)),false);
  assert.deepEqual(JSON.parse(JSON.stringify(calls.find(([action])=>action==='focus'))),['focus','file-a',{fit:false}]);
  context.showPart('file-a',part,{fit:true});
  assert.equal(calls.some(([action])=>action===(kind==='waypoint'?'pan':'fit')),true);
 }
});
test('unfocused traces remain visible and hidden files have no casing', () => {
 const paint=new Map(),layout=new Map();
 const map={addSource(){},addLayer(){},setPaintProperty:(id,key,value)=>paint.set(`${id}/${key}`,value),setLayoutProperty:(id,key,value)=>layout.set(`${id}/${key}`,value),moveLayer(){}};
 const layers=new TrackLayers(map);
 layers.ids=new Set(['a','b']);layers.hidden.add('b');layers.setFocus('a');
 assert.ok([...paint].some(([key,value])=>key.startsWith('track-b-') && key.endsWith('/line-opacity') && value===0.85));
 assert.ok([...layout].filter(([key])=>key.startsWith('track-b-')).every(([,value])=>value==='none'));
 layers.hidden.delete('b');layers.setFocus('a');
 assert.ok([...layout].filter(([key])=>key.startsWith('track-b-')).every(([,value])=>value==='visible'));
});

test('timeline jump uses the same on-track anchor as the stop marker', () => {
 const source = readFileSync(new URL('./web/app.js', import.meta.url), 'utf8');
 const code = source.slice(source.indexOf('function selectStop('), source.indexOf('// waypointAt'));
 const position = [0, 0];
 const calls = [];
 const context = vm.createContext({focusedEntry: () => ({track:{points:[position],stops:[{anchor:0,center:[1,1]}]}}), stopMarkers:{openPopup:index=>calls.push(['popup',index])}, map:{getBounds:()=>({contains:point=>{assert.equal(point,position);return false;}}),easeTo:options=>calls.push(['pan',options.center])}});
 vm.runInContext(code,context);
 context.selectStop(0,{pan:true});
 assert.deepEqual(calls,[['popup',0],['pan',position]]);
});

test('renaming a workspace file preserves its order, identity, selection and camera', () => {
 const source = readFileSync(new URL('./web/app.js', import.meta.url), 'utf8');
 const code = source.slice(source.indexOf('function replaceTrackPath('), source.indexOf('async function renameGPX('));
 const entry = {id:4,track:{name:'Old',path:'draft:4/Old.gpx'},hiddenParts:new Set(['t1'])};
 const state = {tracks:new Map([['a',{}],['draft:4/Old.gpx',entry],['b',{}]]), selection:new Set(['draft:4/Old.gpx']), anchor:'draft:4/Old.gpx',focus:'draft:4/Old.gpx',fill:{preview:{path:'draft:4/Old.gpx'}}};
 const calls=[];
 const context=vm.createContext({state,basename:path=>path.split('/').pop(),setFocus:(path,options)=>{state.focus=path;calls.push(options);},renderTracks:()=>{},save:()=>{}});
 vm.runInContext(code,context);
 context.replaceTrackPath('draft:4/Old.gpx','draft:4/New.gpx');
 assert.deepEqual([...state.tracks.keys()],['a','draft:4/New.gpx','b']);
 assert.equal(state.tracks.get('draft:4/New.gpx'),entry);
 assert.equal(state.selection.has('draft:4/New.gpx'),true);
 assert.equal(state.selection.has('draft:4/Old.gpx'),false);
 assert.equal(state.anchor,'draft:4/New.gpx');
 assert.equal(state.fill.preview.path,'draft:4/New.gpx');
 assert.equal(entry.track.name,'New');
 assert.equal(entry.track.path,'draft:4/New.gpx');
 assert.deepEqual(JSON.parse(JSON.stringify(calls)),[{fit:false}]);
});

function segmentAdder(promptDialog, chooseDialog = () => assert.fail("unexpected multi-segment choice")) {
 const source = readFileSync(new URL('./web/cut.js', import.meta.url), 'utf8');
 const code = source.slice(source.indexOf('  async addSegments('), source.indexOf('  // write sends'));
 return vm.runInNewContext(`({${code}}).addSegments`, {promptDialog, chooseDialog});
}

test('Add 1 segment asks for a name before adding and preserves the source', async () => {
 const segment = {first:10,last:20,name:'Existing name'};
 const target = {mode:'add',path:'draft:1/Trip.gpx'};
 const button = {disabled:false};
 const calls=[];
 const add = segmentAdder(async options => {
  assert.equal(button.disabled,true);
  assert.equal(options.value,'Existing name');
  assert.equal(calls.length,0);
  return '  Named stop  ';
 });
 await add.call({write:async (...args)=>calls.push(args)},[segment],target,button);
 assert.equal(calls.length,1);
 assert.deepEqual(JSON.parse(JSON.stringify(calls[0][0])),[{first:10,last:20,name:'Named stop'}]);
 assert.equal(calls[0][1],target);
 assert.equal(segment.name,'Existing name');
 assert.equal(button.disabled,false);
});

test('cancelling or leaving the name empty adds nothing', async () => {
 for (const name of [null,'','   ']) {
  const button={disabled:false};
  const add=segmentAdder(async()=>name);
  await add.call({write:()=>assert.fail('cancelled addition wrote')},[{first:0,last:1,name:'Segment 1'}],{mode:'add',path:'draft:1/Trip.gpx'},button);
  assert.equal(button.disabled,false);
 }
});

test('multiple segments require a separate-or-merge choice before adding', async () => {
 const segments=[{first:0,last:1,name:'One'},{first:2,last:3,name:'Two'}];
 const button={disabled:false};
 let written, choices=0;
 const add=segmentAdder(()=>assert.fail('separate tracks opened naming prompt'),async options=>{
  choices++;
  assert.equal(button.disabled,true);
  assert.equal(written,undefined);
  assert.deepEqual(JSON.parse(JSON.stringify(options.options.map(option=>option.value))),['separate','merge']);
  return 'separate';
 });
 const target={mode:'add',path:'draft:1/Trip.gpx'};
 await add.call({write:async (value,to)=>{written=value;assert.equal(to,target);}},segments,target,button);
 assert.equal(choices,1);
 assert.equal(written,segments);
 assert.equal(button.disabled,false);
});

test('merge asks for a name and passes the original ranges with a named merge target', async () => {
 const segments=[{first:0,last:2,name:'One'},{first:2,last:4,name:'Two'}];
 const target={mode:'add',path:'draft:1/Trip.gpx'};
 const order=[];
 const add=segmentAdder(async options=>{
  order.push('name');
  assert.equal(options.title,'Name the merged track');
  return '  Combined walk  ';
 },async()=>{order.push('choice');return 'merge';});
 await add.call({write:async (value,to)=>{
  order.push('write');
  assert.equal(value,segments);
  assert.deepEqual(JSON.parse(JSON.stringify(to)),{...target,merge:true,name:'Combined walk'});
 }},segments,target,{disabled:false});
 assert.deepEqual(order,['choice','name','write']);
 assert.deepEqual(target,{mode:'add',path:'draft:1/Trip.gpx'});
 assert.deepEqual(segments.map(one=>one.name),['One','Two']);
});

test('cancelling the multi-segment choice or merged name never adds', async () => {
 const segments=[{name:'One'},{name:'Two'}];
 const cancelled=segmentAdder(()=>assert.fail('cancelled choice opened name dialog'),async()=>null);
 const button={disabled:false};
 await cancelled.call({write:()=>assert.fail('cancelled choice wrote')},segments,{},button);
 assert.equal(button.disabled,false);
 for (const name of [null,'','   ']) {
  const add=segmentAdder(async()=>name,async()=> 'merge');
  await add.call({write:()=>assert.fail('cancelled or empty merged name wrote')},segments,{},button);
  assert.equal(button.disabled,false);
 }
});

test('segment naming restores the button if adding fails', async () => {
 const add=segmentAdder(async()=> 'Named');
 const button={disabled:false};
 await assert.rejects(add.call({write:async()=>{throw new Error('write failed');}},[{name:'One'}],{},button),/write failed/);
 assert.equal(button.disabled,false);
});

test('segment targets include the current GPX only for our files and drafts', () => {
 const source=readFileSync(new URL('./web/cut.js',import.meta.url),'utf8');
 const code=source.slice(source.indexOf('  setWorkspace('),source.indexOf('  hide()'));
 const setWorkspace=vm.runInNewContext(`({${code}}).setWorkspace`);
 for (const [ours,draft,expected] of [[true,false,['current.gpx','other.gpx']],[false,true,['current.gpx','other.gpx']],[false,false,['other.gpx']]]) {
  const panel={track:{path:'current.gpx',ours,draft},workspace:[],addPath:'',render:()=>{}};
  setWorkspace.call(panel,['current.gpx','other.gpx']);
  assert.deepEqual(JSON.parse(JSON.stringify(panel.workspace)),expected);
  assert.equal(panel.addPath,expected[0]);
 }
});
