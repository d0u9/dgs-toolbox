// The button that shows a file in this machine's file manager.

import { api } from "./api.js";
import { FOLDER } from "./icons.js";

// revealButton opens the file manager at path. It exists only when the page
// runs on the machine serving it.
export function revealButton(path) {
  const button = document.createElement("button");
  button.className = "row-action";
  button.innerHTML = FOLDER;
  button.title = "Show in file manager";
  button.setAttribute("aria-label", "Show in file manager");
  button.addEventListener("click", async (event) => {
    event.stopPropagation();
    try {
      await api.reveal(path);
    } catch (error) {
      button.title = error.message;
    }
  });
  return button;
}
