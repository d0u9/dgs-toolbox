/**
 * 国家标签：国旗 + ISO 3166-1 alpha-2 代码，例如 🇨🇳-CN、🇦🇺-AU。
 *
 * 标签是索引键，不是给人读的地址 —— 定宽、ASCII、无歧义比可读性要紧，所以
 * 这里用代码而不是国家名。「中国用中文，其他用英文」那条规则因此只留在
 * countryName.js 一处，管属性和正文里的显示，标签这边不必再实现一遍。
 *
 * 代码认不出来时退回清洗过的国家名 —— 标签里不能有空格和标点，全都折成分隔符。
 * 没有标签比有个坏标签更糟，所以这条回退路留着。
 */
module.exports = function formatCountryTag(countryCode, country) {
    const code = String(countryCode ?? "").toUpperCase();

    if (/^[A-Z]{2}$/.test(code)) {
        const flag = String.fromCodePoint(
            ...code.split("").map((letter) => letter.charCodeAt(0) + 127397)
        );
        return `${flag}-${code}`;
    }

    return String(country ?? "")
        .replace(/[^\p{L}\p{N}]+/gu, "-")
        .replace(/^-+|-+$/g, "");
};
