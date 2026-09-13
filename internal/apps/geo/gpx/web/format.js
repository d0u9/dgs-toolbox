// Display formatting only: units and rounding for people.

export function distance(metres) {
  if (metres == null) return "–";
  return metres < 1000 ? `${Math.round(metres)} m` : `${(metres / 1000).toFixed(metres < 10000 ? 2 : 1)} km`;
}

export function duration(seconds) {
  if (!seconds) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return h ? `${h}h ${String(m).padStart(2, "0")}m` : `${m}m`;
}

export const kmh = (metresPerSecond) => metresPerSecond * 3.6;

// clock shows a moment in a time zone as "2026-02-13 10:34:52". GPX times are
// UTC instants; the zone only changes how they read.
export function clock(millis, timeZone, { seconds = true } = {}) {
  if (millis == null) return "";
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-CA", {
      timeZone,
      hourCycle: "h23",
      year: "numeric", month: "2-digit", day: "2-digit",
      hour: "2-digit", minute: "2-digit", second: seconds ? "2-digit" : undefined,
    }).formatToParts(new Date(millis)).map((part) => [part.type, part.value]),
  );
  const time = `${parts.hour}:${parts.minute}` + (seconds ? `:${parts.second}` : "");
  return `${parts.year}-${parts.month}-${parts.day} ${time}`;
}

// zoneOffset is a zone's UTC offset at a moment, as "UTC+8" or "UTC+5:30".
export function zoneOffset(millis, timeZone) {
  const name = new Intl.DateTimeFormat("en-US", { timeZone, timeZoneName: "longOffset" })
    .formatToParts(new Date(millis)).find((part) => part.type === "timeZoneName")?.value || "GMT";
  return name === "GMT" ? "UTC" : name.replace("GMT", "UTC").replace(/:00$/, "").replace(/([+-])0(\d)/, "$1$2");
}

export const browserTimeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

// timeZones lists the zones to choose from: the browser's own, UTC and a few
// likely ones first, then every zone the browser knows.
export function timeZones() {
  const all = typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : [];
  const first = [browserTimeZone(), "UTC", "Asia/Shanghai", "Australia/Sydney"];
  return [...new Set([...first, ...all])];
}

export function fileSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
