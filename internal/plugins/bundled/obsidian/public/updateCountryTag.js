// 由 formatCountryTag 生成的标签，开头是两个区域指示符组成的国旗。
const COUNTRY_TAG = /^[\u{1F1E6}-\u{1F1FF}]{2}/u;

const toArray = (tags) => {
    if (Array.isArray(tags)) return tags.map((tag) => String(tag));
    if (typeof tags === "string") return tags.split(/[,\s]+/).filter(Boolean);
    return [];
};

/**
 * 更新标签列表里已有的国家标签，原地替换，不新增。
 * 笔记本来就没有国家标签时不要替它加一个，返回 null 表示无需改动。
 */
module.exports = function updateCountryTag(tags, countryTag) {
    if (!countryTag) return null;

    const current = toArray(tags);
    if (!current.some((tag) => COUNTRY_TAG.test(tag))) return null;

    let replaced = false;
    const updated = current
        .filter((tag) => {
            if (!COUNTRY_TAG.test(tag)) return true;
            if (replaced) return false;

            replaced = true;
            return true;
        })
        .map((tag) => COUNTRY_TAG.test(tag) ? countryTag : tag);

    // 标签没有实际变化时也不写回，避免产生无谓的改动。
    const unchanged = Array.isArray(tags)
        && updated.length === current.length
        && updated.every((tag, index) => tag === current[index]);

    return unchanged ? null : updated;
};
