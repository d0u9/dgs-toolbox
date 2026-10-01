<%*
/**
 * 在光标处插入天气。
 *
 * 三问：哪天（可以带时刻）、哪里、气温记一整天还是此刻。
 *
 * 「此刻」问的是一个钟点：今天不写时刻就是现在的实况，写了时刻（或者是别的
 * 日子）就按小时去查那一个点。
 *
 * 时间默认就是现在 —— 在正文里插一句天气，说的多半是此时此刻，要写别的日子
 * 改输入框就是。位置和 `51` / `61` 一样三选一：当前定位、按地名搜索、输入
 * 坐标；选坐标时默认填笔记 frontmatter 里的那一组（有的话）。
 *
 * 只插入文本，不动 frontmatter，也不添加任何标签 —— 想改属性是
 * `52 Update Weather` 的事。
 */

tR = "";

const requestJson = tp.user.requestJson;
const file = app.workspace.getActiveFile();
const frontmatter = file
    ? app.metadataCache.getFileCache(file)?.frontmatter ?? {}
    : {};

// ---- 哪天 ----

// hasTime 要一路带着：「此刻的天气」问的是某个钟点，只有日历日答不了。
const parseDateValue = (value) => {
    if (value instanceof Date) {
        const date = moment(value);
        return date.isValid() ? { date, hasTime: true } : null;
    }

    const text = String(value ?? "").trim();
    if (!text) return null;

    const parsed = tp.user.parseDateInput(moment, text);
    if (parsed) return parsed;

    const withTime = moment.parseZone(
        text,
        [moment.ISO_8601, "YYYY-MM-DD HH:mm Z"],
        true
    );
    if (withTime.isValid()) return { date: withTime, hasTime: true };

    const dayOnly = moment.parseZone(text, "YYYY-MM-DD Z", true);
    return dayOnly.isValid() ? { date: dayOnly, hasTime: false } : null;
};

// 默认就是现在，连时刻带偏移量 —— 查哪一天、哪个钟点都按那个地方的时区算，
// 时区看得见才改得动。要写别的日子，改这个输入框。
const dateInput = await tp.system.prompt(
    "日期 / 时刻 / 时区（留空则用现在）",
    moment().format("YYYY-MM-DD HH:mm Z")
);
if (dateInput === null) return;

const selected = dateInput.trim()
    ? parseDateValue(dateInput)
    : { date: moment(), hasTime: true };

if (!selected) {
    new tp.obsidian.Notice(`Unrecognized date: ${dateInput}`);
    return;
}

const date = selected.date;

// ---- 哪里 ----

const source = await tp.system.suggester(
    ["当前定位", "按地名搜索", "输入坐标"],
    ["device", "search", "coordinates"],
    false,
    "位置从哪来"
);

if (!source) return;

let location = null;

if (source === "device") {
    location = await tp.user.getLocation(tp, requestJson);
} else if (source === "coordinates") {
    // 默认填笔记里的那一组：在一篇笔记里插天气，说的多半就是它记的地方。
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
        // 天气按坐标查，但中国的备用源要城市名，所以还是反查一次地名。
        place = await tp.user.reverseGeocode(
            tp,
            requestJson,
            parsed.latitude,
            parsed.longitude
        );
    } catch (error) {
        console.error("Reverse geocoding failed:", error);
    }

    location = {
        ...(place ?? {}),
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
        new tp.obsidian.Notice("Location lookup failed.");
        return;
    }
}

if (!location?.city && !location?.coordinates) {
    new tp.obsidian.Notice("Location unavailable.");
    return;
}

// ---- 一整天还是此刻 ----

const wholeDayOption = "一整天的最低和最高";
const momentOption = "此刻的气温";
// 过去的日子没有「此刻」可言，只有一整天。「今天」按这个日期自己的时区算
// —— 笔记的 date 带着它当时的偏移量，人在悉尼半夜时北京还是当天下午，拿设备
// 的今天去比会把人家的今天当成过去。
const isToday = date.isSame(moment().utcOffset(date.utcOffset()), "day");
const scopeChoices = isToday
    ? [momentOption, wholeDayOption]
    : [wholeDayOption, momentOption];

const scope = await tp.system.suggester(
    scopeChoices,
    scopeChoices,
    false,
    "气温记哪个"
);
if (!scope) return;

const isWholeDay = scope === wholeDayOption;

// ---- 查 ----

// 「此刻」要的是一个钟点，不是一天。选的就是当下（默认值原样不动，或者
// 输入框留空）时用实况，别的时刻按小时去查那个整点 —— 半小时内都算当下，
// 整点数据本来也就是一小时一个。
const isNow = Math.abs(date.diff(moment())) < 30 * 60 * 1000;
const needsHour = !isWholeDay && !isNow;

if (needsHour && !selected.hasTime) {
    new tp.obsidian.Notice("那天的此刻是几点？日期里补上时刻，或者改选一整天。");
    return;
}

let weather = null;
let dayTemperature = null;

try {
    if (isWholeDay) {
        // 一次请求同时取逐小时状态和当天最低最高；过去统计全天，今天只统计
        // 截至当前时刻已经出现过的状态。
        dayTemperature = await tp.user.getDailyTemperature(
            tp,
            requestJson,
            location,
            date
        );
        weather = dayTemperature;
    } else if (needsHour) {
        weather = await tp.user.getHourlyWeather(tp, requestJson, location, date);
    } else {
        weather = await tp.user.getWeather(tp, location, requestJson);
    }
} catch (error) {
    console.error("Weather lookup failed:", error);
}

if (!weather) {
    // 历史数据有几天延迟，太近的日期查不到。
    new tp.obsidian.Notice(
        isNow
            ? "Weather unavailable."
            : "No weather for that time. The archive lags a few days."
    );
    return;
}

if (isWholeDay && !dayTemperature) {
    new tp.obsidian.Notice("No daily high/low for that date.");
    return;
}

// 正文里读的是一句话，所以在这里才把状态和气温拼起来。属性里不要这种字符串。
const temperatureText = isWholeDay
    ? `${dayTemperature.min}~${dayTemperature.max}°C`
    : Number.isFinite(weather.temperature)
        ? `${weather.temperature}°C`
        : "";

tR = [weather.condition, temperatureText].filter(Boolean).join(", ");
-%>
