/*
 * The Quick Note QuickAdd script: it hands its text to the DGS Toolbox plugin's
 * API and writes nothing itself, so a quick note is rendered and placed by the
 * engine like every other entry in the day's log.
 *
 * Run with make test-timeline.
 */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = fileURLToPath(new URL('.', import.meta.url));
const source = readFileSync(join(here, 'obsidian', 'quickadd', 'quickNote.js'), 'utf8');

/** The script, loaded the way QuickAdd loads a user script. */
function script() {
	const module = { exports: /** @type {any} */ ({}) };
	vm.runInNewContext(source, { module, exports: module.exports, console: { error() {} } });
	return module.exports;
}

/**
 * QuickAdd's params, with the plugin's API when there is one.
 *
 * @param {{api?: any, value?: string, typed?: string | null}} given
 */
function params({ api, value, typed = null }) {
	/** @type {string[]} */
	const notices = [];
	const plugins = api === undefined ? {} : { 'dgs-toolbox': { api } };
	return {
		notices,
		params: {
			obsidian: {
				Notice: class {
					/** @param {string} message */
					constructor(message) {
						notices.push(message);
					}
				},
			},
			app: { plugins: { plugins } },
			variables: value === undefined ? {} : { value },
			quickAddApi: { inputPrompt: async () => typed },
		},
	};
}

/** An API that records what it was asked to write. */
function recordingApi() {
	/** @type {any[]} */
	const calls = [];
	// Copied out, since the script's objects are another realm's.
	return { calls, api: { appendToDailyLog: async (/** @type {any} */ options) => calls.push(JSON.parse(JSON.stringify(options))) } };
}

test('a quick note goes to the plugin with the quick note template, under 速记', async () => {
	const { calls, api } = recordingApi();
	const { params: p, notices } = params({ api, typed: '  买牛奶  ' });
	await script().entry(p, {});
	assert.deepEqual(calls, [{ template: 'quick-note.md', heading: '速记', text: '买牛奶', createIfMissing: true }]);
	assert.deepEqual(notices, ['已记下']);
});

test('the text a macro already has is not asked for again', async () => {
	const { calls, api } = recordingApi();
	const { params: p } = params({ api, value: '从上一步来的', typed: 'not this' });
	await script().entry(p, {});
	assert.equal(calls[0].text, '从上一步来的');
});

test('its settings choose the heading, the template and whether a missing log is made', async () => {
	const { calls, api } = recordingApi();
	const { params: p } = params({ api, typed: 'x' });
	await script().entry(p, { 'Daily log heading': '想说的', 'Entry template': 'mine.md', 'Create the daily log when missing': false });
	assert.deepEqual(calls, [{ template: 'mine.md', heading: '想说的', text: 'x', createIfMissing: false }]);
	// A blank setting is the default, not an empty heading or template.
	await script().entry(p, { 'Daily log heading': '  ', 'Entry template': '' });
	assert.equal(calls[1].heading, '速记');
	assert.equal(calls[1].template, 'quick-note.md');
});

test('without DGS Toolbox nothing is written, and it says so', async () => {
	const { params: p, notices } = params({ typed: 'x' });
	await script().entry(p, {});
	assert.equal(notices.length, 1);
	assert.match(notices[0], /DGS Toolbox/);
	// A plugin of that id without the API is as good as none.
	const { params: old, notices: oldNotices } = params({ api: null, typed: 'x' });
	await script().entry(old, {});
	assert.match(oldNotices[0], /DGS Toolbox/);
});

test('nothing typed is nothing written', async () => {
	for (const typed of [null, '', '   ']) {
		const { calls, api } = recordingApi();
		const { params: p, notices } = params({ api, typed });
		await script().entry(p, {});
		assert.deepEqual(calls, []);
		assert.deepEqual(notices, []);
	}
});

test('a write that fails is said, not thrown, and not reported as written', async () => {
	const api = { appendToDailyLog: async () => { throw new Error('the daily log could not be found or created'); } };
	const { params: p, notices } = params({ api, typed: 'x' });
	await script().entry(p, {});
	assert.equal(notices.length, 1);
	assert.match(notices[0], /没记下：the daily log could not be found or created/);
});
