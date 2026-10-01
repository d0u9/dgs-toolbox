const dateTimeFormats = [
    "MMM D, YYYY, h:mm A",
    "MMM D, YYYY, H:mm",
    "YYYY-MM-DD h:mm A",
    "YYYY-MM-DD H:mm",
    "YYYY/MM/DD h:mm A",
    "YYYY/MM/DD H:mm"
];

const dateOnlyFormats = [
    "YYYYMMDD",
    "YYYY-MM-DD",
    "YYYY/MM/DD",
    "YYYY.MM.DD",
    "YYMMDD",
    "YY-MM-DD",
    "YY/MM/DD"
];

module.exports = function parseDateInput(moment, input) {
    const value = input.trim();

    for (const format of dateTimeFormats) {
        const date = moment(value, format, "en", true);
        if (date.isValid()) return { date, hasTime: true };
    }

    for (const format of dateOnlyFormats) {
        const date = moment(value, format, "en", true);
        if (date.isValid()) return { date, hasTime: false };
    }

    return null;
};
