/**
 * 按地名搜索并返回统一结构的位置信息。
 *
 * 搜索用 Nominatim（OpenStreetMap）。它认得市辖区，也认拼音和英文名：
 * 「拱墅区」「gongshu」「hangzhou」都能命中同一个地方。不指定语言时它返回
 * 当地名称，正好符合「中国用中文、其他国家用英文」。
 *
 * 但 OSM 对中国的行政层级并不一致（「拱墅区」的上级直接跳到浙江省，杭州市
 * 丢了），所以搜索只用来定位到一个坐标，四级地址一律由 reverseGeocode.js
 * 给出，和设备定位那条链共用同一套命名。
 *
 * options:
 *   context  {region, country} 已知的上下文，附在查询词后面帮助排歧义
 *   autoPick 为 true 时直接取第一条，不弹选择框
 *   limit    候选数量
 *   prompt   选择框标题
 */

const SEARCH_URL = "https://nominatim.openstreetmap.org/search";
const TIMEOUT_MS = 8000;

// Nominatim 的使用条款要求调用方在 User-Agent 里标明身份。
const REQUEST_OPTIONS = {
    headers: { "User-Agent": "obsidian-daily-log-location/1.0" }
};

const search = async (tp, requestJson, query, limit) => {
    const url = [
        SEARCH_URL,
        `?q=${encodeURIComponent(query)}`,
        "&format=jsonv2",
        `&limit=${limit}`
    ].join("");

    try {
        const data = await requestJson(tp, url, TIMEOUT_MS, REQUEST_OPTIONS);
        return Array.isArray(data) ? data : [];
    } catch (error) {
        console.warn("Place search failed:", error);
        return [];
    }
};

module.exports = async function findPlace(tp, requestJson, query, options = {}) {
    const {
        context = null,
        autoPick = false,
        limit = 20,
        prompt = "Choose location"
    } = options;

    const trimmedQuery = String(query ?? "").trim();
    if (!trimmedQuery) return null;

    // 同名的地方很多（「朝阳区」北京和长春都有，「Epping」四个国家都有），
    // 已知上下文时把它附在查询词后面，自动取第一条才靠得住。
    const contextTerms = context
        ? [context.region, context.country].filter(Boolean).join(" ")
        : "";
    const searchQuery = contextTerms
        ? `${trimmedQuery} ${contextTerms}`
        : trimmedQuery;

    const results = await search(tp, requestJson, searchQuery, limit);
    if (!results.length) return null;

    // display_name 是完整的行政层级，「朝阳区, 北京市, 中国」和
    // 「朝阳区, 长春市, 吉林省, 中国」一眼能分清。
    const candidate = autoPick
        ? results[0]
        : await tp.system.suggester(
            results.map((result) => result.display_name),
            results,
            false,
            prompt
        );

    if (!candidate) return null;

    const place = await tp.user.reverseGeocode(
        tp,
        requestJson,
        candidate.lat,
        candidate.lon
    );

    if (!place) return null;

    return {
        ...place,
        country: tp.user.countryName(place.countryCode, place.country),
        ...tp.user.formatCoordinates(candidate.lat, candidate.lon)
    };
};
