// A modal confirmation, for anything that changes a file the reader already
// has. It says exactly what will change and waits for a clear yes.

// confirmDialog resolves true only when the confirm button is pressed.
export function confirmDialog({ title, message, detail, confirm = "Continue", cancel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "confirm-dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;
    dialog.append(heading, text);
    if (detail) {
      const code = document.createElement("code");
      code.className = "confirm-detail";
      code.textContent = detail;
      dialog.append(code);
    }
    const buttons = document.createElement("div");
    buttons.className = "confirm-buttons";
    const no = document.createElement("button");
    no.className = "text-button";
    no.textContent = cancel;
    const yes = document.createElement("button");
    yes.className = "chip active" + (danger ? " danger" : "");
    yes.textContent = confirm;
    buttons.append(no, yes);
    dialog.append(buttons);
    let answer = false;
    no.addEventListener("click", () => dialog.close());
    yes.addEventListener("click", () => {
      answer = true;
      dialog.close();
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      resolve(answer);
    });
    document.body.append(dialog);
    dialog.showModal();
    no.focus(); // Enter does not confirm by accident
  });
}
