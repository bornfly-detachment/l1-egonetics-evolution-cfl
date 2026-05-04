import os, sys, math, time, json, gc
from pathlib import Path
from dataclasses import dataclass

import torch
import torch.nn as nn
import torch.nn.functional as F

# ===========================================================================
# AGENT-EDITABLE SECTION — model architecture + hyperparameters
# ===========================================================================

@dataclass
class ModelConfig:
    sequence_len: int = 1024
    vocab_size: int = 32000
    n_layer: int = 8
    n_head: int = 8
    n_kv_head: int = 4
    n_embd: int = 512
    head_dim: int = 64
    mlp_ratio: int = 4
    activation: str = "relu2"
    window_pattern: str = "L"
    use_value_embedding: bool = True
    use_qk_norm: bool = True
    rope_base: float = 10000.0
    logit_softcap: float = 15.0

HYPERPARAMS = {
    "total_batch_size": 2**16,
    "device_batch_size": 32,
    "embedding_lr": 0.3,
    "unembedding_lr": 0.004,
    "matrix_lr": 0.02,
    "scalar_lr": 0.2,
    "weight_decay": 0.1,
    "adam_betas": (0.8, 0.95),
    "warmup_ratio": 0.05,
    "warmdown_ratio": 0.3,
    "final_lr_frac": 0.05,
}

TIME_BUDGET = 300
MAX_SEQ_LEN = 1024
EVAL_TOKENS = 2_000_000
SEED = 42

# ===========================================================================
# MODEL DEFINITION — Agent may modify architecture classes below
# ===========================================================================

def norm(x):
    return F.rms_norm(x, (x.size(-1),))

def apply_rotary_emb(x, cos, sin):
    d = x.shape[3] // 2
    x1, x2 = x[..., :d], x[..., d:]
    y1 = x1 * cos + x2 * sin
    y2 = x1 * (-sin) + x2 * cos
    return torch.cat([y1, y2], 3)

def precompute_rotary(seq_len, head_dim, base, device):
    channel_range = torch.arange(0, head_dim, 2, dtype=torch.float32, device=device)
    inv_freq = 1.0 / (base ** (channel_range / head_dim))
    t = torch.arange(seq_len, dtype=torch.float32, device=device)
    freqs = torch.outer(t, inv_freq)
    cos = freqs.cos().bfloat16()[None, :, None, :]
    sin = freqs.sin().bfloat16()[None, :, None, :]
    return cos, sin

class CausalSelfAttention(nn.Module):
    def __init__(self, config, layer_idx):
        super().__init__()
        self.n_head = config.n_head
        self.n_kv_head = config.n_kv_head
        self.n_embd = config.n_embd
        self.head_dim = config.head_dim
        self.q_lora_rank = config.n_embd // 2

        self.wq_a = nn.Linear(self.n_embd, self.q_lora_rank, bias=False)
        self.q_norm = nn.RMSNorm(self.q_lora_rank, 1e-6)
        self.wq_b = nn.Linear(self.q_lora_rank, self.n_head * self.head_dim, bias=False)
        self.wk = nn.Linear(self.n_embd, self.n_kv_head * self.head_dim, bias=False)
        self.wv = nn.Linear(self.n_embd, self.n_kv_head * self.head_dim, bias=False)
        self.wo = nn.Linear(self.n_head * self.head_dim, self.n_embd, bias=False)

        if config.use_value_embedding and layer_idx % 2 == (config.n_layer - 1) % 2:
            self.ve_gate = nn.Linear(32, self.n_kv_head, bias=False)
        else:
            self.ve_gate = None

        self.softmax_scale = self.head_dim ** -0.5

    def forward(self, x, ve, cos_sin, mask=None):
        B, T, C = x.size()
        qr = self.q_norm(self.wq_a(x))
        q = self.wq_b(qr).view(B, T, self.n_head, self.head_dim)
        k = self.wk(x).view(B, T, self.n_kv_head, self.head_dim)
        v = self.wv(x).view(B, T, self.n_kv_head, self.head_dim)

        if ve is not None and self.ve_gate is not None:
            ve = ve.view(B, T, self.n_kv_head, self.head_dim)
            gate = 2 * torch.sigmoid(self.ve_gate(x[..., :32]))
            v = v + gate.unsqueeze(-1) * ve

        cos, sin = cos_sin
        q_rope, k_rope = q[..., :32], k[..., :32] if self.head_dim > 32 else (q, k)
        q_rope = apply_rotary_emb(q_rope, cos, sin)
        k_rope = apply_rotary_emb(k_rope, cos, sin)
        if self.head_dim > 32:
            q = torch.cat([q_rope, q[..., 32:]], dim=-1)
            k = torch.cat([k_rope, k[..., 32:]], dim=-1)
        else:
            q, k = q_rope, k_rope

        if config.use_qk_norm:
            q, k = norm(q), norm(k)

        if self.n_kv_head < self.n_head:
            g = self.n_head // self.n_kv_head
            k = k.repeat_interleave(g, dim=2)
            v = v.repeat_interleave(g, dim=2)

        q = q.transpose(1, 2)
        k = k.transpose(1, 2)
        v = v.transpose(1, 2)

        y = F.scaled_dot_product_attention(q, k, v, attn_mask=mask, is_causal=(mask is None))
        y = y.transpose(1, 2).contiguous().view(B, T, -1)
        return self.wo(y)

class MLP(nn.Module):
    def __init__(self, config):
        super().__init__()
        hidden_dim = config.mlp_ratio * config.n_embd
        self.c_fc = nn.Linear(config.n_embd, hidden_dim, bias=False)
        self.c_gate = nn.Linear(config.n_embd, hidden_dim, bias=False) if config.activation == "swiglu" else None
        self.c_proj = nn.Linear(hidden_dim, config.n_embd, bias=False)
        self.act = config.activation

    def forward(self, x):
        if self.act == "swiglu":
            return self.c_proj(F.silu(self.c_gate(x)) * self.c_fc(x))
        elif self.act == "gelu":
            return self.c_proj(F.gelu(self.c_fc(x)))
        else:
            return self.c_proj(F.relu(self.c_fc(x)).square())

class Block(nn.Module):
    def __init__(self, config, layer_idx):
        super().__init__()
        self.attn = CausalSelfAttention(config, layer_idx)
        self.mlp = MLP(config)

    def forward(self, x, ve, cos_sin):
        x = x + self.attn(norm(x), ve, cos_sin)
        x = x + self.mlp(norm(x))
        return x

class GPT(nn.Module):
    def __init__(self, config):
        super().__init__()
        self.config = config
        self.wte = nn.Embedding(config.vocab_size, config.n_embd)
        self.blocks = nn.ModuleList([Block(config, i) for i in range(config.n_layer)])
        self.lm_head = nn.Linear(config.n_embd, config.vocab_size, bias=False)
        self.resid_lambdas = nn.Parameter(torch.ones(config.n_layer))
        self.x0_lambdas = nn.Parameter(torch.zeros(config.n_layer))

        if config.use_value_embedding:
            kv_dim = config.n_kv_head * config.head_dim
            self.value_embeds = nn.ModuleDict({
                str(i): nn.Embedding(config.vocab_size, kv_dim)
                for i in range(config.n_layer)
                if i % 2 == (config.n_layer - 1) % 2
            })
        else:
            self.value_embeds = nn.ModuleDict()

        cos, sin = precompute_rotary(config.sequence_len * 10, config.head_dim, config.rope_base, torch.device("cpu"))
        self.register_buffer("cos", cos, persistent=False)
        self.register_buffer("sin", sin, persistent=False)

    def init_weights(self, device):
        std = 3**0.5 * self.config.n_embd**-0.5
        torch.nn.init.uniform_(self.wte.weight, -std, std)
        torch.nn.init.normal_(self.lm_head.weight, mean=0.0, std=0.001)
        for block in self.blocks:
            torch.nn.init.uniform_(block.attn.wq_a.weight, -std, std)
            torch.nn.init.uniform_(block.attn.wq_b.weight, -std, std)
            torch.nn.init.uniform_(block.attn.wk.weight, -std, std)
            torch.nn.init.uniform_(block.attn.wv.weight, -std, std)
            torch.nn.init.zeros_(block.attn.wo.weight)
            torch.nn.init.uniform_(block.mlp.c_fc.weight, -std, std)
            torch.nn.init.zeros_(block.mlp.c_proj.weight)
            if block.mlp.c_gate is not None:
                torch.nn.init.uniform_(block.mlp.c_gate.weight, -std, std)
            if block.attn.ve_gate is not None:
                torch.nn.init.zeros_(block.attn.ve_gate.weight)
        self.resid_lambdas.fill_(1.0)
        self.x0_lambdas.fill_(0.1)
        for ve in self.value_embeds.values():
            torch.nn.init.uniform_(ve.weight, -std, std)

        device_cos, device_sin = precompute_rotary(
            self.config.sequence_len * 10, self.config.head_dim, self.config.rope_base, device
        )
        self.cos, self.sin = device_cos, device_sin

    def forward(self, idx, targets=None):
        B, T = idx.size()
        cos_sin = self.cos[:, :T], self.sin[:, :T]
        x = self.wte(idx)
        x = norm(x)
        x0 = x
        for i, block in enumerate(self.blocks):
            x = self.resid_lambdas[i] * x + self.x0_lambdas[i] * x0
            ve = self.value_embeds[str(i)](idx) if str(i) in self.value_embeds else None
            x = block(x, ve, cos_sin)
        x = norm(x)
        logits = self.lm_head(x).float()
        softcap = self.config.logit_softcap
        logits = softcap * torch.tanh(logits / softcap)
        if targets is not None:
            return F.cross_entropy(logits.view(-1, logits.size(-1)), targets.view(-1), ignore_index=-1)
        return logits

    def setup_optimizer(self):
        mcfg = self.config
        dmodel_lr_scale = (mcfg.n_embd / 768) ** -0.5
        return torch.optim.AdamW([
            {"params": self.wte.parameters(), "lr": HYPERPARAMS["embedding_lr"] * dmodel_lr_scale,
             "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10},
            {"params": self.lm_head.parameters(), "lr": HYPERPARAMS["unembedding_lr"] * dmodel_lr_scale,
             "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": 0.0},
            {"params": self.blocks.parameters(), "lr": HYPERPARAMS["matrix_lr"] * dmodel_lr_scale,
             "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10, "weight_decay": HYPERPARAMS["weight_decay"]},
            {"params": [self.resid_lambdas, self.x0_lambdas], "lr": HYPERPARAMS["scalar_lr"],
             "betas": (0.96, 0.95), "eps": 1e-10},
            {"params": self.value_embeds.parameters(), "lr": HYPERPARAMS["embedding_lr"] * dmodel_lr_scale,
             "betas": HYPERPARAMS["adam_betas"], "eps": 1e-10},
        ])

# ===========================================================================
# DATA — uses SEAI for data generation; fallback to synthetic
# ===========================================================================

def get_training_data(exp_dir, config):
    data_path = exp_dir / "sft_data.json"
    if data_path.exists():
        import json as _json
        samples = _json.loads(data_path.read_text())
        texts = [s.get("output", s.get("instruction", "")) for s in samples[:500]]
        text = "\n\n".join(t for t in texts if t)
        if len(text) > 1000:
            return text
    return _synthetic_data()

def _synthetic_data():
    return "The quick brown fox jumps over the lazy dog. " * 5000

class SimpleTokenizer:
    def __init__(self, text, vocab_size):
        chars = sorted(set(text))
        self.stoi = {ch: i for i, ch in enumerate(chars)}
        self.itos = {i: ch for i, ch in enumerate(chars)}
        self.vocab_size = len(chars)

    def encode(self, s):
        return torch.tensor([self.stoi.get(c, 0) for c in s], dtype=torch.long)

    def decode(self, ids):
        return "".join(self.itos.get(i.item(), "?") for i in ids)

def make_dataloader(tokenizer, text, batch_size, seq_len, device):
    data = tokenizer.encode(text)
    n = len(data) - seq_len - 1
    idx = torch.randint(0, n, (1,)).item()
    while True:
        start = torch.randint(0, n, (1,)).item() if idx >= n - batch_size * seq_len else idx
        x = data[start:start + batch_size * seq_len].view(batch_size, seq_len).to(device)
        y = data[start + 1:start + 1 + batch_size * seq_len].view(batch_size, seq_len).to(device)
        idx = start + batch_size * seq_len
        yield x, y

@torch.no_grad()
def evaluate(model, tokenizer, text, config, device, max_tokens):
    model.eval()
    loader = make_dataloader(tokenizer, text, config["device_batch_size"], config["seq_len"], device)
    total_loss = 0.0
    total_tokens = 0
    autocast = torch.amp.autocast(device_type=device.type, dtype=torch.bfloat16) if device.type == "cuda" else torch.no_grad()
    with autocast:
        for x, y in loader:
            loss = model(x, y)
            total_loss += loss.item() * x.numel()
            total_tokens += x.numel()
            if total_tokens >= max_tokens:
                break
    model.train()
    return total_loss / total_tokens if total_tokens > 0 else float("inf")

# ===========================================================================
# MAIN — fixed training loop (AGENT MUST NOT EDIT BELOW)
# ===========================================================================

def main():
    exp_id = None
    i = 1
    while i < len(sys.argv):
        if sys.argv[i] == "--exp-id" and i + 1 < len(sys.argv):
            exp_id = sys.argv[i + 1]
            i += 2
        else:
            i += 1

    exp_dir = Path.home() / ".egonetics" / "experiments" / (exp_id or "default")
    exp_dir.mkdir(parents=True, exist_ok=True)

    torch.manual_seed(SEED)
    device = torch.device("cuda" if torch.cuda.is_available() else "mps" if torch.backends.mps.is_available() else "cpu")
    dtype = torch.bfloat16 if device.type == "cuda" else torch.float32
    autocast_ctx = torch.amp.autocast(device_type=device.type, dtype=dtype) if device.type == "cuda" else torch.no_grad()

    config = ModelConfig()
    raw_text = get_training_data(exp_dir, config)
    tokenizer = SimpleTokenizer(raw_text, config.vocab_size)
    config.vocab_size = tokenizer.vocab_size

    print(json.dumps({"phase": "init", "vocab_size": config.vocab_size, "device": str(device), "params": sum(p.numel() for p in GPT(config).parameters())}))

    train_batch = min(HYPERPARAMS["device_batch_size"], 16) if device.type != "cuda" else HYPERPARAMS["device_batch_size"]
    seq_len = min(MAX_SEQ_LEN, config.sequence_len)
    grad_accum = max(1, HYPERPARAMS["total_batch_size"] // (train_batch * seq_len))

    model = GPT(config).to(device)
    model.init_weights(device)
    optimizer = model.setup_optimizer()

    train_loader = make_dataloader(tokenizer, raw_text, train_batch, seq_len, device)
    eval_config = {"device_batch_size": train_batch, "seq_len": seq_len}

    t_start = time.time()
    total_training_s = 0.0
    smooth_loss = 0.0
    step = 0

    while total_training_s < TIME_BUDGET:
        torch.cuda.synchronize() if device.type == "cuda" else None
        t0 = time.time()

        for _ in range(grad_accum):
            x, y = next(train_loader)
            with autocast_ctx:
                loss = model(x, y)
            (loss / grad_accum).backward()

        progress = min(total_training_s / TIME_BUDGET, 1.0)
        lrm = _lr_multiplier(progress)
        for group in optimizer.param_groups:
            group["lr"] = group.get("initial_lr", group["lr"] * (1.0 if progress < 0.01 else 1.0)) * lrm
        optimizer.step()
        optimizer.zero_grad(set_to_none=True)

        torch.cuda.synchronize() if device.type == "cuda" else None
        dt = time.time() - t0
        step += 1
        if step > 3:
            total_training_s += dt

        train_loss = loss.item()
        if math.isnan(train_loss) or train_loss > 100:
            print(json.dumps({"ok": False, "error": "loss_exploded", "val_loss": None, "duration_s": total_training_s, "steps": step}))
            sys.exit(1)

        smooth_loss = 0.9 * smooth_loss + 0.1 * train_loss
        if step % 10 == 0:
            pct = progress * 100
            tokens_per_sec = int(HYPERPARAMS["total_batch_size"] / dt) if dt > 0 else 0
            print(f"\rstep {step} ({pct:.0f}%) loss: {smooth_loss:.4f} tok/s: {tokens_per_sec:,} remaining: {max(0, TIME_BUDGET - total_training_s):.0f}s   ", end="", flush=True)

    print()

    val_loss = evaluate(model, tokenizer, raw_text, eval_config, device, EVAL_TOKENS)
    total_s = time.time() - t_start

    result = {
        "ok": True,
        "val_loss": round(val_loss, 6),
        "train_loss": round(smooth_loss, 6),
        "duration_s": round(total_s, 1),
        "training_s": round(total_training_s, 1),
        "samples": 0,
        "checkpoint": str(exp_dir / "checkpoint.pt"),
    }

    torch.save({"model": model.state_dict(), "config": config, "val_loss": val_loss}, result["checkpoint"])
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


if __name__ == "__main__":
    main()
