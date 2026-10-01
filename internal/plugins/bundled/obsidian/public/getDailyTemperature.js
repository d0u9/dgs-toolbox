/**
 * 一次请求取一整天出现过的天气状态，以及最高和最低气温。返回
 * `{ condition, conditions, min, max, range }`（摄氏度，一位小数）。
 * `range` 是写进 frontmatter 的那一个字符串，`最低, 最高` —— 和 `coordinates`
 * 一样是「一个属性、两个数」，Bases 里用 formula 拆开比大小。
 *
 * 日志写的是一整天，某个时刻的气温说明不了那天冷不冷 —— 所以日志记这两个
 * 值；别的笔记写的是某个时刻，用 `getWeather` 那个单独的 `temperature`。
 *
 * 今天和以后查预报，过去查档案：档案里没有今天。日期给带时区的 moment，
 * 「今天」才是那个地方的今天。没有坐标或者查不到返回 null。
 */
module.exports = async function getDailyTemperature(tp, requestJson, location, date) {
    if (!location?.latitude || !location?.longitude) return null;

    // 传进来的是带时区的 moment（笔记的 date 就带着它自己的偏移量）时，
    // 「过去还是未来」按那个时区算 —— 悉尼的深夜是北京的当天下午，拿设备的
    // 今天去比，会把人家的今天当成过去，转去查还没有这一天的历史档案。
    // 只给一个日期串时没有时区可依，只能按设备算。
    const zoned = moment.isMoment(date) ? date : moment(date, "YYYY-MM-DD", true);
    const day = zoned.format("YYYY-MM-DD");
    const nowThere = moment().utcOffset(zoned.utcOffset());

    const isPast = zoned.isBefore(nowThere, "day");
    const endpoint = isPast
        ? "https://archive-api.open-meteo.com/v1/archive"
        : "https://api.open-meteo.com/v1/forecast";

    const descriptions = location.countryCode === "CN"
        ? {
            0: "晴", 1: "多云", 2: "多云", 3: "多云", 45: "雾", 48: "雾",
            51: "毛毛雨", 53: "毛毛雨", 55: "毛毛雨", 56: "毛毛雨", 57: "毛毛雨",
            61: "雨", 63: "雨", 65: "雨", 66: "雨", 67: "雨",
            71: "雪", 73: "雪", 75: "雪", 77: "雪", 80: "阵雨", 81: "阵雨",
            82: "阵雨", 85: "雪", 86: "雪", 95: "雷暴", 96: "雷暴", 99: "雷暴"
        }
        : {
            0: "Clear", 1: "Cloudy", 2: "Cloudy", 3: "Cloudy", 45: "Fog", 48: "Fog",
            51: "Drizzle", 53: "Drizzle", 55: "Drizzle", 56: "Drizzle", 57: "Drizzle",
            61: "Rain", 63: "Rain", 65: "Rain", 66: "Rain", 67: "Rain",
            71: "Snow", 73: "Snow", 75: "Snow", 77: "Snow", 80: "Showers", 81: "Showers",
            82: "Showers", 85: "Snow", 86: "Snow", 95: "Thunderstorm",
            96: "Thunderstorm", 99: "Thunderstorm"
        };

    const url = [
        endpoint,
        `?latitude=${location.latitude}`,
        `&longitude=${location.longitude}`,
        `&start_date=${day}`,
        `&end_date=${day}`,
        "&hourly=weather_code",
        "&daily=temperature_2m_max,temperature_2m_min",
        "&temperature_unit=celsius",
        "&timezone=auto"
    ].join("");

    try {
        const data = await requestJson(tp, url, 8000);
        const max = Number(data?.daily?.temperature_2m_max?.[0]);
        const min = Number(data?.daily?.temperature_2m_min?.[0]);
        const times = data?.hourly?.time;
        const codes = data?.hourly?.weather_code;

        if (
            !Number.isFinite(max)
            || !Number.isFinite(min)
            || !Array.isArray(times)
            || !Array.isArray(codes)
        ) return null;

        // 今天只记截至现在已经出现过的状态，不把今天余下时段的预报写成事实。
        // 响应的小时是地点当地时间，用响应里的 UTC 偏移换成绝对时刻再比较。
        const offsetMinutes = Number(data.utc_offset_seconds ?? 0) / 60;
        const isToday = zoned.isSame(nowThere, "day");
        const now = moment().valueOf();
        const conditions = [];

        times.forEach((time, index) => {
            const instant = moment.utc(time, "YYYY-MM-DDTHH:mm")
                .subtract(offsetMinutes, "minutes")
                .valueOf();
            if (isToday && instant > now) return;

            const condition = descriptions[Number(codes[index])];
            if (condition && !conditions.includes(condition)) conditions.push(condition);
        });

        if (!conditions.length) return null;

        return {
            condition: conditions.join(", "),
            conditions,
            min: Number(min.toFixed(1)),
            max: Number(max.toFixed(1)),
            range: `${min.toFixed(1)}, ${max.toFixed(1)}`
        };
    } catch (error) {
        console.warn("Daily temperature lookup failed:", error);
        return null;
    }
};
