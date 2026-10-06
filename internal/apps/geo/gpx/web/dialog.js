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
