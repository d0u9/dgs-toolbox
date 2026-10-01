const weatherDescription = (code, language) => {
    const descriptions = language === "zh"
        ? {
            clear: "晴",
            cloudy: "多云",
            fog: "雾",
            drizzle: "毛毛雨",
            rain: "雨",
            snow: "雪",
            shower: "阵雨",
            thunderstorm: "雷暴"
        }
        : {
            clear: "Clear",
            cloudy: "Cloudy",
            fog: "Fog",
            drizzle: "Drizzle",
            rain: "Rain",
            snow: "Snow",
            shower: "Showers",
            thunderstorm: "Thunderstorm"
        };

    if (code === 0) return descriptions.clear;
    if ([1, 2, 3].includes(code)) return descriptions.cloudy;
    if ([45, 48].includes(code)) return descriptions.fog;
    if ([51, 53, 55, 56, 57].includes(code)) return descriptions.drizzle;
    if ([61, 63, 65, 66, 67].includes(code)) return descriptions.rain;
    if ([71, 73, 75, 77, 85, 86].includes(code)) return descriptions.snow;
    if ([80, 81, 82].includes(code)) return descriptions.shower;
    if ([95, 96, 99].includes(code)) return descriptions.thunderstorm;
    return null;
};

const openMeteoSource = (location, language) => ({
    name: "Open-Meteo",
    url: [
        "https://api.open-meteo.com/v1/forecast",
        `?latitude=${location.latitude}`,
        `&longitude=${location.longitude}`,
        "&current=temperature_2m,weather_code",
        "&temperature_unit=celsius"
    ].join(""),
    normalize(data) {
        return {
            condition: weatherDescription(data.current?.weather_code, language),
            temperature: data.current?.temperature_2m
        };
    }
});

const wttrSource = (location, language) => ({
    name: "wttr.in",
    url: [
        `https://wttr.in/${location.latitude},${location.longitude}`,
        "?format=j1&m",
        language === "zh" ? "&lang=zh-cn" : ""
    ].join(""),
    normalize(data) {
        const current = data.current_condition?.[0];
        return {
            condition: current?.weatherDesc?.[0]?.value ?? "",
            temperature: current?.temp_C
        };
    }
});

const uapiSource = (location, language) => ({
    name: "UAPI (China backup)",
    url: [
        "https://uapis.cn/api/v1/misc/weather",
        `?city=${encodeURIComponent(location.city)}`,
        `&lang=${language}`
    ].join(""),
    normalize(data) {
        return {
            condition: data.weather ?? "",
            temperature: data.temperature
        };
    }
});

/**
 * 返回 `{ condition, temperature }`：状态是一段文字，气温是数字（摄氏度）。
 * 两者分开写进 frontmatter，Bases 才能按状态分组、按气温比大小。
 * 查不到返回 null。
 */
module.exports = async function getWeather(tp, location, requestJson) {
    if (!location?.city) return null;

    const isChinese = location.countryCode === "CN";
    const language = isChinese ? "zh" : "en";
    const coordinateSources = location.latitude && location.longitude
        ? [openMeteoSource(location, language), wttrSource(location, language)]
        : [];
    const chinaBackup = uapiSource(location, language);
    const sources = isChinese
        ? [chinaBackup, ...coordinateSources]
        : [...coordinateSources, chinaBackup];

    for (const source of sources) {
        try {
            const data = await requestJson(tp, source.url);
            const result = source.normalize(data);
            const temperature = Number(result.temperature);

            if (!result.condition || !Number.isFinite(temperature)) {
                throw new Error("Weather data is empty");
            }

            return {
                condition: result.condition,
                // 一位小数就够，气象源给的精度再高也没有意义。
                temperature: Number(temperature.toFixed(1))
            };
        } catch (error) {
            console.warn(`${source.name} weather lookup failed:`, error);
        }
    }

    return null;
};
