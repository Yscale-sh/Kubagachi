# critterforge — cost & the image-gen knobs

The only real cost in the critter pipeline is the **Gemini image API**; everything
else (Go, the keyer, downscaling) is local. This doc records the current settings
and what *actually* drives the bill, measured from the API's own token usage.

## Current settings (as shipped)

- **Model:** `gemini-3-pro-image-preview` (Nano Banana Pro) for both the keyed
  sheet and the animation decks (`pkg/critterforge/gemini.go`).
- **`--quality`** maps `low|medium|high` → `imageConfig.imageSize` `1K|2K|4K`. The
  default is **`medium` (2K)** (changed from `high`/4K — see below).
- **Per critter:** 1 keyed sheet + 8 animation decks = **9 images** (nori-style
  workload critters have a few extra decks).
- **`maxOutputTokens: 32768`** — bounds the pro model's "thinking" pass so a dense
  prompt still reaches the image instead of finishing `MAX_TOKENS`.
- Every sheet is **downscaled to the ship size 1024×128 (128px/frame)** before
  commit — 4K masters are never shipped.
- Key: `GEMINI_API_KEY`. The cockpit shell exports a `dev-` placeholder that
  shadows the real `.env` key — force it: `KEY=$(grep -m1 '^GEMINI_API_KEY=' .env
  | cut -d= -f2- | tr -d '"'\'' \r')`.

## What actually drives the cost (measured)

Every call returns `usageMetadata`. Output tokens split into **image tokens** and
**thinking ("thoughts") tokens**. Measured on a *simple* one-mascot prompt:

| setting | image tokens | thinking | notes |
|---|---|---|---|
| pro **2K** | **1120** | ~150 | |
| pro **4K** | **2000** | ~50 | ~2× the image tokens of 2K |
| flash-image 2K | 1290 | **0** | no thinking pass at all |

Two **independent** knobs:

1. **Resolution (`imageSize`)** sets the *image* token count — 4K ≈ **2×** 2K.
   Because we downscale everything to 128px/frame, 4K is pure waste, so **2K is a
   ~44% cut on the image slice, for free**.
2. **Thinking** is the pro model's interleaved reasoning. On a simple prompt it's
   ~150 tokens; on our *real* dense prompts (two reference images + all 8 states)
   it balloons to **~16k tokens** — that is what tripped `MAX_TOKENS` and forced
   the `maxOutputTokens` bump. Resolution does **not** touch it. **`flash-image`
   does zero thinking**, so on the real prompts it avoids the ~16k-token thinking
   tax entirely, at comparable image tokens.

**Quality ≠ resolution.** Render fidelity is the *model tier* (pro vs flash);
resolution only changes pixel count (and thus image tokens). Since we downscale,
resolution is free to cut without touching quality.

## The knobs, by impact

| lever | how | saving | risk |
|---|---|---|---|
| **2K default** | `--quality medium` | ~44% of image tokens on every call | none — downscaled anyway. **Done.** |
| **flash-image for decks** | `--model gemini-2.5-flash-image` on `spriteanim` | drops the ~16k-token thinking pass per deck (×8 per critter) | different model → verify identity/quality with an A/B |
| **tiered deck count** | fewer decks for background-pool critters | ~half the deck calls for most of the fleet | less animation richness |
| **fewer rerolls** | grid-aware keyer | removes wasted regens | none. **Shipped.** |

The **keyed sheet stays on `pro`** — it's identity-critical and reference-anchored.
The **decks animate an already-locked identity**, so they are the candidate for the
cheaper `flash-image` tier; that swap is the biggest remaining lever, pending an
identity/quality A/B.

## Example — decks are subject-tiered (Jake the J-dragon)

The **same `running` deck** for the Jake J-dragon, generated three ways and
downscaled to the shipped 1024×128 (both versions kept under `docs/examples/`;
side-by-side in `docs/screenshots/critterforge-quality-jake.png`):

| gen | file | result |
|---|---|---|
| **pro / high (4K)** | `jake-running-deck-pro-high-4k-1024x128.png` | clean, consistent detailed dragon across all 8 frames |
| pro / 2K | `jake-running-deck-pro-2k-1024x128.png` | the dragon scale-jumps and half-vanishes frame to frame |
| flash-image / 2K | `jake-running-deck-flash-2k-1024x128.png` | consistent size but simpler, and it drifts to a chunkier dragon |

The lesson: a **thin, detailed, serpentine subject needs pro at 4K to animate
cleanly** — drop to 2K or flash and it falls apart. So the cost levers are
**subject-tiered, not blanket:**

- The **2K keyed sheet is safe for any critter** (identity is a single reference-
  anchored shot) — keep it the default.
- **Decks** are where the model re-draws the shape 8×: use **flash/2K only for
  chunky, high-contrast critters** (elephant, red panda) and **pro/high for
  intricate or hero critters** (the dragon). Don't blanket flash across the deck
  stage — validate per critter.

(Note: the flat `--quality medium` default still applies; complex/hero critters
just pass `--quality high` and keep the pro model for `spriteanim`.)

## Recipe reminders

Force the `.env` key · `medium` (2K) is the default · reroll only a deck whose
width isn't ÷8 · downscale all 9 sheets to 1024×128 · commit sprites **and**
`critters/manifest.json`. Full pipeline: [critterforge-pipeline.md](critterforge-pipeline.md);
mascot method + reference gen: [mascot-references.md](mascot-references.md).
