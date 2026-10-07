// Doc 2 folder forms. The server owns matching, numbering and validity.
import { $, api, el, label, post, frame, say, exportOutline } from "/common.js";

export async function startSkeleton(state, kind) {
  const isRule=kind==="rules";
  frame(state);
  let definitions=[],rules=[],snapshots=[],draft=null,editing="",saved="",sequence=0,timer=0,grouping=null;
  const host=$(isRule?"rules":"outlines"),form=$("form");
  const text=()=>JSON.stringify(draft);
  const input=(title,value,change,placeholder="")=>{
    const node=el("input",{value:value||"",placeholder,autocomplete:"off",spellcheck:false});
    node.oninput=()=>{change(node.value);changed();};
    return el("label",{className:"vf"},el("span",{className:"vf-label"},title),node);
  };
  const select=(title,value,options,change)=>{
    const node=el("select",{},...options.map(([v,name])=>el("option",{value:v,textContent:name})));
    node.value=value||options[0][0];node.onchange=()=>{change(node.value);changed();};
    return el("label",{className:"vf"},el("span",{className:"vf-label"},title),node);
  };
  const button=(name,action)=>el("button",{className:"button secondary",type:"button",textContent:name,onclick:action});
  const defaults=(value,inherited)=>el("div",{className:"view-group"},
    select("Revisions",value.revision,[...(inherited?[["","Inherit"]]:[]),["head","HEAD"],["all","All revisions"]],v=>{value.revision=v;}),
    select("Validity",value.valid,[["",inherited?"Inherit":"Any"],["today","Current today"],["expired","Expired or superseded"]],v=>{value.valid=v;}));
  const move=(list,index,step)=>{const target=index+step;if(target<0||target>=list.length)return;[list[index],list[target]]=[list[target],list[index]];draw();changed();};
  function folders(list) {
    const body=el("div",{className:"vf-stack"});
    list.forEach((f,index)=>{
      f.defaults||={};f.folders||=[];
      const summary=el("summary",{},f.name||"Folder");
      const row=el("details",{open:true,className:"view-group"},summary,
        input("Folder name",f.name,v=>{f.name=v;summary.textContent=v||"Folder";}),
        input("Take types",(f.take||[]).join(", "),v=>{f.take=v.split(",").map(x=>x.trim()).filter(Boolean);},"id_card, passport"),
        input("If",f.if,v=>{f.if=v;},"country == 中国"),
        input("Each",f.each,v=>{f.each=v;},"owner, issuer, tenancy…"),
        input("File name",f.file,v=>{f.file=v;},"{#}-{name}"),
        input("Order",(f.order||[]).join(", "),v=>{f.order=v.split(",").map(x=>x.trim()).filter(Boolean);},"Names in order, or date"),
        defaults(f.defaults,true),
        el("div",{className:"checks"},button("↑",()=>move(list,index,-1)),button("↓",()=>move(list,index,1)),button("Remove folder",()=>{list.splice(index,1);draw();changed();})),folders(f.folders));
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
      body.append(el("section",{className:"view-group"},el("label",{className:"vf"},el("span",{className:"vf-label"},"Mount"),pick),
        input("At folder",m.at,v=>{m.at=v;},"11-Rental"),input("As name",m.as,v=>{m.as=v;}),
        el("div",{className:"checks"},button("↑",()=>move(draft.mount,index,-1)),button("↓",()=>move(draft.mount,index,1)),button("Remove mount",()=>{draft.mount.splice(index,1);draw();changed();}))));
    });
    body.append(button("Add mount",()=>{draft.mount.push({});draw();changed();}));
    return body;
  }
  function draw() {
    form.replaceChildren(el("section",{className:"view-group"},
      input("Name",draft.name,v=>{draft.name=v;$("title").textContent=v;}),
      isRule?null:input("For",draft.about,v=>{draft.about=v;}),defaults(draft.defaults||={},false)),
      isRule?folders(draft.folders):mounts(),
      el("div",{className:"view-actions"},el("button",{className:"action",type:"submit",textContent:isRule?"Save rule":"Save Outline"}),
        isRule?null:button("Export to…",()=>{if(!editing||text()!==saved){say($("form-message"),"Save the Outline before exporting.",true);return;}exportOutline(state,{...draft,default:definitions.find(o=>o.name===editing)?.default},{say:(message,bad)=>say($("form-message"),message,bad)});}),
        button("Delete…",async()=>{if(!editing||!confirm("Move this definition to trash?"))return;try{await post("/api/defs/"+kind+"/trash",{name:editing});await load();open("");}catch(e){say($("form-message"),e.message,true);}}),
        el("p",{id:"form-message",className:"message"})));
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
    const item=state.items.find(i=>i.id===file.item);
    const row=el("li",{draggable:!isRule},el("a",{href:api("/browse/"+(file.revision?"?digest="+encodeURIComponent(item?.revisions[file.revision-1]?.id||file.digest):""))+"#"+encodeURIComponent(file.item),textContent:file.path||label(state,item||{id:file.item,fields:{}})}));
    row.ondragstart=e=>e.dataTransfer.setData("text/plain",file.item);
    row.append(button("Why here",async()=>{
      try{const xs=await post("/api/outlines/explain",{outline:previewOutline(),item:file.item});say($("form-message"),xs.map(x=>x.text).join(" · "));}catch(e){say($("form-message"),e.message,true);}
    }));
    if(!isRule&&file.path){
      const folder=file.path.includes("/")?file.path.slice(0,file.path.lastIndexOf("/")):"";
      const pin=draft.pins?.[folder];
      row.append(button("↑",()=>reorderItem(folder,file.item,-1)),button("↓",()=>reorderItem(folder,file.item,1)));
      const key=Object.hasOwn(pin?.numbers||{},file.item)?file.item:(file.numberName||file.item);
      row.append(input("Fixed number",pin?.numbers?.[key]||"",value=>{
        draft.pins||={};const p=draft.pins[folder]||={items:[],numbers:{}};p.numbers||={};
        if(value.trim()==="")delete p.numbers[key];else p.numbers[key]=Number(value);
      },"Automatic"));
    }
    return row;
  }
  function folderNode(node) {
    const body=el("ul",{},...(node.files||[]).map(itemRow),...(node.children||[]).map(folderNode));
    const row=el("li",{},el("details",{open:true},el("summary",{},node.name||"Outline"),body));
    if(!isRule&&node.path){
      row.ondragover=e=>{e.preventDefault();e.stopPropagation();};
      row.ondrop=async e=>{
        e.preventDefault();e.stopPropagation();const id=e.dataTransfer.getData("text/plain");
        if(!state.items.some(i=>i.id===id))return;
        draft.pins||={};for(const p of Object.values(draft.pins))p.items=(p.items||[]).filter(x=>x!==id);
        const pin=draft.pins[node.path]||={items:[],numbers:{}};pin.items||=[];pin.items.push(id);
        changed();await preview();
        say($("form-message"),"Pinned here. Save the Outline to keep it.");
      };
    }
    return row;
  }
  const previewOutline=()=>isRule?{name:"preview",mount:[{rule:draft.name}],rules:[draft],defaults:{}}:draft;
  async function preview() {
    const request=++sequence;
    try{
      const g=await post("/api/outlines/group",previewOutline());if(request!==sequence)return;grouping=g;
      $("tree").replaceChildren(el("ul",{},folderNode(g.root)));
      const unplaced=(g.unplaced||[]).map(id=>itemRow({item:id}));
      if(unplaced.length)$("tree").append(el("h3",{},"Not placed"),el("ul",{},...unplaced));
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
    sequence++;draw();saved=text();$("dirty").hidden=true;$("title").textContent=draft.name;
    history.replaceState(history.state,"",name?"#"+encodeURIComponent(name):location.pathname+location.search);
    preview();
  }
  async function load() {
    const [answer,fixed]=await Promise.all([(await fetch(api("/api/defs/"+kind))).json(),(await fetch(api("/api/defs/snapshots"))).json()]);
    if(answer.error)throw new Error(answer.error);
    rules=answer.rules||[];snapshots=fixed.snapshots||[];
    definitions=isRule?rules:answer.outlines||[];
    host.replaceChildren(...definitions.map(d=>button(d.name,()=>open(d.name))),button("New "+(isRule?"rule":"Outline"),()=>open("")));
    $("count").textContent=definitions.length;
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
