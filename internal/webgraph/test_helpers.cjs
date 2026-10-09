const ELK = require("./web/vendor/elk.bundled.js");
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, 'web/app.js'), 'utf8');
function model(graph, view = 'network') {
  const context = vm.createContext({ELK: class {
    constructor() { this.engine = new ELK(); }
    // ELK's GWT runtime expects arrays from its own JavaScript realm.
    layout(input) { return this.engine.layout(JSON.parse(JSON.stringify(input))); }
  }, console, setTimeout, clearTimeout, setInterval, clearInterval});
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'web/vendor/d3.min.js'), 'utf8'), context);
  const helpers = source.slice(0, source.indexOf('let graph, all'));
  const geometry = source.slice(source.indexOf('function visibleTags'), source.indexOf('async function render('));
  context.input = graph;
  context.mode = view;
  vm.runInContext(`${helpers}\nlet graph = input, view = mode, includeDeclarations = false;
    const layouts = new Map(); const elk = new ELK(); let all = elements(graph);
    filterOn = new Set((graph.edges || []).flatMap(e => e.tags || []).concat((graph.nodes || []).flatMap(n => n.tags || [])));
    ${geometry}`, context);
  return context;
}
async function run(code, context) {
  code = code.replace(/geometry\(([^)]*)\)/g, '(await geometry($1))');
  const last = code.lastIndexOf(';')+1;
  return vm.runInContext(`(async () => { ${code.slice(0,last)} return (${code.slice(last)}); })()`, context);
}

module.exports = {model, run, source};
