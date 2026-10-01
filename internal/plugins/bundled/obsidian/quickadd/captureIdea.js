/**
 * QuickAdd：把一个想法直接记进 Writing 的想法清单。
 *
 * 想法不进日志 —— 散在几十篇日志里就没法一眼看完，而清单这一份是按日期
 * 倒序的连续列表，翻起来就是一条线。
 *
 * 今天已经有 `# YYYY-MM-DD` 这一节就接在它末尾（当天的想法按先后排），
 * 没有就在文件最顶上开一节新的。
 */

const TARGET_PATH = "Idea note path";
const LIB_FOLDER = "Lib folder";
const DEFAULT_LIB = "{{dgs:public}}";
const DATE_ONLY = "YYYY-MM-DD";
// 跟着设备走，偏移量写成 +10 这种小时数（见 00 Public/formatOffset.js）。
const TIME = "HH:mm:ss";

// 顶层的编号项才算「一条」，缩进的子项不参与编号。
const NUMBERED = /^\d+[.)]\s+/;
// 元信息缩进一层，和速记那边的三行细节一个形状。
const INDENT = "    ";

module.exports = {
	entry: start,
	settings: {
		name: "Idea",
		author: "Douglas",
		options: {
			[TARGET_PATH]: {
				type: "text",
				defaultValue: "32 Writing/01 Thoughts/01 Ideas.md",
				placeholder: "32 Writing/01 Thoughts/01 Ideas.md",
				description: "想法记到哪个文件",
			},
			[LIB_FOLDER]: {
				type: "text",
				defaultValue: DEFAULT_LIB,
				placeholder: DEFAULT_LIB,
				description: "共享实现所在目录，位置从这里查",
			},
		},
	},
};

async function start(params, settings) {
	const { Notice, TFile, normalizePath } = params.obsidian;

	const idea = await resolveText(params);
	if (!idea) return;

	const path = normalizePath(
		String(settings?.[TARGET_PATH] ?? "").trim() ||
			"32 Writing/01 Thoughts/01 Thought Clues.md",
	);
	const heading = `# ${moment().format(DATE_ONLY)}`;
	const file = params.app.vault.getAbstractFileByPath(path);

	if (!(file instanceof TFile)) {
		new Notice(`QuickAdd: ${path} 不存在`);
		return;
	}

	// 有时候是触景生情，所以把时间和地点也记下来。查不到就只留时间。
	const libFolder = String(settings?.[LIB_FOLDER] ?? "").trim() || DEFAULT_LIB;
	const tp = await loadTp(params, libFolder);
	const now = moment();
	const place = await resolvePlace(tp);
	const time = `\`${now.format(TIME)} ${tp.user.formatOffset(now)}\``;
	const meta = [time, place].filter(Boolean).join(" · ");

	// 想法本身单独一行，时间和地点缩进到下面 —— 挤在一行里，最该看的
	// 那句话反而被前缀盖住了。
	await params.app.vault.process(file, (content) =>
		addIdea(content, heading, `${idea}\n${INDENT}- ${meta}`),
	);

	new Notice(place ? `已记下：${place}` : "已记下这个想法");
}

async function resolveText(params) {
	// 走 Macro 时上一步可能已经把内容放进 value 了，那就不再问一遍。
	const preset = params.variables?.value;
	if (typeof preset === "string" && preset.trim()) return preset.trim();

	const typed = await params.quickAddApi.inputPrompt("想到了什么");
	return typeof typed === "string" ? typed.trim() : "";
}

/**
 * 地点只取最细的一两级 —— 「触景生情」要的是「在哪儿」，四级全写太占地方。
 */
async function resolvePlace(tp) {
	try {
		const location = await tp.user.getLocation(tp, tp.user.requestJson);

		return [location?.locality, location?.city]
			.filter(Boolean)
			.filter((value, index, all) => all.indexOf(value) === index)
			.join(", ")
			|| location?.region
			|| location?.country
			|| "";
	} catch (error) {
		console.warn("Location lookup failed:", error);
		return "";
	}
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

function addIdea(content, heading, idea) {
	// 文件顶上的 frontmatter 和说明 callout 不是内容，新的一节要插在它们下面。
	const { preamble, body } = splitPreamble(content);
	const lines = body.split("\n");
	const first = lines.findIndex((line) => line.trim());
	const join = (value) => (preamble ? `${preamble}\n\n${value}` : value);

	// 今天还没有这一节：在正文最顶上开一节。
	if (first === -1 || lines[first].trim() !== heading) {
		const rest = body.replace(/^\s*/, "");
		return join(
			rest ? `${heading}\n\n1. ${idea}\n\n${rest}` : `${heading}\n\n1. ${idea}\n`,
		);
	}

	// 这一节到下一个 # 标题为止
	let end = lines.length;
	for (let i = first + 1; i < lines.length; i += 1) {
		if (/^#{1,6}\s+/.test(lines[i])) {
			end = i;
			break;
		}
	}

	const section = lines.slice(first + 1, end);

	// 段首的空行留在标题下面，新条目插在它之后、已有条目之前。
	let start = 0;
	while (start < section.length && !section[start].trim()) start += 1;

	// 新的在最上面，所以插完要把顶层编号重排一遍（子项不动）。
	const merged = renumber([`1. ${idea}`, ...section.slice(start)]);

	return join([
		...lines.slice(0, first + 1),
		...section.slice(0, start),
		...merged,
		...lines.slice(end),
	].join("\n"));
}

/**
 * 顶层编号重排成 1、2、3……缩进的子项和空行原样保留。
 */
function renumber(lines) {
	let index = 0;

	return lines.map((line) => {
		if (!NUMBERED.test(line)) return line;
		index += 1;
		return line.replace(NUMBERED, `${index}. `);
	});
}

/**
 * 开头的 frontmatter 和紧随其后的引用块（`>` 开头）都算说明，不是内容。
 */
function splitPreamble(content) {
	const lines = content.split("\n");
	let index = 0;

	if (lines[0]?.trim() === "---") {
		const close = lines.findIndex((line, at) => at > 0 && line.trim() === "---");
		if (close !== -1) index = close + 1;
	}

	while (index < lines.length && !lines[index].trim()) index += 1;
	while (index < lines.length && lines[index].startsWith(">")) index += 1;

	let end = index;
	while (end > 0 && !lines[end - 1].trim()) end -= 1;

	return {
		preamble: lines.slice(0, end).join("\n"),
		// 说明和正文之间的空行由拼接时统一补，别让它跟着正文走。
		body: lines.slice(end).join("\n").replace(/^\n+/, ""),
	};
}
