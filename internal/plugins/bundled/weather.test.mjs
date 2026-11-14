/*
 * The vault's weather lookups, getDailyTemperature and getHourlyWeather: each
 * asks its first service, the archive for a past day and the forecast for
 * today on, and asks the other when the first does not answer or has no
 * numbers. The network is stood in for; moment is the real one.
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
/**
 * Just enough of moment for these two scripts: an instant with a fixed UTC
 * offset in minutes, formatted, compared by day in its own offset, moved by
 * days, hours or minutes. The vault's scripts get the real one from Obsidian.
 */
class Moment {
	/** @param {number} ms @param {number} offset minutes east of UTC */
	constructor(ms, offset) {
		this.ms = ms;
		this.offset = offset;
		this._isAMomentObject = true;
	}
	valueOf() { return this.ms; }
	clone() { return new Moment(this.ms, this.offset); }
	/** @param {number} [minutes] */
	utcOffset(minutes) {
		if (minutes === undefined) return this.offset;
		return new Moment(this.ms, minutes);
	}
	/** The wall clock in this offset, as a UTC Date. */
	wall() { return new Date(this.ms + this.offset * 60000); }
	/** @param {string} layout */
	format(layout) {
		const w = this.wall();
		const pad = (/** @type {number} */ n, size = 2) => String(n).padStart(size, '0');
		return layout
			.replace('YYYY', String(w.getUTCFullYear())).replace('MM', pad(w.getUTCMonth() + 1)).replace('DD', pad(w.getUTCDate()))
			.replace('HH', pad(w.getUTCHours())).replace('mm', pad(w.getUTCMinutes()));
	}
	dayKey() { return this.format('YYYY-MM-DD'); }
	/** @param {Moment} other */
	isBefore(other) { return this.dayKey() < other.utcOffset(this.offset).dayKey(); }
	/** @param {Moment} other */
	isSame(other) { return this.dayKey() === other.utcOffset(this.offset).dayKey(); }
	/** @param {number} n @param {string} unit */
	add(n, unit) {
		const ms = { day: 86400000, days: 86400000, hours: 3600000, hour: 3600000, minutes: 60000 }[unit] ?? 0;
		return new Moment(this.ms + n * ms, this.offset);
	}
	/** @param {number} n @param {string} unit */
	subtract(n, unit) { return this.add(-n, unit); }
	startOf() { const w = this.wall(); return new Moment(Date.UTC(w.getUTCFullYear(), w.getUTCMonth(), w.getUTCDate()) - this.offset * 60000, this.offset); }
	isValid() { return true; }
}

/** Parses "YYYY-MM-DD", "YYYY-MM-DD HH:mm" or "YYYY-MM-DDTHH:mm" as a wall clock at an offset. */
function at(/** @type {string} */ text, offset = 0) {
	const [, y, m, d, hh = '0', mm = '0'] = /^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2}))?/.exec(text) ?? [];
	return new Moment(Date.UTC(Number(y), Number(m) - 1, Number(d), Number(hh), Number(mm)) - offset * 60000, offset);
}

const moment = Object.assign(() => new Moment(Date.now(), 0), {
	isMoment: (/** @type {unknown} */ value) => value instanceof Moment,
	utc: (/** @type {string} */ text) => at(text, 0),
});

const ARCHIVE = 'archive-api.open-meteo.com';
const FORECAST = 'api.open-meteo.com/v1/forecast';

/** @param {string} name */
function load(name) {
	const module = { exports: /** @type {any} */ ({}) };
	vm.runInNewContext(readFileSync(join(here, 'obsidian', 'public', `${name}.js`), 'utf8'), { module, moment, console: { warn() {} } });
	return module.exports;
}

/**
 * A network answering by host: a function of the URL, or an error to throw.
 *
 * @param {Record<string, ((url: string) => unknown) | Error>} answers
 */
function network(answers) {
	/** @type {string[]} */
	const asked = [];
	const requestJson = async (/** @type {unknown} */ _tp, /** @type {string} */ url) => {
		const host = url.includes(ARCHIVE) ? ARCHIVE : FORECAST;
		asked.push(host);
		const answer = answers[host];
		if (answer instanceof Error) throw answer;
		if (!answer) throw new Error('no answer');
		return answer(url);
	};
	return { asked, requestJson };
}

const place = { latitude: '-33.77', longitude: '151.08' };

/** A day's answer: every hour of `day` clear, 10 to 20 degrees, in Sydney time. */
function dayAnswer(/** @type {string} */ url) {
	const day = /start_date=([\d-]+)/.exec(url)?.[1] ?? '';
	return {
		utc_offset_seconds: 39600,
		daily: { temperature_2m_max: [20], temperature_2m_min: [10] },
		hourly: { time: Array.from({ length: 24 }, (_, hour) => `${day}T${String(hour).padStart(2, '0')}:00`), weather_code: Array(24).fill(0) },
	};
}

/** An hourly answer around a day: three days of hours, 15 degrees, cloudy. */
function hoursAnswer(/** @type {string} */ url) {
	const start = at(/start_date=([\d-]+)/.exec(url)?.[1] ?? '');
	const times = Array.from({ length: 72 }, (_, hour) => start.add(hour, 'hours').format('YYYY-MM-DDTHH:mm'));
	return { utc_offset_seconds: 39600, hourly: { time: times, temperature_2m: Array(72).fill(15), weather_code: Array(72).fill(3) } };
}

const empty = () => ({ utc_offset_seconds: 39600, daily: { temperature_2m_max: [null], temperature_2m_min: [null] }, hourly: { time: [], weather_code: [], temperature_2m: [] } });
const down = new Error('timed out');

test('a past day asks the archive, and the forecast only when the archive has nothing', async () => {
	const getDailyTemperature = load('getDailyTemperature');
	const past = at('2026-11-01', 660);
	let net = network({ [ARCHIVE]: dayAnswer, [FORECAST]: dayAnswer });
	assert.equal((await getDailyTemperature({}, net.requestJson, place, past))?.range, '10.0, 20.0');
	assert.deepEqual(net.asked, [ARCHIVE]);

	net = network({ [ARCHIVE]: down, [FORECAST]: dayAnswer });
	assert.equal((await getDailyTemperature({}, net.requestJson, place, past))?.range, '10.0, 20.0');
	assert.deepEqual(net.asked, [ARCHIVE, FORECAST]);
});

test('today asks the forecast, and the archive when the forecast does not answer', async () => {
	const getDailyTemperature = load('getDailyTemperature');
	const today = moment().utcOffset(660).startOf();
	const net = network({ [FORECAST]: down, [ARCHIVE]: dayAnswer });
	const result = await getDailyTemperature({}, net.requestJson, place, today);
	assert.deepEqual(net.asked, [FORECAST, ARCHIVE]);
	assert.equal(result?.range, '10.0, 20.0');
});

test('an answer without numbers counts as none, and two of them are null', async () => {
	const getDailyTemperature = load('getDailyTemperature');
	const past = at('2026-11-01', 660);
	let net = network({ [ARCHIVE]: empty, [FORECAST]: dayAnswer });
	assert.equal((await getDailyTemperature({}, net.requestJson, place, past))?.condition, 'Clear');
	assert.deepEqual(net.asked, [ARCHIVE, FORECAST]);

	net = network({ [ARCHIVE]: down, [FORECAST]: down });
	assert.equal(await getDailyTemperature({}, net.requestJson, place, past), null);
	assert.equal(await getDailyTemperature({}, net.requestJson, {}, past), null, 'no place, no lookup');
});

test('an hour falls back the same way, and is null when neither service has it', async () => {
	const getHourlyWeather = load('getHourlyWeather');
	const past = at('2026-11-01 15:00', 660);
	let net = network({ [ARCHIVE]: down, [FORECAST]: hoursAnswer });
	assert.deepEqual({ ...(await getHourlyWeather({}, net.requestJson, place, past)) }, { condition: 'Cloudy', temperature: 15 });
	assert.deepEqual(net.asked, [ARCHIVE, FORECAST]);

	const now = moment().utcOffset(660);
	net = network({ [FORECAST]: down, [ARCHIVE]: hoursAnswer });
	assert.deepEqual({ ...(await getHourlyWeather({}, net.requestJson, place, now)) }, { condition: 'Cloudy', temperature: 15 });
	assert.deepEqual(net.asked, [FORECAST, ARCHIVE]);

	net = network({ [FORECAST]: empty, [ARCHIVE]: down });
	assert.equal(await getHourlyWeather({}, net.requestJson, place, now), null);
});
