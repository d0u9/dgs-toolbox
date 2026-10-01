<%*
/**
 * 在光标处插入 ISO 8601 时间戳，和 frontmatter 里 `createdAt` 同一种格式。
 * 只插入文本，不动 frontmatter，也不添加任何标签。
 */

tR = "";

const ISO = "YYYY-MM-DDTHH:mm:ssZ";
const DATE_ONLY = "YYYY-MM-DD";

const mode = await tp.system.suggester(
    ["现在", "今天（只要日期）", "指定日期或时间"],
    ["now", "today", "custom"],
    false,
    "插入哪个"
);

if (!mode) return;

let value = "";

if (mode === "now") {
    value = moment().format(ISO);
} else if (mode === "today") {
    value = moment().format(DATE_ONLY);
} else {
    const input = await tp.system.prompt("日期或时间", moment().format(DATE_ONLY));
    if (input === null) return;

    const parsed = input.trim()
        ? tp.user.parseDateInput(moment, input)
        : null;

    if (!parsed) {
        new tp.obsidian.Notice(`Unrecognized date: ${input}`);
        return;
    }

    // 输入里没有时间就别凭空补一个 00:00:00，按原本的精度输出。
    value = parsed.hasTime
        ? parsed.date.format(ISO)
        : parsed.date.format(DATE_ONLY);
}

tR = value;
-%>
