# Senior Engineer Review: Hook System MVP
**Reviewer:** Antigravity Agent (Senior Go/Architecture Role)
**Date:** 2026-01-17
**Target:** `docs/specs_raw/hooks-mvp.md` & `specs/002-hook-system-mvp/`

## Executive Summary

The **Hook System** concept (Crash Recovery + Context Persistence) is solid and addresses a real pain point. The "Text is Truth" alignment with `HOOK.md` vs `hook.json` is excellent architecture.

However, the implementation plan suffers from **"Glarious" Over-Engineering** in the cryptography layer and **High-Risk Invasiveness** in the git integration. The security claims regarding AI forgery are also technically flawed under the current threat model.

---

## 🚨 Critical Issues ("Glarious" Mistakes)

### 1. The "Crypto Overkill" & Dependency Bloat
**Issue:** You are importing `github.com/bsv-blockchain/go-sdk` and implementing BIP44 HD Key Derivation with a custom "Coin Type" (236) just to sign a JSON file locally.
**Why it's bad:**
*   **Massive Dependency**: A blockchain SDK is a heavy dependency for a CLI tool that claims "minimal dependencies" in its Constitution.
*   **Resume-Driven Development**: You don't need Hierarchical Deterministic (HD) keys for simple receipts. A simple Ed25519 keypair per task (or one master key) is sufficient.
*   **Complexity**: Derivation paths (`m/44'/236'/0'/task/receipt`) add zero value to the user but massive complexity to debugging. If a user loses their `master.key`, their local history is unverifiable.
*   **Recommendation**: **Drop the Blockchain SDK.** Use Go's standard `crypto/ed25519`. Generate a random key for the workspace, store it in `~/.atlas/keys/workspace.pem` (chmod 600). Sign with that. It's 100x faster and standard.

### 2. Security Theatre: "Impossible for AI to Forge"
**Issue:** The spec claims: *"Signed with HD-derived keys - impossible for AI to forge"*.
**Why it's bad:**
*   **False Security Model**: The AI agent (e.g., Claude) runs with the user's shell permissions. It can read files. If it can read `~/.atlas/keys/master.key`, it **can** derive the keys and forge the signature.
*   **Reality Check**: Unless the key is protected by a passphrase (which blocks automation) or stored in a hardware token (YubiKey), the AI *technically* has access to it.
*   **Recommendation**: Soften the claim. It prevents *hallucination* (accidental forgery), but it does not prevent *malicious* forgery if the AI goes rogue. Don't sell it as "Impossible".

### 3. Invasive Git Hook "Wrappers"
**Issue:** `FR-006a: System MUST install git hooks using a wrapper approach`.
**Why it's bad:**
*   **High Risk**: Touching `.git/hooks/*` programmatically is the fastest way to make developers hate your tool. If your wrapper has a bug, you break their ability to commit.
*   **Conflict Hell**: Users might use `husky`, `pre-commit`, or other hook managers. A custom Go-based wrapper leads to "who owns this file?" wars.
*   **Recommendation**:
    *   **MVP**: **Do NOT touch git hooks.** Rely on `atlas checkpoint` commands run manually or by the AI.
    *   **Future**: If you must, provide a `atlas install-hooks` command that *prints* the snippet to add, rather than auto-magically overwriting files.

---

## ⚠️ Major Concerns

### 4. File I/O & Locking
**Issue:** `IntervalCheckpointer` runs every 5 minutes.
*   **Risk**: If the AI is mid-write on a large file when the interval ticks, or if `flock` hangs, the CLI feels sluggish.
*   **Fix**: Ensure `hook.json` writes are strictly atomic (write temp -> rename) and the lock timeout is extremely short (fail fast rather than block).

### 5. "Files Touched" Ambiguity
**Issue:** `StepContext` tracks `FilesTouched`.
*   **Question**: How? Does ATLAS parse the AI's output? Or check `git status`?
*   **Risk**: If it checks `git status`, it captures *user* edits too, which confuses recovery ("I didn't touch that file, why does the AI think it did?").
*   **Fix**: Be explicit in the spec about the source of this data. If it's from `git status`, rename it to `WorkspaceChanges`.

---

## ♻️ Refactoring Recommendations

1.  **Simplify Crypto**:
    *   Delete `internal/crypto/hd`.
    *   Use `crypto/ed25519`.
    *   Store a simple keypair.

2.  **Simplify Storage**:
    *   Keep `hook.json` and `HOOK.md`. This pattern is excellent.

3.  **Defer Git Hooks**:
    *   Move "Auto-checkpoint on commit" to 'Post-MVP' or make it a "nice to have" integration usage instruction ("Add this line to your post-commit...").

## Final Grade: B-
*   **Concept**: A+ (Solving a real problem)
*   **Design**: A (Text-is-Truth is great)
*   **Implementation Plan**: C (Over-engineered crypto, invasive hooks)

**Verdict**: Proceed with the feature, but **burn the current crypto implementation** and write a simple one.
