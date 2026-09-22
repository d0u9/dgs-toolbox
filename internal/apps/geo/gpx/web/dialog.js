// A modal confirmation, for anything that changes a file the reader already
// has. It says exactly what will change and waits for a clear yes.


// waypointDialog asks for the destination and details before writing a point.
export function waypointDialog(entries, coordinates) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = "Add waypoint";
    const place = document.createElement("p");
    place.textContent = `${coordinates[1].toFixed(6)}, ${coordinates[0].toFixed(6)} (WGS-84)`;
    const form = document.createElement("form");
    form.method = "dialog";
    const target = document.createElement("select");
    target.className = "cut-path prompt-input";
    for (const entry of entries) {
      const option = document.createElement("option");
      option.value = entry.path;
      option.textContent = `${entry.name} — ${entry.path}`;
      target.append(option);
    }
    const name = document.createElement("input");
    name.className = "cut-path prompt-input";
    name.placeholder = "Waypoint name";
    name.required = true;
    const description = document.createElement("input");
    description.className = "cut-path prompt-input";
    description.placeholder = "Description (optional)";
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "text-button";
    cancel.textContent = "Cancel";
    const save = document.createElement("button");
    save.type = "submit";
    save.className = "chip active";
    save.textContent = "Add";
    buttons.append(cancel, save);
    form.append(target, name, description, buttons);
    dialog.append(heading, place, form);
    let answer = null;
    cancel.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", () => {
      if (name.value.trim()) answer = { path: target.value, name: name.value.trim(), description: description.value.trim() };
    });
    dialog.addEventListener("close", () => { dialog.remove(); resolve(answer); });
    document.body.append(dialog);
    dialog.showModal();
    name.focus();
  });
}

// promptDialog asks for one line of text, such as a file path, and resolves
// to it, or to null when cancelled.
export function promptDialog({ title, message, value = "", confirm = "OK", cancel = "Cancel" }) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;
    const form = document.createElement("form");
    form.method = "dialog";
    const input = document.createElement("input");
    input.className = "cut-path prompt-input";
    input.value = value;
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
    const no = document.createElement("button");
    no.type = "button";
    no.className = "text-button";
    no.textContent = cancel;
    const yes = document.createElement("button");
    yes.type = "submit";
    yes.className = "chip active";
    yes.textContent = confirm;
    buttons.append(no, yes);
    form.append(input, buttons);
    dialog.append(heading, text, form);
    let answer = null;
    no.addEventListener("click", () => dialog.close());
    form.addEventListener("submit", () => {
      if (input.value.trim()) answer = input.value.trim();
    });
    dialog.addEventListener("close", () => {
      dialog.remove();
      resolve(answer);
    });
    document.body.append(dialog);
    dialog.showModal();
    input.focus();
    input.select();
  });
}

// confirmDialog resolves true only when the confirm button is pressed.
export function confirmDialog({ title, message, detail, confirm = "Continue", cancel = "Cancel", danger = false }) {
  return new Promise((resolve) => {
    const dialog = document.createElement("dialog");
    dialog.className = "dialog";
    const heading = document.createElement("h3");
    heading.textContent = title;
    const text = document.createElement("p");
    text.textContent = message;
    dialog.append(heading, text);
    if (detail) {
      const code = document.createElement("code");
      code.className = "dialog-detail";
      code.textContent = detail;
      dialog.append(code);
    }
    const buttons = document.createElement("div");
    buttons.className = "dialog-buttons";
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
