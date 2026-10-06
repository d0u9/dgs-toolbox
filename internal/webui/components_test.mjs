import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync,readdirSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {Element,documentStub} from './test_dom.mjs';

function moduleIn(name,document=documentStub()){
  const code=readFileSync(new URL(`./assets/${name}.js`,import.meta.url),'utf8');
  const names=[...code.matchAll(/export (?:async )?(?:function|const|class) (\w+)/g)].map(match=>match[1]);
  const context=vm.createContext({document,window:{innerWidth:800,innerHeight:600,addEventListener(){}}});
  return {document,api:vm.runInContext(`(()=>{${code.replace(/^export /gm,'')}return {${names.join(',')}};})()`,context)};
}
const form=dialog=>dialog.children.find(element=>element.tagName==='FORM');

test('shared buttons keep button semantics, accessible labels and row-event isolation',async()=>{
  const {api}=moduleIn('controls');let clicked=0;
  const button=api.iconButton('icon-button','<svg></svg>','Show details',()=>{clicked++;});
  assert.equal(button.type,'button');assert.equal(button.attributes['aria-label'],'Show details');
  const event=await button.fire('click');assert.equal(event.stopped,true);assert.equal(clicked,1);
  const text=api.textButton('Save',true,()=>{});assert.equal(text.disabled,true);assert.equal(text.type,'button');
  const row=api.rowLabel('Trip',['20 m','',null,'5 min']);assert.equal(row.children[0].textContent,'Trip');assert.equal(row.children[1].textContent,'20 m · 5 min');
});

test('shared prompt validates text, returns a trimmed name and restores focus',async()=>{
  const document=documentStub(),opener=new Element('button');document.activeElement=opener;
  const {api}=moduleIn('dialog',document);
  const pending=api.promptDialog({title:'Name track',message:'Choose a name',value:'Initial'});
  const dialog=document.body.children[0],input=form(dialog).children[0];
  assert.equal(dialog.open,true);assert.equal(input.focused,true);assert.equal(input.textSelected,true);
  assert.equal(input.attributes['aria-label'],'Name track');
  input.value='  ';const invalid=await form(dialog).fire('submit');assert.equal(invalid.defaultPrevented,true);assert.equal(dialog.open,true);
  input.value='  Named trip  ';await form(dialog).fire('submit');dialog.close();
  assert.equal(await pending,'Named trip');assert.equal(document.body.children.length,0);assert.equal(opener.focused,true);
});

test('cancel button and native close both cancel a prompt without committing',async()=>{
  for(const viaButton of [true,false]){
    const {api,document}=moduleIn('dialog'),pending=api.promptDialog({title:'Rename'});
    const dialog=document.body.children[0];
    if(viaButton)await form(dialog).children.at(-1).children[0].fire('click');else dialog.close();
    assert.equal(await pending,null);assert.equal(document.body.children.length,0);
  }
});

test('shared choice returns the option value and cancellation returns null',async()=>{
  for(const cancel of [false,true]){
    const {api,document}=moduleIn('dialog');const pending=api.chooseDialog({title:'Add segments',options:[{label:'Separate tracks',value:'separate'},{label:'One track',value:'merge'}]});
    const dialog=document.body.children[0],choice=form(dialog).children[0];
    assert.equal(choice.focused,true);assert.equal(choice.children[1].textContent,'One track');
    choice.value='merge';if(!cancel)await form(dialog).fire('submit');dialog.close();
    assert.equal(await pending,cancel?null:'merge');
  }
});

test('dangerous confirmation focuses Cancel and commits only after explicit confirmation',async()=>{
  for(const confirm of [false,true]){
    const {api,document}=moduleIn('dialog'),pending=api.confirmDialog({title:'Discard edits',detail:'synthetic.gpx',danger:true});
    const dialog=document.body.children[0],actions=form(dialog).children.at(-1);
    assert.equal(actions.children[0].focused,true);assert.equal(actions.children[1].className,'button danger');
    if(confirm)await form(dialog).fire('submit');dialog.close();assert.equal(await pending,confirm);
  }
});

test('custom fields share modal validation and return a structured domain value',async()=>{
  const {api,document}=moduleIn('dialog'),input=new Element('input');input.value='';
  const pending=api.formDialog({title:'Add item',fields:[input],read:()=>input.value?{name:input.value}:null});
  const dialog=document.body.children[0];const invalid=await form(dialog).fire('submit');assert.equal(invalid.defaultPrevented,true);
  input.value='Synthetic';await form(dialog).fire('submit');dialog.close();assert.deepEqual(await pending,{name:'Synthetic'});
});


test('complete GPX and shared webui modules parse with their actual import declarations',()=>{
  for(const folder of [new URL('./assets/',import.meta.url),new URL('../apps/geo/gpx/web/',import.meta.url)]){
    for(const file of readdirSync(folder).filter(name=>name.endsWith('.js'))){
      const result=spawnSync(process.execPath,['--check',fileURLToPath(new URL(file,folder))],{encoding:'utf8'});
      assert.equal(result.status,0,`${file}: ${result.stderr}`);
    }
  }
});


test('splitters scale remembered sizes once, preserve drag coordinates and store design sizes',async()=>{
  const saved=new Map([['pane','300']]);
  const localStorage={getItem:key=>saved.get(key),setItem:(key,value)=>saved.set(key,value)};
  const document=documentStub(),context=vm.createContext({document,localStorage,window:{innerWidth:800,innerHeight:600,addEventListener(){}}});
  const code=readFileSync(new URL('./assets/splitter.js',import.meta.url),'utf8');
  const api=vm.runInContext(`(()=>{${code.replace(/^export /gm,'')}return {splitter};})()`,context);
  const make=()=>{const handle=new Element('div'),target=new Element('div');handle.hasAttribute=name=>name in handle.attributes;handle.setPointerCapture=()=>{};handle.removeEventListener=(name,fn)=>handle.listeners.set(name,(handle.listeners.get(name)||[]).filter(item=>item!==fn));target.getBoundingClientRect=()=>({width:parseFloat(target.style.width)});return {handle,target};};
  let {handle,target}=make();api.splitter({handle,target,axis:'x',min:100,key:'pane',scale:.9});
  assert.equal(target.style.width,'270px');await handle.fire('pointerdown',{button:0,clientX:200});await handle.fire('pointermove',{clientX:218});await handle.fire('pointerup');
  assert.equal(target.style.width,'288px');assert.equal(saved.get('pane'),'320');
  ({handle,target}=make());api.splitter({handle,target,axis:'x',min:100,key:'pane',scale:.9});assert.equal(target.style.width,'288px');
});

test("bundled Lucide icons are offline SVGs with legible strokes and a filled disclosure", async()=>{
 const {api:icons}=moduleIn("icons");
 for(const [name,svg] of Object.entries(icons).filter(([,svg])=>typeof svg === "string")){assert.ok(svg.includes("<svg"),name);assert.ok(svg.includes('aria-hidden="true"'),name);assert.ok(!/href=|<script|<image/.test(svg),name);}
 assert.ok(icons.EYE.includes('stroke-width="2.8"'));assert.ok(icons.TRIANGLE_RIGHT.includes('fill="currentColor"'));
});


test('a shared form dialog can be dragged and still validate, submit and cancel normally',async()=>{
 const document=documentStub(),listeners=new Map();document.addEventListener=(type,fn)=>listeners.set(type,fn);
 const {api}=moduleIn('dialog',document);const input=new Element('input');input.value='';
 const pending=api.formDialog({title:'Name track',fields:[input],read:()=>input.value.trim()||null});
 const dialog=document.body.children[0],heading=dialog.children[0];
 dialog.getBoundingClientRect=()=>({left:parseFloat(dialog.style.left)||100,top:parseFloat(dialog.style.top)||100,width:300,height:200});
 heading.closest=()=>dialog;let captured=false;heading.setPointerCapture=()=>{captured=true;};heading.hasPointerCapture=()=>captured;heading.releasePointerCapture=()=>{captured=false;};
 const event={button:0,pointerId:1,clientX:120,clientY:110,preventDefault(){},target:{closest:selector=>selector.startsWith('button')?null:heading}};
 listeners.get('pointerdown')(event);listeners.get('pointermove')({pointerId:1,clientX:220,clientY:160});
 assert.equal(dialog.style.left,'200px');assert.equal(dialog.style.top,'150px');assert.equal(captured,true);
 const invalid=await form(dialog).fire('submit');assert.equal(invalid.defaultPrevented,true);
 listeners.get('pointerup')({pointerId:1});assert.equal(captured,false);
 listeners.get('pointerdown')({...event,target:input});assert.equal(captured,false);
 input.value='  Walk  ';await form(dialog).fire('submit');dialog.close();listeners.get('close')({target:dialog});assert.equal(await pending,'Walk');
 const cancelled=api.promptDialog({title:'Rename track'});document.body.children.at(-1).close();assert.equal(await cancelled,null);
});
