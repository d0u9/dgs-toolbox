/**
 * 国家名称的显示规则：中国用中文，其他国家用英文。
 * countryCode 无法识别时回退到调用方给出的名称。
 */
module.exports = function countryName(countryCode, fallback = "") {
    const code = String(countryCode ?? "").toUpperCase();

    if (!/^[A-Z]{2}$/.test(code)) return fallback;
    if (code === "CN") return "中国";

    try {
        return new Intl.DisplayNames(["en"], { type: "region" }).of(code) ?? fallback;
    } catch {
        return fallback || code;
    }
};
