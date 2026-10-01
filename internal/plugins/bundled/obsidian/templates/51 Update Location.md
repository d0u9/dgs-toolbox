<%*
/**
 * 更新当前笔记 frontmatter 里的四级地址和坐标。
 *
 * 位置有三个来源：
 *   当前定位   设备定位 -> IP，一次性覆盖四级地址和坐标
 *   按地名搜索  地名没变时只补坐标，变了才从候选里挑一个并改写四级地址
 *   输入坐标   直接给纬经度（顺序自动判断），四级地址由坐标反查
 *
 * 只写 frontmatter，不动正文。国家标签只更新笔记里已有的，不新增。
 */

// Templater 会用模板输出替换当前选区；更新属性不该动正文，所以原样输出选区。
tR = tp.file.selection();

const requestJson = tp.user.requestJson;
const file = app.workspace.getActiveFile();
if (!file) {
    new tp.obsidian.Notice("No active note found.");
    return;
}

const frontmatter = app.metadataCache.getFileCache(file)?.frontmatter ?? {};

const source = await tp.system.suggester(
    ["当前定位", "按地名搜索", "输入坐标"],
    ["device", "search", "coordinates"],
    false,
    "位置从哪来"
);

if (!source) return;

let location = null;
// 按城市名搜索且城市没变时，只是来补坐标的，不该动已有的地名。
let updateNames = true;

if (source === "device") {
    location = await tp.user.getLocation(tp, requestJson);

    if (!location.coordinates && !location.city && !location.region && !location.country) {
        new tp.obsidian.Notice("Location unavailable.");
        return;
    }
} else if (source === "coordinates") {
    const existingCoordinates = String(frontmatter.coordinates ?? "").trim();
    const input = await tp.system.prompt("坐标（纬经顺序自动判断）", existingCoordinates);

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

    // 坐标是用户自己给的，反查不到地名也照样写坐标，地名留给已有的值。
    updateNames = Boolean(place);

    location = {
        ...(place ?? {}),
        country: place ? tp.user.countryName(place.countryCode, place.country) : "",
        ...tp.user.formatCoordinates(parsed.latitude, parsed.longitude)
    };
} else {
    const existingCity = String(frontmatter.city ?? "").trim();
    const cityInput = await tp.system.prompt("地名，城市或区县", existingCity);

    if (cityInput === null) return;

    const city = cityInput.trim();
    if (!city) {
        new tp.obsidian.Notice("City cannot be empty.");
        return;
    }

    // 城市没变时沿用已有的地区和国家排序并直接取第一名；
    // 城市变了则让用户从候选里挑。
    const cityChanged = city !== existingCity;
    updateNames = cityChanged;

    try {
        location = await tp.user.findPlace(tp, requestJson, city, {
            context: cityChanged
                ? null
                : { region: frontmatter.region, country: frontmatter.country },
            autoPick: !cityChanged
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

const countryTag = tp.user.formatCountryTag(
    location.countryCode,
    location.country
);

await app.fileManager.processFrontMatter(file, (properties) => {
    // 定位失败只剩地名时别把已有的坐标擦掉。
    if (location.coordinates) properties.coordinates = location.coordinates;

    if (updateNames) {
        properties.country = location.country;
        properties.region = location.region;
        properties.city = location.city;
        properties.locality = location.locality ?? "";

        // 笔记里已有国家标签才更新它，没有就不添加。
        const tags = tp.user.updateCountryTag(properties.tags, countryTag);
        if (tags) properties.tags = tags;
    }

    tp.user.orderFrontmatter(properties);
});

const place = [
    location.locality,
    location.city,
    location.region,
    location.country
].filter(Boolean).join(", ");

new tp.obsidian.Notice(
    updateNames
        ? `Location updated: ${place || location.coordinates}`
        : `Coordinates updated: ${location.coordinates}`
);
-%>
