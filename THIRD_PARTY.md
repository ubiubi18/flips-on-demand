# Third-Party Sources and Licenses

This project is MIT licensed (see `LICENSE`). It is derived from, and will
reuse patterns from, the following projects. Their notices are preserved here
and must stay preserved in any file or component copied from them.

| Source | Revision referenced | License | Notice |
|---|---|---|---|
| IdenaAI | `f2e0dc9d7aee38eac4615b010447b10189f43173` | MIT | `MIT License / Copyright (c) 2020 Idena` (repository `LICENSE`) |
| idena.social contract (vendored in IdenaAI) | `f2e0dc9d7aee38eac4615b010447b10189f43173` | MIT | `MIT License / Copyright (c) 2025 N3CR0M4NC3R` (`LICENCE`) |
| idena.social UI (vendored in IdenaAI) | `f2e0dc9d7aee38eac4615b010447b10189f43173` | MIT | `MIT License / Copyright (c) 2025 N3CR0M4NC3R` (`LICENCE`) |
| idena-go | `9bc57040337fd19f45ba65323939cac8789b9efd` | LGPL-3.0 (`LICENSE`) | No code copied: read-only reference for behavioural facts. Not MIT-compatible for copying; reuse would need its own component and this notice |
| idena-wasm-binding | `f7c71ce32d86c8ed26e74935265c95768b0230d0` | LGPL-3.0 (`LICENSE`) | No code copied: read-only reference. Ships the pinned runtime archives |
| idena-wasm (runtime source) | `7e1138959f9f96f59d20efa11f2c27b134067541` | no licence file at this revision (`docs/SOURCES.md` F-11) | No code copied: read-only reference for behavioural facts. This revision carries no licence grant, so nothing may be conveyed from it |
| viem | `^2.57.0` (`gateway/package.json`) | MIT (`node_modules/viem/LICENSE`) | `MIT License / Copyright (c) 2023-present weth, LLC` (unvendored gateway runtime dependency for EIP-191 signature verification) |
| React, React DOM, Scheduler | `app/package-lock.json` | MIT | Copyright (c) Meta Platforms, Inc. and affiliates. Bundled into `app/dist/flips-on-demand.html`; see notice below |

## Rules

- When reusing code from a source above, carry its copyright notice into the
  copied file or into this document, and record the revision used.
- Do not relicense copied third-party code. All sources listed as MIT are
  compatible with this project's MIT license; anything non-permissive must be
  excluded or isolated with its own notice.
- `idena-go` and `idena-wasm-binding` are LGPL-3.0, and the pinned `idena-wasm`
  revision carries no licence grant. Keep all three read-only references: copy
  no code from them, vendor none of them, and convey no binary that links them.
  `tools/fod` consumes `idena-go` as an unvendored build dependency, which is
  use, not copying; `release/` holds only this repository's own artefacts. If
  reuse ever becomes necessary, isolate it in its own component with the
  upstream licence text and notice instead of merging it into this MIT tree.
- Verified license facts are recorded in `docs/SOURCES.md`.
- `gateway` consumes `viem` as an unvendored MIT runtime dependency. No viem
  code is copied into this tree; the package manager installs it under
  `gateway/node_modules`, and its notice is recorded above.

## Attribution status

The pinned Idena sources above were read for protocol behavior; no source code
was copied from them into this repository. The tracked browser bundle contains
React, React DOM, and Scheduler. The tracked WASM contracts in `release/` are
this project's own builds. The bundled React license notices remain in the HTML;
the full permission notice for those packages follows.

### Documentation illustrations

The README concept drawings embed four unchanged PNG pictures from the owner's IdenaAI 3 balloon / smoke flip (`docs/visuals/balloon-{1,2,3,4}.png`). The owner explicitly requested the complete example for these illustrations. The visible market role is “Seller”; the original author, flip identifier, picture provenance and recorded arrangements are documented in the [visual guide](docs/VISUAL_GUIDE.md#real-flip-previews). This attribution does not extend the repository's MIT license to third-party image rights.

### React, React DOM, and Scheduler (MIT)

Copyright (c) Meta Platforms, Inc. and affiliates.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
