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
 * 调用方不要 await：让它在后台跑，笔记先出现。
 */

// 文件是模板渲染完才落盘的，回填要等它出现。
const FILE_WAIT_TIMEOUT_MS = 15000;
const FILE_WAIT_POLL_MS = 200;

const waitForFile = async (app, path) => {
    const deadline = Date.now() + FILE_WAIT_TIMEOUT_MS;

    while (Date.now() < deadline) {
        const file = app.vault.getAbstractFileByPath(path);
        if (file && "extension" in file) return file;
        await new Promise((resolve) => setTimeout(resolve, FILE_WAIT_POLL_MS));
    }

    return null;
};

/**
 * 创建时还不知道在哪个国家，所以国家标签只能这时候加。已有的原地替换，
 * 没有的追加到末尾 —— 这跟事后修改位置的模板不同，那些不新增标签。
 *
 * 只有日志走到这里（调用方传 countryTag: true）：国旗标签是日志的结构性标签，
 * 普通笔记不该带。
 */
const withCountryTag = (tp, tags, countryTag) => {
    if (!countryTag) return null;

    const replaced = tp.user.updateCountryTag(tags, countryTag);
    if (replaced) return replaced;

    const current = Array.isArray(tags)
        ? tags.map((tag) => String(tag))
        : typeof tags === "string"
            ? tags.split(/[,\s]+/).filter(Boolean)
            : [];

    return [...current, countryTag];
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
    const { dailyTemperature = false, countryTag = false, date = moment() } = options;

    let location = null;
    let weather = null;
    let dayTemperature = null;

    try {
        location = await tp.user.getLocation(tp, tp.user.requestJson);
        if (dailyTemperature && location) {
            // 日志一次请求取全天出现过的状态和最低最高温。
            dayTemperature = await tp.user.getDailyTemperature(
                tp,
                tp.user.requestJson,
                location,
                date
            );
            weather = dayTemperature;
        } else if (location?.city) {
            weather = await tp.user.getWeather(tp, location, tp.user.requestJson);
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
        if (hasLocation) {
            properties.country = location.country;
            properties.region = location.region;
            properties.city = location.city;
            properties.locality = location.locality ?? "";
            properties.coordinates = location.coordinates;

            if (countryTag) {
                const tag = tp.user.formatCountryTag(
                    location.countryCode,
                    location.country
                );
                const tags = withCountryTag(tp, properties.tags, tag);
                if (tags) properties.tags = tags;
            }
        }

        if (weather) {
            properties.weather = weather.condition;
            if (!dailyTemperature) properties.temperature = weather.temperature;
        }

        // 日志的 temperature 是「最低, 最高」一个字符串，不是此刻那个数。
        if (dayTemperature) properties.temperature = dayTemperature.range;

        tp.user.orderFrontmatter(properties);
    });

    if (!weather || (dailyTemperature && !dayTemperature)) {
        new Notice("天气没查全，缺的属性先留空 —— 可以用 52 Update Weather 手动补", 6000);
    }
};
