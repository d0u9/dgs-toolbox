'use strict';

// statusBar is the page's handle on the shared status bar, /ui/statusbar.js.
// The box pages are classic scripts and the bar is a module, so it is loaded
// with import(), which resolves after these scripts have run. A call made
// before then — an error while the page starts — is kept and replayed once the
// bar is there, so no news is lost to the load order.
const statusBar = (() => {
  let bar = null;
  const waiting = [];
  import('/ui/statusbar.js').then((module) => {
    module.mount();
    bar = module;
    for (const [name, args] of waiting.splice(0)) bar[name](...args);
  });
  const call = (name) => (...args) => {
    if (bar) bar[name](...args);
    else waiting.push([name, args]);
  };
  const handle = {};
  for (const name of ['show', 'showError', 'clear', 'setState', 'setSummary', 'setHints']) {
    handle[name] = call(name);
  }
  // error takes what a catch holds: an Error or a string. Nothing takes the
  // error away once what caused it is put right, and only an error: news such
  // as a batch's result stays until the next news replaces it.
  let showingError = false;
  const show = handle.show;
  handle.show = (text, options = {}) => {
    showingError = Boolean(text && options.error);
    show(text, options);
  };
  handle.showError = (text) => handle.show(text, { error: true });
  handle.clear = () => handle.show('');
  handle.error = (err) => {
    if (err) handle.showError(err.message || String(err));
    else if (showingError) handle.clear();
  };
  return handle;
})();
