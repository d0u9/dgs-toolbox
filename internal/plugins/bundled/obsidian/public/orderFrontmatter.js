/**
 * 按库里的统一顺序整理 frontmatter。不存在的属性忽略；不在清单里的属性放在
 * 后面，并保持它们原有的相对顺序。
 */
module.exports = function orderFrontmatter(properties) {
    const orderedKeys = [
        "date",
        "dayOfTheYear",
        "createdAt",
        "updatedAt",
        "coordinates",
        "country",
        "region",
        "city",
        "locality",
        "temperature",
        "weather",
        "weatherNote",
        "tags",
        "arrivedAt",
        "departedAt"
    ];

    const keys = Object.keys(properties);
    const values = new Map(keys.map((key) => [key, properties[key]]));
    const orderedKeySet = new Set(orderedKeys);
    const reordered = [
        ...orderedKeys.filter((key) => values.has(key)),
        ...keys.filter((key) => !orderedKeySet.has(key))
    ];

    for (const key of keys) delete properties[key];
    for (const key of reordered) properties[key] = values.get(key);
};
