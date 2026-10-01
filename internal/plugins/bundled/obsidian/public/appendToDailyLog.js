/**
 * 往当天日志的某个标题下追加一条。
 *
 * 日志不存在时先请 Templater 用日志模板建一篇 —— 它是「先建文件、再异步把
 * 模板渲染进去」，所以建完要等标题出现再追加，否则我们写的内容会被整份
 * 渲染盖掉。
 *
 * 调用方给 `makeBlock(index)`，返回一条完整的条目（可以是多行）；index 是
 * 这条在该段落里的序号，段落原本是「1. 无」这种占位时从 1 开始。
 *
 * options：
 *   date          moment 对象，决定写进哪一天
 *   logFolder     日志所在目录
 *   logFormat     文件名格式，跟 Periodic Notes 保持一致（带斜杠就是分子目录）
 *   heading       写在哪个标题下，不含 # 号；建日志用的模板不在这里配，
 *                 那是 Templater 的文件夹模板管的事
 *   createIfMissing
 */

// 日志模板里要查天气、要定位，慢的时候好几秒；等不到就不等了，先把内容写下来。
const TEMPLATE_RENDER_TIMEOUT_MS = 10000;
const TEMPLATE_RENDER_POLL_MS = 200;
// 追加之后按这几个时刻回头确认，覆盖「慢渲染晚到」的各种时机。
const VERIFY_DELAYS_MS = [1500, 4000, 8000];
// 补写的次数上限。一直被覆盖说明有别的东西在写这个文件，不该无限打架。
const MAX_REWRITES = 2;

// 顶层的列表项才算「一条」，缩进的细节行不参与编号。
const LIST_ITEM = /^(?:\d+[.)]|[-*+])\s+/;
// 模板建出来的空段落是「1. 无」，第一条真内容应该顶掉它，而不是排在它后面。
const PLACEHOLDER = /^\s*(?:\d+[.)]|[-*+])?\s*(?:无|None|N\/A|-|—)\s*$/;

module.exports = async function appendToDailyLog(params, options, makeBlock) {

	const { Notice } = params.obsidian;
	const logPath = `${options.logFolder}/${options.date.format(options.logFormat)}.md`;

	const file = await resolveDailyLog(params, logPath, options);
	if (!file) {
		new Notice(`QuickAdd: ${logPath} not found`);
		return false;
	}

	await params.app.vault.process(file, (content) =>
		appendToSection(content, options.heading, makeBlock),
	);

	// 保险：万一还有一次渲染在路上，把刚写的内容盖掉了，就再写一次。
	await verifyAppend(params, file, options, makeBlock);

	return true;
}

/**
 * 追加完之后盯一会儿。
 *
 * 模板渲染是「整份写回」，晚到的那一次会把我们刚追加的内容抹掉。等待稳定
 * 已经能拦住绝大多数情况，但它有超时；超时之后写下去的内容仍可能被覆盖，
 * 而用户是看不见的 —— 所以这里按几个时刻回头查，没了就补写，并且**明确
 * 告诉用户**发生过什么。
 */
async function verifyAppend(params, file, options, makeBlock) {
	const { Notice } = params.obsidian;
	const probe = makeBlock(1).split("\n")[0].replace(/^\s*(?:\d+[.)]|[-*+])\s+/, "");
	let rewrites = 0;

	for (const delay of VERIFY_DELAYS_MS) {
		await new Promise((resolve) => setTimeout(resolve, delay));

		if ((await params.app.vault.cachedRead(file)).includes(probe)) continue;

		if (rewrites >= MAX_REWRITES) {
			new Notice(
				`QuickAdd: 写进 ${file.path} 的内容又被覆盖了，已放弃重试 —— ` +
					"请检查是不是有别的东西在写这篇日志",
				10000,
			);
			return;
		}

		rewrites += 1;
		console.warn(`Appended entry vanished from ${file.path}, writing it again`);
		await params.app.vault.process(file, (value) =>
			appendToSection(value, options.heading, makeBlock),
		);
		new Notice(`日志被模板重写了一次，刚才那条已补回 ${file.path}`);
	}
}

async function resolveDailyLog(params, logPath, options) {
	const { TFile } = params.obsidian;
	const existing = params.app.vault.getAbstractFileByPath(logPath);

	if (existing instanceof TFile) return existing;
	if (existing || !options.createIfMissing) return null;

	return await createDailyLog(params, logPath, options);
}

/**
 * 建当天的日志。
 *
 * 只是创建一个空文件，剩下的交给 Templater 的文件夹模板 —— 它对
 * `00 Daily Log` 配了 folder template，且 trigger_on_file_creation_mode
 * 是 "folder"，任何文件在这个目录里被创建都会触发一次渲染。
 *
 * 千万别再自己调 create_new_note_from_template：那样会渲染两次，我们追加的
 * 内容会被慢的那次整份写回时抹掉。这个 bug 出现过 —— 速记写进去了，几秒后
 * 连同位置一起被另一次渲染覆盖。
 */
async function createDailyLog(params, logPath, options) {
	const { TFile } = params.obsidian;

	await ensureParentFolders(params.app, logPath);
	await params.app.vault.create(logPath, "");

	const file = params.app.vault.getAbstractFileByPath(logPath);
	if (!(file instanceof TFile)) return null;

	const settled = await waitForStableContent(params.app, file, options.heading);

	if (!settled) {
		new params.obsidian.Notice(
			"日志模板还没渲染完就超时了，接下来写入的内容有可能被它覆盖 —— " +
				"如果发现内容丢了，再记一次即可",
			8000,
		);
	}

	// 文件夹模板没跑（比如设置改过），至少给个标题，别让内容无处可去。
	if (!(await params.app.vault.cachedRead(file)).trim()) {
		await params.app.vault.modify(file, `# ${options.heading}\n\n`);
	}

	return file;
}

/**
 * 等模板渲染完：内容里出现了目标标题，而且连续两次读到的内容一样。
 * 稳定了返回 true，超时返回 false —— 超时不是小事，上层要告诉用户。
 *
 * 只看标题不够 —— 模板是一次性写整份的，但渲染前后可能有多次写入（属性、
 * 正文分开写，或者别的插件插一脚）。等它稳定下来再追加，才不会被覆盖。
 */
async function waitForStableContent(app, file, heading) {
	const rendered = new RegExp(`^#{1,6}\\s+${escapeRegExp(heading)}\\s*$`, "m");
	const deadline = Date.now() + TEMPLATE_RENDER_TIMEOUT_MS;
	let previous = null;

	while (Date.now() < deadline) {
		const content = await app.vault.cachedRead(file);

		if (content === previous && rendered.test(content)) return true;

		previous = content;
		await new Promise((resolve) => setTimeout(resolve, TEMPLATE_RENDER_POLL_MS));
	}

	console.warn(`Timed out waiting for the daily log template to render ${file.path}`);
	return false;
}

function appendToSection(content, heading, makeLine) {
	const lines = content.split("\n");
	const headingLine = new RegExp(`^(#{1,6})\\s+${escapeRegExp(heading)}\\s*$`);
	const start = lines.findIndex((line) => headingLine.test(line));

	if (start === -1) {
		return `${content.replace(/\s*$/, "")}\n\n# ${heading}\n\n${makeLine(1)}\n`;
	}

	const level = lines[start].match(/^#+/)[0].length;
	let end = lines.length;
	for (let i = start + 1; i < lines.length; i += 1) {
		const next = lines[i].match(/^(#{1,6})\s+/);
		if (next && next[1].length <= level) {
			end = i;
			break;
		}
	}

	const body = lines.slice(start + 1, end);
	const filled = body.filter((line) => line.trim());
	const placeholderOnly =
		filled.length > 0 && filled.every((line) => PLACEHOLDER.test(line));
	const kept = placeholderOnly ? [] : trimBlankEdges(body);
	const count = kept.filter((line) => LIST_ITEM.test(line)).length;

	return [
		...lines.slice(0, start + 1),
		"",
		...kept,
		makeLine(count + 1),
		"",
		...lines.slice(end),
	].join("\n");
}

function trimBlankEdges(lines) {
	let first = 0;
	let last = lines.length;
	while (first < last && !lines[first].trim()) first += 1;
	while (last > first && !lines[last - 1].trim()) last -= 1;
	return lines.slice(first, last);
}

const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

async function ensureParentFolders(app, filePath) {
	const parts = filePath.split("/").filter(Boolean);
	parts.pop();
	let folder = "";
	for (const part of parts) {
		folder = folder ? `${folder}/${part}` : part;
		if (!app.vault.getAbstractFileByPath(folder)) {
			await app.vault.createFolder(folder);
		}
	}
}
