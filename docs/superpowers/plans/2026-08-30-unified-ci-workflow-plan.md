# Implementation Plan: Unified CI Workflow

**Goal:** Create `.github/workflows/ci.yml` running parallel backend and frontend test jobs, update repository badges, and remove obsolete workflow files.

---

## Tasks

### Task 1: Create `.github/workflows/ci.yml`
- Create `.github/workflows/ci.yml` with parallel `backend` and `frontend` jobs.
- Enable `concurrency` cancellation.

### Task 2: Remove Obsolete Workflows & Update Badges
- Remove `.github/workflows/go.yml` and `.github/workflows/node.js.yml`.
- Update `README.md` badge to `ci.yml`.

### Task 3: Local Verification
- Run `cd backend && go vet ./... && go test -v -race -count=1 ./... && go build -v .`
- Run `cd frontend && bun test && bun run build`
