//go:build js && wasm

// Command timeline-wasm is internal/timeline for JavaScript: the Obsidian
// plugin edits a timeline note with the same code dgs does. It sets one global,
// dgsTimeline, with
//
//	apiVersion            timeline.APIVersion
//	call(op, argsJSON)    {result} or {error}, as JSON text
//
// and then calls globalThis.dgsTimelineReady, when set, to say it is there.
package main

import (
	"encoding/json"
	"syscall/js"

	"dgs-toolbox/internal/timeline"
)

func main() {
	api := js.Global().Get("Object").New()
	api.Set("apiVersion", timeline.APIVersion)
	api.Set("call", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return reply(nil, "call takes an operation and its arguments as JSON text")
		}
		result, err := timeline.Call(args[0].String(), []byte(args[1].String()))
		if err != nil {
			return reply(nil, err.Error())
		}
		return reply(result, "")
	}))
	js.Global().Set("dgsTimeline", api)
	if ready := js.Global().Get("dgsTimelineReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

func reply(result json.RawMessage, failure string) string {
	var data []byte
	if failure != "" {
		data, _ = json.Marshal(map[string]string{"error": failure})
	} else {
		data, _ = json.Marshal(map[string]json.RawMessage{"result": result})
	}
	return string(data)
}
