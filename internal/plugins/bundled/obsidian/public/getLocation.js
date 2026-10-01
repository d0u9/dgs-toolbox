const emptyLocation = {
    city: "",
    locality: "",
    region: "",
    country: "",
    countryCode: "",
    latitude: "",
    longitude: "",
    coordinates: ""
};

const chinaSource = {
    name: "PConline (China backup)",
    url: "https://whois.pconline.com.cn/ipJson.jsp?json=true",
    normalize(data) {
        return {
            city: data.city ?? "",
            // PConline 的 region 字段给的是区/县，正好是第四级。
            locality: data.region ?? "",
            region: data.pro ?? "",
            country: "中国",
            countryCode: "CN",
            latitude: "",
            longitude: ""
        };
    }
};

const locationSources = [
    {
        name: "IPInfo",
        url: "https://ipinfo.io/json",
        normalize(data) {
            const [latitude = "", longitude = ""] = data.loc?.split(",") ?? [];

            return {
                city: data.city ?? "",
                locality: "",
                region: data.region ?? "",
                country: data.country ?? "",
                countryCode: data.country ?? "",
                latitude,
                longitude
            };
        }
    },
    {
        name: "IPAPI",
        url: "https://ipapi.co/json/",
        normalize(data) {
            return {
                city: data.city ?? "",
                locality: "",
                region: data.region ?? "",
                country: data.country_name ?? data.country_code ?? "",
                countryCode: data.country_code ?? "",
                latitude: data.latitude ?? "",
                longitude: data.longitude ?? ""
            };
        }
    },
    chinaSource
];

// 命名规则和坐标格式统一在这里套用，设备定位和 IP 两条链路共用一份。
const finalize = (tp, result) => ({
    ...result,
    country: tp.user.countryName(result.countryCode, result.country),
    ...tp.user.formatCoordinates(result.latitude, result.longitude)
});

const hasPlaceName = (value) => Boolean(value?.city || value?.region || value?.country);

/**
 * 先试设备自己的定位（移动端 GPS、macOS CoreLocationCLI）。这个平台没有可用的
 * 定位方式、被拒绝或者超时返回 null；拿到坐标但反查不到地名时地名留空。
 */
const deviceLocation = async (tp, requestJson) => {
    try {
        return await tp.user.getDeviceLocation(tp, requestJson);
    } catch (error) {
        console.warn("Device location lookup failed:", error);
        return null;
    }
};

const ipLocation = async (tp, requestJson) => {
    for (const source of locationSources) {
        try {
            const data = await requestJson(tp, source.url);
            let result = source.normalize(data);

            if (!hasPlaceName(result)) {
                throw new Error("Location data is empty");
            }

            if (result.countryCode === "CN" && source !== chinaSource) {
                try {
                    const localData = await requestJson(tp, chinaSource.url);
                    result = {
                        ...result,
                        ...chinaSource.normalize(localData),
                        latitude: result.latitude,
                        longitude: result.longitude
                    };
                } catch (error) {
                    console.warn("Chinese location localization failed:", error);
                }
            }

            return result;
        } catch (error) {
            console.warn(`${source.name} location lookup failed:`, error);
        }
    }

    return null;
};

module.exports = async function getLocation(tp, requestJson) {
    const device = await deviceLocation(tp, requestJson);

    if (hasPlaceName(device)) return finalize(tp, device);

    const ip = await ipLocation(tp, requestJson);

    // 设备定位拿到了坐标、只是反查不到地名：坐标精确到米，IP 的坐标差着几公里，
    // 所以坐标留设备的，只把地名换成 IP 查到的。
    if (device && ip) {
        return finalize(tp, {
            ...ip,
            latitude: device.latitude,
            longitude: device.longitude
        });
    }

    if (ip) return finalize(tp, ip);

    // 只剩坐标也比什么都没有强。
    if (device) return finalize(tp, device);

    return { ...emptyLocation };
};
