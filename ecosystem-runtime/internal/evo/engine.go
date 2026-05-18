package evo

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

func Handle(req Request, startedAt time.Time) Response {
	ecosystemID := defaultStr(req.EcosystemID, "default")
	now := nowMs(req)
	resp := Response{Action: req.Action, EcosystemID: ecosystemID}
	var st *EcosystemState
	var err error

	switch req.Action {
	case "bootstrap":
		st, err = bootstrap(req, ecosystemID, now)
	case "register_direction":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			err = registerDirection(st, req.Direction, now)
		}
	case "submit_task":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var task *Task
			task, err = submitTask(st, req.Task, now)
			resp.Task = task
		}
	case "submit_solution":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var sol *Solution
			sol, err = submitSolution(st, req.Solution, now)
			resp.Solution = sol
			if seed, ok := st.Seeds[req.Solution.SeedID]; ok {
				resp.Seed = &seed
			}
			if task, ok := st.Tasks[req.Solution.TaskID]; ok {
				resp.Task = &task
			}
		}
	case "runtime_tick", "tick_runtimes":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			report := runtimeTick(st, now)
			resp.RuntimeTick = &report
			tickDeaths(st, now)
		}
	case "send_message":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var msg *Message
			msg, err = sendMessage(st, req.Message, now)
			resp.Message = msg
		}
	case "claim_task":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var asg *Assignment
			asg, err = observeTask(st, req.Claim, now)
			resp.Assignment = asg
		}
	case "settle_task":
		err = errors.New("settle_task deprecated: submit_solution executes V CFL directly; external verdict input is forbidden")
	case "advance_stage":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var seed *Seed
			seed, err = advanceStage(st, req.Advance, now)
			resp.Seed = seed
		}
	case "tick":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			tickDeaths(st, now)
		}
	case "escalate":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			var entry *InboxEntry
			entry, err = addInbox(st, req.Escalate, now)
			resp.InboxEntry = entry
		}
	case "audit":
		st, err = loadState(req.StateDir, ecosystemID)
		if err == nil {
			audit := auditState(st)
			resp.Audit = &audit
			resp.Verdict = audit.Verdict
			resp.State = st
			resp.LatencyMs = latencyMs(startedAt)
			return resp
		}
	case "status":
		st, err = loadState(req.StateDir, ecosystemID)
	default:
		resp.Verdict = "fail"
		resp.Error = "unknown action " + req.Action
		return resp
	}

	if err != nil {
		resp.Verdict = "fail"
		resp.Error = err.Error()
		resp.LatencyMs = latencyMs(startedAt)
		return resp
	}
	if st != nil && req.Action != "status" {
		st.UpdatedAt = now
		if err := saveState(req.StateDir, st); err != nil {
			resp.Verdict = "fail"
			resp.Error = err.Error()
			resp.LatencyMs = latencyMs(startedAt)
			return resp
		}
	}
	resp.Verdict = "pass"
	resp.State = st
	resp.LatencyMs = latencyMs(startedAt)
	return resp
}

func bootstrap(req Request, ecosystemID string, now int64) (*EcosystemState, error) {
	if !req.Boot.Force {
		if st, err := loadState(req.StateDir, ecosystemID); err == nil {
			return st, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	st := &EcosystemState{EcosystemID: ecosystemID, Phase: defaultStr(req.Boot.Phase, "L0"), CreatedAt: now, UpdatedAt: now, Metadata: req.Boot.Metadata}
	st.initMaps()
	st.BootRules = defaultBootRules(req.Boot.BootRules, req.Boot.InitialBalances)
	st.CFLRegistry = defaultRegistry()
	for id, ref := range req.Boot.CFLRegistry {
		if ref.ID == "" {
			ref.ID = id
		}
		st.CFLRegistry[id] = ref
	}
	st.Resources = defaultResources()
	for id, pool := range req.Boot.Resources {
		if pool.ID == "" {
			pool.ID = id
		}
		st.Resources[id] = pool
	}
	appendChronicle(st, now, "bootstrap", ecosystemID, map[string]any{"phase": st.Phase, "runtime_model": "goroutine-isolated-seed-runtime"})
	for _, d := range req.Boot.InitialDirections {
		if err := registerDirection(st, d, now); err != nil {
			return nil, err
		}
	}
	for _, t := range req.Boot.InitialTasks {
		if _, err := submitTask(st, t, now); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *EcosystemState) initMaps() {
	if st.CFLRegistry == nil {
		st.CFLRegistry = map[string]CFLRef{}
	}
	if st.Resources == nil {
		st.Resources = map[string]ResourcePool{}
	}
	if st.Directions == nil {
		st.Directions = map[string]ResearchDirection{}
	}
	if st.Seeds == nil {
		st.Seeds = map[string]Seed{}
	}
	if st.Runtimes == nil {
		st.Runtimes = map[string]SeedRuntime{}
	}
	if st.Tasks == nil {
		st.Tasks = map[string]Task{}
	}
	if st.Solutions == nil {
		st.Solutions = map[string]Solution{}
	}
	if st.Assignments == nil {
		st.Assignments = map[string]Assignment{}
	}
}

func defaultBootRules(in BootRules, balances map[string]float64) BootRules {
	if len(in.SeedVariants) == 0 {
		in.SeedVariants = []string{"extreme_a", "median", "extreme_b"}
	}
	if in.BornflyMode == "" {
		in.BornflyMode = "boot_escalation_only"
	}
	if in.RewardPolicy == "" {
		in.RewardPolicy = "base_plus_breakthrough_survival_transfer"
	}
	if in.TaskVisibility == "" {
		in.TaskVisibility = "public_all_seeds_open_problem"
	}
	if in.ExternalAPIAllowedUntil == "" {
		in.ExternalAPIAllowedUntil = "L1"
	}
	if in.OpenNetworkPolicy == "" {
		in.OpenNetworkPolicy = "allowlisted_metered"
	}
	if len(in.DefaultSeedBalance) == 0 {
		in.DefaultSeedBalance = map[string]float64{"api_token": 100, "bornfly_proxy": 2, "network_lookup": 10}
	}
	for k, v := range balances {
		in.DefaultSeedBalance[k] = v
	}
	if len(in.DeathResourceFloors) == 0 {
		in.DeathResourceFloors = map[string]float64{"api_token": 0}
	}
	if len(in.DefaultBaseReward) == 0 {
		in.DefaultBaseReward = map[string]float64{"api_token": 2}
	}
	if len(in.DefaultBreakthroughReward) == 0 {
		in.DefaultBreakthroughReward = multiplyResources(in.DefaultBaseReward, 3)
	}
	return in
}

func defaultResources() map[string]ResourcePool {
	return map[string]ResourcePool{
		"api_token":        {ID: "api_token", Kind: "api_token", Capacity: 10000, Available: 10000, Metered: true, Spendable: true},
		"bornfly_proxy":    {ID: "bornfly_proxy", Kind: "human_proxy", Capacity: 100, Available: 100, Metered: true, Spendable: true},
		"network_lookup":   {ID: "network_lookup", Kind: "allowlisted_network", Capacity: 1000, Available: 1000, Metered: true, Spendable: true},
		"public_knowledge": {ID: "public_knowledge", Kind: "knowledge_snapshot", Capacity: 1, Available: 1, Metered: true, Spendable: false},
	}
}

func defaultRegistry() map[string]CFLRef {
	refs := []CFLRef{
		{ID: "p-cfl", Layer: "P", Module: defaultCFLModule("../p-cfl", "../../p-cfl"), Purpose: "pattern refs for tasks/resources/seeds/solutions", Internal: true},
		{ID: "l0-v-test-execution-cfl", Layer: "V", Module: defaultCFLModule("../value/cmd/l0-v-test-execution-cfl", "../../value-cfl/cmd/l0-v-test-execution-cfl"), Purpose: "task completion verification", Internal: true},
		{ID: "l1-v-evaluator-cfl", Layer: "V", Module: defaultCFLModule("../value/cmd/l1-v-evaluator-cfl", "../../value-cfl/cmd/l1-v-evaluator-cfl"), Purpose: "V-owned contextual scoring", Internal: true},
		{ID: "l2-v-human-validate-cfl", Layer: "V", Module: defaultCFLModule("../value/cmd/l2-v-human-validate-cfl", "../../value-cfl/cmd/l2-v-human-validate-cfl"), Purpose: "human escalation validation", Internal: true},
		{ID: "l0-r-hash-chain-cfl", Layer: "R", Module: defaultCFLModule("../r-cfl/cmd/l0-r-hash-chain-cfl", "../../r-cfl/cmd/l0-r-hash-chain-cfl"), Purpose: "chronicle relation integrity", Internal: true},
		{ID: "l0-r-protocol-validate-cfl", Layer: "R", Module: defaultCFLModule("../r-cfl/cmd/l0-r-protocol-validate-cfl", "../../r-cfl/cmd/l0-r-protocol-validate-cfl"), Purpose: "message bus and settlement protocol", Internal: true},
		{ID: "l1-s-resource-policy-cfl", Layer: "S", Module: defaultCFLModule("../s-cfl/cmd/l1-s-resource-policy-cfl", "../../s-cfl/cmd/l1-s-resource-policy-cfl"), Purpose: "resource stop/warn policy", Internal: true},
		{ID: "l1-s-lifecycle-death-cfl", Layer: "S", Module: defaultCFLModule("../s-cfl/cmd/l1-s-lifecycle-death-cfl", "../../s-cfl/cmd/l1-s-lifecycle-death-cfl"), Purpose: "seed runtime death gate", Internal: true},
		{ID: "l1-s-inbox-router-cfl", Layer: "S", Module: defaultCFLModule("../s-cfl/cmd/l1-s-inbox-router-cfl", "../../s-cfl/cmd/l1-s-inbox-router-cfl"), Purpose: "escalation inbox plan", Internal: true},
	}
	out := map[string]CFLRef{}
	for _, r := range refs {
		out[r.ID] = r
	}
	return out
}

func defaultCFLModule(candidates ...string) string {
	root := moduleRoot()
	for _, candidate := range candidates {
		target := candidate
		if !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(root, target))
		}
		if _, err := os.Stat(target); err == nil {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func registerDirection(st *EcosystemState, in DirectionInput, now int64) error {
	if in.DirectionID == "" || in.ParadigmSeedText == "" {
		return errors.New("direction_id and paradigm_seed_text required")
	}
	if _, exists := st.Directions[in.DirectionID]; exists {
		return fmt.Errorf("direction %s already exists", in.DirectionID)
	}
	variants := in.Variants
	if len(variants) == 0 {
		variants = st.BootRules.SeedVariants
	}
	if len(variants) != 3 {
		return errors.New("direction must allocate exactly 3 seeds")
	}
	seenVariants := map[string]bool{}
	for _, variant := range variants {
		if variant == "" {
			return errors.New("direction seed variants must be non-empty")
		}
		if seenVariants[variant] {
			return fmt.Errorf("duplicate seed variant %s", variant)
		}
		seenVariants[variant] = true
	}
	balances := st.BootRules.DefaultSeedBalance
	if len(in.InitialBalances) > 0 {
		balances = in.InitialBalances
	}
	if err := validateResourceDelta(st, balances, "initial_balances"); err != nil {
		return err
	}
	if err := allocateFromPools(st, multiplyResources(balances, float64(len(variants))), "initial_balances"); err != nil {
		return err
	}
	dir := ResearchDirection{DirectionID: in.DirectionID, Title: in.Title, ParadigmSeedText: in.ParadigmSeedText, PatternRef: patternRef("paradigm_seed", in), Metadata: in.Metadata}
	for _, variant := range variants {
		seedID := "seed:" + in.DirectionID + ":" + variant
		runtimeID := "runtime:" + seedID
		seed := Seed{SeedID: seedID, DirectionID: in.DirectionID, Variant: variant, Stage: "L0", Balances: map[string]float64{}, Alive: true, ExternalAPIAllowed: true, RuntimeID: runtimeID, PatternRef: patternRef("ai_seed", map[string]any{"direction": in.DirectionID, "variant": variant, "seed": in.ParadigmSeedText}), CreatedAt: now, UpdatedAt: now}
		for k, v := range balances {
			seed.Balances[k] = v
		}
		seed.StateRef = seedStateRef(seed)
		rt := SeedRuntime{RuntimeID: runtimeID, SeedID: seedID, Status: "running", Isolation: "goroutine", StartedAt: now}
		rt.StateRef = runtimeStateRef(rt)
		st.Seeds[seedID] = seed
		st.Runtimes[runtimeID] = rt
		dir.SeedIDs = append(dir.SeedIDs, seedID)
	}
	sort.Strings(dir.SeedIDs)
	st.Directions[in.DirectionID] = dir
	refreshTaskVisibility(st)
	appendChronicle(st, now, "register_direction", in.DirectionID, map[string]any{"seeds": dir.SeedIDs, "runtime_isolation": "goroutine"})
	return nil
}

func submitTask(st *EcosystemState, in TaskInput, now int64) (*Task, error) {
	if in.Goal == "" {
		return nil, errors.New("task.goal required")
	}
	if in.Difficulty != "L0" && in.Difficulty != "L1" && in.Difficulty != "L2" {
		return nil, errors.New("task.difficulty must be L0|L1|L2")
	}
	id := in.TaskID
	if id == "" {
		id = shortID("task", map[string]any{"origin": in.Origin, "by": in.SubmittedBy, "difficulty": in.Difficulty, "goal": in.Goal, "resources": in.Resources, "resource_injection": in.ResourceInjection, "base_reward": in.BaseReward, "breakthrough_reward": in.BreakthroughReward, "constitution": in.Constitution})
	}
	if _, exists := st.Tasks[id]; exists {
		return nil, fmt.Errorf("task %s already exists", id)
	}
	origin := defaultStr(in.Origin, "researcher")
	verification := defaultStr(in.VerificationCFL, "l0-v-test-execution-cfl")
	if !isRegisteredLayer(st, verification, "V") {
		return nil, fmt.Errorf("verification_cfl %s is not registered V CFL", verification)
	}
	resources := copyFloatMap(in.Resources)
	if len(resources) == 0 {
		resources["api_token"] = 1
	}
	if err := validateResourceDelta(st, resources, "task.resources"); err != nil {
		return nil, err
	}
	injection := copyFloatMap(in.ResourceInjection)
	if err := validateResourceDelta(st, injection, "task.resource_injection"); err != nil {
		return nil, err
	}
	injectIntoPools(st, injection)
	baseReward := copyFloatMap(in.BaseReward)
	if len(baseReward) == 0 && len(in.Reward) > 0 {
		baseReward = copyFloatMap(in.Reward)
	}
	if len(baseReward) == 0 && len(injection) > 0 {
		baseReward = copyFloatMap(injection)
	}
	if len(baseReward) == 0 {
		baseReward = copyFloatMap(st.BootRules.DefaultBaseReward)
	}
	breakthroughReward := copyFloatMap(in.BreakthroughReward)
	if len(breakthroughReward) == 0 {
		breakthroughReward = copyFloatMap(st.BootRules.DefaultBreakthroughReward)
	}
	if err := validateResourceDelta(st, baseReward, "task.base_reward"); err != nil {
		return nil, err
	}
	if err := validateResourceDelta(st, breakthroughReward, "task.breakthrough_reward"); err != nil {
		return nil, err
	}
	task := Task{TaskID: id, Origin: origin, SubmittedBy: in.SubmittedBy, Difficulty: in.Difficulty, TaskType: in.TaskType, Goal: in.Goal, Resources: resources, BaseReward: baseReward, BreakthroughReward: breakthroughReward, Constitution: append([]string(nil), in.Constitution...), VerificationCFL: verification, Status: "open", Public: true, VisibleTo: livingSeedIDs(st), Metadata: in.Metadata, CreatedAt: now, UpdatedAt: now}
	task.PatternRef = patternRef("open_task", task)
	task.StateRef = taskStateRef(task)
	st.Tasks[id] = task
	appendChronicle(st, now, "submit_open_task", id, map[string]any{"difficulty": task.Difficulty, "task_type": task.TaskType, "pattern_ref": task.PatternRef})
	return &task, nil
}

func observeTask(st *EcosystemState, in ClaimInput, now int64) (*Assignment, error) {
	task, ok := st.Tasks[in.TaskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	seed, ok := st.Seeds[in.SeedID]
	if !ok {
		return nil, errors.New("seed not found")
	}
	if !seed.Alive {
		return nil, errors.New("seed is dead")
	}
	asg := Assignment{AssignmentID: shortID("observe", map[string]any{"task": task.TaskID, "seed": seed.SeedID, "now": now}), TaskID: task.TaskID, SeedID: seed.SeedID, Status: "observing_nonexclusive", Budget: copyFloatMap(task.Resources), StartedAt: now}
	st.Assignments[asg.AssignmentID] = asg
	appendChronicle(st, now, "observe_task_nonexclusive", asg.AssignmentID, map[string]any{"task": task.TaskID, "seed": seed.SeedID, "claim_exclusive": false})
	return &asg, nil
}

func submitSolution(st *EcosystemState, in SolutionInput, now int64) (*Solution, error) {
	if in.TaskID == "" || in.SeedID == "" {
		return nil, errors.New("solution.task_id and solution.seed_id required")
	}
	seed, ok := st.Seeds[in.SeedID]
	if !ok {
		return nil, errors.New("seed not found")
	}
	task, ok := st.Tasks[in.TaskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	if !seed.Alive {
		return nil, errors.New("seed is dead")
	}
	if task.Status != "open" || !task.Public {
		return nil, errors.New("task is not an open public problem")
	}
	if task.Difficulty == "L2" && seed.Stage != "L2" {
		return nil, errors.New("L2 task requires L2 seed stage")
	}
	if err := validateResourceDelta(st, in.Consumed, "solution.consumed"); err != nil {
		return nil, err
	}
	for res, amount := range in.Consumed {
		if seed.Balances[res] < amount {
			return nil, fmt.Errorf("insufficient %s", res)
		}
	}
	id := in.SolutionID
	if id == "" {
		id = shortID("solution", map[string]any{"task": in.TaskID, "seed": in.SeedID, "payload": in.Payload, "evaluation_input": in.EvaluationInput, "now": now, "n": len(st.Solutions) + 1})
	}
	if _, exists := st.Solutions[id]; exists {
		return nil, fmt.Errorf("solution %s already exists", id)
	}
	cflID := defaultStr(in.VerificationCFL, task.VerificationCFL)
	if !isRegisteredLayer(st, cflID, "V") {
		return nil, fmt.Errorf("verification_cfl %s is not registered V CFL", cflID)
	}
	sol := Solution{SolutionID: id, TaskID: task.TaskID, SeedID: seed.SeedID, Status: "pending_v", RewardTier: "none", Consumed: copyFloatMap(in.Consumed), Payload: copyAnyMap(in.Payload), Metadata: in.Metadata, CreatedAt: now, UpdatedAt: now}
	sol.PatternRef = patternRef("solution", map[string]any{"task": task.TaskID, "seed": seed.SeedID, "payload": in.Payload})
	sol.RelationRef = relationRef(map[string]any{"task": task.TaskID, "seed": seed.SeedID, "solution": id, "relation": "submitted_solution"})

	if seed.Stage == "L2" && in.Consumed["api_token"] > 0 {
		seed.Alive = false
		seed.DeathReason = "external_api_used_in_l2"
		seed.ExternalAPIAllowed = false
		sol.Status = "disqualified"
		sol.Assessment = ValueAssessment{CFLID: cflID, Verdict: "fail", Raw: map[string]any{"verdict": "fail", "reason": "external_api_used_in_l2"}}
	} else {
		subtractResources(seed.Balances, in.Consumed)
		assessment, err := assessSolutionWithV(st, task, in, cflID)
		if err != nil {
			return nil, err
		}
		sol.Assessment = assessment
		if assessment.Qualified {
			if !task.Completed {
				sol.RewardTier = "base"
				sol.Status = "base_awarded"
				sol.Reward = copyFloatMap(task.BaseReward)
				task.Completed = true
				task.BestSolutionIDs = appendBestSolution(task.BestSolutionIDs, sol.SolutionID, nil)
			} else if assessment.Breakthrough {
				sol.RewardTier = "breakthrough"
				sol.Status = "breakthrough_awarded"
				sol.Reward = copyFloatMap(task.BreakthroughReward)
				task.BestSolutionIDs = appendBestSolution(task.BestSolutionIDs, sol.SolutionID, supersedesIDs(assessment.Raw))
			} else {
				sol.Status = "qualified_no_reward"
			}
			if len(sol.Reward) > 0 {
				if err := allocateFromPools(st, sol.Reward, "solution.reward"); err != nil {
					return nil, err
				}
				mergeResources(seed.Balances, sol.Reward)
			}
		} else {
			sol.Status = "failed"
			if in.EscalateOnFail {
				_, _ = addInbox(st, EscalateInput{Origin: "evolution:submit_solution", Kind: "task_failed", Severity: "warning", Item: map[string]any{"task_id": task.TaskID, "seed_id": seed.SeedID, "solution_id": sol.SolutionID, "assessment": assessment}}, now)
			}
		}
		evaluateSeedDeath(st, &seed)
	}

	if sol.Assessment.Raw != nil {
		sol.ValueRef = valueRef(sol.Assessment)
	}
	sol.StateRef = solutionStateRef(sol)
	task.SolutionIDs = append(task.SolutionIDs, sol.SolutionID)
	task.ValueRef = sol.ValueRef
	task.BestSolutionRef = stateRef(map[string]any{"task_id": task.TaskID, "best_solution_ids": task.BestSolutionIDs})
	task.UpdatedAt = now
	task.StateRef = taskStateRef(task)
	seed.UpdatedAt = now
	seed.StateRef = seedStateRef(seed)
	st.Solutions[sol.SolutionID] = sol
	st.Tasks[task.TaskID] = task
	st.Seeds[seed.SeedID] = seed
	terminateRuntimeIfDead(st, seed, now)
	appendChronicle(st, now, "submit_solution", sol.SolutionID, map[string]any{"task": task.TaskID, "seed": seed.SeedID, "status": sol.Status, "reward_tier": sol.RewardTier, "value_ref": sol.ValueRef})
	if sol.RewardTier == "base" || sol.RewardTier == "breakthrough" {
		appendChronicle(st, now, "best_solution_update", task.TaskID, map[string]any{"solution": sol.SolutionID, "tier": sol.RewardTier, "best_solution_ids": task.BestSolutionIDs})
	}
	return &sol, nil
}

func assessSolutionWithV(st *EcosystemState, task Task, in SolutionInput, cflID string) (ValueAssessment, error) {
	ref := st.CFLRegistry[cflID]
	input := copyAnyMap(in.EvaluationInput)
	if len(input) == 0 {
		input = defaultEvaluationInput(in.Payload)
	}
	if _, ok := input["task_type"]; !ok && task.TaskType != "" {
		input["task_type"] = task.TaskType
	}
	if _, ok := input["best_solution_so_far"]; !ok && len(task.BestSolutionIDs) > 0 {
		input["best_solution_so_far"] = task.BestSolutionIDs
	}
	raw, err := executeCFL(ref, input)
	if err != nil {
		return ValueAssessment{}, err
	}
	verdict := stringField(raw, "verdict")
	assessment := ValueAssessment{CFLID: cflID, Verdict: verdict, Qualified: verdict == "pass" || verdict == "approved", Breakthrough: boolField(raw, "breakthrough") || boolField(raw, "is_stronger") || boolField(raw, "stronger") || boolField(raw, "exceeds_best_solution"), DimensionVector: extractDimensionVector(raw), EvidenceHash: stringField(raw, "evidence_hash"), Raw: raw}
	if tier := stringField(raw, "reward_tier"); tier == "breakthrough" {
		assessment.Breakthrough = true
	}
	if tier := stringField(raw, "tier"); tier == "breakthrough" {
		assessment.Breakthrough = true
	}
	return assessment, nil
}

func defaultEvaluationInput(payload map[string]any) map[string]any {
	out := copyAnyMap(payload)
	if _, ok := out["scope"]; !ok {
		if _, hasActual := out["actual"]; hasActual {
			if _, hasExpected := out["expected"]; hasExpected {
				out["scope"] = "result"
			}
		}
	}
	return out
}

func advanceStage(st *EcosystemState, in AdvanceInput, now int64) (*Seed, error) {
	seed, ok := st.Seeds[in.SeedID]
	if !ok {
		return nil, errors.New("seed not found")
	}
	if in.Stage != "L0" && in.Stage != "L1" && in.Stage != "L2" {
		return nil, errors.New("stage must be L0|L1|L2")
	}
	seed.Stage = in.Stage
	seed.ExternalAPIAllowed = in.Stage != "L2"
	seed.UpdatedAt = now
	seed.StateRef = seedStateRef(seed)
	st.Seeds[seed.SeedID] = seed
	appendChronicle(st, now, "advance_stage", seed.SeedID, map[string]any{"stage": in.Stage, "external_api_allowed": seed.ExternalAPIAllowed})
	return &seed, nil
}

func tickDeaths(st *EcosystemState, now int64) {
	for id, seed := range st.Seeds {
		before := seed.Alive
		evaluateSeedDeath(st, &seed)
		if before && !seed.Alive {
			appendChronicle(st, now, "seed_death", id, map[string]any{"reason": seed.DeathReason})
		}
		seed.StateRef = seedStateRef(seed)
		st.Seeds[id] = seed
		terminateRuntimeIfDead(st, seed, now)
	}
}

func evaluateSeedDeath(st *EcosystemState, seed *Seed) {
	if !seed.Alive {
		return
	}
	for res, floor := range st.BootRules.DeathResourceFloors {
		if seed.Balances[res] <= floor {
			seed.Alive = false
			seed.DeathReason = "resource_exhausted:" + res
			return
		}
	}
}

func terminateRuntimeIfDead(st *EcosystemState, seed Seed, now int64) {
	if seed.Alive || seed.RuntimeID == "" {
		return
	}
	rt := st.Runtimes[seed.RuntimeID]
	if rt.RuntimeID == "" {
		return
	}
	rt.Status = "terminated"
	if rt.StoppedAt == 0 {
		rt.StoppedAt = now
	}
	rt.StateRef = runtimeStateRef(rt)
	st.Runtimes[rt.RuntimeID] = rt
}

func addInbox(st *EcosystemState, in EscalateInput, now int64) (*InboxEntry, error) {
	if in.Origin == "" {
		return nil, errors.New("escalation origin required")
	}
	entry := InboxEntry{Origin: in.Origin, Kind: defaultStr(in.Kind, "human_decision"), Severity: defaultStr(in.Severity, "warning"), Item: in.Item, SuggestedActions: append([]string(nil), in.SuggestedActions...), CreatedAt: now}
	entry.Priority = priorityFor(entry.Severity, entry.Kind)
	entry.EntryID = shortID("inbox", map[string]any{"origin": entry.Origin, "kind": entry.Kind, "severity": entry.Severity, "item": entry.Item, "actions": entry.SuggestedActions})
	st.Inbox = append(st.Inbox, entry)
	appendChronicle(st, now, "escalate", entry.EntryID, map[string]any{"priority": entry.Priority})
	return &entry, nil
}

func priorityFor(severity, kind string) int {
	base := map[string]int{"critical": 8, "warning": 5, "info": 2}[severity]
	if base == 0 && severity != "info" {
		base = 5
	}
	bonus := map[string]int{"constitution_violation": 1, "task_failed": 1, "uncontrollable": 1, "human_decision": 0, "paradigm_question": 0}[kind]
	p := base + bonus
	if p > 9 {
		p = 9
	}
	if p < 0 {
		p = 0
	}
	return p
}

func sendMessage(st *EcosystemState, in MessageInput, now int64) (*Message, error) {
	if in.FromSeedID == "" {
		return nil, errors.New("message.from_seed_id required")
	}
	from, ok := st.Seeds[in.FromSeedID]
	if !ok || !from.Alive {
		return nil, errors.New("message sender seed not alive")
	}
	var recipients []string
	if in.ToSeedID == "" || in.ToSeedID == "all" {
		for _, sid := range livingSeedIDs(st) {
			if sid != in.FromSeedID {
				recipients = append(recipients, sid)
			}
		}
	} else {
		to, ok := st.Seeds[in.ToSeedID]
		if !ok || !to.Alive {
			return nil, errors.New("message recipient seed not alive")
		}
		recipients = []string{in.ToSeedID}
	}
	var first *Message
	for _, to := range recipients {
		msg := makeMessage(in.FromSeedID, to, defaultStr(in.Channel, "inter_seed"), defaultStr(in.Kind, "freeform"), in.Body, now)
		appendMessage(st, msg)
		if first == nil {
			m := msg
			first = &m
		}
	}
	if first == nil {
		return nil, errors.New("message has no recipients")
	}
	appendChronicle(st, now, "send_message", first.MessageID, map[string]any{"from": in.FromSeedID, "to_count": len(recipients), "channel": defaultStr(in.Channel, "inter_seed")})
	return first, nil
}

func runtimeTick(st *EcosystemState, now int64) RuntimeTickReport {
	taskIDs := []string{}
	for _, tid := range sortedTaskIDs(st.Tasks) {
		t := st.Tasks[tid]
		if t.Public && t.Status == "open" {
			taskIDs = append(taskIDs, tid)
		}
	}
	type tickOut struct {
		seedID   string
		runtime  SeedRuntime
		messages []Message
	}
	ch := make(chan tickOut)
	var wg sync.WaitGroup
	for _, seedID := range sortedSeedIDs(st.Seeds) {
		seed := st.Seeds[seedID]
		rt := st.Runtimes[seed.RuntimeID]
		if !seed.Alive || rt.Status != "running" {
			continue
		}
		wg.Add(1)
		go func(seed Seed, rt SeedRuntime) {
			defer wg.Done()
			rt.WorkLoopTicks++
			rt.LastTickAt = now
			msgs := []Message{}
			for _, taskID := range taskIDs {
				body := map[string]any{"task_id": taskID, "broadcast": true, "open_problem": true}
				msgs = append(msgs, makeMessage("evolution:task_broadcast", seed.SeedID, "task_broadcast", "open_task", body, now))
			}
			rt.StateRef = runtimeStateRef(rt)
			ch <- tickOut{seedID: seed.SeedID, runtime: rt, messages: msgs}
		}(seed, rt)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	outs := []tickOut{}
	for out := range ch {
		outs = append(outs, out)
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].seedID < outs[j].seedID })
	report := RuntimeTickReport{}
	for _, out := range outs {
		st.Runtimes[out.runtime.RuntimeID] = out.runtime
		report.RuntimeCount++
		report.SeedIDs = append(report.SeedIDs, out.seedID)
		for _, msg := range out.messages {
			appendMessage(st, msg)
			report.MessagesPublished++
		}
	}
	appendChronicle(st, now, "runtime_tick", st.EcosystemID, map[string]any{"runtimes": report.RuntimeCount, "messages": report.MessagesPublished})
	return report
}

func makeMessage(from, to, channel, kind string, body map[string]any, now int64) Message {
	msg := Message{FromSeedID: from, ToSeedID: to, Channel: channel, Kind: kind, Body: copyAnyMap(body), CreatedAt: now}
	msg.MessageID = shortID("message", map[string]any{"from": from, "to": to, "channel": channel, "kind": kind, "body": body, "now": now})
	msg.PatternRef = patternRef("message", msg)
	msg.RelationRef = relationRef(map[string]any{"from": from, "to": to, "channel": channel, "message": msg.MessageID})
	return msg
}

func appendMessage(st *EcosystemState, msg Message) {
	st.Messages = append(st.Messages, msg)
	for rid, rt := range st.Runtimes {
		if rt.SeedID == msg.ToSeedID && rt.Status == "running" {
			rt.Mailbox = append(rt.Mailbox, msg.MessageID)
			rt.StateRef = runtimeStateRef(rt)
			st.Runtimes[rid] = rt
			return
		}
	}
}

func auditState(st *EcosystemState) AuditReport {
	a := AuditReport{Verdict: "pass"}
	need := map[string]string{"p-cfl": "P", "l0-v-test-execution-cfl": "V", "l0-r-protocol-validate-cfl": "R", "l0-r-hash-chain-cfl": "R", "l1-s-lifecycle-death-cfl": "S"}
	for id, layer := range need {
		if !isRegisteredLayer(st, id, layer) {
			a.Violations = append(a.Violations, "missing_internal_cfl:"+id)
		}
	}
	if st.BootRules.BornflyMode != "boot_escalation_only" {
		a.Violations = append(a.Violations, "bornfly_runtime_operator_not_allowed")
	}
	for id, pool := range st.Resources {
		if pool.Spendable && (!pool.Metered || pool.Capacity <= 0 || pool.Available < 0) {
			a.Violations = append(a.Violations, "resource_not_finite_metered:"+id)
		}
	}
	for id, dir := range st.Directions {
		if len(dir.SeedIDs) != 3 {
			a.Violations = append(a.Violations, "direction_not_three_seeds:"+id)
			continue
		}
		variants := map[string]bool{}
		for _, sid := range dir.SeedIDs {
			variants[st.Seeds[sid].Variant] = true
		}
		for _, v := range []string{"extreme_a", "median", "extreme_b"} {
			if !variants[v] {
				a.Violations = append(a.Violations, "direction_missing_variant:"+id+":"+v)
			}
		}
	}
	living := stringSet(livingSeedIDs(st))
	for _, id := range sortedTaskIDs(st.Tasks) {
		t := st.Tasks[id]
		if !t.Public {
			a.Violations = append(a.Violations, "task_not_public:"+id)
		}
		if t.Status != "open" {
			a.Violations = append(a.Violations, "task_not_open_problem:"+id)
		}
		vis := stringSet(t.VisibleTo)
		for sid := range living {
			if !vis[sid] {
				a.Violations = append(a.Violations, "task_not_visible_to_living_seed:"+id+":"+sid)
			}
		}
		if t.Completed && len(t.BestSolutionIDs) == 0 {
			a.Violations = append(a.Violations, "completed_task_missing_best_solution:"+id)
		}
	}
	for id, seed := range st.Seeds {
		if seed.Stage == "L2" && seed.ExternalAPIAllowed {
			a.Violations = append(a.Violations, "l2_seed_external_api_allowed:"+id)
		}
		rt, ok := st.Runtimes[seed.RuntimeID]
		if !ok {
			a.Violations = append(a.Violations, "seed_missing_independent_runtime:"+id)
		} else if seed.Alive && rt.Status != "running" {
			a.Violations = append(a.Violations, "alive_seed_runtime_not_running:"+id)
		} else if !seed.Alive && rt.Status != "terminated" {
			a.Violations = append(a.Violations, "dead_seed_runtime_not_terminated:"+id)
		}
	}
	for id, sol := range st.Solutions {
		if sol.ValueRef == "" || sol.PatternRef == "" || sol.RelationRef == "" {
			a.Violations = append(a.Violations, "solution_missing_prv_ref:"+id)
		}
	}
	if len(a.Violations) > 0 {
		a.Verdict = "fail"
	}
	sort.Strings(a.Violations)
	sort.Strings(a.Warnings)
	return a
}

func isRegisteredLayer(st *EcosystemState, id, layer string) bool {
	ref, ok := st.CFLRegistry[id]
	return ok && ref.Layer == layer && ref.Internal
}
func copyFloatMap(m map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
func copyAnyMap(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
func multiplyResources(m map[string]float64, factor float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range m {
		out[k] = round6(v * factor)
	}
	return out
}
func validateResourceDelta(st *EcosystemState, m map[string]float64, field string) error {
	for id, v := range m {
		pool, ok := st.Resources[id]
		if !ok {
			return fmt.Errorf("%s references unknown resource %s", field, id)
		}
		if !pool.Metered {
			return fmt.Errorf("%s references unmetered resource %s", field, id)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("%s has invalid amount for %s", field, id)
		}
	}
	return nil
}
func allocateFromPools(st *EcosystemState, m map[string]float64, field string) error {
	if err := validateResourceDelta(st, m, field); err != nil {
		return err
	}
	for id, amount := range m {
		pool := st.Resources[id]
		if !pool.Spendable {
			continue
		}
		if pool.Available < amount {
			return fmt.Errorf("%s insufficient global resource %s", field, id)
		}
	}
	for id, amount := range m {
		pool := st.Resources[id]
		if !pool.Spendable {
			continue
		}
		pool.Available = round6(pool.Available - amount)
		st.Resources[id] = pool
	}
	return nil
}
func injectIntoPools(st *EcosystemState, m map[string]float64) {
	for id, amount := range m {
		pool := st.Resources[id]
		pool.Available = round6(pool.Available + amount)
		if pool.Available > pool.Capacity {
			pool.Capacity = pool.Available
		}
		st.Resources[id] = pool
	}
}
func livingSeedIDs(st *EcosystemState) []string {
	ids := []string{}
	for _, id := range sortedSeedIDs(st.Seeds) {
		if st.Seeds[id].Alive {
			ids = append(ids, id)
		}
	}
	return ids
}
func refreshTaskVisibility(st *EcosystemState) {
	ids := livingSeedIDs(st)
	for tid, t := range st.Tasks {
		if t.Public && t.Status == "open" {
			t.VisibleTo = append([]string(nil), ids...)
			t.StateRef = taskStateRef(t)
			st.Tasks[tid] = t
		}
	}
}
func appendChronicle(st *EcosystemState, now int64, kind, ref string, detail map[string]any) {
	st.Chronicle = append(st.Chronicle, ChronicleEvent{Ts: now, Kind: kind, Ref: ref, Detail: detail})
}
func appendBestSolution(current []string, solutionID string, supersedes []string) []string {
	remove := stringSet(supersedes)
	out := []string{}
	seen := map[string]bool{}
	for _, id := range current {
		if remove[id] || id == solutionID || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if !seen[solutionID] {
		out = append(out, solutionID)
	}
	sort.Strings(out)
	return out
}
func supersedesIDs(raw map[string]any) []string {
	for _, key := range []string{"supersedes_solution_ids", "supersedes", "replaces_solution_ids"} {
		if v, ok := raw[key]; ok {
			return stringSlice(v)
		}
	}
	return nil
}
func stringSlice(v any) []string {
	out := []string{}
	switch xs := v.(type) {
	case []string:
		out = append(out, xs...)
	case []any:
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
func extractDimensionVector(raw map[string]any) map[string]any {
	for _, key := range []string{"dimensions", "dimension_vector", "value_dimensions"} {
		if m, ok := raw[key].(map[string]any); ok {
			return copyAnyMap(m)
		}
	}
	return copyAnyMap(raw)
}
func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
func boolField(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case bool:
			return x
		case string:
			return x == "true" || x == "yes" || x == "pass"
		}
	}
	return false
}
func seedStateRef(seed Seed) string {
	return stateRef(map[string]any{"seed_id": seed.SeedID, "direction_id": seed.DirectionID, "variant": seed.Variant, "stage": seed.Stage, "balances": seed.Balances, "alive": seed.Alive, "external_api_allowed": seed.ExternalAPIAllowed, "death_reason": seed.DeathReason, "runtime_id": seed.RuntimeID})
}
func runtimeStateRef(rt SeedRuntime) string {
	return stateRef(map[string]any{"runtime_id": rt.RuntimeID, "seed_id": rt.SeedID, "status": rt.Status, "isolation": rt.Isolation, "mailbox": rt.Mailbox, "ticks": rt.WorkLoopTicks})
}
func taskStateRef(task Task) string {
	return stateRef(map[string]any{"task_id": task.TaskID, "difficulty": task.Difficulty, "task_type": task.TaskType, "status": task.Status, "completed": task.Completed, "public": task.Public, "visible_to": task.VisibleTo, "solution_ids": task.SolutionIDs, "best_solution_ids": task.BestSolutionIDs})
}
func solutionStateRef(sol Solution) string {
	return stateRef(map[string]any{"solution_id": sol.SolutionID, "task_id": sol.TaskID, "seed_id": sol.SeedID, "status": sol.Status, "reward_tier": sol.RewardTier, "qualified": sol.Assessment.Qualified, "breakthrough": sol.Assessment.Breakthrough})
}
