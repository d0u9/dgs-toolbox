// Doc 2 folder forms. The server owns matching, numbering and validity.
import { $, api, el, label, post, frame, say, exportOutline, nameTree, currentFields } from "/common.js";

import { Folded, resizable } from "/outlinetree.js";
import { fileTree } from "/ui/filetree.js";
import { iconSvg } from "/ui/icons.js";

export async function startSkeleton(state, kind) {
  const isRule=kind==="rules";
  frame(state);
  let definitions=[],rules=[],snapshots=[],draft=null,editing="",saved="",sequence=0,timer=0,grouping=null;
  const host=$(isRule?"rules":"outlines"),form=$("form");
  const closed=new Folded();
  resizable(document.querySelector(".outline-main"), "dgs-doc-"+kind+"-tree");
  form.className=(form.className||"")+" skeleton-form";
  const text=()=>JSON.stringify(draft);
  const list=()=>nameTree(host,definitions.map(d=>({name:d.name,note:(isRule?d.folders:d.mount)?.length||0,title:d.about||""})),{selected:editing,onPick:d=>open(d.name),unsaved:draft&&!editing?"New "+(isRule?"rule":"Outline"):""});
  const input=(title,value,change,placeholder="",options=null)=>{
    const node=el("input",{value:value||"",placeholder,autocomplete:"off",spellcheck:false});
    node.oninput=()=>{change(node.value);changed();};
    return el("label",{className:"vf"},el("span",{className:"vf-label"},title),node,options?complete(node,options):null);
  };
  // complete lists under an input what may follow its text: the text so far
  // with each candidate for the word being typed. options(text) answers
  // [the text before that word, the candidates].
  let lists=0;
  const complete=(node,options)=>{
    const list=el("datalist",{id:"complete-"+(++lists)});
    node.setAttribute("list",list.id);
    const fill=()=>{const [head,candidates]=options(node.value);list.replaceChildren(...[...new Set(candidates)].slice(0,200).map(c=>el("option",{value:head+c})));};
    const typed=node.oninput;node.oninput=event=>{typed?.(event);fill();};node.onfocus=fill;
    return list;
  };
  const lineage=type=>(state.templates||[]).find(t=>t.type===type)?.lineage||[type];
  const allTypes=()=>[...new Set((state.templates||[]).flatMap(t=>[t.type,...(t.lineage||[])]))].sort();
  // A folder's context is the Items its take, and every take above it, admit.
  const itemsFor=takes=>(state.items||[]).filter(i=>i.revisions).filter(i=>takes().every(take=>!take.length||lineage(i.type).some(t=>take.includes(t))));
  const keysFor=takes=>{
    const types=new Set(itemsFor(takes).map(i=>i.type)),out=new Set(["type","tags"]);
    for(const t of state.templates||[])if(!types.size||types.has(t.type))for(const f of t.fields||[])out.add(f.key);
    return [...out].sort();
  };
  const valuesFor=(key,takes)=>{
    if(key==="type")return allTypes();
    const out=new Set();
    for(const i of itemsFor(takes)){
      const v=key==="tags"?(i.tags||[]).join(", "):currentFields(i)[key];
      if(v)v.split(", ").forEach(x=>out.add(x));
    }
    for(const t of state.templates||[])for(const f of t.fields||[])if(f.key===key)(f.options||[]).forEach(o=>out.add(o));
    return [...out].sort();
  };
  const commaList=all=>text=>{const i=text.lastIndexOf(","),done=text.split(",").map(x=>x.trim());return [i<0?"":text.slice(0,i+1)+" ",all().filter(x=>!done.includes(x))];};
  const quote=v=>/[\s,()\[\]"!=~&|]/.test(v)?JSON.stringify(v):v;
  const conditionOptions=takes=>text=>{
    const word=text.match(/"[^"]*$|[^\s()\[\],!=~&|"]*$/)[0],head=text.slice(0,text.length-word.length);
    const compared=head.match(/([\w.]+)\s*(?:==|!=|~)\s*$/)||head.match(/([\w.]+)\s+in\s*\[[^\]]*$/);
    return [head,compared?valuesFor(compared[1],takes).map(quote):keysFor(takes)];
  };
  const layoutOptions=takes=>text=>{
    const open=text.match(/\{[^{}]*$/);
    return open?[text.slice(0,open.index+1),["#",...keysFor(takes)].map(k=>k+"}")]:[text,[]];
  };
  const select=(title,value,options,change)=>{
    const node=el("select",{},...options.map(([v,name])=>el("option",{value:v,textContent:name})));
    node.value=value||options[0][0];node.onchange=()=>{change(node.value);changed();};
    return el("label",{className:"vf"},el("span",{className:"vf-label"},title),node);
  };
  const button=(name,action)=>el("button",{className:"button secondary",type:"button",textContent:name,onclick:action});
  const defaults=(value,inherited)=>el("div",{className:"vf-stack"},
    select("Revisions",value.revision,[...(inherited?[["","Inherit"]]:[]),["head","HEAD"],["all","All revisions"]],v=>{value.revision=v;}),
    select("Validity",value.valid,[["",inherited?"Inherit":"Any"],["today","Current today"],["expired","Expired or superseded"]],v=>{value.valid=v;}));
  const move=(list,index,step)=>{const target=index+step;if(target<0||target>=list.length)return;[list[index],list[target]]=[list[target],list[index]];draw();changed();};
  function folders(list,above=()=>[]) {
    const body=el("div",{className:"vf-stack rule-folders"});
    list.forEach((f,index)=>{
      f.defaults||={};f.folders||=[];
      // takes is read when asked, so a type just typed counts.
      const takes=()=>[...above(),f.take||[]];
      const name=el("input",{className:"folder-name",value:f.name||"",placeholder:"Folder name",autocomplete:"off",spellcheck:false,"aria-label":"Folder name"});
      name.setAttribute("aria-label","Folder name");
      name.onclick=event=>event.stopPropagation();
      name.onkeydown=event=>event.stopPropagation();
      name.oninput=()=>{f.name=name.value;changed();};
      const action=(title,run)=>button(title,event=>{event.preventDefault();event.stopPropagation();run();});
      const up=action("↑",()=>move(list,index,-1)),down=action("↓",()=>move(list,index,1));
      up.title="Move folder up";down.title="Move folder down";
      up.setAttribute("aria-label",up.title);down.setAttribute("aria-label",down.title);
      up.disabled=index===0;down.disabled=index===list.length-1;
      const summary=el("summary",{className:"folder-heading"},name,
        el("span",{className:"folder-actions"},up,down,action("Remove folder",()=>{list.splice(index,1);draw();changed();})),complete(name,layoutOptions(takes)));
      const options=el("div",{className:"folder-fields"},
        input("Take types",(f.take||[]).join(", "),v=>{f.take=v.split(",").map(x=>x.trim()).filter(Boolean);},"Types to include",commaList(allTypes)),
        input("If",f.if,v=>{f.if=v;},"Optional condition",conditionOptions(takes)),
        input("Each",f.each,v=>{f.each=v;},"Group by a key",text=>["",[...keysFor(takes),...allTypes()]]),
        input("Order",(f.order||[]).join(", "),v=>{f.order=v.split(",").map(x=>x.trim()).filter(Boolean);},"Names in order, or date",commaList(()=>f.each?valuesFor(f.each,takes):["date"])),
        input("Start at",f.start||"",v=>{const n=parseInt(v,10);if(n>0)f.start=n;else delete f.start;},"1"),
        input("File name",f.file,v=>{f.file=v;},"{#}-{name}",layoutOptions(takes)),
        defaults(f.defaults,true));
      const row=el("details",{open:true,className:"folder-form"},summary,
        el("div",{className:"folder-branch"},options,folders(f.folders,takes)));
      body.append(row);
    });
    body.append(button("Add folder",()=>{list.push({name:"New folder",take:[],folders:[]});draw();changed();}));
    return body;
  }
  function mounts() {
    const body=el("div",{className:"vf-stack"});
    draft.mount.forEach((m,index)=>{
      const pick=el("select",{},el("option",{value:"",textContent:"Choose a rule or Snapshot"}),
        ...rules.map(r=>el("option",{value:"rule:"+r.name,textContent:"Rule · "+r.name})),
        ...snapshots.map(s=>el("option",{value:"snapshot:"+s.name,textContent:"Snapshot · "+s.name})));
      pick.value=m.rule?"rule:"+m.rule:m.snapshot?"snapshot:"+m.snapshot:"";
      pick.onchange=()=>{delete m.rule;delete m.snapshot;const [type,name]=pick.value.split(":");if(name)m[type]=name;changed();};
      body.append(el("section",{className:"folder-form"},el("label",{className:"vf"},el("span",{className:"vf-label"},"Mount"),pick),
        input("At folder",m.at,v=>{m.at=v;},"11-Rental"),input("As name",m.as,v=>{m.as=v;}),
        el("div",{className:"checks"},button("↑",()=>move(draft.mount,index,-1)),button("↓",()=>move(draft.mount,index,1)),button("Remove mount",()=>{draft.mount.splice(index,1);draw();changed();}))));
    });
    body.append(button("Add mount",()=>{draft.mount.push({});draw();changed();}));
    return body;
  }
  function draw() {
    form.replaceChildren(el("section",{className:"view-group"},
      input("Name",draft.name,v=>{draft.name=v;$("title").textContent=v;}),
      isRule?null:input("For",draft.about,v=>{draft.about=v;}),defaults(draft.defaults||={},false),
      isRule?null:el("div",{className:"vf"},el("span",{className:"vf-label"},"Export"),button("Export to…",()=>{if(!editing||text()!==saved){say($("form-message"),"Save the Outline before exporting.",true);return;}exportOutline(state,{...draft,default:definitions.find(o=>o.name===editing)?.default},{say:(message,bad)=>say($("form-message"),message,bad)});}))),
      el("section",{className:"view-group"},el("h3",{},isRule?"Folders":"Rules and Snapshots"),isRule?folders(draft.folders):mounts()),
      el("div",{className:"view-actions"},el("button",{className:"action",type:"submit",textContent:isRule?"Save rule":"Save Outline"}),
        el("p",{id:"form-message",className:"message"}),
        el("button",{className:"button danger",type:"button",textContent:"Delete…",hidden:!editing,onclick:async()=>{if(!editing||!confirm("Move this definition to trash?"))return;try{await post("/api/defs/"+kind+"/trash",{name:editing});await load();open("");}catch(e){say($("form-message"),e.message,true);}}})));
  }
  function changed() {
    $("dirty").hidden=text()===saved;
    clearTimeout(timer);timer=setTimeout(preview,180);
  }
  function reorderItem(folder,id,step){
    let node=null;
    const find=n=>{if(n.path===folder)node=n;for(const c of n.children||[])find(c);};find(grouping.root);
    if(!node)return;
    const ids=[...new Set(node.files.map(f=>f.item))];const index=ids.indexOf(id),target=index+step;
    if(index<0||target<0||target>=ids.length)return;
    [ids[index],ids[target]]=[ids[target],ids[index]];
    draft.pins||={};const pin=draft.pins[folder]||={numbers:{}};pin.items=ids;changed();
  }
  function itemRow(file) {
    const row=el("span",{className:"ft-extras"});
    const why=button("?",async()=>{
      try{const xs=await post("/api/outlines/explain",{outline:previewOutline(),item:file.item});say($("form-message"),xs.map(x=>x.text).join(" · "));}catch(e){say($("form-message"),e.message,true);}
    });
    why.className="ft-reveal";why.title="Why here";why.setAttribute("aria-label","Why here");row.append(why,el("button",{type:"button",className:"ft-reveal",title:"Show in Finder","aria-label":"Show in Finder",innerHTML:iconSvg("square-arrow-out-up-right"),onclick:()=>post("/api/reveal",{item:file.item,digest:file.digest}).catch(e=>say($("form-message"),e.message,true))}));
    if(!isRule&&file.path){
      const folder=file.path.includes("/")?file.path.slice(0,file.path.lastIndexOf("/")):"";
      const pin=draft.pins?.[folder];
      const up=button("↑",()=>reorderItem(folder,file.item,-1)),down=button("↓",()=>reorderItem(folder,file.item,1));
      up.className=down.className="ft-reveal";up.title="Move earlier";down.title="Move later";row.append(up,down);
      const key=Object.hasOwn(pin?.numbers||{},file.item)?file.item:(file.numberName||file.item);
      const number=input("Fixed number",pin?.numbers?.[key]||"",value=>{
        draft.pins||={};const p=draft.pins[folder]||={items:[],numbers:{}};p.numbers||={};
        if(value.trim()==="")delete p.numbers[key];else p.numbers[key]=Number(value);
      },"Auto");
      number.className="outline-number";number.lastChild.type="number";number.lastChild.min="1";number.lastChild.setAttribute("aria-label","Fixed number");number.lastChild.title="Leave blank for automatic numbering";row.append(number);
    }
    return row;
  }
  function drawTree(g) {
    const nodes=new Map();
    const walk=node=>{if(node.path)nodes.set(node.path+"/",node);for(const child of node.children||[])walk(child);};
    walk(g.root);
    $("tree").replaceChildren(fileTree(g.files,{
      closed,folders:[...nodes.keys()],
      href:file=>{const item=state.items.find(i=>i.id===file.item);return api("/browse/"+(file.revision?"?digest="+encodeURIComponent(item?.revisions[file.revision-1]?.id||file.digest):""))+"#"+encodeURIComponent(file.item);},
      folderExtra:path=>el("span",{className:"numeric"},String(nodes.get(path)?.count??"")),
      folderIcon:path=>nodes.get(path)?.snapshot?iconSvg("camera"):"",
      folderClass:path=>nodes.get(path)?.snapshot?"outline-snapshot":"",
      fileExtra:itemRow,
      decorate:(row,{kind:rowKind,path,file})=>{
        if(isRule)return;
        if(rowKind==="file"){
          row.draggable=true;row.ondragstart=e=>e.dataTransfer.setData("text/plain",file.item);
          return;
        }
        row.ondragover=e=>{e.preventDefault();e.stopPropagation();};
        row.ondrop=async e=>{
          e.preventDefault();e.stopPropagation();const id=e.dataTransfer.getData("text/plain");
          if(!state.items.some(i=>i.id===id))return;
          draft.pins||={};for(const p of Object.values(draft.pins))p.items=(p.items||[]).filter(x=>x!==id);
          const pin=draft.pins[path.slice(0,-1)]||={items:[],numbers:{}};pin.items||=[];pin.items.push(id);
          changed();await preview();say($("form-message"),"Pinned here. Save the Outline to keep it.");
        };
      },
    }));
    if(!g.files.length&&!nodes.size)$("tree").append(el("p",{className:"muted outline-empty"},"No Item matches."));
  }
  const previewOutline=()=>isRule?{name:"preview",mount:[{rule:draft.name}],rules:[draft],defaults:{}}:draft;
  async function preview() {
    const request=++sequence;
    try{
      const g=await post("/api/outlines/group",previewOutline());if(request!==sequence)return;grouping=g;
      // A condition half typed fails; the next good preview clears its error.
      if(String($("form-message").className).includes("error"))say($("form-message"),"");
      drawTree(g);
      // A rule or Outline takes what it is for; the rest is listed, folded,
      // to look through, not flagged as a fault.
      const unplaced=g.unplaced||[];
      $("unplaced").replaceChildren(...(unplaced.length?[el("details",{className:"outside"},el("summary",{},unplaced.length+" Items outside this "+(isRule?"rule":"Outline")),el("ul",{className:"outline-files"},...(g.unplaced||[]).map(id=>el("li",{},el("a",{href:api("/browse/")+"#"+encodeURIComponent(id)},label(state,state.items.find(i=>i.id===id)||{id,fields:{}})),itemRow({item:id})))) )]:[]));
      for(const [rule,p]of Object.entries(g.plans||{})){
        for(const missing of p.missing||[])$("tree").append(el("p",{className:"message error"},rule+": "+missing.item+" lacks "+missing.keys.join(", ")));
        for(const clash of p.clashes||[])$("tree").append(el("p",{className:"message error"},"Conflicting path: "+clash.path));
      }
      for(const clash of g.clashes||[])$("tree").append(el("p",{className:"message error"},"Conflicting path: "+clash.path));
      $("total").textContent=g.files.length+" files";
    }catch(e){if(request===sequence)say($("form-message"),e.message,true);}
  }
  function open(name) {
    if(draft&&text()!==saved&&!confirm("Drop unsaved changes?"))return;
    editing=name;
    const stored=definitions.find(d=>d.name===name);
    draft=stored?JSON.parse(JSON.stringify(stored)):isRule?{name:"new-rule",folders:[],defaults:{}}:{name:"new-outline",mount:[],defaults:{revision:"head",valid:"today"},pins:{}};
    delete draft.default;if(!isRule)delete draft.rules;
    sequence++;closed.load(state.root+":"+kind+":"+name);draw();saved=text();list();$("dirty").hidden=true;$("title").textContent=draft.name;
    history.replaceState(history.state,"",name?"#"+encodeURIComponent(name):location.pathname+location.search);
    preview();
  }
  async function load() {
    const [answer,fixed]=await Promise.all([(await fetch(api("/api/defs/"+kind))).json(),(await fetch(api("/api/defs/snapshots"))).json()]);
    if(answer.error)throw new Error(answer.error);
    rules=answer.rules||[];snapshots=fixed.snapshots||[];
    definitions=isRule?rules:answer.outlines||[];
    list();
    $("count").textContent=definitions.length;
    nowhere();
  }
  // nowhere lists the Items no saved Outline places, under the Outlines.
  async function nowhere() {
    const box=$("nowhere");
    if(isRule||!box)return;
    try{
      const groups=await Promise.all(definitions.map(o=>post("/api/outlines/group",o)));
      const placed=new Set(groups.flatMap(g=>g.files.map(f=>f.item)));
      const ids=state.items.map(i=>i.id).filter(id=>!placed.has(id));
      box.replaceChildren(...(ids.length?[el("details",{className:"outside"},el("summary",{},ids.length+" Items in no Outline"),
        el("ul",{className:"outline-files"},...ids.map(id=>el("li",{},el("a",{href:api("/browse/")+"#"+encodeURIComponent(id)},label(state,state.items.find(i=>i.id===id)))))))]:[]));
    }catch(e){box.replaceChildren(el("p",{className:"message error"},e.message));}
  }
  form.onsubmit=async e=>{
    e.preventDefault();
    try{
      await post("/api/defs/"+kind,{[isRule?"rule":"outline"]:draft,previous:editing});
      editing=draft.name;saved=text();await load();$("dirty").hidden=true;
      say($("form-message"),"Saved.");await preview();
    }catch(error){say($("form-message"),error.message,true);}
  };
  $("new").onclick=()=>open("");
  await load();open(decodeURIComponent(location.hash.slice(1)));
}
