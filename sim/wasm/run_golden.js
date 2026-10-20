// Runs every golden replay (sim/testdata/replays/G*.json) through the WASM build and prints one JSON line per
// replay: {"name", "run", "stepped"} where run = the reply of fireline_sim.run and stepped = the reply of
// fireline_sim.result after feeding the commands one by one through newBattle/apply, and states = FNV-1a 32 hashes (8 hex digits) of the
// replies of fireline_sim.state: before the first command and after every command (hashes keep the output small).
// Usage: node run_golden.js <dist dir with sim.wasm and wasm_exec.js> <replays dir>
"use strict";
const fs = require("fs");
const path = require("path");

// FNV-1a 32 over the UTF-16 code units (the replies are ASCII).
function fnv(str) {
  let h = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 0x01000193) >>> 0; }
  return h.toString(16).padStart(8, "0");
}

const [dist, replays] = process.argv.slice(2);
require(path.resolve(dist, "wasm_exec.js"));

const go = new Go();
globalThis.fireline_sim_ready = () => {
  const sim = globalThis.fireline_sim;
  const out = [];
  const files = fs.readdirSync(replays).filter((f) => /^G\d+\.json$/.test(f)).sort();
  for (const f of files) {
    const g = JSON.parse(fs.readFileSync(path.join(replays, f), "utf8"));
    const snap = JSON.stringify(g.snapshot);
    const seed = String(g.seed);
    const run = sim.run(snap, JSON.stringify(g.commands), seed);
    const nb = JSON.parse(sim.newBattle(snap, seed));
    let stepped = "";
    const states = [];
    if (nb.ok) {
      states.push(fnv(sim.state(nb.result.handle)));
      for (const c of g.commands) {
        const r = JSON.parse(sim.apply(nb.result.handle, JSON.stringify(c)));
        if (!r.ok) { stepped = JSON.stringify(r); break; }
        states.push(fnv(sim.state(nb.result.handle)));
      }
      if (!stepped) stepped = sim.result(nb.result.handle);
      sim.free(nb.result.handle);
    } else {
      stepped = JSON.stringify(nb);
    }
    out.push(JSON.stringify({ name: g.name, engine: sim.engineVersion, run, stepped, states }));
  }
  fs.writeSync(1, out.join(String.fromCharCode(10)) + String.fromCharCode(10));
  process.exit(0);
};

WebAssembly.instantiate(fs.readFileSync(path.resolve(dist, "sim.wasm")), go.importObject)
  .then((r) => go.run(r.instance))
  .catch((e) => { console.error(e); process.exit(1); });
