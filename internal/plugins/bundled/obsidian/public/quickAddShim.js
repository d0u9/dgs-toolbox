/**
 * 把 QuickAdd 的 params 包成 Templater 的 `tp`，让这个目录里的脚本不必知道
 * 自己被谁调用 —— 它们只依赖 `tp.obsidian`、`tp.user`、`tp.system`。
 *
 * 这个文件住在 00 Public，是因为它服务的是「实现层怎么被别人用」；QuickAdd 那边
 * 只需要读它、eval 它，剩下的加载工作在这里做一次。
 *
 * 必须一次把整个目录都加载好：getLocation 内部是同步调用 tp.user.countryName
 * 和 tp.user.formatCoordinates 的，惰性加载会让它们变成 Promise。
 */
module.exports = async function quickAddShim(params, libFolder) {
	const user = await loadLib(params.app, libFolder);

	return {
		obsidian: params.obsidian,
		user,
		system: {
			// Templater 的 prompt 取消时返回 null，QuickAdd 返回 undefined，
			// 统一成 null，脚本里的 `=== null` 判断才成立。
			prompt: async (header, defaultValue) => {
				const value = await params.quickAddApi.inputPrompt(
					header,
					"",
					defaultValue ?? "",
				);
				return value === undefined ? null : value;
			},
			suggester: async (displayItems, actualItems) => {
				const value = await params.quickAddApi.suggester(
					displayItems,
					actualItems,
				);
				return value === undefined ? null : value;
			},
		},
	};
};

async function loadLib(app, folder) {
	// .js 不是 Obsidian 认识的笔记格式，用 adapter 直接读文件系统最省事。
	// 子目录也要进来：vendor/ 里的库和顶层脚本一样按文件名注册，和 Templater
	// 递归扫描的结果保持一致。
	const user = {};

	const walk = async (path) => {
		const listing = await app.vault.adapter.list(path);

		for (const file of listing?.files ?? []) {
			if (!file.endsWith(".js")) continue;
			const name = file.split("/").pop().replace(/\.js$/, "");
			try {
				user[name] = evaluateScript(await app.vault.adapter.read(file), file);
			} catch (error) {
				console.warn(`Loading ${file} failed:`, error);
			}
		}

		for (const child of listing?.folders ?? []) await walk(child);
	};

	await walk(folder);

	return user;
};

function evaluateScript(code, path) {
	const module = { exports: {} };
	const load = new Function(
		"module",
		"exports",
		"require",
		`${code}\n//# sourceURL=${path}`,
	);

	// getDeviceLocation 在桌面端要 require("child_process")，移动端没有
	// require，它自己会走 typeof 判断退回 null。
	load(
		module,
		module.exports,
		typeof require === "function" ? require : undefined,
	);

	return module.exports;
}
