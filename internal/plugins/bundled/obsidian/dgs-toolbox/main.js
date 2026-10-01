// @ts-check
/*
 * DGS Toolbox for Obsidian: what dgs does to a vault, from inside Obsidian.
 * So far that is the timelines — notes that grow at the top, newest first, a
 * day at a time, with past years moved into an archive — and the same entry
 * in the day's log.
 *
 * dgs installs it (`dgs plugins install`); this file is not run as it is. The
 * installed main.js is Go's wasm_exec.js, then engine.js, then this, which is
 * why startDgsTimeline is used here without being imported.
 *
 * Where this device is, and the address, come from the vault's own scripts
 * (the Templater user scripts folder, "Lib folder" in the settings), as do
 * the map links and the daily log append: one implementation of each in the
 * vault, whatever calls it. A Capture from dgs brings its own position.
 */

const obsidian = require('obsidian');
const { Modal, Notice, Plugin, PluginSettingTab, Setting, TFile, moment, normalizePath } = obsidian;

/** How the plugin names a day in a marker, Sunday first. */
const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

/** The engine ships beside main.js under this name. */
const WASM_FILE = 'timeline.wasm';

/** Detail lines are indented four spaces, as the vault writes them by hand. */
const INDENT = '    ';

const WHEN_FORMAT = 'YYYY-MM-DD HH:mm:ss';
const WHEN = /^(\d{4}-\d{2}-\d{2})[ T](\d{1,2}:\d{2}(?::\d{2})?)(?:\s*([+-]\d{1,2}(?:[.:]\d{1,2})?))?$/;

/**
 * @typedef {{
 *   timelinesFile: string,
 *   libFolder: string,
 *   dailyFolder: string,
 *   dailyFormat: string,
 *   dailyHeading: string,
 *   copyChoice: string,
 *   mapLinks: string,
 * }} Settings
 * @typedef {{name: string, note: string, archive?: string, split: string, cssclass: string, title: string, template?: string, contentRequired?: boolean}} Definition
 * @typedef {{country?: string, region?: string, city?: string, locality?: string, latitude?: string | number, longitude?: string | number}} Place
 * @typedef {{when: any, text: string, place: Place | null}} NewEntry
 */

/** @type {Settings} */
const DEFAULT_SETTINGS = {
	timelinesFile: '',
	libFolder: '99 Toolkit/91 Scripts/00 Lib',
	dailyFolder: '00 Daily Log',
	dailyFormat: 'YYYY/YYYY-MM-DD',
	dailyHeading: '去过哪里',
	copyChoice: 'Copy Coordinates (lng, lat)',
	mapLinks: 'Apple, 高德, Google',
};

class DgsToolboxPlugin extends Plugin {
	async onload() {
		/** @type {Settings} */
		this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
		this.timelines = new TimelineService(this.app, this.manifest.dir ?? '', () => this.settings.timelinesFile);
		this.vaultLib = new VaultLib(this.app, () => this.settings);
		this.addCommand({
			id: 'insert-timeline-entry',
			name: 'Insert timeline entry',
			callback: () => void this.insertTimelineEntry(),
		});
		this.addSettingTab(new DgsToolboxSettingTab(this.app, this));
	}

	async saveSettings() {
		await this.saveData(this.settings);
	}

	/**
	 * Opens the insert form. The open note's timeline is chosen when the open
	 * note is one, else the first defined.
	 */
	async insertTimelineEntry() {
		try {
			const timelines = await this.timelines.definitions();
			const first = timelines[0];
			if (!first) {
				new Notice('The timelines file defines no timeline.');
				return;
			}
			const open = this.app.workspace.getActiveFile()?.path;
			const preselected = timelines.find((timeline) => timeline.note === open) ?? first;
			new InsertTimelineEntryModal(this, timelines, preselected).open();
		} catch (error) {
			console.error('DGS Toolbox: timelines unavailable', error);
			new Notice(error instanceof Error ? error.message : String(error));
		}
	}
}

/**
 * The vault's own scripts, loaded the way QuickAdd and Templater load them:
 * 00 Lib/quickAddShim.js reads every script in the folder and hands back a
 * Templater-shaped `tp`, so the scripts do not know who called them.
 */
class VaultLib {
	/**
	 * @param {import('obsidian').App} app
	 * @param {() => Settings} settings
	 */
	constructor(app, settings) {
		this.app = app;
		this.settings = settings;
	}

	/** The params the scripts expect from QuickAdd: the app and the API. */
	params() {
		return { app: this.app, obsidian, quickAddApi: null };
	}

	/** @returns {Promise<any>} a Templater-shaped tp over the scripts */
	async tp() {
		const folder = normalizePath(this.settings().libFolder);
		const path = `${folder}/quickAddShim.js`;
		const code = await this.app.vault.adapter.read(path);
		const module = { exports: /** @type {any} */ ({}) };
		new Function('module', 'exports', 'require', `${code}\n//# sourceURL=${path}`)(
			module,
			module.exports,
			typeof require === 'function' ? require : undefined,
		);
		return module.exports(this.params(), folder);
	}

	/**
	 * Where this device is, with its address: GPS on a phone, CoreLocationCLI
	 * on a Mac, the network's address otherwise. Null when nothing answers.
	 *
	 * @returns {Promise<Place | null>}
	 */
	async locate() {
		const tp = await this.tp();
		return tp.user.getLocation(tp, tp.user.requestJson);
	}

	/**
	 * The detail lines a place adds under an entry, as the locations timeline
	 * has them: the address from coarse to fine, the coordinates as a link
	 * copying them, and one link per map.
	 *
	 * @param {Place | null} place
	 * @returns {Promise<string[]>}
	 */
	async placeLines(place) {
		if (!place) return [];
		const lines = [];
		const address = [place.country, place.region, place.city, place.locality].filter(Boolean);
		if (address.length) lines.push(address.join(', '));
		if (place.latitude && place.longitude) {
			const { copyChoice, mapLinks } = this.settings();
			const shown = `(${place.latitude}, ${place.longitude})`;
			const value = encodeURIComponent(`${place.latitude}, ${place.longitude}`);
			lines.push(copyChoice
				? `[${shown}](obsidian://quickadd?choice=${encodeURIComponent(copyChoice)}&value-coordinates=${value})`
				: shown);
			const tp = await this.tp();
			const label = place.locality || place.city || place.region || place.country;
			const links = tp.user.mapLinks(place.latitude, place.longitude, label, mapLinks);
			lines.push(links.map((/** @type {{short: string, url: string}} */ link) => `[${link.short}](${link.url})`).join(' · '));
		}
		return lines;
	}

	/**
	 * Appends a block under the day's heading in its log, creating the log
	 * through Templater's folder template when there is none. 00 Lib's
	 * appendToDailyLog waits for the template to settle and checks afterwards
	 * that nothing overwrote the block.
	 *
	 * @param {any} when
	 * @param {string} block
	 */
	async appendToDailyLog(when, block) {
		const tp = await this.tp();
		const { dailyFolder, dailyFormat, dailyHeading } = this.settings();
		const written = await tp.user.appendToDailyLog(
			this.params(),
			{ date: when, logFolder: normalizePath(dailyFolder), logFormat: dailyFormat, heading: dailyHeading, createIfMissing: true },
			() => block,
		);
		if (!written) throw new Error('the daily log could not be found or created');
		return `${normalizePath(dailyFolder)}/${when.format(dailyFormat)}.md`;
	}
}

/**
 * Reads the timelines file and writes entries onto a timeline. Every change to
 * a note is computed by the engine; this only reads and writes the files, in
 * the order that loses nothing if it stops halfway: archives first, then the
 * running note.
 */
class TimelineService {
	/**
	 * @param {import('obsidian').App} app
	 * @param {string} pluginDir
	 * @param {() => string} timelinesFile
	 */
	constructor(app, pluginDir, timelinesFile) {
		this.app = app;
		this.pluginDir = pluginDir;
		this.timelinesFile = timelinesFile;
	}

	/** The engine, started on first use: most sessions never need it. */
	async engine() {
		const bytes = await this.app.vault.adapter.readBinary(normalizePath(`${this.pluginDir}/${WASM_FILE}`));
		return startDgsTimeline(bytes);
	}

	/** @returns {Promise<Definition[]>} every timeline the file defines, by name */
	async definitions() {
		const path = this.timelinesFile().trim();
		if (!path) throw new Error('Set the timelines file in DGS Toolbox settings first.');
		const file = this.app.vault.getAbstractFileByPath(normalizePath(path));
		if (!(file instanceof TFile)) throw new Error(`The timelines file ${path} does not exist.`);
		const text = await this.app.vault.read(file);
		return (await this.engine()).call('definitions', { text });
	}

	/**
	 * @param {Definition} definition
	 * @param {any} when the moment, in the offset it happened in
	 * @param {string} block the entry's lines, as formatEntry writes them
	 * @returns {Promise<{kind: 'inserted', path: string, archived: string[]} | {kind: 'duplicate', path: string}>}
	 */
	async insert(definition, when, block) {
		const engine = await this.engine();
		const date = when.format('YYYY-MM-DD');
		const year = when.format('YYYY');
		const current = moment().format('YYYY');
		const header = {
			cssclass: definition.cssclass,
			title: definition.title,
			source: definition.note.split('/').pop() ?? definition.note,
		};
		const archiveFolder = definition.archive?.trim() || parentFolder(definition.note);
		/** @param {string} y */
		const archivePath = (y) => normalizePath(`${archiveFolder}/${y}.md`);
		const marker = engine.call('marker', { date, weekday: WEEKDAYS[when.day()] ?? '' });

		// A day from a year that has rolled over belongs in that year's file,
		// not at the top of this year's note.
		if (year !== current) {
			const path = archivePath(year);
			const existing = await this.readOrEmpty(path);
			const content = existing || engine.call('header', { header, year });
			const result = engine.call('insert', { content, date, marker, entry: block });
			if (!result.inserted) return { kind: 'duplicate', path };
			await this.write(path, existing, result.content);
			return { kind: 'inserted', path, archived: [] };
		}

		const path = normalizePath(definition.note);
		const existing = await this.readOrEmpty(path);
		const { remaining, moved } = engine.call('archive', { content: existing, current });
		const result = engine.call('insert', { content: remaining, date, marker, entry: block });
		if (!result.inserted) return { kind: 'duplicate', path };
		for (const leaving of moved) {
			const target = archivePath(leaving.year);
			const before = await this.readOrEmpty(target);
			await this.write(target, before, engine.call('merge', { archive: before, header, year: leaving }));
		}
		await this.write(path, existing, result.content);
		return { kind: 'inserted', path, archived: moved.map((/** @type {{year: string}} */ leaving) => leaving.year) };
	}

	/** @param {string} path */
	async readOrEmpty(path) {
		const file = this.app.vault.getAbstractFileByPath(path);
		return file instanceof TFile ? this.app.vault.read(file) : '';
	}

	/**
	 * Writes a note computed from `before`, refusing when the note changed in
	 * between: the engine's answer is only right for what it was given.
	 *
	 * @param {string} path
	 * @param {string} before
	 * @param {string} after
	 */
	async write(path, before, after) {
		const file = this.app.vault.getAbstractFileByPath(path);
		if (file instanceof TFile) {
			await this.app.vault.process(file, (now) => {
				if (now !== before) throw new Error(`${path} changed while it was being written; try again.`);
				return after;
			});
			return;
		}
		if (before !== '') throw new Error(`${path} disappeared while it was being written; try again.`);
		const folder = parentFolder(path);
		if (folder && !this.app.vault.getAbstractFileByPath(folder)) await this.app.vault.createFolder(folder);
		await this.app.vault.create(path, after);
	}
}

/**
 * Asks which timelines, when, and what happened, then writes the entry onto
 * each, and into the day's log when asked. Fields are stacked full width, each
 * with a one-line hint under it, so the space goes to what is typed.
 */
class InsertTimelineEntryModal extends Modal {
	/**
	 * @param {DgsToolboxPlugin} plugin
	 * @param {Definition[]} timelines
	 * @param {Definition} preselected
	 */
	constructor(plugin, timelines, preselected) {
		super(plugin.app);
		this.plugin = plugin;
		this.timelines = timelines;
		/** @type {Set<string>} */
		this.chosen = new Set([preselected.name]);
		this.daily = true;
		this.located = true;
		/** @type {Promise<Place | null> | null} */
		this.place = null;
	}

	onOpen() {
		this.setTitle('Insert timeline entry');
		const { contentEl } = this;
		contentEl.addClass('dgs-timeline-entry');

		const timelines = field(contentEl, 'Timelines', 'Every one chosen gets the entry.');
		const chips = timelines.createDiv({ cls: 'dgs-timeline-entry__chips' });
		for (const timeline of this.timelines) {
			chip(chips, timeline.title || timeline.name, this.chosen.has(timeline.name), (on) => {
				if (on) this.chosen.add(timeline.name);
				else this.chosen.delete(timeline.name);
			});
		}
		chip(chips, 'Daily log', this.daily, (on) => {
			this.daily = on;
		}).addClass('dgs-timeline-entry__chip--daily');

		const when = field(contentEl, 'When', 'Seconds optional; the offset is the time zone it happened in: 2026-09-24 17:00:00 +10.');
		this.whenInput = when.createEl('input', { type: 'text', cls: 'dgs-timeline-entry__input' });
		this.whenInput.value = formatWhen(moment());

		const where = field(contentEl, 'Where', 'Where this device is now. Turn it off for an entry written later.');
		const row = where.createDiv({ cls: 'dgs-timeline-entry__where' });
		const status = row.createDiv({ cls: 'dgs-timeline-entry__place', text: 'Finding where this device is…' });
		chip(row, 'Record', this.located, (on) => {
			this.located = on;
			status.toggleClass('is-off', !on);
		});
		this.place = this.plugin.vaultLib.locate().catch((error) => {
			console.warn('DGS Toolbox: no location', error);
			return null;
		});
		void this.place.then((place) => {
			const address = [place?.locality, place?.city, place?.region, place?.country].filter(Boolean).join(', ');
			const coordinates = place?.latitude && place.longitude ? `(${place.latitude}, ${place.longitude})` : '';
			status.setText([address, coordinates].filter(Boolean).join(' · ') || 'This device could not say where it is.');
		});

		const what = field(contentEl, 'What happened', 'The first line is the entry; further lines go under it. ⌘↩ inserts.');
		this.textInput = what.createEl('textarea', { cls: 'dgs-timeline-entry__text' });
		this.textInput.rows = 6;
		this.textInput.addEventListener('keydown', (event) => {
			if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
				event.preventDefault();
				void this.submit();
			}
		});
		window.setTimeout(() => this.textInput?.focus(), 0);

		const buttons = contentEl.createDiv({ cls: 'modal-button-container' });
		this.insertButton = buttons.createEl('button', { text: 'Insert', cls: 'mod-cta' });
		this.insertButton.addEventListener('click', () => void this.submit());
	}

	onClose() {
		this.contentEl.empty();
	}

	async submit() {
		const chosen = this.timelines.filter((timeline) => this.chosen.has(timeline.name));
		if (!chosen.length && !this.daily) {
			new Notice('Choose at least one timeline, or the daily log.');
			return;
		}
		const when = parseWhen(this.whenInput?.value ?? '');
		if (!when) {
			new Notice('Write the time as 2026-09-24 17:00:00 +10: the offset is the time zone, seconds are optional.');
			return;
		}
		const text = this.textInput?.value ?? '';
		const needy = chosen.find((timeline) => timeline.contentRequired);
		if (needy && !text.trim()) {
			new Notice(`The ${needy.name} timeline needs to say what happened.`);
			return;
		}
		if (this.insertButton) this.insertButton.disabled = true;

		// One place at a time, each reported: a failure on one does not undo
		// the others, so the reader is told exactly where the entry went.
		const done = [];
		let failed = '';
		try {
			const place = this.located ? await this.place : null;
			const block = formatEntry({ when, text, place }, await this.plugin.vaultLib.placeLines(place));
			for (const timeline of chosen) {
				failed = timeline.name;
				const outcome = await this.plugin.timelines.insert(timeline, when, block);
				if (outcome.kind === 'duplicate') {
					done.push(`already in ${outcome.path}`);
					continue;
				}
				const archived = outcome.archived.length ? ` (moved ${outcome.archived.join(', ')} into the archive)` : '';
				done.push(`added to ${outcome.path}${archived}`);
			}
			if (this.daily) {
				failed = 'the daily log';
				done.push(`added to ${await this.plugin.vaultLib.appendToDailyLog(when, block)}`);
			}
		} catch (error) {
			console.error('DGS Toolbox: insert failed', error);
			new Notice(`${done.length ? capitalise(done.join('; ')) + '. ' : ''}Could not write to ${failed}: ${error instanceof Error ? error.message : String(error)}`, 10000);
			if (this.insertButton) this.insertButton.disabled = false;
			return;
		}
		new Notice(capitalise(done.join('; ')) + '.');
		this.close();
	}
}

/**
 * A labelled field: the name, the control the caller adds, and a hint under it.
 *
 * @param {HTMLElement} parent
 * @param {string} name
 * @param {string} hint
 */
function field(parent, name, hint) {
	const wrapper = parent.createDiv({ cls: 'dgs-timeline-entry__field' });
	wrapper.createDiv({ text: name, cls: 'dgs-timeline-entry__label' });
	const control = wrapper.createDiv();
	wrapper.createDiv({ text: hint, cls: 'dgs-timeline-entry__hint' });
	return control;
}

/**
 * A button that stays pressed or not.
 *
 * @param {HTMLElement} parent
 * @param {string} text
 * @param {boolean} on
 * @param {(on: boolean) => void} changed
 */
function chip(parent, text, on, changed) {
	const button = parent.createEl('button', { text, cls: 'dgs-timeline-entry__chip' });
	button.setAttr('type', 'button');
	const show = () => {
		button.toggleClass('is-active', on);
		button.setAttr('aria-pressed', String(on));
	};
	show();
	button.addEventListener('click', () => {
		on = !on;
		show();
		changed(on);
	});
	return button;
}

class DgsToolboxSettingTab extends PluginSettingTab {
	/**
	 * @param {import('obsidian').App} app
	 * @param {DgsToolboxPlugin} plugin
	 */
	constructor(app, plugin) {
		super(app, plugin);
		this.plugin = plugin;
	}

	display() {
		const { containerEl } = this;
		containerEl.empty();
		/**
		 * @param {string} name
		 * @param {string} desc
		 * @param {keyof Settings} key
		 */
		const text = (name, desc, key) => new Setting(containerEl)
			.setName(name)
			.setDesc(desc)
			.addText((input) => input
				.setPlaceholder(DEFAULT_SETTINGS[key])
				.setValue(this.plugin.settings[key])
				.onChange(async (value) => {
					this.plugin.settings[key] = value.trim();
					await this.plugin.saveSettings();
				}));

		new Setting(containerEl)
			.setName('Timelines')
			.setDesc('Notes that grow at the top, newest first, a day at a time, with past years moved into an archive. dgs capture reads the same file.')
			.setHeading();
		text('Timelines file', 'Vault-relative path of the JSON file that defines every timeline, for example 99 Toolkit/timelines.json.', 'timelinesFile');

		new Setting(containerEl).setName('Daily log').setHeading();
		text('Folder', 'Where the daily logs are.', 'dailyFolder');
		text('File name', 'Their names, as a moment format; a slash is a folder. Keep it as Periodic Notes has it.', 'dailyFormat');
		text('Heading', 'The heading an entry is written under, without #.', 'dailyHeading');

		new Setting(containerEl).setName('Where').setHeading();
		text('Lib folder', 'The vault scripts that find this device, name the place, link the maps and append to the daily log: the Templater user scripts folder.', 'libFolder');
		text('Copy coordinates choice', 'The QuickAdd choice the coordinates link to, which copies them. Empty writes them without a link.', 'copyChoice');
		text('Map links', 'Which maps to link, comma separated: Apple, 高德, Google, 百度, OSM.', 'mapLinks');

		new Setting(containerEl)
			.setName('Installed by dgs')
			.setDesc('Update with dgs plugins update; files changed here are reported as modified and not overwritten.');
	}
}

/**
 * Reads "2026-09-24 17:00:00 +10", seconds and the offset optional: "+9.5",
 * "+09:30". Without an offset the time is this device's.
 *
 * @param {string} input
 */
function parseWhen(input) {
	const match = WHEN.exec(input.trim());
	if (!match) return null;
	const [, date, clock = '', offset] = match;
	const local = moment(`${date} ${clock}`, ['YYYY-MM-DD H:mm:ss', 'YYYY-MM-DD H:mm'], true);
	if (!local.isValid()) return null;
	if (!offset) return local;
	const [, sign, hours = '0', rest] = /^([+-])(\d{1,2})(?:[.:](\d{1,2}))?$/.exec(offset) ?? [];
	if (!sign) return null;
	const minutes = Number(hours) * 60 + (offset.includes(':') ? Number(rest ?? 0) : Number(`0.${rest ?? 0}`) * 60);
	if (minutes > 14 * 60) return null;
	return local.clone().utcOffset((sign === '-' ? -1 : 1) * minutes, true);
}

/**
 * A moment as the form shows it: date, time and offset.
 *
 * @param {any} when
 */
function formatWhen(when) {
	return `${when.format(WHEN_FORMAT)} ${formatOffset(when.utcOffset())}`;
}

/**
 * One entry as the timeline and the daily log write it: the clock and its
 * offset in backticks, then the text; under it the place's lines, then any
 * further lines of text.
 *
 * @param {NewEntry} entry
 * @param {string[]} placeLines
 */
function formatEntry(entry, placeLines) {
	const [first = '', ...rest] = entry.text.trim().split('\n').map((line) => line.trim());
	const clock = `\`${entry.when.format('HH:mm:ss')} ${formatOffset(entry.when.utcOffset())}\``;
	return [
		first ? `- ${clock} · ${first}` : `- ${clock}`,
		...[...placeLines, ...rest.filter(Boolean)].map((line) => `${INDENT}- ${line}`),
	].join('\n');
}

/**
 * An offset in minutes as the vault writes it: "+10", "+9.5", "-3".
 *
 * @param {number} minutes
 */
function formatOffset(minutes) {
	const hours = Math.abs(minutes) / 60;
	return `${minutes < 0 ? '-' : '+'}${Number(hours.toFixed(2))}`;
}

/** @param {string} path */
function parentFolder(path) {
	const index = path.lastIndexOf('/');
	return index < 0 ? '' : path.slice(0, index);
}

/** @param {string} text */
function capitalise(text) {
	return text.replace(/^./, (first) => first.toUpperCase());
}

module.exports = DgsToolboxPlugin;
