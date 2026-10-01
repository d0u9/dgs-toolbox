/*
 * The Obsidian plugin's engine.js against internal/timeline's shared cases.
 * Go runs the same cases natively and in its WebAssembly test build; this runs
 * them through the module and the wrapper the plugin ships, loaded the way the
 * installed main.js loads them — wasm_exec.js, then engine.js, as one script —
 * so the JSON going in and out, the field names and the error path are pinned
 * too.
 *
 * Run with make test-timeline, which builds the module first.
 */
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = fileURLToPath(new URL('.', import.meta.url));
const plugin = join(here, 'obsidian', 'dgs-toolbox');
const casesDir = join(here, '..', '..', 'timeline', 'testdata', 'cases');

vm.runInThisContext(
	[join(plugin, 'generated', 'wasm_exec.js'), join(plugin, 'engine.js')]
		.map((path) => readFileSync(path, 'utf8'))
		.join('\n'),
);
const engine = await globalThis.startDgsTimeline(readFileSync(join(plugin, 'generated', 'timeline.wasm')));

/** Replaces every "@name" string with the content of that file in the case. */
function resolve(dir, value) {
	if (typeof value === 'string') {
		return value.startsWith('@') ? readFileSync(join(dir, value.slice(1)), 'utf8') : value;
	}
	if (Array.isArray(value)) return value.map((item) => resolve(dir, item));
	if (value && typeof value === 'object') {
		return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, resolve(dir, item)]));
	}
	return value;
}

const names = readdirSync(casesDir).sort();

test('the shared cases are there', () => {
	assert.ok(names.length > 0, `no cases in ${casesDir}`);
});

for (const name of names) {
	const dir = join(casesDir, name);
	const spec = JSON.parse(readFileSync(join(dir, 'case.json'), 'utf8'));
	test(`case ${name}`, () => {
		const args = resolve(dir, spec.args);
		if (spec.error) {
			assert.throws(() => engine.call(spec.op, args), (error) => error.message.includes(spec.error));
			return;
		}
		assert.deepEqual(engine.call(spec.op, args), resolve(dir, spec.want));
	});
}
