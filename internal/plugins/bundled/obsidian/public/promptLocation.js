const countries = [
    {
        label: "🇨🇳 中国",
        value: {
            country: "中国",
            countryCode: "CN"
        }
    },
    {
        label: "🇦🇺 Australia",
        value: {
            country: "Australia",
            countryCode: "AU"
        }
    }
];

// 查不到地名时退回纯手工输入，此时没有坐标，天气只能靠城市名去查。
const manualLocation = async (tp) => {
    const selectedCountry = await tp.system.suggester(
        countries.map(({ label }) => label),
        countries.map(({ value }) => value),
        false,
        "Country"
    );
    const region = await tp.system.prompt("Region", "");
    const city = await tp.system.prompt("City", "");
    const locality = await tp.system.prompt("Locality（区/县，可留空）", "");

    return {
        city: city ?? "",
        locality: locality ?? "",
        region: region ?? "",
        country: selectedCountry?.country ?? "",
        countryCode: selectedCountry?.countryCode ?? "",
        latitude: "",
        longitude: "",
        coordinates: ""
    };
};

/**
 * 自动定位失败时手动指定位置。
 * 先按地名搜索，拿到的结果带坐标，天气才能查得到。
 */
module.exports = async function promptLocation(tp) {
    const cityInput = await tp.system.prompt("地名，城市或区县（留空则手动输入）", "");
    const city = (cityInput ?? "").trim();

    if (city) {
        try {
            const location = await tp.user.findPlace(tp, tp.user.requestJson, city);
            if (location) return location;
        } catch (error) {
            console.warn("Location lookup failed:", error);
        }

        new tp.obsidian.Notice("No matching location found. Enter it manually.");
    }

    return manualLocation(tp);
};
