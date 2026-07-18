# Kubagachi Critters — Nori & the Yscale family

Design capture — the spec behind the shipped Nori/Yscale critters. Generation
is **done and verified** (see the **Status** section at the end); this doc is the
design intent it was built from.

## Locked decisions

- **One mascot: Nori** — a gray-and-white chibi cat — is the shared character for the
  whole Yscale family (yscale, jaK3s, yscale-media, cartogopher-db). Projects are told
  apart by their **workload animations**, not by different animals (exactly like
  box-network's Nori: same cat, scanning gets a headlamp, travel gets a backpack,
  secure gets a shield).
- **Style** (from box-network's Nori sheet): chibi proportions, very cute; limited
  palette — gray / white / soft-pink / dark outline, with accent colors
  (green / blue / red / purple); soft shading; crisp 1–2px outlines; readable at 32×32.
  Base art: `refs/nori.png`.
- **Generator: Gemini** (`gemini-3-pro-image-preview`) at **`--quality high`** (4K). This is
  what the original critters used and what gives crisp, consistent sheets. *(Now wired:
  `--provider gemini` is the default across `critterforge`/`spriteanim`; `--quality high`
  maps to Gemini `imageSize: 4K`.)*
- **Pipeline — no single-tile gens.** The canonical flow is two generation stages
  plus an automatic normalize:
  1. `critterforge sheet` → one **keyed sprite sheet** (all states in a row), anchored to
     `refs/nori.png` so every frame is the same cat.
  2. `spriteanim` → a **per-state 8-frame animation sheet** for each state.
  3. **normalize** (built into both stages) → see below.
  Then the browser fits each frame onto the **128 pixel mesh** at render. The per-state
  single-tile `generate` step is retired from the flow.
- **Normalize (the Gemini-output keyer).** Gemini's image-preview model does *not* return
  transparent PNGs — it returns an **opaque JPEG with a baked white+gray checkerboard**
  standing in for transparency, and it **free-arranges frames into a grid** instead of one
  row. Raw output is therefore unsliceable. `pkg/critterforge/normalize.go` fixes both,
  automatically, after every gen:
  - **Flood-fill alpha.** The background is light/near-neutral *and* connected to the
    border, so we BFS from the border keying it to real alpha. Enclosed light areas (Nori's
    white belly, in-body sparkles) are unreachable from the border, so they survive — a flat
    color-key would punch holes in them. The brightness floor is **learned from the border
    ring** per image (the keyed sheet's checker is ~198; a deck's can be ~150), not
    hard-coded.
  - **Grid reflow + merge-to-expected.** Detect frame blobs (row bands → column clusters),
    then fold the narrowest cluster into its nearest neighbor until the count matches the
    expected frame count — this absorbs detached particles (`?`, `Z`, sparkles) and rejoins
    a frame split by a transparent gap (the dissolving ghost). Survivors are tight-cropped,
    centered, and laid out as one even row.
  - Each stage also writes the untouched model output to `sprite-sheet[-<state>]-raw.png`, so
    `critterforge normalize` can re-key the whole set **for free** after tuning — no
    regeneration.
- **Commands:**
  ```
  critterforge sheet     --only nori --provider gemini --quality high   # stage 1 (+normalize)
  go run ./cmd/spriteanim --only nori --provider gemini --quality high   # stage 2 (+normalize)
  critterforge normalize --only nori                                     # re-key from raw, no API
  ```

## Base health states (k8s → Nori)

The eight states the pipeline always makes, mapped to Nori expressions:

| kubagachi status | Nori |
|---|---|
| running | **online** — content, upright, gentle wifi/glow |
| pending | **thinking** — head tilt, `?`, dotted ring |
| completed | **happy** — eyes squinted `^_^`, sparkles |
| crashloop | **alert / intrusion** — `X_X`, red sparks, fur on end, panic |
| backoff | **sleeping** — curled, big `Zzz` |
| terminating | **disconnected** — dissolving into drifting pixels |
| unknown | **ghost-outline** — hollow white silhouette |
| imagepull / error | **warning** — red `!`, tense |

## Workload animations (Nori with props) — by project

Each is Nori performing the activity with a prop/effect, exaggerated for 32px
readability, in the project's accent. Recipes are written like `stateRecipes` so they
drop straight into the prompt pipeline.

### `yscale` — burst-to-cloud  *(BurstPool / GPUWorkload / EdgeFleet / HybridPool)*
- **bursting** — Nori inflates and *pops* out small copies of itself that rocket toward
  cloud-portal icons (AWS orange, GCP blue, Linode green, Hetzner red, Azure cyan);
  node-orbs multiply and scatter; triumphant.
- **scaling** — stacking glowing node-tiles into a rising tower; steady, an up-arrow.
- **gpu-workload** — straps on a glowing GPU jetpack, fan-blade eyes, green heat shimmer.
- **edge-fleet** — launches tiny scout-drones to map-pins lighting up at the edges.
- **draining** — cordons a node, scoops its pods into a basket, carries them home; careful.

### `jaK3s` — golden-image forge  *(real build stages)*
- **fetching** — pulls ingots off a shelf onto the anvil (Debian base, k3s, cilium, helm).
- **forging** *(signature)* — hammers a glowing disk-platter, sparks flying, stamps a k3s
  lightning sigil.
- **hardening** — quenches the glowing disk in a trough → steam burst (harden/strip stage).
- **layering** — folds glowing layers onto the disk (base + k3s + cilium + kube-vip).
- **sparsifying** — squeezes the disk in a vise until compact (the `virt-sparsify` zero-out).
- **shipping** — loads the finished image onto a Proxmox cart or a Linode rocket.

### `yscale-media` — transcode / stream  *(scan → shard → encode → upload → stream)*
- **scanning** — flashlight crawling a shelf of film reels; TMDB posters fade in.
- **sharding** *(signature — the disk-splitting you wanted, done right)* — slices a long
  film strip into numbered ~4s segments with a clapperboard-cutter and **flings the chunks**
  to a row of worker bins (a shared work queue). Temporal sharding as dealing
  cards — *not* physical disk striping (which it doesn't do).
- **encoding** — feeds a segment into a GPU grinder; smaller frames pop out; heat glow
  (green NVENC / blue VAAPI), rising util bar.
- **uploading** — stacks encoded fMP4 boxes into a glowing bucket (MinIO/S3) that fills.
- **streaming** — works the projector, beam out, buffer bar filling.
- **direct-play** — clean straight beam reel→screen, no grinder (the remux bypass).

### `cartogopher-db` — the chart room  *(bake → embed → query → impact → watch)*
Nori in the cartographer's tricorn (the pirate-cat sibling of the cartogopher gopher).
- **indexing** *(signature)* — plots nodes (functions) and inks edges (call graph) across a
  sea chart; measures with a sextant (the `bake`).
- **embedding** — sprinkles a star-chart of constellations onto the map (vector embeddings).
- **querying** — sweeps a spyglass; an "X marks the spot" glows on a match.
- **impact** — drops a stone on a node; ripples spread along the edges (blast radius).
- **watching** — a chart section smudges as files change; quickly re-inks it (incremental re-bake).

## Assignment + triggering (kubagachi side — IMPLEMENTED, namespace/owner keyword)

`internal/k8s/workload.go`, called from `MapPod`:

1. **Reserved mascots (`assignCritter`).** Project mascots are reserved by namespace/owner
   keyword and kept out of the general pool via `critters.AssignExcept`, so they never land
   on infra pods:
   - `cartogopher` → the pirate-gopher critter
   - `yscale` / `jak3s` / `kubagachi` → **Nori**
   - everything else → the remaining critters (never Nori/Cartogopher)
2. **Workload overlay (`applyWorkloadAnimation`).** A **healthy** Nori pod whose
   namespace/owner/name matches a workload keyword plays that animation; an unhealthy one
   keeps its health state (problems stay visible), still on the Nori sprite.

| keyword (word-boundary, case-insensitive) | animation |
|---|---|
| `autoscal` | scaling |
| `burst` | bursting |
| `gpu` | gpu-workload |
| `edge` | edge-fleet |
| `drain` | draining |
| `scale` | scaling |

**Matching is word-boundary (`tokenMatch`), not substring** — the brand "yscale" contains
"scale", so naive `Contains` tagged every yscale pod as `scaling`. With boundaries, only a
real burst/gpu/edge/drain/scale token triggers; on the live cluster only `yscale-burst`
animates (bursting), the rest of yscale idles as Nori.

**Web path:** `webPod.critterState` (canonical anim key) is sent to the browser; the
frontend plays `pod.critterState ?? pod.status` while `status` drives color/label.
`internal/sprites.Scan` globs `sprite-sheet-*.png` so workload decks are served.

## Status

- **Done:** manifest `animations:` block, `spriteanim` custom decks (`customDeckPrompt`),
  Gemini normalize keyer (no raw files), Nori's 8 base + 5 yscale workload decks,
  Cartogopher regenerated high-quality from `refs/cartogopher.png`, bursting = shadow-clone
  jutsu, reserved-mascot assignment + word-boundary trigger + web critterState plumbing
  (all with tests). Verified live on `optiplex-pg`.
- **Vendor pinning (shipped):** `projectMascots` now pins postgres + redis (ordered
  before cartogopher/nori) so a database keeps its own mascot even inside a cartogopher
  or yscale namespace — vendor identity supersedes the brand family, first-match-wins.
  Nori still covers all remaining yscale/kubagachi pods. Reserved from the general pool
  so vendors only ever show on their own workloads. (`internal/k8s/workload.go`, tested.)
- **Next:** author jaK3s / yscale-media (`sharding`!) / cartogopher-db recipes into
  `critters.yaml`; add label-based triggering (`yscale.io/workload=…`); per-project
  workload keyword sets.
- **Idea — combined / hybrid critter types (later).** Today a pod that matches two
  identities resolves to ONE winner (e.g. `cartogopher-dev-postgres` → postgres, beating
  the gopher). But that pod is really *cartogopher's postgres*, so it could render a
  **combined critter** instead: a hybrid sprite (gopher + elephant motifs) or a
  base-critter + accessory overlay (e.g. postgres wearing a cartographer hat), rather
  than one mascot simply superseding the other. Open questions: how to compose two 8-mood
  sheets (overlay layer vs. a freshly generated hybrid sheet), which pairs are worth
  authoring (cartogopher×postgres, cartogopher×redis, yscale×postgres…), and how the
  first-match-wins matcher becomes a set-match that selects/roles a hybrid. Bring the
  crude fleet to premium first; then prototype one hybrid pair.
