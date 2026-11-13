/**
 * 事后回填位置和天气。
 *
 * 位置和天气都要走网络，等它们会把新建笔记拖成几秒；而模板是「渲染完整份
 * 写回」，那几秒里往正文追加的内容（速记、去过哪里）会被抹掉。
 *
 * 所以创建时只写空属性、秒建，查到之后由这里回填。它用
 * `processFrontMatter` —— **只改属性，不碰正文**，所以无论回填多晚落地，
 * 都不可能覆盖掉正文里的东西。
 *
 * **只填空的**：已经有值的属性一律不动。dgs capture 建的日志带着 capture 记下的
 * 坐标和地名，这里只补它补不了的天气、气温和国旗标签；已经有坐标就按这个坐标
 * 查，不会换成这台设备此刻的位置。只有坐标也空着，才用设备的定位 —— 而且只在
 * 调用方说可以的时候（locateDevice，默认可以）：补一篇过去的日志时，设备此刻在
 * 哪儿跟那一天无关，宁可留空。
 *
 * 调用方不要 await：让它在后台跑，笔记先出现。
 */

// 文件是模板渲染完才落盘的，回填要等它出现。
const FILE_WAIT_TIMEOUT_MS = 15000;
const FILE_WAIT_POLL_MS = 200;

// 由 formatCountryTag 生成的标签，开头是两个区域指示符组成的国旗。
const COUNTRY_TAG = /^[\u{1F1E6}-\u{1F1FF}]{2}/u;

const waitForFile = async (app, path) => {
    const deadline = Date.now() + FILE_WAIT_TIMEOUT_MS;

    while (Date.now() < deadline) {
        const file = app.vault.getAbstractFileByPath(path);
        if (file && "extension" in file) return file;
        await new Promise((resolve) => setTimeout(resolve, FILE_WAIT_POLL_MS));
    }

    return null;
};

const blank = (value) => value === undefined || value === null || String(value).trim() === "";

const toTags = (tags) => {
    if (Array.isArray(tags)) return tags.map((tag) => String(tag));
    if (typeof tags === "string") return tags.split(/[,\s]+/).filter(Boolean);
    return [];
};

const hasCountryTag = (tags) => toTags(tags).some((tag) => COUNTRY_TAG.test(tag));

/**
 * 属性里的坐标，按「纬度, 经度」读。读不出来就当没有 —— 不像手动的模板那样
 * 问人，这里在后台跑。
 */
const coordinatesOf = (value) => {
    const parts = String(value ?? "").split(/[,，\s]+/).filter(Boolean).map(Number);
    if (parts.length !== 2 || !parts.every(Number.isFinite)) return null;
    const [latitude, longitude] = parts;
    if (Math.abs(latitude) > 90 || Math.abs(longitude) > 180) return null;
    return { latitude, longitude };
};

/** 文件现在的属性。刚建好的文件可能还没进 metadataCache，所以直接读。 */
const readFrontmatter = async (tp, file) => {
    const text = await app.vault.read(file);
    const match = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/.exec(text);
    if (!match) return {};
    try {
        return tp.obsidian.parseYaml(match[1]) ?? {};
    } catch {
        return {};
    }
};

/**
 * 笔记在哪儿：已有坐标就用它，地名或国家代码缺了再按坐标反查；没有坐标才问设备。
 * 返回的地名只是「查到的」，写不写由调用方按空不空决定。
 */
const locate = async (tp, existing, wantCountryCode, locateDevice) => {
    const known = coordinatesOf(existing.coordinates);
    if (!known) return locateDevice ? tp.user.getLocation(tp, tp.user.requestJson) : null;

    const named = ["country", "region", "city", "locality"].some((key) => !blank(existing[key]));
    let place = null;
    if (!named || wantCountryCode) {
        try {
            place = await tp.user.reverseGeocode(tp, tp.user.requestJson, known.latitude, known.longitude);
        } catch (error) {
            console.warn("Backfill reverse geocoding failed:", error);
        }
    }
    return {
        ...(place ?? {}),
        ...(place ? { country: tp.user.countryName(place.countryCode, place.country) } : {}),
        latitude: known.latitude,
        longitude: known.longitude,
        coordinates: existing.coordinates
    };
};

module.exports = async function backfillLocationWeather(tp, path, options = {}) {
    const { Notice } = tp.obsidian;
    const file = await waitForFile(app, path);

    if (!file) {
        console.warn(`Backfill gave up: ${path} never appeared`);
        return;
    }

    // 日志记的是一整天的最高最低气温，别的笔记记此刻的那一个数。
    // date 是带时区的 moment（也收日期串），「那一天」按它自己的时区算。
    const { dailyTemperature = false, countryTag = false, date = moment(), locateDevice = true } = options;

    const existing = await readFrontmatter(tp, file);
    const wantsWeather = blank(existing.weather) || blank(existing.temperature);
    const wantsTag = countryTag && !hasCountryTag(existing.tags);
    const wantsPlace = ["coordinates", "country", "region", "city", "locality"].some((key) => blank(existing[key]));

    if (!wantsWeather && !wantsTag && !wantsPlace) return;
    // 没坐标、又不让问设备：查不了任何东西，安静地留空。
    if (!locateDevice && !coordinatesOf(existing.coordinates)) return;

    let location = null;
    let weather = null;
    let dayTemperature = null;

    try {
        location = await locate(tp, existing, wantsTag, locateDevice);

        if (wantsWeather && location) {
            if (dailyTemperature) {
                // 日志一次请求取全天出现过的状态和最低最高温。
                dayTemperature = await tp.user.getDailyTemperature(
                    tp,
                    tp.user.requestJson,
                    location,
                    date
                );
                weather = dayTemperature;
            } else if (location.city || (location.latitude && location.longitude)) {
                weather = await tp.user.getWeather(tp, location, tp.user.requestJson);
            }
        }
    } catch (error) {
        console.warn("Backfill lookup failed:", error);
    }

    const hasLocation = Boolean(
        location && (location.country || location.region || location.city || location.coordinates)
    );

    if (!hasLocation && !weather && !dayTemperature) {
        // 属性留空，比填一个错的值好；51 / 52 两个模板可以随时手动补。
        new Notice("位置和天气都没查到，属性先留空 —— 可以用 51 / 52 手动补", 8000);
        return;
    }

    await app.fileManager.processFrontMatter(file, (properties) => {
        const fill = (key, value) => {
            if (blank(properties[key]) && !blank(value)) properties[key] = value;
        };

        if (hasLocation) {
            fill("coordinates", location.coordinates);
            fill("country", location.country);
            fill("region", location.region);
            fill("city", location.city);
            fill("locality", location.locality);

            if (countryTag && !hasCountryTag(properties.tags)) {
                // 国名取笔记里的（刚填上的或原本就有的），查不到国家代码时用得上。
                const tag = tp.user.formatCountryTag(location.countryCode, properties.country || location.country);
                // 没有国家代码时标签是国名，不是国旗：加过就不再加。
                const tags = toTags(properties.tags);
                if (tag && !tags.includes(tag)) properties.tags = [...tags, tag];
            }
        }

        if (weather) {
            fill("weather", weather.condition);
            if (!dailyTemperature) fill("temperature", weather.temperature);
        }

        // 日志的 temperature 是「最低, 最高」一个字符串，不是此刻那个数。
        if (dayTemperature) fill("temperature", dayTemperature.range);

        tp.user.orderFrontmatter(properties);
    });

    if (wantsWeather && (!weather || (dailyTemperature && !dayTemperature))) {
        new Notice("天气没查全，缺的属性先留空 —— 可以用 52 Update Weather 手动补", 6000);
    }
};
