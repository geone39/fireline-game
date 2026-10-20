// Command simbattle plays bot attacks through the sim against the reference bases (content/reference_bases) and
// reports win rates per composition and HQ level (docs/01-architecture.md §11, module qa).
//
//	go -C tools/simbattle run . [-seeds 20] [-bases all] [-strategies all] [-out tools/simbattle/out]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

type job struct {
	n     int
	ref   *RefBase
	strat *Strategy
	seed  uint64
}

// RunAll plays every (base, strategy, seed) combination on a worker pool. The result order is the job order, so
// the report does not depend on scheduling.
func (e *Env) RunAll(refs []*RefBase, strats []Strategy, seeds int, seedBase uint64, workers int, progress func(done, total int)) ([]*Outcome, error) {
	var jobs []job
	for _, r := range refs {
		for si := range strats {
			for k := 0; k < seeds; k++ {
				jobs = append(jobs, job{n: len(jobs), ref: r, strat: &strats[si], seed: seedBase + uint64(k)})
			}
		}
	}
	out := make([]*Outcome, len(jobs))
	errs := make([]error, len(jobs))
	ch := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for w := 0; w < max(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				o, err := e.safePlay(j.ref, j.strat, j.seed)
				out[j.n], errs[j.n] = o, err
				mu.Lock()
				done++
				if progress != nil {
					progress(done, len(jobs))
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		ch <- j
	}
	close(ch)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// safePlay turns a panic of one battle into an error that names it.
func (e *Env) safePlay(ref *RefBase, s *Strategy, seed uint64) (o *Outcome, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("battle %s/%s seed %d panicked: %v\n%s", ref.Short(), s.ID, seed, r, debug.Stack())
		}
	}()
	return e.Play(ref, s, seed)
}

func main() {
	var (
		contentDir  = flag.String("content", "", "content directory (default: found above the working directory)")
		outDir      = flag.String("out", "", "output directory for report.md / report.json (default: tools/simbattle/out in the repository)")
		seeds       = flag.Int("seeds", 20, "battles per (base, strategy)")
		seedBase    = flag.Uint64("seed-base", 1, "first seed")
		bases       = flag.String("bases", "all", "reference bases: all or a comma list (hq05,hq10_coastal)")
		strategies  = flag.String("strategies", "all", "strategies: all or a comma list")
		levels      = flag.String("levels", "", "unit level policy: parity | research | one (default: assumptions.yaml)")
		garrison    = flag.String("garrison", "", "bunker garrison: rifle | none (default: assumptions.yaml)")
		turnCap     = flag.Int("turn-cap", 0, "turn limit of the bot (default: assumptions.yaml)")
		assumptions = flag.String("assumptions", "", "assumptions.yaml replacement")
		stratFile   = flag.String("strategy-file", "", "strategies.yaml replacement")
		workers     = flag.Int("workers", runtime.NumCPU(), "parallel battles")
		quiet       = flag.Bool("quiet", false, "no progress output")
		winStars    = flag.Int("win-stars", 0, "stars that count as a win (default: assumptions.yaml)")
		failOnBad   = flag.Bool("check", false, "exit 1 if the sim refused a bot command")
	)
	flag.Parse()
	if err := run(*contentDir, *outDir, *seeds, *seedBase, *bases, *strategies, *levels, *garrison, *turnCap, *assumptions, *stratFile, *workers, *quiet, *failOnBad, *winStars); err != nil {
		fmt.Fprintln(os.Stderr, "simbattle:", err)
		os.Exit(1)
	}
}

func run(contentDir, outDir string, seeds int, seedBase uint64, bases, strategies, levels, garrison string, turnCap int, assumptions, stratFile string, workers int, quiet, check bool, winStars int) error {
	if contentDir == "" {
		d, err := FindContentDir(".")
		if err != nil {
			return err
		}
		contentDir = d
	}
	env, err := LoadEnv(contentDir, assumptions, stratFile)
	if err != nil {
		return err
	}
	if levels != "" {
		env.A.Levels.Policy = levels
	}
	if garrison != "" {
		env.A.Garrison.Policy = garrison
	}
	if winStars > 0 {
		env.A.Win.MinStars = winStars
	}
	if turnCap > 0 {
		env.A.Battle.TurnCap = turnCap
	}
	refs, err := env.SelectRefs(bases)
	if err != nil {
		return err
	}
	strats, err := env.SelectStrategies(strategies)
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(contentDir), "tools", "simbattle", "out")
	}
	start := time.Now()
	var last time.Time
	progress := func(done, total int) {
		if quiet || (time.Since(last) < 2*time.Second && done != total) {
			return
		}
		last = time.Now()
		fmt.Fprintf(os.Stderr, "\r%d/%d battles, %s", done, total, time.Since(start).Round(time.Second))
	}
	outs, err := env.RunAll(refs, strats, seeds, seedBase, workers, progress)
	if !quiet {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		return err
	}
	for _, o := range outs {
		for _, info := range o.RejectInfo {
			fmt.Fprintf(os.Stderr, "refused command in %s/%s seed %d: %s\n", o.Ref, o.Strategy, o.Seed, info)
		}
	}
	rep := BuildReport(env, outs, ReportMeta{Seeds: seeds, SeedBase: seedBase, Bases: names(refs), Strategies: stratNames(strats),
		Levels: env.A.Levels.Policy, Garrison: env.A.Garrison.Policy, TurnCap: env.A.Battle.TurnCap})
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := rep.Write(outDir); err != nil {
		return err
	}
	fmt.Print(rep.Summary())
	fmt.Fprintf(os.Stderr, "report: %s (%s)\n", filepath.Join(outDir, "report.md"), time.Since(start).Round(time.Second))
	if check && rep.Totals.Rejected > 0 {
		return fmt.Errorf("bot bug: the sim refused %d commands", rep.Totals.Rejected)
	}
	return nil
}

func names(refs []*RefBase) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Short())
	}
	return out
}

func stratNames(s []Strategy) []string {
	var out []string
	for _, x := range s {
		out = append(out, x.ID)
	}
	return out
}
