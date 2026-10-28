# OpenWrt YAML editor: measured size

Measured on 2026-10-28 against the existing textarea-based source editor. The
comparison uses production `pnpm run build:web`, the same ARM64 management
binary (UPX LZMA), and `openwrt/package.py --skip-build --version 0.3.0-dev.1`.
The baseline preserves all preceding device-policy and profile-editor changes;
this measures only the editor enhancement, not the difference from a release.

| Artifact | Before (bytes) | After (bytes) | Increase |
| --- | ---: | ---: | ---: |
| Frontend files, including HTML/CSS/images | 1,431,429 | 1,911,202 | 479,773 (468.5 KiB) |
| clashcube-web IPK download | 3,804,124 | 3,958,004 | 153,880 (150.3 KiB, 4.0%) |
| clashcube-web installed payload | 4,656,754 | 5,136,635 | 479,881 (468.6 KiB, 10.3%) |
| clashcube-core IPK download | 14,414,789 | 14,414,789 | 0 |
| clashcube-core installed payload | 14,406,895 | 14,406,895 | 0 |

The resulting management package is **3.77 MiB download / 4.90 MiB installed
payload**. All four IPKs together are **17.53 MiB download / 18.65 MiB installed
payload**. Sizes exclude dependency packages, filesystem allocation overhead,
retained previous-deployment assets, configurations, providers and GEO data.
They are not `node_modules` sizes or a router `df` measurement. The package
manifest records file payload bytes; opkg's `Installed-Size` also accounts for
tar entries and padding.

The existing configuration editor and YAML module editor used textarea inputs.
The merged-configuration page already has a read-only renderer with basic
highlighting, line numbers and backend-generated diff. It does not implement
editable documents, cursor/selection mapping, undo history or local diagnostics.
The enhancement replaces the source editor's textarea with CodeMirror 6 and
keeps its existing save/revision/rollback contract.

The lazy editor chunk is 383,275 bytes (including its existing modal code),
and the YAML parser worker is 97,198 bytes. Editor CSS is 3,905 bytes. Shared
code splitting reduces initial main JavaScript from 482,846 to 479,597 bytes.
Opening the editor loads these local assets; live checks do not make RPC or
Internet requests. Dependencies are tree-shaken and no other language packs
or Monaco/VS Code assets are included.

`wails3 task test` passed the production frontend build, full internal Go suite
and 47 frontend tests; `go vet ./internal/...` passed. The real Docker QWRT
LuCI iframe smoke test covers highlighting/line numbers, undo/redo, diff and
baseline updates, pending/invalid syntax refusal, a 3,004-line document's
virtualized rendering and complete save, core refusal rollback, stale
revisions, stopped-core saving and subscription copies. Desktop, dark
and 390px screenshots remain under `bin/openwrt/profile-editor-*.png`.

Rebuild and inspect `bin/openwrt/packages/packages.json` for exact IPK and
installed-payload sizes. The preserved local baseline is under
`bin/openwrt/editor-size-before/` (ignored generated artifacts).
