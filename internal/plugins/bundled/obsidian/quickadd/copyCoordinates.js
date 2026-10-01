/**
 * QuickAdd：把坐标放进剪贴板，顺序由设置决定。
 *
 * 库里统一用「纬度, 经度」—— frontmatter 的 coordinates、正文里显示的、
 * Apple 地图要的，都是这个顺序，所以默认原样复制、不做翻转。
 * 「经度, 纬度」那份留给外面的工具（有些地图 API 和 GeoJSON 是经度在前）。
 *
 * 三种用法：
 *   点链接   `[(纬度, 经度)](obsidian://quickadd?choice=Copy%20Coordinates&value-coordinates=...)`
 *   选中     选中一段带两个数的文字，跑这条命令
 *   光标     什么都不选，取光标所在行；行里没有就取本篇 frontmatter 的 coordinates
 */

const OUTPUT_ORDER = "Clipboard order";
const FRONTMATTER_KEY = "Frontmatter property";

// 纬度只到 ±90，有一个数超过 90 就能定下顺序。两个都在 ±90 以内时问一次。
const MAX_LATITUDE = 90;
const NUMBER = /-?\d+(?:\.\d+)?/g;

module.exports = {
	entry: start,
	settings: {
		name: "Copy Coordinates",
		author: "Douglas",
		options: {
			[OUTPUT_ORDER]: {
				type: "dropdown",
				defaultValue: "纬度, 经度",
				options: ["纬度, 经度", "经度, 纬度"],
				description: "复制进剪贴板的顺序。库里到处都是「纬度, 经度」",
			},
			[FRONTMATTER_KEY]: {
				type: "text",
				defaultValue: "coordinates",
				placeholder: "coordinates",
				description: "行里找不到坐标时，退回读这个属性",
			},
		},
	},
};

async function start(params, settings) {
	const { Notice } = params.obsidian;
	const lngFirst = String(settings?.[OUTPUT_ORDER] ?? "纬度, 经度").trim() === "经度, 纬度";
	const key = String(settings?.[FRONTMATTER_KEY] ?? "coordinates").trim() || "coordinates";

	const source = resolveSource(params, key);

	if (!source) {
		new Notice("没找到坐标：选中一段文字、把光标放到带坐标的行上，或者让本篇有 coordinates 属性");
		return;
	}

	const parsed = await parseCoordinates(params, source);
	if (!parsed) return;

	const value = lngFirst
		? `${parsed.longitude}, ${parsed.latitude}`
		: `${parsed.latitude}, ${parsed.longitude}`;

	await copy(value);
	new Notice(`已复制：${value}`);
}

/**
 * 链接传进来的最优先，它自带顺序（我们生成的链接一律「纬度, 经度」）。
 */
function resolveSource(params, key) {
	const fromUri = params.variables?.coordinates;
	if (typeof fromUri === "string" && fromUri.trim()) {
		return { text: fromUri, order: "lat-first" };
	}

	const editor = params.app.workspace.activeEditor?.editor;

	if (editor) {
		const selection = editor.getSelection();
		if (selection.trim() && hasTwoNumbers(selection)) {
			return { text: selection, order: "unknown" };
		}

		const line = editor.getLine(editor.getCursor().line);
		if (hasTwoNumbers(line)) return { text: line, order: "unknown" };
	}

	const file = params.app.workspace.getActiveFile();
	const frontmatter = file
		? params.app.metadataCache.getFileCache(file)?.frontmatter
		: null;
	const property = frontmatter?.[key];

	// frontmatter 里的写法是固定的（「纬度, 经度」），不用猜。
	if (typeof property === "string" && hasTwoNumbers(property)) {
		return { text: property, order: "lat-first" };
	}

	return null;
}

const numbersIn = (text) => (String(text).match(NUMBER) ?? []).map(Number);

const hasTwoNumbers = (text) => numbersIn(text).length >= 2;

async function parseCoordinates(params, source) {
	const [first, second] = numbersIn(source.text);

	if (!Number.isFinite(first) || !Number.isFinite(second)) return null;

	if (source.order === "lat-first") {
		return { latitude: first, longitude: second };
	}

	if (source.order === "lng-first") {
		return { latitude: second, longitude: first };
	}

	// 超过 ±90 的那个只能是经度。
	if (Math.abs(first) > MAX_LATITUDE) return { latitude: second, longitude: first };
	if (Math.abs(second) > MAX_LATITUDE) return { latitude: first, longitude: second };

	const chosen = await params.quickAddApi.suggester(
		[`${first} 是纬度，${second} 是经度`, `${first} 是经度，${second} 是纬度`],
		["lat-first", "lng-first"],
	);

	if (!chosen) return null;

	return chosen === "lat-first"
		? { latitude: first, longitude: second }
		: { latitude: second, longitude: first };
}

async function copy(value) {
	try {
		await navigator.clipboard.writeText(value);
	} catch (error) {
		console.warn("navigator.clipboard failed, falling back to Electron:", error);
		// 移动端没有 electron，桌面端偶尔会因为窗口没聚焦被拒。
		const electron = typeof require === "function" ? require("electron") : null;
		const clipboard = electron?.clipboard ?? electron?.remote?.clipboard;
		if (!clipboard) throw error;
		clipboard.writeText(value);
	}
}
