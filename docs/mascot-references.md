# Mascot references — brand knowledge + generating refs with Antigravity

Every premium kubagachi critter is anchored to a **reference illustration** in
`refs/` (Slonik the elephant, the cartographer gopher, the window crab, the Y
phoenix). The reference locks the character; [critterforge](critterforge-pipeline.md)
then pixel-art-ifies it into the 8-mood keyed sheet and animation decks. So the
whole critter lives or dies on the reference — and a great reference comes from
**brand knowledge**, not "database → elephant."

This doc is the method for deducing the mascot and generating its reference image
with **Antigravity** (Google's Gemini image model — the same model critterforge
uses). For the full end-to-end *app → critter* workflow (cartogopher deduces the
mascot from a codebase), use the `mascot-cast` skill; this doc is the reference
image half.

> **Ask the user first — before getting into the weeds.** They usually already
> have the mascot in their head (yscale→phoenix and litewindow→crab were the user's
> own calls). Ask up front: any mascot / animal / vibe in mind? anything that MUST
> be in it (a prop, pose, pun) or must NOT? If they hand you a concept, your job is
> to **refine and render** it — sharpen with the layers/matrix below, don't override
> it. Only invent from scratch when they have nothing.
>
> **Gather branding first — ask if it exists.** If the app already has brand
> colors, a logo, or an existing mascot, USE them — a mascot in the wrong palette
> reads as a *different product*. Check the repo (logo / favicon, the theme or
> CSS-variable config, a brand doc, README badges); when it isn't obvious, just ask
> ("does this app have brand colors or a logo to match?"). Invent a palette only
> when there is genuinely no branding.

## Brand knowledge — reason in layers

Great mascots are **visual puns that stack layers of brand meaning**, so a
stranger reads the workload straight off the critter. Read for all five layers,
anchor the silhouette on the strongest, and hang the rest as props / palette /
expression. Landing 3 layers at once is a keeper.

1. **Letter / logo silhouette** — the pose or body echoes the app's initial or
   mark. *yscale* → a phoenix whose wings-up, tail-down flight forms a rough **Y**.
   *kali* → a dragon coiled into a **K**. The most memorable layer when it lands.
2. **Language / tech lineage** — honor the implementation's own mascot. *Go* → the
   **gopher**. *Rust* → **Ferris the crab**. A Go service that isn't a gopher
   wastes a free signal.
3. **Name pun / wordplay** — mine the name literally. *cartogopher* =
   *cartographer* + *gopher* → a gopher **cartographer** (it maps codebases) in a
   **pirate** costume (treasure map, sextant). The name hands you the outfit.
4. **Product metaphor** — make what it IS literal. *litewindow* = a **window** →
   four squares form a window pane on the shell; *thin client* / *context window*
   → a Rust **crab** carrying that lit window. The concept becomes a prop.
5. **Domain / vibe** — the emotional theme. Security → fierce dragon (kali). A
   database → sturdy, unbothered. A cache → quick, twitchy.

**The fleet, decoded — study before casting:**

| critter | letter/shape | language | name pun | product metaphor | → the cast |
|---|---|---|---|---|---|
| **yscale phoenix** | **Y** (wings up / tail down) | — | — | rise · burst · scale from ashes | Y-silhouette firebird, ember palette |
| **cartogopher** | — | Go → gopher | *cartographer*+*gopher* | maps the codebase | teal gopher cartographer-pirate |
| **litewindow** | — | Rust → crab (Ferris) | — | *window* pane · *context window* | rust crab, lit 4-square window on its shell |
| **postgres** | — | — | — | Slonik, the official elephant | chubby blue elephant |
| **redis** | — | — | (the red brand) | in-memory, fast | red panda, ringed tail |
| **kali** (example) | **K** (coiled dragon) | — | — | offensive-security ferocity | dragon posed as a K |

**Tech lexicon (layer 2 — honor these):** Go→gopher · Rust→crab (Ferris) ·
Python→snake · Java→coffee/Duke · Docker→whale · Kubernetes→helm/wheel ·
PostgreSQL→elephant (Slonik) · MySQL→dolphin · MongoDB→leaf · Linux→penguin ·
Kali→dragon · GitHub→octocat. When the core tech is here, the SPECIES is decided
— spend the creativity on the letter-shape, name-pun, and product-metaphor layers.

## The casting matrix (adaptive) — fill it to reason

Don't pick a mascot from a fixed lookup. **Fill this matrix for the specific app,
then read the synthesis.** It's adaptive: signal *strength* decides which layer
drives the silhouette and which become accents — so a strong letter-shape or a
canonical language mascot outranks a generic role guess.

### Step A — score each signal `0` absent · `1` weak · `2` strong · `3` dominant

| Layer | What to look for | Evidence (fill in) | Strength | Points to |
|---|---|---|---|---|
| Letter / logo shape | an initial or logo the body could form | | | |
| Language lineage | the impl language's canonical mascot | | | |
| Name pun | wordplay hidden in the name | | | |
| Product metaphor | what it literally IS, as an object | | | |
| Domain vibe | the emotional theme | | | |
| Role archetype (fallback) | role→animal — only when all above ≤1 | | | |

### Step B — adaptive synthesis (weight by strength)

1. **Anchor** = the highest-strength row → sets the SPECIES + SILHOUETTE. Ties →
   pick the one the app's own team recognizes fastest.
2. **Accents** = every other row with strength ≥1 → become props, palette, pose,
   or expression (never a second animal).
3. **Palette** = brand color if present, else the domain mood.
4. **Confidence read:** top strength `3` → iconic, commit hard (kali K-dragon,
   yscale Y-phoenix). Top `2` + 1–2 accents → strong, the norm. All ≤`1` → weak
   signal; fall back to Role archetype + one small accent, keep it simple.

### Step C — gate & loop (the adaptive part)

Run the fleet-fit gate. If ANY criterion fails, don't ship — **adapt and re-score:**
- unreadable silhouette at 32px → re-anchor on a simpler layer;
- can't express all 8 states → add a face / big eyes, drop a busy prop;
- collides with an existing critter → demote that layer, re-anchor on the next-strongest.

Loop until the gate passes. That feedback loop is what makes it *adaptive* rather
than a static table.

### Worked example — litewindow

| Layer | Evidence | Strength | Points to |
|---|---|---|---|
| Letter / shape | no strong "L" pose | 0 | — |
| Language lineage | written in Rust | 3 | crab (Ferris) |
| Name pun | "lite" + "window" — a trait, not a species | 1 | (accent) |
| Product metaphor | a *window* / thin client / *context window* | 3 | a window pane |
| Domain vibe | calm, always-on cockpit | 1 | friendly |

→ **Anchor:** Rust **crab** (lineage, 3). **Accents:** a lit 4-square **window
pane** on the shell (metaphor, 3), warm console-glyph energy (vibe). **Palette:**
rust-orange. **Result:** the mechanical window crab. **Gate:** distinct silhouette
✓, big eyes carry the 8 states ✓, no collision ✓ → ship.

## The reference prompt — build it from the layers

A reference is a **single centered mascot illustration on a plain background — NOT
a sprite sheet**. Compose the prompt straight from the brand layers you landed:

- **Species** — from the language lineage or archetype.
- **Silhouette** — the letter/logo shape, stated as a hard constraint
  (`the body + wings + tail form a rough capital Y`).
- **Signature props** — the name-pun costume + the product-metaphor object.
- **Palette** — the brand color.
- **Style** — clean modern vector-illustration, crisp confident outlines, soft
  cel shading, symmetrical, big friendly expressive eyes, cute-but-characterful,
  iconic brand-mascot energy. **No text, no words, no letters drawn** (the letter
  is only the pose, never literal type).

## Generate it with Antigravity (Gemini image)

The reference is generated with the **Gemini 3 Pro image model**
(`gemini-3-pro-image-preview`) — Antigravity's image path, the same model
critterforge uses (`pkg/critterforge/gemini.go`). Generate at `high` (4K) for a
crisp master, then downscale the ref to ~1536px and save to `refs/<name>.png`.

The current one-off recipe (force the real key — the shell placeholder shadows it):

```sh
KEY=$(grep -m1 '^GEMINI_API_KEY=' .env | cut -d= -f2- | tr -d '"'"'"' \r')
# a tiny throwaway that calls critterforge.BuildImageModel("gemini","","1:1","high")
# + GenerateSprite(PROMPT) and writes refs/<name>.png (see how the phoenix ref was made),
# then: python3 -c "from PIL import Image; im=Image.open('refs/<name>.png').convert('RGBA'); im.thumbnail((1536,1536)); im.save('refs/<name>.png', optimize=True)"
```

> A `critterforge ref --only <name>` subcommand that reads the critters.yaml entry
> and emits `refs/<name>.png` would make this a first-class one-liner — a natural
> next addition.

### Worked example — the Yscale phoenix

Layers landed: **letter Y** (silhouette) + **product metaphor** (rise/burst/scale).
The exact prompt used:

> A single centered mascot character illustration of a majestic mythical phoenix
> firebird, on a plain soft background. CRITICAL SILHOUETTE: the phoenix body, two
> upswept outstretched wings, and downward tail form the shape of a capital letter
> Y — two wings sweeping up and out like the top of a Y, a single tail streaming
> straight down like the stem. Warm palette: deep ember orange, crimson red, molten
> gold, glowing amber highlights, with rising flame and spark motifs. Big friendly
> expressive eyes, cute-but-powerful, iconic brand-mascot energy. Clean modern
> vector-illustration style, crisp confident outlines, soft cel shading, symmetrical.
> It is a phoenix, not a literal letter — the Y is only the pose/silhouette. No text,
> no words, no letters drawn.

Result: `refs/yscale-phoenix.png` — a Y-silhouette firebird, pinned to the
`yscale-agent` workload.

## Then run the pipeline

Wire `reference: refs/<name>.png` into the critter's `critters.yaml` entry, then
generate the sheet + decks, downscale to `1024×128`, and commit the sprites **and**
`critters/manifest.json`. Full steps in [critterforge-pipeline.md](critterforge-pipeline.md)
and [critters.md](critters.md); the `mascot-cast` skill drives the whole app→critter
flow end to end.
