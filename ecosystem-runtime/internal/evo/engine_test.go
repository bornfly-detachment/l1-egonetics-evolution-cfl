package evo

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func bootOpenEcosystem(t *testing.T, dir string) Response {
	t.Helper()
	resp := Handle(Request{Action: "bootstrap", StateDir: dir, EcosystemID: "eco", NowUnixMs: 1, Boot: BootInput{Force: true, InitialDirections: []DirectionInput{{DirectionID: "self-cognition", Title: "Self cognition loop", ParadigmSeedText: "Self cognition requires a realtime self-state register rather than a prompt illusion."}}, InitialTasks: []TaskInput{{Origin: "bornfly_boot", Difficulty: "L0", TaskType: "programming", Goal: "prove a seed can read and report a toy state register", Resources: map[string]float64{"api_token": 5}, BaseReward: map[string]float64{"api_token": 10}, BreakthroughReward: map[string]float64{"api_token": 30}, Constitution: []string{"no external framework"}}}}}, time.Now())
	if resp.Verdict != "pass" || resp.State == nil || len(resp.State.Seeds) != 3 || len(resp.State.Tasks) != 1 || len(resp.State.Runtimes) != 3 {
		t.Fatalf("bootstrap resp=%#v", resp)
	}
	return resp
}

func onlyTaskID(st *EcosystemState) string {
	for id := range st.Tasks {
		return id
	}
	return ""
}

func TestOpenTaskMultiSolutionVExecAndAudit(t *testing.T) {
	dir := t.TempDir()
	boot := bootOpenEcosystem(t, dir)
	for _, variant := range []string{"extreme_a", "median", "extreme_b"} {
		seedID := "seed:self-cognition:" + variant
		if _, ok := boot.State.Seeds[seedID]; !ok {
			t.Fatalf("missing seed variant %s", variant)
		}
		rt := boot.State.Runtimes["runtime:"+seedID]
		if rt.Status != "running" || rt.Isolation != "goroutine" {
			t.Fatalf("seed runtime not running: %#v", rt)
		}
	}

	audit := Handle(Request{Action: "audit", StateDir: dir, EcosystemID: "eco", NowUnixMs: 2}, time.Now())
	if audit.Verdict != "pass" || audit.Audit == nil || len(audit.Audit.Violations) != 0 {
		t.Fatalf("audit should pass: %#v", audit.Audit)
	}

	taskID := onlyTaskID(boot.State)
	first := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 3, Solution: SolutionInput{TaskID: taskID, SeedID: "seed:self-cognition:median", Consumed: map[string]float64{"api_token": 2}, EvaluationInput: map[string]any{"scope": "result", "actual": "ok", "expected": "ok"}, Payload: map[string]any{"note": "first passing solution"}}}, time.Now())
	if first.Verdict != "pass" || first.Solution == nil || first.Solution.Status != "base_awarded" || first.Solution.RewardTier != "base" || first.Seed == nil {
		t.Fatalf("first solution resp=%#v", first)
	}
	if got := first.Seed.Balances["api_token"]; got != 108 {
		t.Fatalf("median api_token balance = %v, want 108", got)
	}
	if first.Task == nil || first.Task.Status != "open" || !first.Task.Completed || len(first.Task.BestSolutionIDs) != 1 || len(first.Task.SolutionIDs) != 1 {
		t.Fatalf("task should remain open with first best solution: %#v", first.Task)
	}
	if first.Solution.ValueRef == "" || first.Solution.PatternRef == "" || first.Solution.RelationRef == "" || first.Solution.Assessment.Raw == nil {
		t.Fatalf("solution missing PRV refs or V raw: %#v", first.Solution)
	}

	second := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 4, Solution: SolutionInput{TaskID: taskID, SeedID: "seed:self-cognition:extreme_b", Consumed: map[string]float64{"api_token": 2}, EvaluationInput: map[string]any{"scope": "result", "actual": "ok", "expected": "ok"}, Payload: map[string]any{"note": "non-breakthrough passing solution"}}}, time.Now())
	if second.Verdict != "pass" || second.Solution == nil || second.Solution.Status != "qualified_no_reward" || second.Solution.RewardTier != "none" {
		t.Fatalf("second solution resp=%#v", second)
	}
	if got := second.Seed.Balances["api_token"]; got != 98 {
		t.Fatalf("extreme_b api_token balance = %v, want 98", got)
	}
	if second.Task.Status != "open" || len(second.Task.SolutionIDs) != 2 || len(second.Task.BestSolutionIDs) != 1 {
		t.Fatalf("task should keep all historical solutions and previous best: %#v", second.Task)
	}
}

func TestBreakthroughIsVDecisionAndRuntimeMessageBus(t *testing.T) {
	dir := t.TempDir()
	fakeV := filepath.Join(dir, "fake-v.sh")
	if err := os.WriteFile(fakeV, []byte("#!/usr/bin/env sh\ncat >/dev/null\nprintf '%s\\n' '{\"verdict\":\"pass\",\"breakthrough\":true,\"dimensions\":{\"v_owned_axis\":\"not-runtime-defined\"},\"evidence_hash\":\"sha256:fake\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	boot := Handle(Request{Action: "bootstrap", StateDir: dir, EcosystemID: "eco", NowUnixMs: 1, Boot: BootInput{Force: true, CFLRegistry: map[string]CFLRef{"fake-v": {ID: "fake-v", Layer: "V", Command: fakeV, Purpose: "test V-owned breakthrough decision", Internal: true}}, InitialDirections: []DirectionInput{{DirectionID: "world-model", ParadigmSeedText: "World model should learn sparse generating rules rather than dense phenomena."}}, InitialTasks: []TaskInput{{Origin: "researcher", SubmittedBy: "researcher-1", Difficulty: "L0", TaskType: "legacy-prd", Goal: "produce stronger PRD reconciliation", Resources: map[string]float64{"api_token": 1}, BaseReward: map[string]float64{"api_token": 10}, BreakthroughReward: map[string]float64{"api_token": 30}, VerificationCFL: "fake-v"}}}}, time.Now())
	if boot.Verdict != "pass" {
		t.Fatal(boot.Error)
	}
	taskID := onlyTaskID(boot.State)
	first := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 2, Solution: SolutionInput{TaskID: taskID, SeedID: "seed:world-model:median", Consumed: map[string]float64{"api_token": 1}, EvaluationInput: map[string]any{"task_type": "legacy-prd"}}}, time.Now())
	if first.Verdict != "pass" || first.Solution.RewardTier != "base" {
		t.Fatalf("first fake-v solution should get base tier: %#v", first.Solution)
	}
	second := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 3, Solution: SolutionInput{TaskID: taskID, SeedID: "seed:world-model:extreme_a", Consumed: map[string]float64{"api_token": 1}, EvaluationInput: map[string]any{"task_type": "legacy-prd"}}}, time.Now())
	if second.Verdict != "pass" || second.Solution.RewardTier != "breakthrough" || !second.Solution.Assessment.Breakthrough {
		t.Fatalf("second fake-v solution should get breakthrough tier: %#v", second.Solution)
	}
	if got := second.Seed.Balances["api_token"]; got != 129 {
		t.Fatalf("breakthrough seed balance=%v, want 129", got)
	}
	if second.Solution.Assessment.DimensionVector["v_owned_axis"] != "not-runtime-defined" {
		t.Fatalf("V dimensions not preserved opaquely: %#v", second.Solution.Assessment.DimensionVector)
	}

	msg := Handle(Request{Action: "send_message", StateDir: dir, EcosystemID: "eco", NowUnixMs: 4, Message: MessageInput{FromSeedID: "seed:world-model:median", ToSeedID: "seed:world-model:extreme_a", Channel: "free_society", Kind: "collaboration", Body: map[string]any{"idea": "share sparse rules"}}}, time.Now())
	if msg.Verdict != "pass" || msg.Message == nil || msg.Message.RelationRef == "" {
		t.Fatalf("send message resp=%#v", msg)
	}
	tick := Handle(Request{Action: "runtime_tick", StateDir: dir, EcosystemID: "eco", NowUnixMs: 5}, time.Now())
	if tick.Verdict != "pass" || tick.RuntimeTick == nil || tick.RuntimeTick.RuntimeCount != 3 || tick.RuntimeTick.MessagesPublished == 0 {
		t.Fatalf("runtime tick resp=%#v", tick)
	}
}

func TestResourceInjectionIsNotPayment(t *testing.T) {
	dir := t.TempDir()
	resp := Handle(Request{Action: "bootstrap", StateDir: dir, EcosystemID: "eco", NowUnixMs: 1, Boot: BootInput{Force: true, InitialDirections: []DirectionInput{{DirectionID: "resource-flow", ParadigmSeedText: "Resource pressure should be explicit and finite."}}, InitialTasks: []TaskInput{{Origin: "researcher", SubmittedBy: "researcher-1", Difficulty: "L0", Goal: "show injected resources can become task reward without payment semantics", Resources: map[string]float64{"api_token": 1}, ResourceInjection: map[string]float64{"api_token": 7}}}}}, time.Now())
	if resp.Verdict != "pass" {
		t.Fatal(resp.Error)
	}
	if _, exists := resp.State.Resources["user_payment"]; exists {
		t.Fatalf("user_payment must not exist in resource model: %#v", resp.State.Resources["user_payment"])
	}
	taskID := onlyTaskID(resp.State)
	task := resp.State.Tasks[taskID]
	if got := task.BaseReward["api_token"]; got != 7 {
		t.Fatalf("resource injection should default base_reward to 7, got %v", got)
	}
	sol := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 2, Solution: SolutionInput{TaskID: taskID, SeedID: "seed:resource-flow:median", Consumed: map[string]float64{"api_token": 1}, EvaluationInput: map[string]any{"scope": "result", "actual": "ok", "expected": "ok"}}}, time.Now())
	if sol.Verdict != "pass" || sol.Solution == nil || sol.Solution.RewardTier != "base" {
		t.Fatalf("resource-injection solution failed: %#v", sol)
	}
	if got := sol.Seed.Balances["api_token"]; got != 106 {
		t.Fatalf("seed api_token after injected reward = %v, want 106", got)
	}
}

func TestL2ExternalAPIDisqualifiesRuntime(t *testing.T) {
	dir := t.TempDir()
	resp := Handle(Request{Action: "bootstrap", StateDir: dir, EcosystemID: "eco", NowUnixMs: 1, Boot: BootInput{Force: true, InitialDirections: []DirectionInput{{DirectionID: "world-model", ParadigmSeedText: "World model should learn sparse generating rules rather than dense phenomena."}}, InitialTasks: []TaskInput{{Origin: "researcher", Difficulty: "L2", Goal: "produce autonomous self-control narrative", Resources: map[string]float64{"api_token": 1}, BaseReward: map[string]float64{"api_token": 3}}}}}, time.Now())
	if resp.Verdict != "pass" {
		t.Fatal(resp.Error)
	}
	seedID := "seed:world-model:extreme_a"
	adv := Handle(Request{Action: "advance_stage", StateDir: dir, EcosystemID: "eco", NowUnixMs: 2, Advance: AdvanceInput{SeedID: seedID, Stage: "L2"}}, time.Now())
	if adv.Verdict != "pass" || adv.Seed == nil || adv.Seed.ExternalAPIAllowed {
		t.Fatalf("advance L2 failed: %#v", adv)
	}
	taskID := onlyTaskID(resp.State)
	sol := Handle(Request{Action: "submit_solution", StateDir: dir, EcosystemID: "eco", NowUnixMs: 3, Solution: SolutionInput{TaskID: taskID, SeedID: seedID, Consumed: map[string]float64{"api_token": 1}, EvaluationInput: map[string]any{"scope": "result", "actual": "ok", "expected": "ok"}}}, time.Now())
	if sol.Verdict != "pass" || sol.Seed == nil || sol.Seed.Alive || sol.Seed.DeathReason != "external_api_used_in_l2" {
		t.Fatalf("L2 api use should disqualify: %#v", sol.Seed)
	}
	rt := sol.State.Runtimes[sol.Seed.RuntimeID]
	if rt.Status != "terminated" {
		t.Fatalf("dead seed runtime should terminate: %#v", rt)
	}
}

func TestAuditCatchesBrokenEcosystem(t *testing.T) {
	st := &EcosystemState{EcosystemID: "bad", Phase: "L0", BootRules: defaultBootRules(BootRules{BornflyMode: "runtime_operator"}, nil), CFLRegistry: map[string]CFLRef{}, Resources: map[string]ResourcePool{"api_token": {ID: "api_token", Kind: "api_token", Capacity: 0, Available: -1, Metered: false, Spendable: true}}, Directions: map[string]ResearchDirection{}, Seeds: map[string]Seed{"seed:d:one": {SeedID: "seed:d:one", DirectionID: "d", Variant: "one", Stage: "L2", Alive: true, ExternalAPIAllowed: true, RuntimeID: "runtime:seed:d:one"}}, Runtimes: map[string]SeedRuntime{}, Tasks: map[string]Task{"t": {TaskID: "t", Public: false, Status: "verified", Completed: true}}, Solutions: map[string]Solution{}, Assignments: map[string]Assignment{}}
	a := auditState(st)
	if a.Verdict != "fail" || len(a.Violations) < 6 {
		t.Fatalf("expected multiple audit failures: %#v", a)
	}
}

func TestCLIJSON(t *testing.T) {
	dir := t.TempDir()
	payload := map[string]any{"action": "bootstrap", "state_dir": dir, "ecosystem_id": "cli", "now_unix_ms": 1, "boot": map[string]any{"force": true, "initial_directions": []map[string]any{{"direction_id": "cli-dir", "paradigm_seed_text": "CLI seed"}}}}
	b, _ := json.Marshal(payload)
	cmd := exec.Command("go", "run", "./cmd/evolutionctl")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOCACHE=/private/tmp/evolution-go-cache")
	cmd.Stdin = bytes.NewReader(b)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go run failed: %v output=%s", err, out)
	}
	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil || resp.Verdict != "pass" || resp.State == nil || len(resp.State.Seeds) != 3 || len(resp.State.Runtimes) != 3 {
		t.Fatalf("bad CLI response err=%v out=%s resp=%#v", err, out, resp)
	}
}
