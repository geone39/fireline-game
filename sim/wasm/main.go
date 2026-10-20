//go:build js && wasm

// Command wasm exposes package sim to JavaScript as the global object `fireline_sim` (see api.d.ts).
// Build: sim/wasm/build.ps1 or build.sh. The Go runtime stays alive after main (select{}), so the functions
// remain callable.
package main

import (
	"syscall/js"

	"github.com/OWNER/fireline/sim/wasm/wire"
)

func main() {
	var battles wire.Battles
	api := js.Global().Get("Object").New()
	api.Set("engineVersion", wire.EngineVersion())
	api.Set("run", js.FuncOf(func(_ js.Value, a []js.Value) any {
		return wire.Run(str(a, 0), str(a, 1), seed(a, 2))
	}))
	api.Set("newBattle", js.FuncOf(func(_ js.Value, a []js.Value) any {
		return battles.New(str(a, 0), seed(a, 1))
	}))
	api.Set("apply", js.FuncOf(func(_ js.Value, a []js.Value) any {
		return battles.Apply(handle(a, 0), str(a, 1))
	}))
	api.Set("result", js.FuncOf(func(_ js.Value, a []js.Value) any { return battles.Result(handle(a, 0)) }))
	api.Set("state", js.FuncOf(func(_ js.Value, a []js.Value) any { return battles.State(handle(a, 0)) }))
	api.Set("free", js.FuncOf(func(_ js.Value, a []js.Value) any { return battles.Free(handle(a, 0)) }))
	js.Global().Set("fireline_sim", api)
	if cb := js.Global().Get("fireline_sim_ready"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}
	select {}
}

func str(a []js.Value, i int) string {
	if i >= len(a) || a[i].Type() != js.TypeString {
		return ""
	}
	return a[i].String()
}

// seed accepts a decimal string (any uint64) or a JS number (safe integers only).
func seed(a []js.Value, i int) string {
	if i < len(a) && a[i].Type() == js.TypeNumber {
		return js.Global().Get("String").Invoke(a[i]).String()
	}
	return str(a, i)
}

func handle(a []js.Value, i int) int {
	if i >= len(a) || a[i].Type() != js.TypeNumber {
		return 0
	}
	return a[i].Int()
}
