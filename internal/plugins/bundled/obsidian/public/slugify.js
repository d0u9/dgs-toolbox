/**
 * URL slug for a Writing bundle. Chinese is transliterated to toneless pinyin,
 * so 星落仲夏夜 becomes xing-luo-zhong-xia-ye; everything else is lowercased and
 * hyphenated. Unlike the site's tag slugs, the syllables of one run of Han
 * characters stay hyphenated — a title is a phrase, not a single word.
 *
 * The transliteration comes from vendor/pinyin-pro, which Templater loads as
 * tp.user.pinyinPro; it is the same pinyin-pro version the site itself uses.
 *
 * Returns "" when nothing usable survives, which the caller can fall back to
 * `auto` on. Otherwise the result matches the site's urlSlug rule:
 * ^[a-z0-9]+(?:-[a-z0-9]+)*$
 */
module.exports = function slugify(tp, text) {
    const pinyinPro = tp.user.pinyinPro;
    if (!pinyinPro || typeof pinyinPro.pinyin !== "function") {
        throw new Error("找不到拼音库: {{dgs:public}}/vendor/pinyin-pro");
    }

    const romanised = String(text ?? "").replace(
        /\p{Script=Han}+/gu,
        (run) =>
            ` ${pinyinPro.pinyin(run, { toneType: "none", type: "array" }).join("-")} `
    );

    return romanised
        .normalize("NFD")
        .replace(/\p{M}+/gu, "")
        .toLowerCase()
        .replace(/['’]/g, "")
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "");
};
