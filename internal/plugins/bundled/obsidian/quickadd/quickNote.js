/**
 * QuickAdd：往当天日志的「速记」段落追加一句话。
 *
 * 和 DGS Toolbox 的 Insert timeline entry 的区别就是不查位置 —— 只要一段文字加一个时间。
 * 条目由 DGS Toolbox 写：它用条目模板（默认 quick-note.md）经 timeline 引擎渲染，
 * 再由引擎决定放在段落的哪里，和 dgs capture 是同一份代码。
 * 日志在哪个文件夹、叫什么名字，看 DGS Toolbox 的 Daily log 设置。
 *
 * 没有 DGS Toolbox（没装或没开）就不写，并提示一声：一种格式只有一份实现。
 */

const LOG_HEADING = "Daily log heading";
const TEMPLATE = "Entry template";
const CREATE_IF_MISSING = "Create the daily log when missing";

module.exports = {
	entry: start,
	settings: {
		name: "Quick Note",
		author: "Douglas",
		options: {
			[LOG_HEADING]: {
				type: "text",
				defaultValue: "速记",
				placeholder: "速记",
				description: "写在哪个标题下（不含 # 号）",
			},
			[TEMPLATE]: {
				type: "text",
				defaultValue: "quick-note.md",
				placeholder: "quick-note.md",
				description: "条目模板，在 DGS Toolbox 的 Entry templates 文件夹里",
			},
			[CREATE_IF_MISSING]: {
				type: "toggle",
				defaultValue: true,
				description: "当天日志不存在时创建它",
			},
		},
	},
};

async function start(params, settings) {
	const { Notice } = params.obsidian;

	const api = params.app.plugins?.plugins?.["dgs-toolbox"]?.api;
	if (!api?.appendToDailyLog) {
		new Notice("Quick Note 需要 DGS Toolbox：请先安装并打开它");
		return;
	}

	const note = await resolveNoteText(params);
	if (!note) return;

	try {
		await api.appendToDailyLog({
			template: text(settings?.[TEMPLATE], "quick-note.md"),
			heading: text(settings?.[LOG_HEADING], "速记"),
			text: note,
			createIfMissing: settings?.[CREATE_IF_MISSING] !== false,
		});
	} catch (error) {
		console.error("Quick Note failed", error);
		new Notice(`没记下：${error?.message ?? String(error)}`, 10000);
		return;
	}
	new Notice("已记下");
}

async function resolveNoteText(params) {
	// 走 Macro 时上一步可能已经把内容放进 value 了，那就不再问一遍。
	const preset = params.variables?.value;
	if (typeof preset === "string" && preset.trim()) return preset.trim();

	const typed = await params.quickAddApi.inputPrompt("记点什么");
	return typeof typed === "string" ? typed.trim() : "";
}

const text = (value, fallback) => String(value ?? "").trim() || fallback;
