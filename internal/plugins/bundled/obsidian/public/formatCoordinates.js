const formatCoordinate = (value) => {
    // Number("") is 0, so an empty value has to be rejected before converting.
    if (value === null || value === undefined || String(value).trim() === "") {
        return "";
    }

    const coordinate = Number(value);
    return Number.isFinite(coordinate) ? coordinate.toFixed(8) : "";
};

/**
 * 统一坐标格式：保留 8 位小数，字符串按「纬度, 经度」排列。
 */
module.exports = function formatCoordinates(latitude, longitude) {
    const formattedLatitude = formatCoordinate(latitude);
    const formattedLongitude = formatCoordinate(longitude);

    return {
        latitude: formattedLatitude,
        longitude: formattedLongitude,
        coordinates: formattedLatitude && formattedLongitude
            ? `${formattedLatitude}, ${formattedLongitude}`
            : ""
    };
};
