/**
 * 某一个**时刻**的天气，返回 `{ condition, temperature }`，形状和
 * `getWeather` 一样。
 *
 * `getWeather` 只会给「现在」，`getHistoricalWeather` 给的是一整天，所以
 * 「昨天下午三点多云 18 度」这种事只有它能答：按小时取，取离目标最近的整点。
 *
 * 日期一定要带时区（parseZone 出来的 moment）。整点是按气象源那边的当地时间
 * 排的，两边时区不同就会差好几个小时 —— 所以这里不比字符串，一律换算成
 * UTC 时刻再找最近的那个点。响应里的 `utc_offset_seconds` 就是为此要的。
 *
 * 过去查档案、今天和以后查预报，和 `getDailyTemperature` 一条规矩。查不到
 * 返回 null。
 */

// 和 getHistoricalWeather 同一套映射：档案那条链上的状态一律英文。
const weatherDescription = (code) => {
    if (code === 0) return "Clear";
    if ([1, 2, 3].includes(code)) return "Cloudy";
    if ([45, 48].includes(code)) return "Fog";
    if ([51, 53, 55, 56, 57].includes(code)) return "Drizzle";
    if ([61, 63, 65, 66, 67].includes(code)) return "Rain";
    if ([71, 73, 75, 77, 85, 86].includes(code)) return "Snow";
    if ([80, 81, 82].includes(code)) return "Showers";
    if ([95, 96, 99].includes(code)) return "Thunderstorm";
    return "";
};

module.exports = async function getHourlyWeather(tp, requestJson, location, date) {
    if (!location?.latitude || !location?.longitude) return null;
    if (!moment.isMoment(date)) return null;

    const nowThere = moment().utcOffset(date.utcOffset());
    const isPast = date.isBefore(nowThere, "day");
    const endpoint = isPast
        ? "https://archive-api.open-meteo.com/v1/archive"
        : "https://api.open-meteo.com/v1/forecast";

    // 那个时刻在气象源当地可能落在前一天或后一天，所以前后各多要一天。
    const url = [
        endpoint,
        `?latitude=${location.latitude}`,
        `&longitude=${location.longitude}`,
        `&start_date=${date.clone().subtract(1, "day").format("YYYY-MM-DD")}`,
        `&end_date=${date.clone().add(1, "day").format("YYYY-MM-DD")}`,
        "&hourly=temperature_2m,weather_code",
        "&temperature_unit=celsius",
        "&timezone=auto"
    ].join("");

    try {
        const data = await requestJson(tp, url, 8000);
        const times = data?.hourly?.time;
        if (!Array.isArray(times) || !times.length) return null;

        const offsetMinutes = Number(data.utc_offset_seconds ?? 0) / 60;
        const target = date.valueOf();

        let index = -1;
        let closest = Infinity;

        times.forEach((time, i) => {
            const instant = moment.utc(time, "YYYY-MM-DDTHH:mm").subtract(offsetMinutes, "minutes");
            const distance = Math.abs(instant.valueOf() - target);
            if (distance < closest) {
                closest = distance;
                index = i;
            }
        });

        // 差出一个多小时说明这个时刻不在返回的范围里（预报只到十几天后），
        // 与其给一个隔天的数字，不如说没有。
        if (index === -1 || closest > 90 * 60 * 1000) return null;

        const temperature = Number(data.hourly.temperature_2m?.[index]);
        const code = Number(data.hourly.weather_code?.[index]);

        if (!Number.isFinite(temperature) || !Number.isFinite(code)) return null;

        return {
            condition: weatherDescription(code),
            temperature: Number(temperature.toFixed(1))
        };
    } catch (error) {
        console.warn("Hourly weather lookup failed:", error);
        return null;
    }
};
