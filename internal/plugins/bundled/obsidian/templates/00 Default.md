<%*
/**
 * 位置和天气都要走网络，等它们会把创建拖成几秒。这里只写空属性，笔记秒建；
 * 查到之后由 backfillLocationWeather 回填，它用 processFrontMatter，
 * 只改属性、不碰正文。故意不 await。
 */

const folder = tp.file.folder(true);
tp.user.backfillLocationWeather(tp, `${folder}/${tp.file.title}.md`);
-%>
---
createdAt: <% tp.date.now("YYYY-MM-DDTHH:mm:ssZ") %>
coordinates: ""
country: ""
region: ""
city: ""
locality: ""
temperature:
weather: ""
weatherNote: ""
tags: []

---
