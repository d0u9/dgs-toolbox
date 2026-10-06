// GPX-specific fields use the shared modal lifecycle and action styles.
import { formDialog } from "/ui/dialog.js";

export function waypointDialog(entries, coordinates) {
  const target = document.createElement("select"); target.className = "field prompt-input"; target.setAttribute("aria-label", "Target GPX");
  for (const entry of entries) {
    const option = document.createElement("option"); option.value = entry.path; option.textContent = `${entry.name} — ${entry.path}`; target.append(option);
  }
  const name = document.createElement("input"); name.className = "field prompt-input"; name.placeholder = "Waypoint name"; name.required = true; name.setAttribute("aria-label", "Waypoint name");
  const description = document.createElement("input"); description.className = "field prompt-input"; description.placeholder = "Description (optional)"; description.setAttribute("aria-label", "Description");
  return formDialog({ title: "Add waypoint", message: `${coordinates[1].toFixed(6)}, ${coordinates[0].toFixed(6)} (WGS-84)`, fields: [target, name, description], focus: name, confirm: "Add", read: () => name.value.trim() ? { path: target.value, name: name.value.trim(), description: description.value.trim() } : null });
}


export function segmentSaveDialog({ targets, target, source, segments }) {
  const destination = document.createElement("select"); destination.className = "field prompt-input"; destination.setAttribute("aria-label", "Save to");
  for (const path of targets) {
    const option = document.createElement("option"); option.value = path; option.textContent = `${path === source ? "Current GPX — " : ""}${path.split(/[\\/]/).pop()}`; option.title = path; destination.append(option);
  }
  destination.value = targets.includes(target) ? target : targets[0] || "";
  const mode = document.createElement("select"); mode.className = "field prompt-input"; mode.setAttribute("aria-label", "Add as");
  for (const [value, text] of [["separate", "Separate tracks"], ["merge", "Merge into one track"]]) {
    const option = document.createElement("option"); option.value = value; option.textContent = text; mode.append(option);
  }
  mode.value = "separate";
  const name = document.createElement("input"); name.className = "field prompt-input"; name.setAttribute("aria-label", "Track name"); name.value = segments.length === 1 ? segments[0].name : "Merged track";
  const field = (text, input) => { const label = document.createElement("label"); label.textContent = text; label.append(input); return label; };
  const nameField = field("Track name", name);
  const sync = () => { nameField.hidden = segments.length > 1 && mode.value !== "merge"; name.required = !nameField.hidden; };
  mode.addEventListener("change", sync); sync();
  return formDialog({ title: `Add ${segments.length} segment${segments.length === 1 ? "" : "s"}`, message: "Choose the GPX to save to. Save that GPX to write the added tracks.",
    fields: [field("Save to", destination), ...(segments.length > 1 ? [field("Add as", mode)] : []), nameField], focus: destination, confirm: "Add",
    read: () => {
      const merge = segments.length > 1 && mode.value === "merge";
      if (!targets.includes(destination.value) || ((segments.length === 1 || merge) && !name.value.trim())) return null;
      return { path: destination.value, merge, name: name.value.trim() };
    },
  });
}
