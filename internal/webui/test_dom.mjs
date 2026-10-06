// A minimal DOM boundary: the production code creates controls and registers
// its real listeners. No rendering, layout or browser defaults are emulated.
export class Element {
  constructor(tag) {
    this.tagName=tag.toUpperCase();this.children=[];this.style={};this.dataset={};
    this.attributes={};this.listeners=new Map();this.className='';this.disabled=false;
    this.classList={
      contains: name => this.className.split(/\s+/).includes(name),
      add: (...names) => {this.className=[...new Set([...this.className.split(/\s+/).filter(Boolean),...names])].join(' ');},
      remove: (...names) => {this.className=this.className.split(/\s+/).filter(name=>!names.includes(name)).join(' ');},
      toggle: (name,on) => {const yes=on??!this.classList.contains(name);this.classList[yes?'add':'remove'](name);return yes;},
    };
  }
  closest(selector){
    for(let node=this;node;node=node.parentElement){
      if(selector.split(',').some(part=>{const value=part.trim();return value.startsWith('.')?node.classList.contains(value.slice(1)):node.tagName===value.toUpperCase();}))return node;
    }
    return null;
  }
  removeAttribute(name){delete this.attributes[name];}
  focus(){this.focused=true;}
  select(){this.textSelected=true;}
  showModal(){this.open=true;}
  close(){this.open=false;for(const listener of this.listeners.get('close')||[])listener({target:this});}
  remove(){this.removed=true;if(this.parentElement)this.parentElement.children=this.parentElement.children.filter(child=>child!==this);}
  get isConnected(){return !this.removed;}
  get firstElementChild(){return this.children[0];}
  append(...children){for(const child of children)if(child instanceof Element)child.parentElement=this;this.children.push(...children);}
  replaceChildren(...children){this.children=children;}
  setAttribute(name,value){this.attributes[name]=value;}
  addEventListener(type,listener){const listeners=this.listeners.get(type)||[];listeners.push(listener);this.listeners.set(type,listeners);}
  async fire(type,values={}){
    const event={target:this,timeStamp:100,clientX:10,clientY:10,detail:1,stopPropagation(){this.stopped=true;},preventDefault(){this.defaultPrevented=true;},...values};
    for(const listener of this.listeners.get(type)||[])await listener(event);
    return event;
  }
  all(predicate){return [this,...this.children.flatMap(child=>child instanceof Element?child.all(predicate):[])].filter(predicate);}
}
export const documentStub=()=>({createElement:tag=>new Element(tag),body:new Element("body"),addEventListener(){},removeEventListener(){}});
