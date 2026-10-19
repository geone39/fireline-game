// Runs every golden replay (sim/testdata/replays/G*.json) through the WASM build and prints one JSON line per
// replay: {"name", "run", "stepped"} where run = the reply of fireline_sim.run and stepped = the reply of
// fireline_sim.result after feeding the commands one by one through newBattle/apply.
// Usage: node run_golden.js <dist dir with sim.wasm and wasm_exec.js> <replays dir>
"use strict";
const fs = require("fs");
const path = require("path");

const [dist, replays] = process.argv.slice(2);
require(path.resolve(dist, "wasm_exec.js"));

const go = new Go();
globalThis.fireline_sim_ready = () => {
  const sim = globalThis.fireline_sim;
  const files = fs.readdirSync(replays).filter((f) => /^G\d+\.json$/.test(f)).sort();
  for (const f of files) {
    const g = JSON.parse(fs.readFileSync(path.join(replays, f), "utf8"));
    const snap = JSON.stringify(g.snapshot);
    const seed = String(g.seed);
    const run = sim.run(snap, JSON.stringify(g.commands), seed);
    const nb = JSON.parse(sim.newBattle(snap, seed));
    let stepped = "";
    if (nb.ok) {
      for (const c of g.commands) {
        const r = JSON.parse(sim.apply(nb.result.handle, JSON.stringify(c)));
        if (!r.ok) { stepped = JSON.stringify(r); break; }
      }
      if (!stepped) stepped = sim.result(nb.result.handle);
      sim.free(nb.result.handle);
    } else {
      stepped = JSON.stringify(nb);
    }
    console.log(JSON.stringify({ name: g.name, engine: sim.engineVersion, run, stepped }));
  }
  process.exit(0);
};

WebAssembly.instantiate(fs.readFileSync(path.resolve(dist, "sim.wasm")), go.importObject)
  .then((r) => go.run(r.instance))
  .catch((e) => { console.error(e); process.exit(1); });
