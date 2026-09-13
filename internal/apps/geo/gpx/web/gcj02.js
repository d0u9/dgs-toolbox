// GCJ-02, the system base maps published in China draw in, for positions the
// page holds itself — a planned route's waypoints — rather than receives drawn.
// The same conversion as internal/geo/gcj02, for [lon, lat] pairs.

const A = 6378245.0;
const EE = 0.00669342162296594323;

const inChina = ([lon, lat]) => lon >= 72.004 && lon <= 137.8347 && lat >= 0.8293 && lat <= 55.8271;

function transformLat(x, y) {
  let r = -100 + 2 * x + 3 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  r += ((20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2) / 3;
  r += ((20 * Math.sin(y * Math.PI) + 40 * Math.sin((y / 3) * Math.PI)) * 2) / 3;
  r += ((160 * Math.sin((y / 12) * Math.PI) + 320 * Math.sin((y * Math.PI) / 30)) * 2) / 3;
  return r;
}

function transformLon(x, y) {
  let r = 300 + x + 2 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  r += ((20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2) / 3;
  r += ((20 * Math.sin(x * Math.PI) + 40 * Math.sin((x / 3) * Math.PI)) * 2) / 3;
  r += ((150 * Math.sin((x / 12) * Math.PI) + 300 * Math.sin((x / 30) * Math.PI)) * 2) / 3;
  return r;
}

// fromWGS84 is the GCJ-02 position of a WGS-84 one.
export function fromWGS84(point) {
  if (!inChina(point)) return point;
  const [lon, lat] = point;
  const x = lon - 105, y = lat - 35;
  const radLat = (lat / 180) * Math.PI;
  let magic = Math.sin(radLat);
  magic = 1 - EE * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  const dLat = (transformLat(x, y) * 180) / (((A * (1 - EE)) / (magic * sqrtMagic)) * Math.PI);
  const dLon = (transformLon(x, y) * 180) / ((A / sqrtMagic) * Math.cos(radLat) * Math.PI);
  return [lon + dLon, lat + dLat];
}

// toWGS84 inverts fromWGS84, for a place clicked on a GCJ-02 map.
export function toWGS84(point) {
  if (!inChina(point)) return point;
  let [lon, lat] = point;
  for (let i = 0; i < 10; i++) {
    const [gLon, gLat] = fromWGS84([lon, lat]);
    lon -= gLon - point[0];
    lat -= gLat - point[1];
  }
  return [lon, lat];
}
