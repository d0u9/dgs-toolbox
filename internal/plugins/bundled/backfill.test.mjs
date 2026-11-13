/*
 * The vault's backfillLocationWeather: it fills only what a log's properties
 * leave empty. A log Obsidian just made is filled from where this device is; a
 * log dgs capture made keeps the place its Capture recorded and gets only the
 * weather, the temperature and the flag tag, looked up at that place.
 *
 * The network is stood in for: every lookup the script asks for is recorded.
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
const source = readFileSync(join(here, 'obsidian', 'public', 'backfillLocationWeather.js'), 'utf8');
const countryName = (() => {
	const module = { exports: /** @type {any} */ ({}) };
	vm.runInNewContext(readFileSync(join(here, 'obsidian', 'public', 'countryName.js'), 'utf8'), { module, Intl });
	return module.exports;
})();
const formatCountryTag = (() => {
	const module = { exports: /** @type {any} */ ({}) };
	vm.runInNewContext(readFileSync(join(here, 'obsidian', 'public', 'formatCountryTag.js'), 'utf8'), { module, String });
	return module.exports;
})();

const path = '00 Daily Log/2026/2026-11-08.md';
const device = {
	country: 'Australia', region: 'New South Wales', city: 'Sydney', locality: 'Epping', countryCode: 'AU',
	latitude: '-33.76917500', longitude: '151.08246900', coordinates: '-33.76917500, 151.08246900',
};
const shanghai = { country: 'China', region: 'Shanghai', city: 'Shanghai', locality: 'Huangpu', countryCode: 'CN' };
const day = { condition: 'Cloudy, Clear', range: '8.8, 17.4' };

/**
 * A vault holding one log with these properties, and the network's answers.
 *
 * @param {Record<string, unknown>} properties
 * @param {{reverse?: any, reverseThrows?: boolean, device?: any, day?: any}} [answers]
 */
function setup(properties, answers = {}) {
	/** @type {string[]} */
	const asked = [];
	/** @type {string[]} */
	const notices = [];
	const store = { properties: structuredClone(properties) };
	const file = { path, extension: 'md' };
	const app = {
		vault: {
			getAbstractFileByPath: (/** @type {string} */ wanted) => (wanted === path ? file : null),
			read: async () => `---\n${JSON.stringify(store.properties)}\n---\n\n# 速记\n`,
		},
		fileManager: {
			processFrontMatter: async (/** @type {unknown} */ _file, /** @type {(p: any) => void} */ change) => {
				const copy = structuredClone(store.properties);
				change(copy);
				store.properties = copy;
			},
		},
	};
	const tp = {
		obsidian: {
			parseYaml: (/** @type {string} */ text) => JSON.parse(text),
			Notice: class {
				/** @param {string} message */
				constructor(message) {
					notices.push(message);
				}
			},
		},
		user: {
			requestJson: () => {},
			getLocation: async () => {
				asked.push('device');
				return 'device' in answers ? answers.device : device;
			},
			reverseGeocode: async (/** @type {unknown} */ _tp, /** @type {unknown} */ _r, /** @type {number} */ latitude, /** @type {number} */ longitude) => {
				asked.push(`reverse ${latitude},${longitude}`);
				if (answers.reverseThrows) throw new Error('offline');
				return 'reverse' in answers ? answers.reverse : shanghai;
			},
			getDailyTemperature: async (/** @type {unknown} */ _tp, /** @type {unknown} */ _r, /** @type {any} */ location, /** @type {any} */ date) => {
				asked.push(`day ${location.latitude},${location.longitude} ${date}`);
				return 'day' in answers ? answers.day : day;
			},
			getWeather: async () => {
				asked.push('weather');
				return { condition: 'Sunny', temperature: 20 };
			},
			countryName,
			formatCountryTag,
			orderFrontmatter: () => {},
		},
	};
	const module = { exports: /** @type {any} */ ({}) };
	vm.runInNewContext(source, { module, app, moment: () => 'today', console: { warn() {} }, setTimeout });
	const run = (/** @type {Record<string, unknown>} */ extra = {}) => module.exports(tp, path, { dailyTemperature: true, countryTag: true, date: '2026-11-08', ...extra });
	return { run, store, asked, notices };
}

const blankLog = {
	date: '2026-11-08', coordinates: '', country: '', region: '', city: '', locality: '',
	temperature: '', weather: '', weatherNote: '', tags: ['log'],
};

test('a log Obsidian just made is filled from where this device is', async () => {
	const { run, store, asked } = setup(blankLog);
	await run();
	assert.deepEqual(asked, ['device', 'day -33.76917500,151.08246900 2026-11-08']);
	assert.equal(store.properties.coordinates, device.coordinates);
	assert.equal(store.properties.region, 'New South Wales');
	assert.equal(store.properties.weather, 'Cloudy, Clear');
	assert.equal(store.properties.temperature, '8.8, 17.4');
	assert.deepEqual([...store.properties.tags], ['log', '🇦🇺-AU']);
});

test('a log dgs made keeps its place and gets the weather there, not where this device is', async () => {
	const { run, store, asked } = setup({
		...blankLog, coordinates: '31.23040, 121.47370', country: 'China', region: '上海', city: '上海市', locality: '外滩',
	});
	await run();
	assert.ok(!asked.includes('device'), 'the device was asked where it is');
	assert.deepEqual(asked, ['reverse 31.2304,121.4737', 'day 31.2304,121.4737 2026-11-08']);
	assert.equal(store.properties.coordinates, '31.23040, 121.47370', 'the coordinates were rewritten');
	assert.equal(store.properties.region, '上海', 'a place the Capture named was replaced');
	assert.equal(store.properties.locality, '外滩');
	assert.equal(store.properties.weather, 'Cloudy, Clear');
	assert.deepEqual([...store.properties.tags], ['log', '🇨🇳-CN']);
});

test('a log with coordinates and no address is named from them', async () => {
	const { run, store, asked } = setup({ ...blankLog, coordinates: '31.23040, 121.47370' });
	await run();
	assert.ok(!asked.includes('device'));
	assert.equal(store.properties.country, '中国', 'the country is named as getLocation names it');
	assert.equal(store.properties.city, 'Shanghai');
	assert.equal(store.properties.coordinates, '31.23040, 121.47370');
});

test('a complete log is left alone, with nothing looked up', async () => {
	const { run, store, asked, notices } = setup({
		...blankLog, coordinates: '1, 2', country: 'A', region: 'B', city: 'C', locality: 'D',
		temperature: '1, 2', weather: 'Rain', tags: ['log', '🇦🇺-AU'],
	});
	const before = structuredClone(store.properties);
	await run();
	assert.deepEqual(asked, []);
	assert.deepEqual(store.properties, before);
	assert.deepEqual(notices, []);
});

test('what is there stays: weather, temperature and a flag tag are not replaced', async () => {
	const { run, store, asked } = setup({
		...blankLog, coordinates: '31.23040, 121.47370', country: 'China', region: 'R', city: 'C', locality: 'L',
		weather: 'Snow', temperature: '', tags: ['log', '🇦🇺-AU'],
	});
	await run();
	assert.ok(!asked.some((ask) => ask.startsWith('reverse')), 'a flag tag and an address need no reverse lookup');
	assert.equal(store.properties.weather, 'Snow');
	assert.equal(store.properties.temperature, '8.8, 17.4');
	assert.deepEqual([...store.properties.tags], ['log', '🇦🇺-AU']);
});

test('without a country code the tag is the country’s name, added once however often it runs', async () => {
	const { run, store } = setup({ ...blankLog, coordinates: '1, 2', country: 'Australia', region: 'R', city: 'C', locality: 'L' }, { reverseThrows: true });
	await run();
	await run();
	assert.deepEqual([...store.properties.tags], ['log', 'Australia']);
	assert.equal(store.properties.weather, 'Cloudy, Clear');
});

test('coordinates that do not read as a place are as good as none', async () => {
	for (const coordinates of ['nowhere', '1', '95, 10', '1, 2, 3']) {
		const { run, asked } = setup({ ...blankLog, coordinates });
		await run();
		assert.equal(asked[0], 'device', `${coordinates} was read as a place`);
	}
});

test('nothing found is said, and nothing is written', async () => {
	const { run, store, notices } = setup(blankLog, { device: null });
	await run();
	assert.deepEqual(store.properties, blankLog);
	assert.match(notices[0], /位置和天气都没查到/);
});

test('weather that could not be found is said, the place still written', async () => {
	const { run, store, notices } = setup(blankLog, { day: null });
	await run();
	assert.equal(store.properties.city, 'Sydney');
	assert.equal(store.properties.weather, '');
	assert.match(notices[0], /天气没查全/);
});

test('a past log without coordinates is not given where this device is now', async () => {
	const { run, store, asked, notices } = setup(blankLog);
	await run({ locateDevice: false });
	assert.deepEqual(asked, [], 'something was looked up');
	assert.deepEqual(store.properties, blankLog);
	assert.deepEqual(notices, [], 'a log that cannot be filled is left quietly');
});

test('a past log with coordinates is still filled at its own place', async () => {
	const { run, store, asked } = setup({ ...blankLog, coordinates: '31.23040, 121.47370' });
	await run({ locateDevice: false });
	assert.ok(!asked.includes('device'));
	assert.equal(store.properties.weather, 'Cloudy, Clear');
	assert.equal(store.properties.city, 'Shanghai');
});
