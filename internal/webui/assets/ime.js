// Typing through an input method — pinyin, kana, a dead key — composes a
// word before it is entered: "vehi" is typed as v, ve, veh, vehi, shown
// underlined, and only then given to the field. Every page dgs serves loads
// this first so its handlers see the word, not the pieces: while a word is
// composed, no input or key event reaches them (Enter there picks a
// candidate, not a row), and when it is entered, one input event follows on
// the field, as if it had been typed at once.
//
// The listeners are on window in the capture phase, so they run before any
// page's own and a page needs no code of its own for it.

const composing = (event) => event.isComposing || event.keyCode === 229;

for (const type of ["keydown", "keypress", "keyup", "input", "beforeinput"]) {
  window.addEventListener(type, (event) => {
    if (!composing(event)) return;
    event.stopImmediatePropagation();
    // Tab picks a candidate too; it must not move the focus off the word.
    if (event.type === "keydown" && event.key === "Tab") event.preventDefault();
  }, true);
}

window.addEventListener("compositionend", (event) => {
  // After the browser's own last input event, which some send composing.
  const field = event.target;
  setTimeout(() => field.dispatchEvent(new Event("input", { bubbles: true })), 0);
}, true);
