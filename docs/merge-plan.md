# Merge Plan

The initial public repository keeps the two projects separate. The target
merged product should offer two download modes in one Web backend:

- bounding-box range download
- administrative-region download

The selected implementation direction is a single Go backend with SQLite as the
control database. The existing .NET range downloader remains only as a reference
until the bounding-box workflow has been ported and validated in Go.

## Product Direction

Keep `apps/admin-region-tiler` as the base for backend capabilities:

- multi-task and child-task execution
- persistent database state
- administrative GeoJSON region resources
- scheduled jobs
- deployment assets
- output packaging and downloads

Use `apps/range-downloader` as the reference for the user-facing range flow:

- map click and drag selection
- bounding-box preview
- compact Tianditu token input
- simple `img`, `cia`, and `vec` layer download path
- clear task status and failure retry experience

## Interface Boundaries

Before moving code, define stable interfaces for:

- map source configuration
- area selection, including bounding boxes and GeoJSON regions
- task creation
- task and child-task status
- failure records and retry behavior
- output artifact creation and download

Do not merge by directly copying files between projects. First isolate the
domain model and API contracts, then port only the pieces that fit the target
architecture.

## Suggested Phases

1. Keep the repository structure stable under `apps/` and release both apps as
   independently runnable examples.
2. Extract a common task model that can represent both range downloads and
   administrative-region downloads.
3. Normalize map source configuration and token handling.
4. Add bounding-box task creation to the Go backend.
5. Port the range selection UI into the unified Web frontend.
6. Retire duplicated download logic after both modes use the same task engine
   and artifact pipeline.

## Non-Goals Before Merge

- Do not import old source folders, UI packages, temporary files, screenshots,
  runtime databases, downloaded tiles, or release archives.
- Do not commit real service tokens.
- Do not make deployment depend on machine-local paths.
