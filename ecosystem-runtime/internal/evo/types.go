package evo

type Request struct {
	Action      string `json:"action"`
	StateDir    string `json:"state_dir,omitempty"`
	EcosystemID string `json:"ecosystem_id,omitempty"`
	NowUnixMs   int64  `json:"now_unix_ms,omitempty"`

	Boot      BootInput      `json:"boot,omitempty"`
	Direction DirectionInput `json:"direction,omitempty"`
	Task      TaskInput      `json:"task,omitempty"`
	Solution  SolutionInput  `json:"solution,omitempty"`
	Message   MessageInput   `json:"message,omitempty"`
	Claim     ClaimInput     `json:"claim,omitempty"`  // legacy observe-only; claim exclusivity is forbidden.
	Settle    SettleInput    `json:"settle,omitempty"` // deprecated; submit_solution must execute V CFL itself.
	Advance   AdvanceInput   `json:"advance,omitempty"`
	Escalate  EscalateInput  `json:"escalate,omitempty"`
}

type Response struct {
	Verdict     string             `json:"verdict"`
	Action      string             `json:"action,omitempty"`
	Error       string             `json:"error,omitempty"`
	EcosystemID string             `json:"ecosystem_id,omitempty"`
	State       *EcosystemState    `json:"state,omitempty"`
	Task        *Task              `json:"task,omitempty"`
	Seed        *Seed              `json:"seed,omitempty"`
	Solution    *Solution          `json:"solution,omitempty"`
	Message     *Message           `json:"message,omitempty"`
	Assignment  *Assignment        `json:"assignment,omitempty"`
	InboxEntry  *InboxEntry        `json:"inbox_entry,omitempty"`
	RuntimeTick *RuntimeTickReport `json:"runtime_tick,omitempty"`
	Audit       *AuditReport       `json:"audit,omitempty"`
	LatencyMs   int64              `json:"latency_ms"`
}

type BootInput struct {
	Force             bool                    `json:"force,omitempty"`
	Phase             string                  `json:"phase,omitempty"`
	BootRules         BootRules               `json:"boot_rules,omitempty"`
	CFLRegistry       map[string]CFLRef       `json:"cfl_registry,omitempty"`
	Resources         map[string]ResourcePool `json:"resources,omitempty"`
	InitialDirections []DirectionInput        `json:"initial_directions,omitempty"`
	InitialTasks      []TaskInput             `json:"initial_tasks,omitempty"`
	InitialBalances   map[string]float64      `json:"initial_balances,omitempty"`
	Metadata          map[string]any          `json:"metadata,omitempty"`
}

type BootRules struct {
	BornflyMode               string             `json:"bornfly_mode"`
	SeedVariants              []string           `json:"seed_variants"`
	DefaultSeedBalance        map[string]float64 `json:"default_seed_balance"`
	DeathResourceFloors       map[string]float64 `json:"death_resource_floors"`
	RewardPolicy              string             `json:"reward_policy"`
	TaskVisibility            string             `json:"task_visibility"`
	ExternalAPIAllowedUntil   string             `json:"external_api_allowed_until"`
	OpenNetworkPolicy         string             `json:"open_network_policy"`
	DefaultBaseReward         map[string]float64 `json:"default_base_reward,omitempty"`
	DefaultBreakthroughReward map[string]float64 `json:"default_breakthrough_reward,omitempty"`
}

type CFLRef struct {
	ID       string `json:"id"`
	Layer    string `json:"layer"` // P/R/V/S
	Command  string `json:"command,omitempty"`
	Module   string `json:"module,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Internal bool   `json:"internal"`
}

type ResourcePool struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Capacity  float64 `json:"capacity"`
	Available float64 `json:"available"`
	Metered   bool    `json:"metered"`
	Spendable bool    `json:"spendable"`
}

type EcosystemState struct {
	EcosystemID string                       `json:"ecosystem_id"`
	Phase       string                       `json:"phase"`
	BootRules   BootRules                    `json:"boot_rules"`
	CFLRegistry map[string]CFLRef            `json:"cfl_registry"`
	Resources   map[string]ResourcePool      `json:"resources"`
	Directions  map[string]ResearchDirection `json:"directions"`
	Seeds       map[string]Seed              `json:"seeds"`
	Runtimes    map[string]SeedRuntime       `json:"runtimes"`
	Tasks       map[string]Task              `json:"tasks"`
	Solutions   map[string]Solution          `json:"solutions"`
	Assignments map[string]Assignment        `json:"assignments,omitempty"` // legacy observe records only.
	Messages    []Message                    `json:"messages"`
	Inbox       []InboxEntry                 `json:"inbox"`
	Chronicle   []ChronicleEvent             `json:"chronicle"`
	Metadata    map[string]any               `json:"metadata,omitempty"`
	CreatedAt   int64                        `json:"created_at"`
	UpdatedAt   int64                        `json:"updated_at"`
}

type DirectionInput struct {
	DirectionID      string             `json:"direction_id"`
	Title            string             `json:"title"`
	ParadigmSeedText string             `json:"paradigm_seed_text"`
	Variants         []string           `json:"variants,omitempty"`
	InitialBalances  map[string]float64 `json:"initial_balances,omitempty"`
	Metadata         map[string]any     `json:"metadata,omitempty"`
}

type ResearchDirection struct {
	DirectionID      string         `json:"direction_id"`
	Title            string         `json:"title"`
	ParadigmSeedText string         `json:"paradigm_seed_text"`
	PatternRef       string         `json:"pattern_ref"`
	SeedIDs          []string       `json:"seed_ids"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type Seed struct {
	SeedID             string             `json:"seed_id"`
	DirectionID        string             `json:"direction_id"`
	Variant            string             `json:"variant"`
	Stage              string             `json:"stage"`
	Balances           map[string]float64 `json:"balances"`
	Alive              bool               `json:"alive"`
	ExternalAPIAllowed bool               `json:"external_api_allowed"`
	DeathReason        string             `json:"death_reason,omitempty"`
	RuntimeID          string             `json:"runtime_id"`
	PatternRef         string             `json:"pattern_ref"`
	StateRef           string             `json:"state_ref,omitempty"`
	CreatedAt          int64              `json:"created_at"`
	UpdatedAt          int64              `json:"updated_at"`
}

type SeedRuntime struct {
	RuntimeID     string   `json:"runtime_id"`
	SeedID        string   `json:"seed_id"`
	Status        string   `json:"status"` // running/terminated
	Isolation     string   `json:"isolation"`
	Mailbox       []string `json:"mailbox,omitempty"`
	WorkLoopTicks int      `json:"work_loop_ticks"`
	StartedAt     int64    `json:"started_at"`
	LastTickAt    int64    `json:"last_tick_at,omitempty"`
	StoppedAt     int64    `json:"stopped_at,omitempty"`
	StateRef      string   `json:"state_ref,omitempty"`
}

type TaskInput struct {
	TaskID             string             `json:"task_id,omitempty"`
	Origin             string             `json:"origin"` // researcher/paradigm_seed/bornfly_boot
	SubmittedBy        string             `json:"submitted_by,omitempty"`
	Difficulty         string             `json:"difficulty"` // L0/L1/L2
	TaskType           string             `json:"task_type,omitempty"`
	Goal               string             `json:"goal"`
	Resources          map[string]float64 `json:"resources,omitempty"`
	ResourceInjection  map[string]float64 `json:"resource_injection,omitempty"`
	Reward             map[string]float64 `json:"reward,omitempty"` // legacy alias for base_reward.
	BaseReward         map[string]float64 `json:"base_reward,omitempty"`
	BreakthroughReward map[string]float64 `json:"breakthrough_reward,omitempty"`
	Constitution       []string           `json:"constitution,omitempty"`
	VerificationCFL    string             `json:"verification_cfl,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
}

type Task struct {
	TaskID             string             `json:"task_id"`
	Origin             string             `json:"origin"`
	SubmittedBy        string             `json:"submitted_by,omitempty"`
	Difficulty         string             `json:"difficulty"`
	TaskType           string             `json:"task_type,omitempty"`
	Goal               string             `json:"goal"`
	Resources          map[string]float64 `json:"resources"`
	BaseReward         map[string]float64 `json:"base_reward"`
	BreakthroughReward map[string]float64 `json:"breakthrough_reward"`
	Constitution       []string           `json:"constitution"`
	VerificationCFL    string             `json:"verification_cfl"`
	Status             string             `json:"status"` // always open for open-problem tasks.
	Completed          bool               `json:"completed"`
	Public             bool               `json:"public"`
	VisibleTo          []string           `json:"visible_to"`
	SolutionIDs        []string           `json:"solution_ids,omitempty"`
	BestSolutionIDs    []string           `json:"best_solution_ids,omitempty"`
	BestSolutionRef    string             `json:"best_solution_ref,omitempty"`
	PatternRef         string             `json:"pattern_ref"`
	ValueRef           string             `json:"value_ref,omitempty"`
	RelationRef        string             `json:"relation_ref,omitempty"`
	StateRef           string             `json:"state_ref,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
	CreatedAt          int64              `json:"created_at"`
	UpdatedAt          int64              `json:"updated_at"`
}

type SolutionInput struct {
	SolutionID      string             `json:"solution_id,omitempty"`
	TaskID          string             `json:"task_id"`
	SeedID          string             `json:"seed_id"`
	Payload         map[string]any     `json:"payload,omitempty"`
	Consumed        map[string]float64 `json:"consumed,omitempty"`
	VerificationCFL string             `json:"verification_cfl,omitempty"`
	EvaluationInput map[string]any     `json:"evaluation_input,omitempty"`
	EscalateOnFail  bool               `json:"escalate_on_fail,omitempty"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
}

type Solution struct {
	SolutionID  string             `json:"solution_id"`
	TaskID      string             `json:"task_id"`
	SeedID      string             `json:"seed_id"`
	Status      string             `json:"status"`
	RewardTier  string             `json:"reward_tier"`
	Consumed    map[string]float64 `json:"consumed,omitempty"`
	Reward      map[string]float64 `json:"reward,omitempty"`
	Payload     map[string]any     `json:"payload,omitempty"`
	Assessment  ValueAssessment    `json:"assessment"`
	PatternRef  string             `json:"pattern_ref"`
	ValueRef    string             `json:"value_ref,omitempty"`
	RelationRef string             `json:"relation_ref,omitempty"`
	StateRef    string             `json:"state_ref,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
	CreatedAt   int64              `json:"created_at"`
	UpdatedAt   int64              `json:"updated_at"`
}

type ValueAssessment struct {
	CFLID           string         `json:"cfl_id"`
	Verdict         string         `json:"verdict"`
	Qualified       bool           `json:"qualified"`
	Breakthrough    bool           `json:"breakthrough"`
	DimensionVector map[string]any `json:"dimension_vector,omitempty"`
	EvidenceHash    string         `json:"evidence_hash,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type MessageInput struct {
	FromSeedID string         `json:"from_seed_id"`
	ToSeedID   string         `json:"to_seed_id,omitempty"` // empty or all broadcasts to all other live runtimes.
	Channel    string         `json:"channel,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Body       map[string]any `json:"body,omitempty"`
}

type Message struct {
	MessageID   string         `json:"message_id"`
	FromSeedID  string         `json:"from_seed_id"`
	ToSeedID    string         `json:"to_seed_id"`
	Channel     string         `json:"channel"`
	Kind        string         `json:"kind"`
	Body        map[string]any `json:"body,omitempty"`
	PatternRef  string         `json:"pattern_ref"`
	RelationRef string         `json:"relation_ref"`
	CreatedAt   int64          `json:"created_at"`
}

type RuntimeTickReport struct {
	RuntimeCount      int      `json:"runtime_count"`
	MessagesPublished int      `json:"messages_published"`
	SeedIDs           []string `json:"seed_ids"`
}

type ClaimInput struct {
	TaskID string `json:"task_id"`
	SeedID string `json:"seed_id"`
}

type Assignment struct {
	AssignmentID string             `json:"assignment_id"`
	TaskID       string             `json:"task_id"`
	SeedID       string             `json:"seed_id"`
	Status       string             `json:"status"`
	Budget       map[string]float64 `json:"budget"`
	StartedAt    int64              `json:"started_at"`
	SettledAt    int64              `json:"settled_at,omitempty"`
	Verification Verification       `json:"verification,omitempty"`
}

type SettleInput struct {
	TaskID         string             `json:"task_id"`
	SeedID         string             `json:"seed_id"`
	Consumed       map[string]float64 `json:"consumed,omitempty"`
	Verification   Verification       `json:"verification"`
	EscalateOnFail bool               `json:"escalate_on_fail,omitempty"`
}

type Verification struct {
	CFLID        string         `json:"cfl_id"`
	Verdict      string         `json:"verdict"`
	Score        float64        `json:"score,omitempty"`
	EvidenceHash string         `json:"evidence_hash,omitempty"`
	Raw          map[string]any `json:"raw,omitempty"`
}

type AdvanceInput struct {
	SeedID string `json:"seed_id"`
	Stage  string `json:"stage"`
}

type EscalateInput struct {
	Origin           string         `json:"origin"`
	Kind             string         `json:"kind"`
	Severity         string         `json:"severity"`
	Item             map[string]any `json:"item"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
}

type InboxEntry struct {
	EntryID          string         `json:"entry_id"`
	Origin           string         `json:"origin"`
	Kind             string         `json:"kind"`
	Severity         string         `json:"severity"`
	Priority         int            `json:"priority"`
	Item             map[string]any `json:"item"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
	CreatedAt        int64          `json:"created_at"`
}

type ChronicleEvent struct {
	Ts     int64          `json:"ts"`
	Kind   string         `json:"kind"`
	Ref    string         `json:"ref,omitempty"`
	Detail map[string]any `json:"detail,omitempty"`
}

type AuditReport struct {
	Verdict    string   `json:"verdict"`
	Violations []string `json:"violations"`
	Warnings   []string `json:"warnings"`
}
