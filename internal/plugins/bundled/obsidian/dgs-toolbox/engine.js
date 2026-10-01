// @ts-check
/*
 * The timeline engine: internal/timeline, compiled to WebAssembly by
 * cmd/timeline-wasm. Every edit to a timeline note goes through it, so this
 * plugin and `dgs capture` write the same Markdown.
 *
 * dgs installs this file after Go's wasm_exec.js and before main.js, as one
 * main.js; it defines globals rather than exporting, so the shared test cases
 * can load it into Node the same way. Nothing here touches Obsidian.
 */

/** The engine's interface version this plugin was written against. */
const DGS_TIMELINE_API_VERSION = 2;

/** @type {Promise<{call: (op: string, args: unknown) => any}> | null} */
let dgsTimelineStarting = null;

/**
 * Starts the engine from the module's bytes. A Go program owns one global
 * namespace, so the module is started once per window and every caller shares
 * it. call(op, args) returns the operation's result or throws its error.
 *
 * @param {ArrayBuffer | Uint8Array} bytes
 */
function startDgsTimeline(bytes) {
	dgsTimelineStarting ??= startDgsTimelineOnce(bytes).catch((error) => {
		dgsTimelineStarting = null;
		throw error;
	});
	return dgsTimelineStarting;
}

/** @param {ArrayBuffer | Uint8Array} bytes */
async function startDgsTimelineOnce(bytes) {
	// Go's runtime glue installs itself on globalThis, and the module answers
	// there, so this is where the engine is found, in any window or in Node.
	/** @type {any} */
	const scope = globalThis;
	const ready = new Promise((resolve) => {
		scope.dgsTimelineReady = resolve;
	});
	const go = new scope.Go();
	const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
	// run() resolves only when the Go program exits, which it never does: it
	// stays to answer calls.
	void go.run(instance);
	await ready;
	delete scope.dgsTimelineReady;

	const exported = scope.dgsTimeline;
	if (!exported) throw new Error('the timeline engine did not start');
	if (exported.apiVersion !== DGS_TIMELINE_API_VERSION) {
		throw new Error(`the timeline engine speaks version ${exported.apiVersion}; this plugin needs ${DGS_TIMELINE_API_VERSION}`);
	}
	return {
		/**
		 * @param {string} op
		 * @param {unknown} args
		 */
		call(op, args) {
			const reply = JSON.parse(exported.call(op, JSON.stringify(args)));
			if (reply.error !== undefined) throw new Error(`timeline ${op}: ${reply.error}`);
			return reply.result;
		},
	};
}
