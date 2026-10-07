import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {Element} from './test_dom.mjs';

test('tooltip closes on pointer movement outside or after its owner is rebuilt',async()=>{
 const listeners=new Map(),timers=new Map(),body=new Element('body');let next=0;
 const node=()=>{const e=new Element('div');e.getBoundingClientRect=()=>({left:0,right:100,top:0,bottom:20,width:100,height:20});e.contains=target=>target===e||target.parentElement===e;return e;};
 const context=vm.createContext({document:{body,createElement:node,addEventListener:(type,fn)=>listeners.set(type,fn),removeEventListener:type=>listeners.delete(type)},
  window:{innerWidth:1000,innerHeight:800,addEventListener(){},removeEventListener(){}},
  Date:{now:()=>1000},setTimeout:fn=>{timers.set(++next,fn);return next;},clearTimeout:id=>timers.delete(id)});
 const source=readFileSync(new URL('./assets/tooltip.js',import.meta.url),'utf8');
 vm.runInContext(source.replace(/^export /gm,''),context);
 const owner=node();context.tooltip(owner,['Synthetic track','20 km']);
 await owner.fire('pointerenter');for(const fn of [...timers.values()])fn();timers.clear();
 const tip=body.children[0];assert.equal(tip.hidden,false);
 listeners.get('pointermove')({target:owner});assert.equal(tip.hidden,false);
 listeners.get('pointermove')({target:node()});assert.equal(tip.hidden,true);assert.equal(listeners.has('pointermove'),false);
 await owner.fire('pointerenter');assert.equal(tip.hidden,false);owner.removed=true;
 listeners.get('pointermove')({target:owner});assert.equal(tip.hidden,true);
 const other=node();context.tooltip(other,['Another synthetic track']);await other.fire('pointerenter');assert.equal(tip.hidden,false);
 await other.fire('pointerleave');assert.equal(tip.hidden,true);
});
