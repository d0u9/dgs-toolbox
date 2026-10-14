import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';
import {Element,documentStub} from './test_dom.mjs';

test('Open and Save expose the folder path through a keyboard-accessible button',async()=>{
  for(const mode of ['openFile','saveFile']){
    const document=documentStub();
    document.createTextNode=text=>text;
    document.createElement=tag=>{
      const element=new Element(tag);
      element.setSelectionRange=()=>{};
      element.blur=()=>element.fire('blur');
      return element;
    };
    const code=readFileSync(new URL('./assets/filedialog.js',import.meta.url),'utf8');
    const context=vm.createContext({document,URLSearchParams,
      localStorage:{getItem:()=> 'off'},
      ResizeObserver:class{observe(){}},
      listTable:()=>({draw(){}}),iconSvg:()=>'',closeMenu(){},
      fetch:async()=>({ok:true,json:async()=>({path:'/data',parent:'/',dirs:[{path:'/data/folder-a',name:'folder-a'}],files:[]})}),
    });
    const api=vm.runInContext(`(()=>{${code.replace(/^import .*;\n/gm,'').replace(/^export /gm,'')}return {openFile,saveFile};})()`,context);
    const pending=api[mode]({folder:'/data',places:[],name:'Synthetic.gpx'});
    await new Promise(setImmediate);
    const dialog=document.body.children[0];
    const where=dialog.all(element=>element.attributes['aria-label']==='Folder path')[0];
    const crumbs=dialog.all(element=>element.classList.contains('file-crumbs'))[0];
    const button=dialog.all(element=>element.textContent==='Edit path')[0];
    assert.equal(button.tagName,'BUTTON');assert.equal(button.type,'button');assert.equal(button.disabled,false);
    assert.equal(where.hidden,true);
    await dialog.fire('keydown',{key:'ArrowDown'});
    for(const key of ['Enter',' ']){
      const event=await dialog.fire('keydown',{target:button,key});
      assert.equal(event.defaultPrevented,undefined,'native button activation stays available');
    }
    await button.fire('click');
    assert.equal(where.hidden,false);assert.equal(crumbs.hidden,true);
    assert.equal(where.value,'/data');assert.equal(where.focused,true);assert.equal(where.textSelected,true);
    await where.fire('keydown',{key:'Escape'});
    assert.equal(where.hidden,true);assert.equal(crumbs.hidden,false);
    dialog.close();assert.equal(await pending,null);
  }
});
