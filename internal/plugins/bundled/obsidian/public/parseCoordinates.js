/**
 * 解析手工输入的坐标，并自动判断是「经度, 纬度」还是「纬度, 经度」。
 *
 * 分隔符不挑，逗号、中文逗号或者空格都行。
 *
 * 纬度只到 ±90，经度到 ±180：只要有一个数超过 90，它必然是经度，顺序就定了。
 * 两个数都在 ±90 以内时两种读法都成立（地图复制出来的多是「纬度, 经度」，
 * 笔记里存的也是「纬度, 经度」），猜错就是记到别的地方去，所以弹选单让人挑，
 * 选项里直接写出两种读法各自对应的经纬度。
 *
 * 解析不出来返回 null，选单里取消返回 undefined——取消是用户自己的选择，
 * 不该再弹一次「格式不对」。
 */
const inRange = (longitude, latitude) =>
    Math.abs(longitude) <= 180 && Math.abs(latitude) <= 90;

module.exports = async function parseCoordinates(tp, input) {
    const parts = String(input ?? "")
        .split(/[,，\s]+/)
        .filter(Boolean);

    if (parts.length !== 2) return null;

    const [first, second] = parts.map(Number);
    if (!Number.isFinite(first) || !Number.isFinite(second)) return null;

    const lonLat = inRange(first, second) ? { latitude: second, longitude: first } : null;
    const latLon = inRange(second, first) ? { latitude: first, longitude: second } : null;

    if (!lonLat) return latLon;
    if (!latLon) return lonLat;

    const picked = await tp.system.suggester(
        [
            `纬度 ${first}, 经度 ${second}`,
            `经度 ${first}, 纬度 ${second}`
        ],
        [latLon, lonLat],
        false,
        "这两个数哪个是纬度"
    );

    return picked ?? undefined;
};
