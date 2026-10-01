<%*
/**
 * 位置和天气都要走网络，等它们会把创建拖成几秒；而模板是「渲染完整份写回」，
 * 那几秒里往正文追加的内容（速记、去过哪里）会被抹掉。
 *
 * 所以这里只写空属性，笔记秒建；查到之后由 backfillLocationWeather 回填，
 * 它用 processFrontMatter，只改属性、不碰正文。故意不 await。
 */

// 日志文件名就是日期。文件名不是日期时（例如从 Untitled 建的），
// 严格解析会失败，这时问一次，否则 frontmatter 会写进 Invalid date。
let date = moment(tp.file.title, "YYYY-MM-DD", true);

if (!date.isValid()) {
    date = tp.user.parseDateInput(moment, tp.file.title)?.date ?? date;
}

if (!date.isValid()) {
    const dateInput = await tp.system.prompt("Date", moment().format("YYYY-MM-DD"));
    const parsed = dateInput?.trim()
        ? tp.user.parseDateInput(moment, dateInput)
        : null;

    if (dateInput?.trim() && !parsed) {
        new tp.obsidian.Notice(`Unrecognized date: ${dateInput}. Using today.`);
    }

    date = parsed?.date ?? moment();
}

// 不 await：让笔记先出现，位置和天气在后台补。
// 日志是一整天，气温回填的是当天的最高和最低，不是创建那一刻的温度。
const folder = tp.file.folder(true);
tp.user.backfillLocationWeather(tp, `${folder}/${tp.file.title}.md`, {
    dailyTemperature: true,
    // 国旗国家标签是日志的结构性标签，普通笔记不加。
    countryTag: true,
    // 传 moment 本身，带着时区 —— 查的是那个地方的那一天。
    date
});
-%>
---
date: <% date.format("YYYY-MM-DD") %>
dayOfTheYear: <% date.format("DDDD") %>
createdAt: <% tp.date.now("YYYY-MM-DDTHH:mm:ssZ") %>
coordinates: ""
country: ""
region: ""
city: ""
locality: ""
temperature: ""
weather: ""
weatherNote: ""
tags: ["log"]
people: []
mood: []
---

# 速记

# 今日活动

# 去过哪里

--
