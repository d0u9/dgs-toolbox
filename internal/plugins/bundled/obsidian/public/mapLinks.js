/**
 * 给一个坐标生成各家地图的链接。
 *
 * 坐标一律按 WGS-84 交出去（设备定位和反查给的就是这个）。国内那两家用的是
 * 偏移过的坐标系，所以都显式声明源坐标系，让它们自己换算：
 *   高德   coordinate=wgs84
 *   百度   coord_type=wgs84
 * 不声明的话在国内会差出几百米，在国外没有区别。
 *
 * `label` 只是落图钉时的名字，位置由坐标决定。
 */

const PROVIDERS = [
    {
        name: "Apple 地图",
        short: "Apple",
        url: (lat, lng, label) =>
            `https://maps.apple.com/?ll=${lat},${lng}${label ? `&q=${label}` : ""}`
    },
    {
        name: "高德地图",
        short: "高德",
        url: (lat, lng, label) =>
            `https://uri.amap.com/marker?position=${lng},${lat}&coordinate=wgs84${label ? `&name=${label}` : ""}`
    },
    {
        name: "Google 地图",
        short: "Google",
        url: (lat, lng) =>
            `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`
    },
    {
        name: "百度地图",
        short: "百度",
        url: (lat, lng, label) =>
            `https://api.map.baidu.com/marker?location=${lat},${lng}&coord_type=wgs84&output=html${label ? `&title=${label}` : ""}`
    },
    {
        name: "OpenStreetMap",
        short: "OSM",
        url: (lat, lng) =>
            `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=17/${lat}/${lng}`
    }
];

/**
 * wanted 是想要哪几家，按名字或简称给，逗号分隔的字符串或数组都行。
 * 不给就是全部。返回 `[{ name, short, url }]`，顺序按 wanted 来。
 */
module.exports = function mapLinks(latitude, longitude, label = "", wanted = null) {
    const encoded = label ? encodeURIComponent(label) : "";

    const names = Array.isArray(wanted)
        ? wanted
        : String(wanted ?? "").split(",").map((value) => value.trim()).filter(Boolean);

    const picked = names.length
        ? names
            .map((value) => PROVIDERS.find((p) => p.name === value || p.short === value))
            .filter(Boolean)
        : PROVIDERS;

    return picked.map((provider) => ({
        name: provider.name,
        short: provider.short,
        url: provider.url(latitude, longitude, encoded)
    }));
};

module.exports.providers = PROVIDERS;
