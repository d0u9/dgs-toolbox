/*
 * The Obsidian plugin writing an entry: which template it renders with, what it
 * hands the engine, and how a daily log's section is filled. The vault is held
 * in memory and the engine is the real one, loaded as engine.test.mjs loads it,
 * so what the plugin writes is what dgs capture writes for the same input.
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
const obsidian = {
	AbstractInputSuggest: Stub, Modal: Stub, Plugin: Stub, PluginSettingTab: Stub, Setting: Stub, TFolder: Stub, TFile,
	Notice: Stub,
	moment: () => ({}),
	normalizePath: (/** @type {string} */ path) => path.replace(/\/+/g, '/').replace(/^\/|\/$/g, ''),
};
const { DgsToolboxPlugin, TimelineService, VaultLib, entryInput, timelineTemplates, dailyLogDate, needsBackfill } = vm.runInNewContext(
	`${readFileSync(join(plugin, 'main.js'), 'utf8')}\n;({ DgsToolboxPlugin, TimelineService, VaultLib, entryInput, timelineTemplates, dailyLogDate, needsBackfill })`,
	{ require: () => obsidian, module: {}, console },
);

const folder = '99 Toolkit/03 Entry Templates';

/**
 * A vault holding these files, by path.
 *
 * @param {Record<string, string>} files
 */
function service(files) {
	const app = {
		vault: {
			getAbstractFileByPath: (/** @type {string} */ path) => (path in files ? new TFile(path) : null),
			read: async (/** @type {TFile} */ file) => files[file.path],
		},
	};
	const timelines = new TimelineService(app, '', () => '', () => folder);
	timelines.engine = async () => engine;
	return timelines;
}

/** A moment as parseWhen gives one: its wall clock in the offset it happened in. */
const when = { format: (/** @type {string} */ layout) => (layout === 'YYYY-MM-DDTHH:mm:ssZ' ? '2026-11-08T11:35:44+11:00' : '') };

/** A place as the vault's getLocation gives one: coordinates as strings, to eight decimals. */
const place = {
	country: 'Australia', region: 'New South Wales', city: 'Sydney', locality: 'The Rocks',
	latitude: '-33.86123400', longitude: '151.21234500',
};

const daily = '- `{{.Clock}} {{.Offset}}`{{with .ContentLines}} · {{index . 0}}{{end}} {{.ID}}\n'
	+ '    - {{.Address}}\n'
	+ '{{- if .Latitude}}\n'
	+ '    - {{.CopyLink "Copy Position"}}{{with .Altitude}}, {{.}}{{end}}\n'
	+ '    - {{.MapLinks "Apple, Google"}}\n'
	+ '{{- end}}\n'
	+ '{{- range $index, $line := .ContentLines}}{{if $index}}\n'
	+ '    - {{$line}}{{end}}{{end}}\n';

test('an entry is what the vault template says, written by the engine', async () => {
	const block = await service({ [`${folder}/daily-entry.md`]: daily }).render(['daily-entry.md'], {
		when, text: '可能因为前一天攀岩\n  手指出血了  \n', place,
	});
	assert.equal(block, [
		'- `11:35:44 +11` · 可能因为前一天攀岩',
		'    - Australia, New South Wales, Sydney, The Rocks',
		'    - [(-33.86123, 151.21234)](obsidian://quickadd?choice=Copy%20Position&value-coordinates=-33.86123%2C%20151.21234)',
		'    - [Apple](https://maps.apple.com/place?coordinate=-33.86123,151.21234&name=The%20Rocks) · [Google](https://www.google.com/maps/search/?api=1&query=-33.86123,151.21234)',
		'    - 手指出血了',
	].join('\n'));
});

test('an entry written without a place has no place lines and no trailing space', async () => {
	const block = await service({ [`${folder}/daily-entry.md`]: daily }).render(['daily-entry.md'], { when, text: 'here', place: null });
	assert.equal(block, '- `11:35:44 +11` · here');
});

test('a vault without the template gets the engine’s own, the one dgs capture falls back to', async () => {
	const block = await service({}).render(['daily-entry.md'], { when, text: 'here', place: null });
	assert.equal(block, engine.call('entry', { name: 'daily-entry.md', input: { createdAt: '2026-11-08T11:35:44+11:00', content: 'here' } }));
	assert.match(block, /^- Date: 2026-11-08 11:35:44 \+11$/m);
});

test('a timeline tries its own template, then timeline-entry.md, then location-entry.md', async () => {
	const own = { name: 'family', note: 'Family.md', template: 'family-entry.md' };
	assert.deepEqual([...timelineTemplates(own)], ['family-entry.md', 'timeline-entry.md', 'location-entry.md']);
	assert.deepEqual([...timelineTemplates({ name: 'places', note: 'Places.md' })], ['timeline-entry.md', 'location-entry.md']);
	assert.deepEqual([...timelineTemplates({ name: 'places', note: 'Places.md', template: '  ' })], ['timeline-entry.md', 'location-entry.md']);

	const entry = { when, text: 'here', place: null };
	const names = timelineTemplates(own);
	assert.equal(await service({ [`${folder}/family-entry.md`]: '- own {{.Content}}', [`${folder}/location-entry.md`]: '- place {{.Content}}' })
		.render(names, entry), '- own here');
	assert.equal(await service({ [`${folder}/location-entry.md`]: '- place {{.Content}}' }).render(names, entry), '- place here');
	// None in the vault: the first the engine carries, which is location-entry.md.
	assert.equal(await service({}).render(names, entry), '- `11:35:44 +11` · here');
});

test('no template anywhere is said, naming what was looked for', async () => {
	await assert.rejects(service({}).render(['nothing.md'], { when, text: '', place: null }), /nothing\.md/);
});

test('a template the engine cannot read is an error, not a broken entry', async () => {
	await assert.rejects(service({ [`${folder}/daily-entry.md`]: '{{.Clock' }).render(['daily-entry.md'], { when, text: '', place: null }), /template daily-entry\.md/);
	await assert.rejects(service({ [`${folder}/daily-entry.md`]: '{{.Nope}}' }).render(['daily-entry.md'], { when, text: '', place: null }), /Nope/);
});

test('the input carries the place only when it has one, and coordinates only when both are numbers', () => {
	assert.deepEqual(JSON.parse(JSON.stringify(entryInput({ when, text: '  x  ', place: null }))), { createdAt: '2026-11-08T11:35:44+11:00', content: 'x' });
	const placed = JSON.parse(JSON.stringify(entryInput({ when, text: '', place })));
	assert.deepEqual(placed.position, { latitude: -33.861234, longitude: 151.212345 });
	assert.equal(placed.place.locality, 'The Rocks');
	assert.equal(placed.id, undefined, 'an entry written here carries no block id');
	const nameless = JSON.parse(JSON.stringify(entryInput({ when, text: '', place: { ...place, latitude: '', longitude: '' } })));
	assert.equal(nameless.position, undefined);
	const broken = JSON.parse(JSON.stringify(entryInput({ when, text: '', place: { ...place, latitude: 'n/a' } })));
	assert.equal(broken.position, undefined);
});

test('the daily log gets the entry where the engine puts it, under its heading at any level', async () => {
	/** @type {any} */
	let options;
	const lib = new VaultLib({}, () => ({ dailyFolder: '00 Daily Log', dailyFormat: 'YYYY/YYYY-MM-DD', dailyHeading: '去过哪里' }));
	lib.tp = async () => ({
		user: {
			appendToDailyLog: async (/** @type {unknown} */ _params, /** @type {any} */ given) => {
				options = given;
				return true;
			},
		},
	});
	const day = { format: () => '2026/2026-11-08' };
	const path = await lib.appendToDailyLog(day, '- `11:35:44 +11` · here', engine);
	assert.equal(path, '00 Daily Log/2026/2026-11-08.md');
	assert.equal(options.heading, '去过哪里');
	assert.equal(
		options.insert('# 速记\n\n## 去过哪里\n\n1. 无\n\n# 今日活动\n'),
		'# 速记\n\n## 去过哪里\n\n- `11:35:44 +11` · here\n\n# 今日活动\n',
	);
});

/**
 * A VaultLib whose vault scripts are stood in for: appendToDailyLog records
 * what it was asked and says it wrote.
 */
function recordingLib() {
	/** @type {{options: any}} */
	const seen = { options: null };
	const lib = new VaultLib({}, () => ({ dailyFolder: '00 Daily Log', dailyFormat: 'YYYY/YYYY-MM-DD', dailyHeading: '去过哪里' }));
	lib.tp = async () => ({
		user: {
			appendToDailyLog: async (/** @type {unknown} */ _params, /** @type {any} */ options) => {
				seen.options = options;
				return true;
			},
		},
	});
	return { lib, seen };
}

/** The plugin as onload leaves it, over an in-memory vault. */
function loadedPlugin(/** @type {Record<string, string>} */ files) {
	const plugin = Object.create(DgsToolboxPlugin.prototype);
	const { lib, seen } = recordingLib();
	plugin.timelines = service(files);
	plugin.vaultLib = lib;
	return { plugin, seen };
}

const day = { format: (/** @type {string} */ layout) => (layout === 'YYYY-MM-DDTHH:mm:ssZ' ? '2026-11-13T09:05:00+11:00' : '2026/2026-11-13') };

test('the plugin’s API writes a quick note under its own heading, with the built-in template', async () => {
	const { plugin, seen } = loadedPlugin({});
	const path = await plugin.appendToDailyLog({ template: 'quick-note.md', heading: '速记', text: '买牛奶\n还有面包', when: day });
	assert.equal(path, '00 Daily Log/2026/2026-11-13.md');
	assert.equal(seen.options.heading, '速记');
	assert.equal(seen.options.createIfMissing, true);
	assert.equal(
		seen.options.insert('# 速记\n\n- 手写的一条\n\n# 去过哪里\n\n1. 无\n'),
		'# 速记\n\n- 手写的一条\n- `09:05:00 +11` · 买牛奶\n    - 还有面包\n\n# 去过哪里\n\n1. 无\n',
	);
});

test('the vault’s quick-note.md wins over the built-in one', async () => {
	const { plugin, seen } = loadedPlugin({ [`${folder}/quick-note.md`]: '- {{.Clock}} {{.Content}}' });
	await plugin.appendToDailyLog({ template: 'quick-note.md', heading: '速记', text: 'x', when: day });
	assert.equal(seen.options.insert('# 速记\n'), '# 速记\n\n- 09:05:00 x\n');
});

test('without a heading the entry goes under the Log heading setting, and a missing log can be left alone', async () => {
	const { plugin, seen } = loadedPlugin({});
	await plugin.appendToDailyLog({ template: 'quick-note.md', text: 'x', when: day, createIfMissing: false });
	assert.equal(seen.options.heading, '去过哪里');
	assert.equal(seen.options.createIfMissing, false);
	await plugin.appendToDailyLog({ template: 'quick-note.md', heading: '   ', text: 'x', when: day });
	assert.equal(seen.options.heading, '去过哪里', 'a blank heading is no heading');
});

test('a log that cannot be found or created is an error the caller sees', async () => {
	const { plugin } = loadedPlugin({});
	plugin.vaultLib.tp = async () => ({ user: { appendToDailyLog: async () => false } });
	await assert.rejects(plugin.appendToDailyLog({ template: 'quick-note.md', text: 'x', when: day }), /could not be found or created/);
});

/** A moment for a day, as far as the plugin reads one. */
function dayOf(/** @type {string} */ iso) {
	return { format: (/** @type {string} */ layout) => (layout === 'YYYY-MM-DD' ? iso : `${iso}T08:00:00+11:00`), isValid: () => true };
}

test('a day’s log is the engine’s daily-note.md: the vault’s, else the plugin’s own', async () => {
	const now = { format: () => '2026-11-13T09:05:00+11:00' };
	const builtin = await service({}).renderNote(dayOf('2026-01-05'), now);
	assert.equal(builtin, engine.call('note', { input: { date: '2026-01-05', now: '2026-11-13T09:05:00+11:00' } }));
	assert.match(builtin, /^---\ndate: 2026-01-05\ndayOfTheYear: 005\ncreatedAt: 2026-11-13T09:05:00\+11:00\ncoordinates: ""\n/);
	assert.match(builtin, /\n# 速记\n\n# 今日活动\n\n# 去过哪里\n/);
	const own = await service({ [`${folder}/daily-note.md`]: '---\ndate: {{.Date}}\n---\n' }).renderNote(dayOf('2026-01-05'), now);
	assert.equal(own, '---\ndate: 2026-01-05\n---\n');
});

const logSettings = { dailyFolder: '00 Daily Log', dailyFormat: 'YYYY/YYYY-MM-DD' };
/** A strict parse of the one format the tests use. */
const parse = (/** @type {string} */ text, /** @type {string} */ format) => {
	const valid = format === 'YYYY/YYYY-MM-DD' && /^(\d{4})\/\1-\d{2}-\d{2}$/.test(text);
	// Only 2026-12-xx is after "now" in these tests.
	return { isValid: () => valid, text, isAfter: () => text.includes('-12-') };
};

test('a path is a day’s log only inside the Log folder, named by the Log file name format', () => {
	assert.equal(dailyLogDate('00 Daily Log/2026/2026-11-08.md', logSettings, parse)?.text, '2026/2026-11-08');
	for (const path of ['00 Daily Log/2026/notes.md', '00 Daily Log/2026/2026-11-08.canvas', 'Elsewhere/2026/2026-11-08.md', '00 Daily Logs/2026/2026-11-08.md']) {
		assert.equal(dailyLogDate(path, logSettings, parse), null, path);
	}
	assert.equal(dailyLogDate('00 Daily Log/2026/2026-11-08.md', { ...logSettings, dailyFolder: '  ' }, parse), null);
});

test('a log needs the backfill while weather, temperature or the flag tag is missing', () => {
	const done = { weather: 'Rain', temperature: '1, 2', tags: ['log', '🇦🇺-AU'] };
	assert.equal(needsBackfill(done), false);
	assert.equal(needsBackfill({ ...done, weather: '' }), true);
	assert.equal(needsBackfill({ ...done, temperature: null }), true);
	assert.equal(needsBackfill({ ...done, tags: ['log'] }), true);
	assert.equal(needsBackfill({ ...done, tags: 'log 🇦🇺-AU' }), false, 'tags written as one string');
	assert.equal(needsBackfill(undefined), false, 'a file without properties');
});

test('opening a log dgs made fills it once a session, and leaves alone what it should', async () => {
	/** @type {any[]} */
	const runs = [];
	const plugin = Object.create(DgsToolboxPlugin.prototype);
	plugin.settings = logSettings;
	plugin.backfilled = new Set();
	/** @type {Record<string, any>} */
	const frontmatter = { weather: '', temperature: '', tags: ['log'] };
	plugin.app = { metadataCache: { getFileCache: () => ({ frontmatter }) } };
	plugin.vaultLib = { tp: async () => ({ user: { backfillLocationWeather: async (/** @type {unknown} */ _tp, /** @type {string} */ path) => runs.push(path) } }) };
	const log = Object.assign(new TFile('00 Daily Log/2026/2026-11-08.md'), { stat: { ctime: 0 } });
	const later = 10 * 60 * 1000;

	await plugin.backfillDailyLog(log, later, parse);
	await plugin.backfillDailyLog(log, later, parse);
	assert.deepEqual(runs, ['00 Daily Log/2026/2026-11-08.md'], 'not once, or more than once');

	const fresh = Object.assign(new TFile('00 Daily Log/2026/2026-11-09.md'), { stat: { ctime: later - 5000 } });
	await plugin.backfillDailyLog(fresh, later, parse);
	await plugin.backfillDailyLog(Object.assign(new TFile('00 Daily Log/2026/2026-12-24.md'), { stat: { ctime: 0 } }), later, parse);
	assert.equal(runs.length, 1, 'a log Templater is still filling, or a day still to come');

	await plugin.backfillDailyLog(Object.assign(new TFile('Notes/2026-11-08.md'), { stat: { ctime: 0 } }), later, parse);
	await plugin.backfillDailyLog(null, later, parse);
	assert.equal(runs.length, 1, 'a file that is not a log');

	frontmatter.weather = 'Rain';
	frontmatter.temperature = '1, 2';
	frontmatter.tags = ['log', '🇦🇺-AU'];
	await plugin.backfillDailyLog(Object.assign(new TFile('00 Daily Log/2026/2026-11-10.md'), { stat: { ctime: 0 } }), later, parse);
	assert.equal(runs.length, 1, 'a log with nothing missing');
});
