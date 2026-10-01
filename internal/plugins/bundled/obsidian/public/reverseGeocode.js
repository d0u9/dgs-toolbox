/**
 * 按坐标反查四级地址。
 *
 * 设备定位和按地名搜索都从这里拿 country / region / city / locality，
 * 四级地址的命名才不会各说各话。
 *
 * 中国的地名用中文、其他国家用英文。localityLanguage=zh 会混进繁体
 * （深圳返回「龍崗區」），所以中文用 zh-Hans。
 */

const REVERSE_URL = "https://api.bigdatacloud.net/data/reverse-geocode-client";
const NOMINATIM_URL = "https://nominatim.openstreetmap.org/reverse";
const TIMEOUT_MS = 8000;
const CHINESE_LOCALE = "zh-Hans";

// Nominatim 的使用条款要求调用方在 User-Agent 里标明身份。
const NOMINATIM_OPTIONS = {
    headers: { "User-Agent": "obsidian-daily-log-location/1.0" }
};

const fetchPlace = async (tp, requestJson, latitude, longitude, language) => {
    const url = [
        REVERSE_URL,
        `?latitude=${latitude}`,
        `&longitude=${longitude}`,
        `&localityLanguage=${language}`
    ].join("");

    try {
        const data = await requestJson(tp, url, TIMEOUT_MS);

        const place = {
            city: data?.city ?? "",
            locality: data?.locality ?? "",
            region: data?.principalSubdivision ?? "",
            country: data?.countryName ?? "",
            countryCode: (data?.countryCode ?? "").toUpperCase()
        };

        // 小地方两级会重名，重复一遍没有信息量。
        if (place.locality === place.city) place.locality = "";

        // 一个地名都没有的结果没有意义。
        if (!place.city && !place.region && !place.country) return null;

        return place;
    } catch (error) {
        console.warn(`Reverse geocoding failed (${language}):`, error);
        return null;
    }
};

/**
 * Nominatim 的备用反查。字段名和 BigDataCloud 不一样，这里归一成同一形状。
 *
 * 它的行政层级各国不一致（中国的市辖区上级有时直接跳到省），所以只当备用：
 * 主源挂了的时候，有地名总比没有强 —— 没有的话整条链会退到 IP，
 * 而 IP 给的是市中心，连区县都没有。
 */
const fetchFromNominatim = async (tp, requestJson, latitude, longitude, language) => {
    const url = [
        NOMINATIM_URL,
        `?lat=${latitude}`,
        `&lon=${longitude}`,
        "&format=jsonv2",
        "&zoom=14",
        `&accept-language=${language}`
    ].join("");

    try {
        const data = await requestJson(tp, url, TIMEOUT_MS, NOMINATIM_OPTIONS);
        const address = data?.address;

        if (!address) return null;

        const place = {
            city: address.city ?? address.town ?? address.village ?? address.county ?? "",
            locality: address.suburb ?? address.city_district ?? address.district ?? "",
            region: address.state ?? address.province ?? "",
            country: address.country ?? "",
            countryCode: (address.country_code ?? "").toUpperCase()
        };

        if (place.locality === place.city) place.locality = "";
        if (!place.city && !place.region && !place.country) return null;

        return place;
    } catch (error) {
        console.warn(`Nominatim reverse geocoding failed (${language}):`, error);
        return null;
    }
};

module.exports = async function reverseGeocode(tp, requestJson, latitude, longitude) {
    const place = await fetchPlace(tp, requestJson, latitude, longitude, "en")
        ?? await fetchFromNominatim(tp, requestJson, latitude, longitude, "en");

    if (!place) return null;
    if (place.countryCode !== "CN") return place;

    const localized = await fetchPlace(
        tp,
        requestJson,
        latitude,
        longitude,
        CHINESE_LOCALE
    ) ?? await fetchFromNominatim(
        tp,
        requestJson,
        latitude,
        longitude,
        CHINESE_LOCALE
    );

    return localized ?? place;
};
