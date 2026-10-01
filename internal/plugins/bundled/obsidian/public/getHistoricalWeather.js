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

module.exports = async function getHistoricalWeather(tp, requestJson, location, date) {
    const url = [
        "https://archive-api.open-meteo.com/v1/archive",
        `?latitude=${location.latitude}`,
        `&longitude=${location.longitude}`,
        `&start_date=${date}`,
        `&end_date=${date}`,
        "&daily=weather_code,temperature_2m_mean",
        "&temperature_unit=celsius",
        "&timezone=auto"
    ].join("");

    const data = await requestJson(tp, url, 8000);
    const code = Number(data?.daily?.weather_code?.[0]);
    const temperature = Number(data?.daily?.temperature_2m_mean?.[0]);

    if (!Number.isFinite(code) || !Number.isFinite(temperature)) return null;

    // 和 getWeather 同一个形状：状态一段文字，气温一个数字。
    return {
        condition: weatherDescription(code),
        temperature: Number(temperature.toFixed(1))
    };
};
