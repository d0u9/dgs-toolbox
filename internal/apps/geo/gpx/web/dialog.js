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

// photosDialog edits one waypoint's description and photos. It answers
// { description, photos: [{ name, caption }], files, maxSide, quality } —
// files are the pictures chosen to add, encoded as WebP at maxSide (0 keeps
// their size) and quality — or null when cancelled. Without a WebP encoder
// nothing can be added.
export function photosDialog(part, url, { canAdd, maxSide, quality: qualityValue }) {
  const description = document.createElement("input"); description.className = "field prompt-input photo-description"; description.placeholder = "Description"; description.value = part.description || ""; description.setAttribute("aria-label", "Description");
  const list = document.createElement("div"); list.className = "photo-list";
  for (const photo of part.photos || []) {
    const row = document.createElement("div"); row.className = "photo-row"; row.dataset.name = photo.name;
    const img = document.createElement("img"); img.src = url(photo.name); img.alt = photo.caption || "";
    const caption = document.createElement("input"); caption.className = "field"; caption.placeholder = "Caption"; caption.value = photo.caption || ""; caption.setAttribute("aria-label", "Caption");
    const remove = document.createElement("button"); remove.type = "button"; remove.className = "button"; remove.textContent = "Remove";
    remove.addEventListener("click", () => row.remove());
    row.append(img, caption, remove); list.append(row);
  }
  const files = document.createElement("input"); files.type = "file"; files.accept = "image/*"; files.multiple = true; files.className = "field"; files.setAttribute("aria-label", "Add photos");
  const size = document.createElement("select"); size.className = "field"; size.setAttribute("aria-label", "Longest side");
  for (const [value, text] of [
    // Phone screens are about 1170–1290 device pixels wide; 1600 fills a
    // tablet or an 800-pixel window at 2×; 1920 a Full HD desktop; 2048 and
    // 2560 a large or Retina desktop.
    [1280, "1280 px — phone"], [1600, "1600 px — tablet"], [1920, "1920 px — Full HD"],
    [2048, "2048 px — desktop"], [2560, "2560 px — Retina"], [0, "Original size"],
  ]) {
    const option = document.createElement("option"); option.value = value; option.textContent = text; size.append(option);
  }
  size.value = String(maxSide);
  const quality = document.createElement("input"); quality.type = "number"; quality.min = 1; quality.max = 100; quality.value = String(qualityValue); quality.className = "field"; quality.setAttribute("aria-label", "WebP quality");
  const field = (text, input) => { const label = document.createElement("label"); label.className = "photo-field"; label.append(text, input); return label; };
  const settings = document.createElement("div"); settings.className = "photo-settings";
  settings.append(field("Add photos", files), field("Longest side", size), field("WebP quality", quality));
  const adding = canAdd ? [settings] : [];
  return formDialog({ title: `Photos of ${part.name}`, message: "Photos are copied beside the GPX as WebP. The GPX links them when the edit is saved.",
    fields: [description, list, ...adding], focus: description, confirm: "Apply",
    read: () => ({
      description: description.value.trim(),
      photos: [...list.children].map((row) => ({ name: row.dataset.name, caption: row.querySelector("input").value.trim() })),
      files: canAdd ? [...files.files] : [],
      maxSide: Number(size.value),
      quality: Math.min(100, Math.max(1, Number(quality.value) || 90)),
    }),
  });
}
