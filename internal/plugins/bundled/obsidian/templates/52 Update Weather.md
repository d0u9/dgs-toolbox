<%*
/**
 * 更新当前笔记 frontmatter 里的天气 —— 只有 `weather` 和 `temperature`
 * 这两个属性，位置一概不动（那是 `51 Update Location` 的事）。
 *
 * 日期留空就是现在；过去查档案，今天和未来查预报。档案有几天延迟，太近的
 * 日期查不到，这时会明说是延迟而不是笼统的失败。
 *
 * 日期可以带时刻：选「此刻的气温」时问的就是那一个钟点，按小时查。
 *
 * 气温记哪个由选单定：一整天的最低最高，还是此刻那一个数。日志和别的笔记
 * 各有各的默认，排在选单第一个，但两种笔记都能挑另一种。
 *
 * 只写 frontmatter，不动正文。
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

const parseDateValue = (value) => {
    if (value instanceof Date) {
        const date = moment(value);
        return date.isValid() ? { date, hasTime: true } : null;
    }

    const text = String(value ?? "").trim();
    if (!text) return null;

    const parsedInput = tp.user.parseDateInput(moment, text);
    if (parsedInput) return parsedInput;

    const withTime = moment.parseZone(
        text,
        [moment.ISO_8601, "YYYY-MM-DD HH:mm Z"],
        true
    );
    if (withTime.isValid()) return { date: withTime, hasTime: true };

    const dayOnly = moment.parseZone(text, "YYYY-MM-DD Z", true);
    return dayOnly.isValid() ? { date: dayOnly, hasTime: false } : null;
};

const initialDate = parseDateValue(frontmatter.date)
    ?? parseDateValue(frontmatter.createdAt)
    ?? { date: moment(), hasTime: true };
// 原本就带时刻的（createdAt、文章的 date）默认值里也留着时刻 —— 「此刻的
// 气温」问的是一个钟点，只有日历日答不了。
const defaultDate = initialDate.date.format(
    initialDate.hasTime ? "YYYY-MM-DD HH:mm Z" : "YYYY-MM-DD Z"
);

const dateInput = await tp.system.prompt(
    "Date / time / timezone (clear to use now)",
    defaultDate
);
if (dateInput === null) return;

const selectedDate = dateInput.trim()
    ? parseDateValue(dateInput)
    : { date: moment(), hasTime: true };

if (!selectedDate) {
    new tp.obsidian.Notice("Invalid date or time.");
    return;
}

const date = selectedDate.date;
const hasTime = selectedDate.hasTime;

// 按哪儿查天气：笔记自己有坐标就用它，没有才问。属性里的 coordinates 是自己
// 写的，一律「纬度, 经度」，不必再猜顺序。
const noteCoordinates = String(frontmatter.coordinates ?? "")
    .split(/[,，\s]+/)
    .filter(Boolean)
    .map(Number);

let location = null;

if (noteCoordinates.length === 2 && noteCoordinates.every(Number.isFinite)) {
    const [latitude, longitude] = noteCoordinates;
    // 天气按坐标查，但中国的备用源要城市名，所以还是反查一次地名。
    const place = await tp.user.reverseGeocode(tp, requestJson, latitude, longitude);
    location = {
        ...(place ?? {}),
        ...tp.user.formatCoordinates(latitude, longitude)
    };
} else {
    const source = await tp.system.suggester(
        ["当前位置", "输入城市"],
        ["here", "search"],
        false,
        "用哪里的天气"
    );

    if (!source) return;

    if (source === "here") {
        location = await tp.user.getLocation(tp, requestJson);
    } else {
        const query = await tp.system.prompt("城市（或邮编）", String(frontmatter.city ?? "").trim());
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
    }
}

if (!location?.city && !location?.coordinates) {
    new tp.obsidian.Notice("No matching location found.");
    return;
}

// 历史档案里没有今天，今天得走实时天气那条链。「今天」按这个日期自己的时区
// 算 —— 笔记的 date 带着它当时的偏移量，人在悉尼半夜时北京还是当天下午，
// 拿设备的今天去比会把人家的今天当成过去。
// 日志写的是一整天，气温默认记当天的最高和最低；别的笔记默认记此刻那一个数。
// 默认排在选单第一个，但两种笔记都能选另一种 —— 补记的日志想留下当时那个
// 温度，或者给一篇游记记那天的冷热，都由人决定。
const isDailyLog = file.path.startsWith("00 Daily Log/");

const wholeDayOption = "一整天的最低和最高";
const momentOption = "此刻的气温";
const scopeChoices = isDailyLog
    ? [wholeDayOption, momentOption]
    : [momentOption, wholeDayOption];

const scope = await tp.system.suggester(
    scopeChoices,
    scopeChoices,
    false,
    "气温记哪个"
);
if (!scope) return;

const isWholeDay = scope === wholeDayOption;

// 「此刻」要的是一个钟点，不是一天。选的就是当下（默认值原样不动，或者
// 输入框留空）时用实况，别的时刻按小时去查那个整点 —— 半小时内都算当下，
// 整点数据本来也就是一小时一个。
const isNow = Math.abs(date.diff(moment())) < 30 * 60 * 1000;
const needsHour = !isWholeDay && !isNow;

if (needsHour && !hasTime) {
    new tp.obsidian.Notice("那天的此刻是几点？日期里补上时刻，或者改选一整天。");
    return;
}

let weather = "";
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
    new tp.obsidian.Notice(
        isNow
            ? "Weather unavailable."
            : "No weather for that time. The archive lags a few days."
    );
    return;
}

// 只写天气那两个属性。位置是笔记自己的事实，这里问的位置只是「按哪儿查
// 天气」—— 查完就丢，不回写 country/region/city/locality/coordinates，也不动
// 国家标签。要改位置用 51 Update Location。
await app.fileManager.processFrontMatter(file, (properties) => {
    properties.weather = weather.condition;

    // 一整天记「最低, 最高」，此刻记那一个数。查不到就不动原来的值。
    if (isWholeDay) {
        if (dayTemperature) properties.temperature = dayTemperature.range;
    } else {
        properties.temperature = weather.temperature;
    }

    tp.user.orderFrontmatter(properties);
});

new tp.obsidian.Notice(
    isWholeDay && !dayTemperature
        ? `Weather updated, but the daily high/low is unavailable: ${location.city}, ${date.format("YYYY-MM-DD")}`
        : `Weather updated: ${location.city}, ${date.format("YYYY-MM-DD")}`
);
-%>
