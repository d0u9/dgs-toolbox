/**
 * 时区偏移量写成小时数：`+10`、`+8`、`-3`，半小时的写成 `+9.5`，
 * 尼泊尔那种 45 分钟的写成 `+5.75`。
 *
 * 比 ISO 的 `+10:00` 短，一眼能看出差几个小时；条目里时间本来就挤，
 * 省下三个字符是实在的。
 */
module.exports = function formatOffset(date) {
    const minutes = date.utcOffset();
    const hours = minutes / 60;
    const sign = minutes < 0 ? "-" : "+";

    // 整点不写小数，半点写 .5，四分之三小时写 .75
    const value = Math.abs(hours)
        .toFixed(2)
        .replace(/0+$/, "")
        .replace(/\.$/, "");

    return `${sign}${value}`;
};
