<%*
/**
 * 在光标处插入位置信息：先问位置从哪来，再问插哪一项。
 * 选地图链接时再选地图服务，插入可点击的 Markdown 链接。
 *
 * 三个来源和 `51 Update Location` 一样 —— 写一篇别处的笔记，插进正文的地名
 * 也未必是此刻站着的地方：
 *   当前定位   设备定位（移动端 GPS、macOS CoreLocationCLI）-> IP 查询
 *   按地名搜索  同名的地方很多，从候选列表里自己挑
 *   输入坐标   直接给纬经度（顺序自动判断），四级地址由坐标反查
 *
 * 只插入文本，不动 frontmatter，也不添加任何标签。
 */

tR = "";

const requestJson = tp.user.requestJson;
const file = app.workspace.getActiveFile();
const frontmatter = file
    ? app.metadataCache.getFileCache(file)?.frontmatter ?? {}
    : {};

const source = await tp.system.suggester(
    ["当前定位", "按地名搜索", "输入坐标"],
    ["device", "search", "coordinates"],
    false,
    "位置从哪来"
);

if (!source) return;

// 四级地址和坐标全都没有才算没查到。
const isEmpty = (value) => !value?.coordinates
    && !value?.locality
    && !value?.city
    && !value?.region
    && !value?.country;

let location = null;

if (source === "device") {
    location = await tp.user.getLocation(tp, requestJson);

    // 定位链路全断时还能手动补一个，这条路原来就有。
    if (isEmpty(location)) location = await tp.user.promptLocation(tp);
} else if (source === "coordinates") {
    const input = await tp.system.prompt(
        "坐标（纬经顺序自动判断）",
        String(frontmatter.coordinates ?? "").trim()
    );
    if (input === null) return;

    const parsed = await tp.user.parseCoordinates(tp, input);
    if (parsed === null) {
        new tp.obsidian.Notice("Enter two numbers: a latitude and a longitude.");
    }
    if (!parsed) return;

    let place = null;
    try {
        place = await tp.user.reverseGeocode(
            tp,
            requestJson,
            parsed.latitude,
            parsed.longitude
        );
    } catch (error) {
        console.error("Reverse geocoding failed:", error);
    }

    // 坐标是自己给的，反查不到地名也照样能插坐标。
    location = {
        ...(place ?? {}),
        country: place ? tp.user.countryName(place.countryCode, place.country) : "",
        ...tp.user.formatCoordinates(parsed.latitude, parsed.longitude)
    };
} else {
    const query = await tp.system.prompt(
        "地名，城市或区县",
        String(frontmatter.city ?? "").trim()
    );
    if (!query?.trim()) return;

    try {
        // 同名的地方很多，findPlace 会把候选列表弹出来让人挑。
        location = await tp.user.findPlace(tp, requestJson, query.trim(), {
            prompt: "选一个地方"
        });
    } catch (error) {
        console.error("Location lookup failed:", error);
        new tp.obsidian.Notice("Location lookup failed. Check your connection and try again.");
        return;
    }

    if (!location) {
        new tp.obsidian.Notice("No matching location found.");
        return;
    }
}

if (isEmpty(location)) {
    new tp.obsidian.Notice("Location unavailable.");
    return;
}

const address = [
    location.locality,
    location.city,
    location.region,
    location.country
].filter(Boolean);

// 这次没查到的那几级不进选单，手机上少翻两屏。
const options = [
    {
        label: "地图链接（高德 · Google · Apple）",
        kind: "map-group",
        preview: address.join(", ") || location.coordinates
    },
    { label: "坐标", value: location.coordinates },
    { label: "完整地址", value: address.join(", ") },
    { label: "区/县", value: location.locality },
    { label: "城市", value: location.city },
    { label: "省/州", value: location.region },
    { label: "国家", value: location.country },
    {
        label: "完整地址 + 坐标",
        value: [address.join(", "), location.coordinates].filter(Boolean).join(" · ")
    },
    {
        label: "地图链接（选一家）",
        kind: "map",
        preview: address.join(", ") || location.coordinates
    }
].filter((option) => option.kind === "map" || option.kind === "map-group"
    ? location.latitude && location.longitude
    : option.value);

if (!options.length) {
    new tp.obsidian.Notice("No location found.");
    return;
}

// 选单里直接显示各项的实际内容，选之前就知道会插进去什么。
const chosen = await tp.system.suggester(
    options.map((option) => `${option.label}  —  ${option.preview ?? option.value}`),
    options,
    false,
    "插入什么"
);

if (!chosen) return;

if (chosen.kind === "map" || chosen.kind === "map-group") {
    const label = location.locality || location.city || location.region || location.country;
    const links = tp.user.mapLinks(
        location.latitude,
        location.longitude,
        label,
        chosen.kind === "map-group" ? ["高德", "Google", "Apple"] : null
    );
    if (chosen.kind === "map-group") {
        tR = links.map((link) => `[${link.short}](${link.url})`).join(" · ");
    } else {
        const picked = await tp.system.suggester(
            links.map((link) => link.name),
            links,
            false,
            "用哪家地图"
        );
        if (!picked) return;

        tR = `[${picked.short}](${picked.url})`;
    }
} else {
    tR = chosen.value;
}
-%>
