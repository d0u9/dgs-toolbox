/*
 * The Obsidian plugin's settlePictures, against a vault held in memory. Obsidian
 * is not there to run it, so the part of its API the plugin calls is stood in
 * for here; the engine is the real one, loaded as engine.test.mjs loads it.
 *
 * Run with make test-timeline, which builds the module first.
 */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = fileURLToPath(new URL('.', import.meta.url));
const plugin = join(here, 'obsidian', 'dgs-toolbox');

vm.runInThisContext(
	[join(plugin, 'generated', 'wasm_exec.js'), join(plugin, 'engine.js')]
		.map((path) => readFileSync(path, 'utf8'))
		.join('\n'),
);
const engine = await globalThis.startDgsTimeline(readFileSync(join(plugin, 'generated', 'timeline.wasm')));

class TFile {
	/** @param {string} path */
	constructor(path) {
		this.path = path;
	}
}
class Stub {}
/** @type {string[]} */
const notices = [];
const obsidian = {
	AbstractInputSuggest: Stub, Modal: Stub, Plugin: Stub, PluginSettingTab: Stub, Setting: Stub, TFolder: Stub, TFile,
	Notice: class {
		/** @param {string} message */
		constructor(message) {
			notices.push(message);
		}
	},
	moment: () => ({}),
	normalizePath: (/** @type {string} */ path) => path.replace(/\/+/g, '/').replace(/^\/|\/$/g, ''),
};
const { TimelineService } = vm.runInNewContext(
	`${readFileSync(join(plugin, 'main.js'), 'utf8')}\n;({ TimelineService })`,
	{ require: () => obsidian, module: {}, console },
);

/**
 * A vault of files and folders by path, with the Custom Attachment Location
 * layout: a note's attachments go in assets/<note> beside it.
 *
 * @param {string[]} files
 */
function vault(files) {
	const entries = new Map(files.map((path) => [path, new TFile(path)]));
	/** @type {string[]} */
	const asked = [];
	const app = {
		vault: {
			getAbstractFileByPath: (/** @type {string} */ path) => entries.get(path) ?? null,
			createFolder: async (/** @type {string} */ path) => {
				entries.set(path, {});
			},
			rename: async (/** @type {TFile} */ file, /** @type {string} */ path) => {
				if (app.failRename) throw new Error('disk full');
				entries.delete(file.path);
				entries.set(path, new TFile(path));
			},
		},
		fileManager: {
			getAvailablePathForAttachment: async (/** @type {string} */ name, /** @type {string} */ note) => {
				asked.push(note);
				const folder = note.slice(0, note.lastIndexOf('/'));
				const base = note.slice(note.lastIndexOf('/') + 1).replace(/\.md$/, '');
				return `${folder}/assets/${base}/${name}`;
			},
		},
		failRename: false,
	};
	return { app, has: (/** @type {string} */ path) => entries.get(path) instanceof TFile, asked };
}

const note = '88 Inbox/06 Locations.md';
const archive = '88 Inbox/06 Locations/2025.md';
const picture = 'file-20251231230000000-1.jpg';
const running = `88 Inbox/assets/06 Locations/${picture}`;
const daily = `00 Daily Log/2025/assets/2025-12-31/${picture}`;
const settled = `88 Inbox/06 Locations/assets/2025/${picture}`;
const year = `- *2025-12-31 周三*\n- \`23:00:00 +10\` · 去年 ^dgs-b\n    - ![[${picture}|200]]\n    - ![[gone.jpg]]\n`;

test('a year moving into its archive takes its pictures along, and the daily copy stays', async () => {
	notices.length = 0;
	const { app, has } = vault([note, archive, running, daily]);
	await new TimelineService(app, '', () => '').settlePictures(engine, [note, archive], [{ path: archive, text: year }]);
	assert.ok(has(settled), 'the picture is not beside its archive');
	assert.ok(!has(running), 'the picture is still beside the running note');
	assert.ok(has(daily), "the daily log's copy moved");
	assert.deepEqual(notices, []);
});

test('a picture already in place is kept, and nothing is overwritten', async () => {
	const { app, has } = vault([note, archive, running, settled]);
	await new TimelineService(app, '', () => '').settlePictures(engine, [note, archive], [{ path: archive, text: year }]);
	assert.ok(has(running) && has(settled));
});

test('a file with no pictures asks for no attachment folder', async () => {
	const { app, asked } = vault([note, archive]);
	await new TimelineService(app, '', () => '').settlePictures(engine, [note, archive], [{ path: archive, text: '- *2025-12-31 周三*\n' }]);
	assert.deepEqual(asked, []);
});

test('a picture that cannot move is said, not thrown', async () => {
	notices.length = 0;
	const { app, has } = vault([note, archive, running]);
	app.failRename = true;
	await new TimelineService(app, '', () => '').settlePictures(engine, [note, archive], [{ path: archive, text: year }]);
	assert.ok(has(running));
	assert.equal(notices.length, 1);
	assert.match(notices[0], /disk full/);
});
