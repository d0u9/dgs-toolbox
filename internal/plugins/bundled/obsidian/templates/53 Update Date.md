<%*
/**
 * 更新当前笔记 frontmatter 里的 `date`。
 *
 * 写成哪种格式：
 *   原本是 ISO      -> 写 ISO
 *   原本只到日      -> 写 YYYY-MM-DD，日志的日期本来就只是个日历日
 *   输入带了时刻    -> 写 ISO，给了时刻就是想要它，不能默默丢掉
 *   本来就没有      -> 问一次
 *
 * 「现在」带此刻的时分秒，「今天」是当天零点。只有这两个，和你手动敲的
 * 不含时刻的日期，才会出现 `T00:00:00`——那是输入本身没有精度，不是我们
 * 把它抹掉了。
 *
 * 只动 `date`，不碰 `createdAt`——创建时间是既成事实，不该被事后改写。
 * 只写 frontmatter，不动正文，也不碰标签。
 */

// Templater 会用模板输出替换当前选区；更新属性不该动正文，所以原样输出选区。
tR = tp.file.selection();

const ISO = "YYYY-MM-DDTHH:mm:ssZ";
const DATE_ONLY = "YYYY-MM-DD";
const TIME_ONLY = "HH:mm";
// 没有 Modal forms 时手动输入用的格式：短、好改，parseDateInput 也认。
const EDIT = "YYYY-MM-DD HH:mm";
const dateOnlyPattern = /^\d{4}-\d{2}-\d{2}$/;

const file = app.workspace.getActiveFile();
if (!file) {
    new tp.obsidian.Notice("No active note found.");
    return;
}

/**
 * 直接读原文而不走 metadataCache：`date` 在 types.json 里注册成了 datetime，
 * 缓存里可能已经是 Date 对象，既分不出原本是哪种写法，也说不准它是按本地
 * 时间还是 UTC 解析的。原文是确定的。
 */
const content = await app.vault.read(file);
const frontmatterText = content.match(/^---\n([\s\S]*?)\n---/)?.[1] ?? "";
const existingText = (frontmatterText.match(/^date:\s*(.*)$/m)?.[1] ?? "")
    .trim()
    .replace(/^['"]|['"]$/g, "");

/**
 * parseDateInput 不认完整的 ISO 串，所以先自己试一次。
 * parseZone 保留原来的时区偏移，不偷偷换算成本地时间。
 */
const parseValue = (text) => {
    const value = String(text ?? "").trim();
    if (!value) return null;

    const iso = moment.parseZone(value, moment.ISO_8601, true);
    if (iso.isValid()) return { date: iso, hasTime: true };

    return tp.user.parseDateInput(moment, value);
};

const existing = parseValue(existingText);

/**
 * Modal forms 提供日期和时间的原生选择器，比让人去改一串 ISO 好用得多。
 * 时间留空就表示只到日，正好对上我们「有没有时刻」这个区分。
 */
const askWithForm = async (api) => {
    const values = {
        date: (existing?.date ?? moment()).format(DATE_ONLY),
        time: existing?.hasTime ? existing.date.format(TIME_ONLY) : ""
    };

    const result = await api.openForm({
        title: "Update date",
        name: "update-date",
        fields: [
            {
                name: "date",
                label: "日期",
                description: "",
                isRequired: true,
                input: { type: "date" }
            },
            {
                name: "time",
                label: "时间",
                description: "留空表示只到日",
                isRequired: false,
                input: { type: "time" }
            }
        ]
    }, { values });

    if (result?.status !== "ok") return null;

    const data = result.getData();
    const date = String(data.date ?? "").trim();
    const time = String(data.time ?? "").trim();

    if (!date) return null;

    // 一个字段都没改就沿用原值，连时区偏移一起保留。
    if (existing && date === values.date && time === values.time) return existing;

    return time
        ? { date: moment(`${date} ${time}`, EDIT), hasTime: true }
        : { date: moment(date, DATE_ONLY), hasTime: false };
};

// Modal forms 没启用时的退路：填一个短格式让人手改。
const askWithPrompt = async () => {
    const prefill = existing
        ? existing.date.format(existing.hasTime ? EDIT : DATE_ONLY)
        : moment().format(EDIT);

    const input = await tp.system.prompt("日期或时间", prefill);
    if (input === null) return null;

    if (existing && input.trim() === prefill) return existing;

    const parsed = parseValue(input);
    if (!parsed) {
        new tp.obsidian.Notice(`Unrecognized date: ${input}`);
        return null;
    }

    return parsed;
};

// 常用的两种不用打字，只有「指定」才需要真的去编辑一个日期。
const source = await tp.system.suggester(
    ["现在", "今天", "指定日期或时间"],
    ["now", "today", "custom"],
    false,
    "日期怎么给"
);

if (!source) return;

let parsed;

if (source === "now") {
    parsed = { date: moment(), hasTime: true };
} else if (source === "today") {
    parsed = { date: moment(), hasTime: false };
} else {
    const api = app.plugins.plugins.modalforms?.api;
    parsed = api ? await askWithForm(api) : await askWithPrompt();
}

if (!parsed) return;

// 能推断出来就别问：有原值跟着原值，输入里带了时刻就必须是 ISO。
let useIso;

if (existingText) {
    useIso = !dateOnlyPattern.test(existingText);
} else if (parsed.hasTime) {
    useIso = true;
} else {
    const format = await tp.system.suggester(
        ["ISO 时间戳", "只到日"],
        [true, false],
        false,
        "写成哪种格式"
    );

    if (format === null || format === undefined) return;
    useIso = format;
}

const value = useIso
    ? (parsed.hasTime ? parsed.date : parsed.date.startOf("day")).format(ISO)
    : parsed.date.format(DATE_ONLY);

await app.fileManager.processFrontMatter(file, (properties) => {
    properties.date = value;
    tp.user.orderFrontmatter(properties);
});

new tp.obsidian.Notice(`date: ${value}`);
-%>
