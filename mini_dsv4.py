import os, sys, math, time, json, gc
from pathlib import Path
from dataclasses import dataclass, field

import torch
import torch.nn as nn
import torch.nn.functional as F

# ===========================================================================
# AGENT-EDITABLE CONFIG — model architecture, hyperparams, training budget
# ===========================================================================

@dataclass
class ModelConfig:
    vocab_size: int = 32000
    dim: int = 2048
    n_layers: int = 16
    n_heads: int = 32
    n_kv_heads: int = 8
    head_dim: int = 64
    max_seq_len: int = 2048

    q_lora_rank: int = 1024
    kv_lora_rank: int = 512

    n_routed_experts: int = 8
    n_activated_experts: int = 2
    n_shared_experts: int = 1
    expert_inter_dim: int = 8192
    score_func: str = "softmax"
    route_scale: float = 1.0

    use_value_embedding: bool = True
    use_qk_norm: bool = True
    rope_base: float = 500000.0
    rope_dim: int = 32

    window_pattern: str = "L"
    activation: str = "swiglu"
    logit_softcap: float = 30.0
    norm_eps: float = 1e-5

HYPERPARAMS = {
    "total_batch_size": 2 ** 18,
    "device_batch_size": 8,
    "embedding_lr": 0.3,
    "unembedding_lr": 0.008,
    "matrix_lr": 0.02,
    "scalar_lr": 0.3,
    "expert_lr": 0.02,
    "weight_decay": 0.1,
    "adam_betas": (0.8, 0.95),
    "warmup_ratio": 0.03,
    "warmdown_ratio": 0.5,
    "final_lr_frac": 0.05,
}

TIME_BUDGET = 300
EVAL_STEPS = 20
SEED = 42

# ===========================================================================
# MODEL DEFINITION
# ===========================================================================

def rms_norm(x, eps=1e-5):
    return F.rms_norm(x, (x.size(-1),), eps)

def precompute_rotary(seq_len, dim, base, device):
    channel_range = torch.arange(0, dim, 2, dtype=torch.float32, device=device)
    inv_freq = 1.0 / (base ** (channel_range / dim))
    t = torch.arange(seq_len, dtype=torch.float32, device=device)
    freqs = torch.outer(t, inv_freq)
    cos = freqs.cos().bfloat16()[None, :, None, :]
    sin = freqs.sin().bfloat16()[None, :, None, :]
    return cos, sin

def apply_rotary_emb(x, cos, sin):
    d = x.shape[3] // 2
    x1, x2 = x[..., :d], x[..., d:]
    return torch.cat([x1 * cos + x2 * sin, x1 * (-sin) + x2 * cos], 3)

def count_params(model):
    return sum(p.numel() for p in model.parameters())

# ------ Multi-head Latent Attention (MLA) ------

class MLA(nn.Module):
    def __init__(self, config, layer_idx):
        super().__init__()
        self.n_heads = config.n_heads
        self.n_kv_heads = config.n_kv_heads
        self.head_dim = config.head_dim
        self.rope_dim = config.rope_dim
        self.nope_dim = self.head_dim - self.rope_dim
        self.q_lora_rank = config.q_lora_rank
        self.kv_lora_rank = config.kv_lora_rank
        self.softmax_scale = self.head_dim ** -0.5

        self.wq_a = nn.Linear(config.dim, self.q_lora_rank, bias=False)
        self.q_norm = nn.RMSNorm(self.q_lora_rank, config.norm_eps)
        self.wq_b = nn.Linear(self.q_lora_rank, self.n_heads * self.head_dim, bias=False)

        self.wkv_a = nn.Linear(config.dim, self.kv_lora_rank + self.rope_dim, bias=False)
        self.kv_norm = nn.RMSNorm(self.kv_lora_rank, config.norm_eps)
        self.wkv_b = nn.Linear(self.kv_lora_rank, self.n_kv_heads * (self.head_dim + self.rope_dim), bias=False)

        self.wo = nn.Linear(self.n_heads * self.head_dim, config.dim, bias=False)

        self.use_ve = config.use_value_embedding and layer_idx % 2 == (config.n_layers - 1) % 2
        if self.use_ve:
            self.ve_gate = nn.Linear(32, self.n_kv_heads, bias=False)

    def forward(self, x, ve, cos_sin, mask=None):
        B, T, C = x.size()

        qr = self.q_norm(self.wq_a(x))
        q = self.wq_b(qr).view(B, T, self.n_heads, self.head_dim)

        kv_a = self.wkv_a(x)
        kv_lora, k_rope = kv_a.split([self.kv_lora_rank, self.rope_dim], dim=-1)
        kv_lora = self.kv_norm(kv_lora)
        kv = self.wkv_b(kv_lora).view(B, T, self.n_kv_heads, self.head_dim + self.rope_dim)
        k_nope, v = kv.split([self.head_dim, self.rope_dim], dim=-1)
        k_rope = k_rope.view(B, T, self.n_kv_heads, self.rope_dim)

        cos, sin = cos_sin
        q_rope, q_nope = q[..., :self.rope_dim], q[..., self.rope_dim:]
        q_rope = apply_rotary_emb(q_rope, cos, sin)
        k_rope = apply_rotary_emb(k_rope, cos, sin)

        q = torch.cat([q_rope, q_nope], dim=-1)
        k = torch.cat([k_rope, k_nope], dim=-1)

        if ve is not None and self.use_ve:
            gate = 2 * torch.sigmoid(self.ve_gate(x[..., :32]))
            v = v + gate.unsqueeze(-1) * ve.view(B, T, self.n_kv_heads, self.head_dim)

        if config.use_qk_norm:
            q, k = rms_norm(q), rms_norm(k)

        if self.n_kv_heads < self.n_heads:
            g = self.n_heads // self.n_kv_heads
            k = k.repeat_interleave(g, dim=2)
            v = v.repeat_interleave(g, dim=2)

        q, k, v = q.transpose(1, 2), k.transpose(1, 2), v.transpose(1, 2)
        y = F.scaled_dot_product_attention(q, k, v, attn_mask=mask, is_causal=(mask is None))
        y = y.transpose(1, 2).contiguous().view(B, T, -1)
        return self.wo(y)

# ------ MoE (Mixture of Experts) ------

class Expert(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.gate_proj = nn.Linear(config.dim, config.expert_inter_dim, bias=False)
        self.up_proj = nn.Linear(config.dim, config.expert_inter_dim, bias=False)
        self.down_proj = nn.Linear(config.expert_inter_dim, config.dim, bias=False)
        self.act = config.activation

    def forward(self, x, weights=None):
        gate = self.gate_proj(x)
        up = self.up_proj(x)
        if self.act == "swiglu":
            y = F.silu(gate) * up
        elif self.act == "relu2":
            y = F.relu(gate).square() * up
        else:
            y = F.gelu(gate) * up
        if weights is not None:
            y = weights * y
        return self.down_proj(y)

class Gate(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.top_k = config.n_activated_experts
        self.n_routed = config.n_routed_experts
        self.score_func = config.score_func
        self.route_scale = config.route_scale
        self.weight = nn.Parameter(torch.empty(config.n_routed_experts, config.dim))
        self.bias = nn.Parameter(torch.zeros(config.n_routed_experts))

    def forward(self, x):
        scores = F.linear(x.float(), self.weight.float())
        scores = scores + self.bias
        if self.score_func == "softmax":
            routing_weights = scores.softmax(dim=-1)
            indices = scores.topk(self.top_k, dim=-1)[1]
        elif self.score_func == "sigmoid":
            routing_weights = scores.sigmoid()
            indices = scores.topk(self.top_k, dim=-1)[1]
        else:
            routing_weights = F.softplus(scores).sqrt()
            indices = scores.topk(self.top_k, dim=-1)[1]
        weights = routing_weights.gather(1, indices)
        if self.score_func != "softmax":
            weights = weights / weights.sum(dim=-1, keepdim=True)
        weights = weights * self.route_scale
        return weights, indices

class MoE(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.gate = Gate(config)
        self.experts = nn.ModuleList([Expert(config) for _ in range(config.n_routed_experts)])
        if config.n_shared_experts > 0:
            self.shared_expert = Expert(config)
        else:
            self.shared_expert = None

    def forward(self, x):
        B, T, C = x.size()
        flat_x = x.view(-1, C)
        weights, indices = self.gate(flat_x)
        y = torch.zeros_like(flat_x, dtype=torch.float32)
        for i in range(config.n_routed_experts):
            idx, top = torch.where(indices == i)
            if idx.numel() > 0:
                y[idx] += self.experts[i](flat_x[idx], weights[idx, top, None])
        if self.shared_expert is not None:
            y = y + self.shared_expert(flat_x)
        return y.type_as(x).view(B, T, C)

# ------ Block ------

class Block(nn.Module):
    def __init__(self, config, layer_idx):
        super().__init__()
        self.attn = MLA(config, layer_idx)
        self.moe = MoE(config)
        self.attn_norm = nn.RMSNorm(config.dim, config.norm_eps)
        self.moe_norm = nn.RMSNorm(config.dim, config.norm_eps)

    def forward(self, x, ve, cos_sin):
        x = x + self.attn(self.attn_norm(x), ve, cos_sin)
        x = x + self.moe(self.moe_norm(x))
        return x

# ------ Full Model ------

class MiniDeepSeekV4(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.config = config

        self.tok_embeddings = nn.Embedding(config.vocab_size, config.dim)
        self.layers = nn.ModuleList([Block(config, i) for i in range(config.n_layers)])
        self.norm = nn.RMSNorm(config.dim, config.norm_eps)
        self.lm_head = nn.Linear(config.dim, config.vocab_size, bias=False)

        self.resid_lambdas = nn.Parameter(torch.ones(config.n_layers))
        self.x0_lambdas = nn.Parameter(torch.zeros(config.n_layers))

        if config.use_value_embedding:
            kv_dim = config.n_kv_heads * config.head_dim
            self.value_embeds = nn.ModuleDict({
                str(i): nn.Embedding(config.vocab_size, kv_dim)
                for i in range(config.n_layers)
                if i % 2 == (config.n_layers - 1) % 2
            })
        else:
            self.value_embeds = nn.ModuleDict()

        cos, sin = precompute_rotary(
            config.max_seq_len * 4, config.rope_dim, config.rope_base, torch.device("cpu")
        )
        self.register_buffer("cos", cos, persistent=False)
        self.register_buffer("sin", sin, persistent=False)

    def init_weights(self, device):
        std = (3.0 / self.config.dim) ** 0.5
        torch.nn.init.normal_(self.tok_embeddings.weight, mean=0.0, std=0.02)
        torch.nn.init.normal_(self.lm_head.weight, mean=0.0, std=0.001)
        for layer in self.layers:
            for name, param in layer.named_parameters():
                if "proj" in name or "down_proj" in name:
                    torch.nn.init.zeros_(param)
                elif hasattr(param, 'data') and param.dim() >= 2:
                    torch.nn.init.normal_(param, mean=0.0, std=std)
            if hasattr(layer.moe, 'gate'):
                torch.nn.init.normal_(layer.moe.gate.weight, mean=0.0, std=0.02)
                torch.nn.init.zeros_(layer.moe.gate.bias)
        self.resid_lambdas.fill_(1.0)
        for i in range(self.config.n_layers):
            self.x0_lambdas.data[i] = 0.2 - 0.15 * i / max(self.config.n_layers - 1, 1)
        for ve in self.value_embeds.values():
            torch.nn.init.normal_(ve.weight, mean=0.0, std=std)
        for layer in self.layers:
            if hasattr(layer.attn, 've_gate') and layer.attn.ve_gate is not None:
                torch.nn.init.zeros_(layer.attn.ve_gate.weight)
        device_cos, device_sin = precompute_rotary(
            self.config.max_seq_len * 4, self.config.rope_dim, self.config.rope_base, device
        )
        self.cos, self.sin = device_cos, device_sin

    def setup_optimizer(self):
        dmodel_lr_scale = (self.config.dim / 2048) ** -0.5
        return torch.optim.AdamW([
            {"params": self.tok_embeddings.parameters(),
             "lr": HYPERPARAMS["embedding_lr"] * dmodel_lr_scale, "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": 0.001},
            {"params": self.lm_head.parameters(),
             "lr": HYPERPARAMS["unembedding_lr"] * dmodel_lr_scale, "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": 0.0},
            {"params": self.layers.parameters(),
             "lr": HYPERPARAMS["matrix_lr"] * dmodel_lr_scale, "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": HYPERPARAMS["weight_decay"]},
            {"params": [self.resid_lambdas, self.x0_lambdas],
             "lr": HYPERPARAMS["scalar_lr"], "betas": (0.96, 0.95), "eps": 1e-10},
            {"params": self.value_embeds.parameters(),
             "lr": HYPERPARAMS["embedding_lr"] * dmodel_lr_scale * 0.5, "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": 0.01},
        ])

    def forward(self, idx, targets=None):
        B, T = idx.size()
        assert T <= self.cos.size(1), f"seq too long: {T} > {self.cos.size(1)}"

        cos_sin = self.cos[:, :T], self.sin[:, :T]
        x = self.tok_embeddings(idx)
        x = rms_norm(x)
        x0 = x
        for i, layer in enumerate(self.layers):
            x = self.resid_lambdas[i] * x + self.x0_lambdas[i] * x0
            ve = self.value_embeds[str(i)](idx) if str(i) in self.value_embeds else None
            x = layer(x, ve, cos_sin)
        x = self.norm(x)

        logits = self.lm_head(x).float()
        logits = self.config.logit_softcap * torch.tanh(logits / self.config.logit_softcap)

        if targets is not None:
            return F.cross_entropy(logits.view(-1, logits.size(-1)), targets.view(-1), ignore_index=-1)
        return logits

# ===========================================================================
# DATA
# ===========================================================================

def get_training_text(exp_dir):
    data_path = exp_dir / "sft_data.json"
    if data_path.exists():
        samples = json.loads(data_path.read_text())
        texts = [s.get("output", s.get("instruction", "")) for s in samples[:500]]
        return "\n\n".join(t for t in texts if t)
    return "The quick brown fox jumps over the lazy dog. " * 5000

class CharTokenizer:
    def __init__(self, text):
        chars = sorted(set(text))
        self.stoi = {ch: i for i, ch in enumerate(chars)}
        self.itos = {i: ch for i, ch in enumerate(chars)}
    def encode(self, s):
        return torch.tensor([self.stoi.get(c, 0) for c in s], dtype=torch.long)
    def decode(self, ids):
        return "".join(self.itos.get(i.item(), "?") for i in ids)

def make_stream(text, tokenizer, batch_size, seq_len, device):
    data = tokenizer.encode(text)
    n = max(1, len(data) - seq_len - 1)
    while True:
        starts = torch.randint(0, n - 1, (batch_size,))
        x = torch.stack([data[s:s + seq_len] for s in starts]).to(device)
        y = torch.stack([data[s + 1:s + 1 + seq_len] for s in starts]).to(device)
        yield x, y

@torch.no_grad()
def evaluate(model, text, tokenizer, batch_size, seq_len, device, steps):
    model.eval()
    loader = make_stream(text, tokenizer, batch_size, seq_len, device)
    total_loss = 0.0
    total_tokens = 0
    autocast = torch.amp.autocast(device_type=device.type, dtype=torch.bfloat16) if device.type == "cuda" else torch.no_grad()
    for _, (x, y) in zip(range(steps), loader):
        with autocast:
            loss = model(x, y)
        total_loss += loss.item() * x.numel()
        total_tokens += x.numel()
    model.train()
    return total_loss / total_tokens if total_tokens > 0 else float("inf")

# ===========================================================================
# MAIN — fixed training loop
# ===========================================================================

def main():
    exp_id = None
    i = 1
    while i < len(sys.argv):
        if sys.argv[i] == "--exp-id" and i + 1 < len(sys.argv):
            exp_id = sys.argv[i + 1]
            i += 2
        elif sys.argv[i] == "--scale" and i + 1 < len(sys.argv):
            scale = sys.argv[i + 1]
            scale_config(scale)
            i += 2
        else:
            i += 1

    exp_dir = Path.home() / ".egonetics" / "experiments" / (exp_id or "default")
    exp_dir.mkdir(parents=True, exist_ok=True)

    torch.manual_seed(SEED)
    if torch.cuda.is_available():
        device = torch.device("cuda")
    elif torch.backends.mps.is_available():
        device = torch.device("mps")
    else:
        device = torch.device("cpu")
    dtype = torch.bfloat16 if device.type == "cuda" else torch.float32
    autocast_ctx = torch.amp.autocast(device_type=device.type, dtype=dtype) if device.type == "cuda" else torch.no_grad()

    text = get_training_text(exp_dir)
    tokenizer = CharTokenizer(text)
    config.vocab_size = tokenizer.stoi.__len__()

    global config
    model = MiniDeepSeekV4(config).to(device)
    model.init_weights(device)

    n_params = count_params(model)
    print(json.dumps({"phase": "init", "params": n_params, "device": str(device)}))

    train_batch = min(HYPERPARAMS["device_batch_size"], 4 if device.type != "cuda" else HYPERPARAMS["device_batch_size"])
    seq_len = min(config.max_seq_len, 256 if device.type != "cuda" else config.max_seq_len)
    grad_accum = max(1, HYPERPARAMS["total_batch_size"] // (train_batch * seq_len))

    optimizer = model.setup_optimizer()
    loader = make_stream(text, tokenizer, train_batch, seq_len, device)

    t_start = time.time()
    total_training_s = 0.0
    smooth_loss = 0.0
    step = 0

    while total_training_s < TIME_BUDGET:
        if device.type == "cuda":
            torch.cuda.synchronize()
        t0 = time.time()

        for _ in range(grad_accum):
            x, y = next(loader)
            with autocast_ctx:
                loss = model(x, y)
            (loss / grad_accum).backward()

        progress = min(total_training_s / TIME_BUDGET, 1.0)
        lrm = _lr_multiplier(progress)
        for group in optimizer.param_groups:
            initial_lr = group.get("initial_lr", group["lr"])
            group["lr"] = initial_lr * lrm
        optimizer.step()
        optimizer.zero_grad(set_to_none=True)

        if device.type == "cuda":
            torch.cuda.synchronize()
        dt = time.time() - t0
        step += 1
        if step > 3:
            total_training_s += dt

        train_loss = loss.item()
        if math.isnan(train_loss) or train_loss > 100:
            print(json.dumps({"ok": False, "error": "loss_exploded", "step": step}))
            sys.exit(1)

        smooth_loss = 0.9 * smooth_loss + 0.1 * train_loss
        if step % 5 == 0:
            pct = progress * 100
            tok_per_sec = int(HYPERPARAMS["total_batch_size"] / dt) if dt > 0 else 0
            print(f"\rstep {step} ({pct:.0f}%) loss: {smooth_loss:.4f} tok/s: {tok_per_sec:,} remaining: {max(0, TIME_BUDGET - total_training_s):.0f}s   ", end="", flush=True)

    print()
    val_loss = evaluate(model, text, tokenizer, train_batch, seq_len, device, EVAL_STEPS)
    total_s = time.time() - t_start

    ckpt_path = str(exp_dir / "checkpoint.pt")
    torch.save({"model": model.state_dict(), "config": asdict(config), "val_loss": val_loss}, ckpt_path)

    result = {
        "ok": True,
        "val_loss": round(val_loss, 6),
        "train_loss": round(smooth_loss, 6),
        "duration_s": round(total_s, 1),
        "training_s": round(total_training_s, 1),
        "steps": step,
        "params": n_params,
        "checkpoint": ckpt_path,
    }
    print(json.dumps(result))


def _lr_multiplier(progress):
    wu, wd = HYPERPARAMS["warmup_ratio"], HYPERPARAMS["warmdown_ratio"]
    ff = HYPERPARAMS["final_lr_frac"]
    if progress < wu:
        return progress / wu if wu > 0 else 1.0
    elif progress < 1.0 - wd:
        return 1.0
    else:
        cooldown = (1.0 - progress) / wd
        return cooldown * 1.0 + (1 - cooldown) * ff

# ===========================================================================
# SCALE PRESETS — for comparison with Qwen2.5-0.8B and smaller
# ===========================================================================

def scale_config(name: str):
    global config
    presets = {
        "tiny": ModelConfig(dim=384, n_layers=6, n_heads=6, n_kv_heads=2, head_dim=64,
                            n_routed_experts=4, n_activated_experts=2, expert_inter_dim=1536,
                            q_lora_rank=192, kv_lora_rank=128, max_seq_len=512),
        "small": ModelConfig(dim=768, n_layers=12, n_heads=12, n_kv_heads=4, head_dim=64,
                             n_routed_experts=6, n_activated_experts=2, expert_inter_dim=3072,
                             q_lora_rank=384, kv_lora_rank=256, max_seq_len=1024),
        "medium": ModelConfig(dim=1536, n_layers=16, n_heads=24, n_kv_heads=6, head_dim=64,
                              n_routed_experts=8, n_activated_experts=2, expert_inter_dim=6144,
                              q_lora_rank=768, kv_lora_rank=384, max_seq_len=2048),
        "base": ModelConfig(dim=2048, n_layers=16, n_heads=32, n_kv_heads=8, head_dim=64,
                            n_routed_experts=8, n_activated_experts=2, expert_inter_dim=8192,
                            q_lora_rank=1024, kv_lora_rank=512, max_seq_len=2048),
    }
    if name in presets:
        config = presets[name]

config = ModelConfig()

if __name__ == "__main__":
    main()
