/**
 * QuickAdd：往当天日志的「速记」段落追加一句话。
 *
 * 和 DGS Toolbox 的 Insert timeline entry 的区别就是不查位置 —— 只要一段文字加一个时间。
 * 写日志的活儿交给 00 Public/appendToDailyLog.js，两条命令共用一份。
 */

const LOG_FOLDER = "Daily log folder";
const LOG_FORMAT = "Daily log file format";
const LOG_HEADING = "Daily log heading";
const LIB_FOLDER = "Lib folder";
const CREATE_IF_MISSING = "Create the daily log when missing";

// 跟着设备走，偏移量写成 +10 这种小时数（见 00 Public/formatOffset.js）。
const TIME = "HH:mm:ss";
const DEFAULT_LIB = "{{dgs:public}}";

module.exports = {
	entry: start,
	settings: {
		name: "Quick Note",
		author: "Douglas",
		options: {
			[LOG_FOLDER]: {
				type: "text",
				defaultValue: "00 Daily Log",
				placeholder: "00 Daily Log",
				description: "日志所在文件夹",
			},
			[LOG_FORMAT]: {
				type: "text",
				defaultValue: "YYYY/YYYY-MM-DD",
				placeholder: "YYYY/YYYY-MM-DD",
				description: "日志的文件名格式，跟 Periodic Notes 保持一致",
			},
			[LOG_HEADING]: {
				type: "text",
				defaultValue: "速记",
				placeholder: "速记",
				description: "写在哪个标题下（不含 # 号）",
			},
			[LIB_FOLDER]: {
				type: "text",
				defaultValue: DEFAULT_LIB,
				placeholder: DEFAULT_LIB,
				description: "共享实现所在目录",
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
	const { Notice, normalizePath } = params.obsidian;

	const note = await resolveNoteText(params);
	if (!note) return;

	const libFolder = normalizePath(text(settings?.[LIB_FOLDER], DEFAULT_LIB));
	const tp = await loadTp(params, libFolder);
	const now = moment();

	const written = await tp.user.appendToDailyLog(
		params,
		{
			date: now,
			logFolder: normalizePath(text(settings?.[LOG_FOLDER], "00 Daily Log")),
			logFormat: text(settings?.[LOG_FORMAT], "YYYY/YYYY-MM-DD"),
			heading: text(settings?.[LOG_HEADING], "速记"),
			createIfMissing: settings?.[CREATE_IF_MISSING] !== false,
		},
		// 时间包成行内代码（等宽让各行数字竖着对齐），再用破折号和正文隔开。
		(index) =>
			`${index}. \`${now.format(TIME)} ${tp.user.formatOffset(now)}\` · ${note}`,
	);

	if (written) new Notice("已记下");
}

async function resolveNoteText(params) {
	// 走 Macro 时上一步可能已经把内容放进 value 了，那就不再问一遍。
	const preset = params.variables?.value;
	if (typeof preset === "string" && preset.trim()) return preset.trim();

	const typed = await params.quickAddApi.inputPrompt("记点什么");
	return typeof typed === "string" ? typed.trim() : "";
}

/**
 * 只手工加载 quickAddShim 这一个文件，其余的由它去装。
 */
async function loadTp(params, libFolder) {
	const path = `${libFolder}/quickAddShim.js`;
	const code = await params.app.vault.adapter.read(path);
	const module = { exports: {} };

	new Function("module", "exports", "require", `${code}\n//# sourceURL=${path}`)(
		module,
		module.exports,
		typeof require === "function" ? require : undefined,
	);

	return module.exports(params, libFolder);
}

const text = (value, fallback) => String(value ?? "").trim() || fallback;
