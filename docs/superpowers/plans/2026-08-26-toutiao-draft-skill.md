# Toutiao Draft Publisher Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and install a self-contained Codex Skill that safely saves illustrated Toutiao articles as drafts, verifies the new draft, and cannot publish without a two-part explicit unlock.

**Architecture:** Harden the existing Go MCP server at its service and transport boundaries, then package a pinned copy of that server inside a local Skill with deterministic build, login, doctor, and stdio launch scripts. Draft verification compares exact-title draft IDs before and after the browser operation, with bounded retries and a creation-time fallback; the normal Skill workflow exposes only the dedicated draft tool.

**Tech Stack:** Go 1.20+, go-rod, Model Context Protocol stdio, Gin (localhost-only optional HTTP), POSIX shell, Codex Skills.

---

## File Map

Server files modified:

- `main.go`: keep stdio transport port-free.
- `app_server.go`: centralize localhost listen address.
- `middleware.go`: restrict CORS and require a bearer token for HTTP mutations.
- `routes.go`: apply write authentication to mutating REST endpoints.
- `service.go`: enforce publish unlock and verify newly created drafts.
- `mcp_server.go`: add `confirm_publish` fields and a dedicated `save_article_draft` tool.
- `mcp_handlers.go`: map the dedicated draft handler and publish confirmation.
- `handlers_api.go`: map HTTP publish confirmation.
- `toutiaohao/publish_article.go`: carry the explicit publish confirmation option.
- `cookies/cookies.go`: persist Cookie data with mode `0600`.
- `.gitignore`: preserve credential and runtime-artifact isolation.

Tests modified or created:

- `app_server_test.go`: localhost address invariant.
- `middleware_test.go`: CORS and bearer-token behavior.
- `service_test.go`: publish gate and draft matching/retry behavior.
- `mcp_handlers_test.go`: dedicated draft option mapping.
- `cookies/cookies_test.go`: Cookie mode invariant.
- `main_test.go`: stdio startup configuration is port-free.

Skill package created outside the server checkout at `../toutiao-draft-publisher/`:

- `SKILL.md`: trigger, authorization boundary, and operating workflow.
- `agents/openai.yaml`: UI metadata with implicit discovery enabled.
- `scripts/build.sh`: local deterministic build.
- `scripts/doctor.sh`: non-secret dependency and state checks.
- `scripts/login.sh`: interactive Cookie initialization.
- `scripts/run-mcp.sh`: port-free stdio launch.
- `references/troubleshooting.md`: bounded failure recovery.
- `runtime/toutiaohao-server/`: pinned hardened source copy.

### Task 1: Make stdio port-free and HTTP localhost-only

**Files:**
- Create: `main_test.go`
- Create: `app_server_test.go`
- Modify: `main.go`
- Modify: `app_server.go`

- [ ] **Step 1: Write failing transport-boundary tests**

Add tests that require `serverListenAddr("8080") == "127.0.0.1:8080"` and require a `startStdioServer` helper to run only the supplied MCP runner, without invoking an HTTP starter:

```go
func TestServerListenAddrIsLoopback(t *testing.T) {
	if got := serverListenAddr("8080"); got != "127.0.0.1:8080" {
		t.Fatalf("serverListenAddr() = %q", got)
	}
}

func TestStartStdioServerDoesNotStartHTTP(t *testing.T) {
	runs := 0
	err := startStdioServer(context.Background(), func(context.Context) error {
		runs++
		return nil
	})
	if err != nil || runs != 1 {
		t.Fatalf("err=%v runs=%d", err, runs)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./... -run 'TestServerListenAddrIsLoopback|TestStartStdioServerDoesNotStartHTTP'`

Expected: compile failure because both helpers are absent.

- [ ] **Step 3: Implement the minimal transport changes**

Add:

```go
func serverListenAddr(port string) string {
	return net.JoinHostPort("127.0.0.1", port)
}

func startStdioServer(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}
```

Use `serverListenAddr` in `AppServer.Start`. In `main.go`, remove `appServer.StartBackground(*port)` and invoke the MCP runner through `startStdioServer`.

- [ ] **Step 4: Run focused and full tests**

Run: `go test ./... -run 'TestServerListenAddrIsLoopback|TestStartStdioServerDoesNotStartHTTP'`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go app_server.go app_server_test.go
git commit -m "fix: isolate stdio and bind HTTP to loopback"
```

### Task 2: Protect optional HTTP access

**Files:**
- Create: `middleware_test.go`
- Modify: `middleware.go`
- Modify: `routes.go`

- [ ] **Step 1: Write failing HTTP security tests**

Cover these observable behaviors with `httptest`:

```go
func requestThrough(middleware gin.HandlerFunc, method, origin, authorization string) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(middleware)
	router.Any("/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(method, "/probe", nil)
	if origin != "" { req.Header.Set("Origin", origin) }
	if authorization != "" { req.Header.Set("Authorization", authorization) }
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestCORSDeniesUnknownOrigin(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOWED_ORIGIN", "http://127.0.0.1")
	response := requestThrough(corsMiddleware(), http.MethodOptions, "https://evil.example", "")
	if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("code=%d allow-origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOWED_ORIGIN", "http://127.0.0.1")
	response := requestThrough(corsMiddleware(), http.MethodOptions, "http://127.0.0.1", "")
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1" {
		t.Fatalf("code=%d allow-origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWriteAuthRejectsMissingServerToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "")
	if response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", ""); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", response.Code)
	}
}

func TestWriteAuthRejectsWrongToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "expected")
	if response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", "Bearer wrong"); response.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", response.Code)
	}
}

func TestWriteAuthAcceptsBearerToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "expected")
	if response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", "Bearer expected"); response.Code != http.StatusNoContent {
		t.Fatalf("code=%d", response.Code)
	}
}
```

Each test must set and restore `TOUTIAO_HTTP_TOKEN` or `TOUTIAO_ALLOWED_ORIGIN` with `t.Setenv`.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test . -run 'TestCORS|TestWriteAuth'`

Expected: compile failure because `writeAuthMiddleware` does not exist and current CORS permits the unknown origin.

- [ ] **Step 3: Implement CORS and write authentication**

Implement:

```go
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := os.Getenv("TOUTIAO_ALLOWED_ORIGIN")
		if allowed == "" {
			allowed = "http://127.0.0.1"
		}
		if origin != "" && origin == allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		if c.Request.Method == http.MethodOptions {
			if origin != "" && origin != allowed {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func writeAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := os.Getenv("TOUTIAO_HTTP_TOKEN")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, ErrorResponse{Success: false, Message: "HTTP write API is disabled"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Success: false, Message: "Unauthorized"})
			return
		}
		c.Next()
	}
}
```

Apply `writeAuthMiddleware()` to every POST/PUT/DELETE REST route. Leave `/health` and read-only GET routes unauthenticated on loopback.

- [ ] **Step 4: Verify**

Run: `go test . -run 'TestCORS|TestWriteAuth'`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add middleware.go middleware_test.go routes.go
git commit -m "feat: secure local HTTP write endpoints"
```

### Task 3: Enforce the two-part publish unlock

**Files:**
- Modify: `toutiaohao/publish_article.go`
- Modify: `service.go`
- Modify: `service_test.go`
- Modify: `mcp_server.go`
- Modify: `mcp_handlers.go`
- Modify: `handlers_api.go`

- [ ] **Step 1: Write failing publish-gate tests**

Add:

```go
func TestRequirePublishUnlock(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "")
	if err := requirePublishUnlock(true); err == nil { t.Fatal("expected environment gate") }
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "1")
	if err := requirePublishUnlock(false); err == nil { t.Fatal("expected request confirmation gate") }
	if err := requirePublishUnlock(true); err != nil { t.Fatalf("unexpected error: %v", err) }
}

func TestDraftModeDoesNotRequirePublishUnlock(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "")
	if err := enforceArticleWriteMode(&toutiaohao.ArticleOptions{SaveAsDraft: true}); err != nil {
		t.Fatalf("draft blocked: %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test . -run 'TestRequirePublishUnlock|TestDraftModeDoesNotRequirePublishUnlock'`

Expected: compile failure for missing gate helpers.

- [ ] **Step 3: Implement the gate before browser or network work**

Extend `ArticleOptions` and request types:

```go
ConfirmPublish bool `json:"confirm_publish,omitempty"`
```

Implement:

```go
var errPublishLocked = errors.New("正式发布已锁定：需要 TOUTIAO_ALLOW_PUBLISH=1 且请求 confirm_publish=true")

func requirePublishUnlock(confirm bool) error {
	if os.Getenv("TOUTIAO_ALLOW_PUBLISH") != "1" || !confirm {
		return errPublishLocked
	}
	return nil
}

func enforceArticleWriteMode(opts *toutiaohao.ArticleOptions) error {
	if opts != nil && opts.SaveAsDraft {
		return nil
	}
	return requirePublishUnlock(opts != nil && opts.ConfirmPublish)
}
```

Call `enforceArticleWriteMode` at the first line of `PublishArticle` and `UpdateArticle`. Add an equivalent confirmation parameter to micro-post publishing and check it before validation or browser creation. Thread `confirm_publish` through MCP and HTTP request mapping.

- [ ] **Step 4: Verify**

Run: `go test ./... -run 'TestRequirePublishUnlock|TestDraftModeDoesNotRequirePublishUnlock'`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add toutiaohao/publish_article.go service.go service_test.go mcp_server.go mcp_handlers.go handlers_api.go
git commit -m "feat: require explicit unlock for publishing"
```

### Task 4: Verify a newly saved draft

**Files:**
- Modify: `service.go`
- Modify: `service_test.go`

- [ ] **Step 1: Write failing draft verification tests**

Introduce table-driven tests around this API:

```go
type articleListFetcher func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error)

func verifyNewDraft(
	ctx context.Context,
	title string,
	baseline map[string]struct{},
	startedAt time.Time,
	attempts int,
	delay time.Duration,
	fetch articleListFetcher,
) (*toutiaohao.ArticleItem, error)
```

Test all cases:

- exact title + draft status + new ID returns the item;
- an ID present in `baseline` is ignored;
- a non-draft status is ignored;
- two new exact-title drafts return an ambiguity error;
- temporary fetch error is retried and then succeeds;
- exhaustion returns an error containing `不能确认草稿保存成功`.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test . -run 'TestVerifyNewDraft'`

Expected: compile failure because `verifyNewDraft` is absent.

- [ ] **Step 3: Implement bounded verification**

Normalize titles with `strings.TrimSpace`. On each attempt fetch `{Page: 1, PageSize: 50, Status: "draft"}`, collect exact-title items whose status is draft and whose non-empty ID was not in the baseline. If the baseline request failed, accept only items whose parseable `CreateTime` is at or after `startedAt`; do not guess when the time is absent or unparseable. Return one match, reject more than one, and retry zero matches. Respect `ctx.Done()` and never open another editor page.

Add:

```go
func draftIDsByTitle(resp *toutiaohao.ArticleListResponse, title string) map[string]struct{} {
	ids := make(map[string]struct{})
	if resp == nil { return ids }
	for _, item := range resp.Articles {
		if strings.TrimSpace(item.Title) == strings.TrimSpace(title) && toutiaohao.ArticleStatusIsDraft(item.Status) && item.ArticleID != "" {
			ids[item.ArticleID] = struct{}{}
		}
	}
	return ids
}
```

In `PublishArticle`, fetch the baseline draft IDs before launching the browser when `SaveAsDraft` is true. After the browser flow, call `verifyNewDraft(ctx, title, baseline, startedAt, 6, 2*time.Second, s.GetArticleList)`. Return `PublishResult{Success: true, Message: "文章已保存为草稿并通过草稿箱验证", ArticleID: item.ArticleID}` only after it succeeds.

- [ ] **Step 4: Verify**

Run: `go test . -run 'TestVerifyNewDraft|TestDraftIDsByTitle'`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add service.go service_test.go
git commit -m "feat: verify newly saved article drafts"
```

### Task 5: Add a dedicated MCP draft tool

**Files:**
- Modify: `mcp_server.go`
- Modify: `mcp_handlers.go`
- Modify: `mcp_handlers_test.go`

- [ ] **Step 1: Write failing handler tests**

Extract option construction into a pure helper and test that the dedicated path forces draft mode and clears publishing fields:

```go
func TestBuildDraftArticleOptions(t *testing.T) {
	opts := buildDraftArticleOptions(map[string]interface{}{
		"cover_image": "/tmp/cover.png",
		"publish_time": "2030-01-01 10:00",
		"confirm_publish": true,
	})
	if !opts.SaveAsDraft || opts.PublishTime != nil || opts.ConfirmPublish {
		t.Fatalf("unsafe draft options: %+v", opts)
	}
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test . -run TestBuildDraftArticleOptions`

Expected: compile failure because the helper is absent.

- [ ] **Step 3: Implement tool schema and handler**

Add `SaveArticleDraftArgs` containing title, content, images, tags, category, cover image, original, and fiction, but no publish time or confirmation. Register:

```go
mcp.AddTool(server, &mcp.Tool{
	Name: "save_article_draft",
	Description: "将图文文章保存到今日头条草稿箱并回查确认；不会发布文章。",
	Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
}, withPanicRecovery("save_article_draft", func(ctx context.Context, req *mcp.CallToolRequest, args SaveArticleDraftArgs) (*mcp.CallToolResult, any, error) {
	result := appServer.handleSaveArticleDraft(ctx, saveArticleDraftArgsMap(args))
	return convertToMCPResult(result), nil, nil
}))
```

`handleSaveArticleDraft` must call `PublishArticle` with options returned by `buildDraftArticleOptions`, which always sets `SaveAsDraft: true`, `PublishTime: nil`, and `ConfirmPublish: false`.

- [ ] **Step 4: Verify**

Run: `go test . -run 'TestBuildDraftArticleOptions|TestLoginToolSchema'`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add mcp_server.go mcp_handlers.go mcp_handlers_test.go
git commit -m "feat: expose verified article draft tool"
```

### Task 6: Secure Cookie file permissions

**Files:**
- Modify: `cookies/cookies_test.go`
- Modify: `cookies/cookies.go`

- [ ] **Step 1: Write the failing mode test**

```go
func TestCookieSaveUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	store := NewFileCookieStore(path)
	if err := store.SaveCookies([]byte(`[{"name":"sid","value":"secret"}]`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil { t.Fatal(err) }
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode=%#o want=0600", got)
	}
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./cookies -run TestCookieSaveUsesOwnerOnlyPermissions`

Expected: FAIL with mode `0644`.

- [ ] **Step 3: Implement secure persistence**

Create the state directory with `0700`, write the Cookie file with `0600`, and call `os.Chmod(s.filePath, 0600)` after writing so an existing permissive file is corrected.

- [ ] **Step 4: Verify**

Run: `go test ./cookies`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cookies/cookies.go cookies/cookies_test.go
git commit -m "fix: restrict Toutiao cookie permissions"
```

### Task 7: Build the local Skill package and scripts

**Files:**
- Create: `../toutiao-draft-publisher/SKILL.md`
- Create: `../toutiao-draft-publisher/agents/openai.yaml`
- Create: `../toutiao-draft-publisher/scripts/build.sh`
- Create: `../toutiao-draft-publisher/scripts/doctor.sh`
- Create: `../toutiao-draft-publisher/scripts/login.sh`
- Create: `../toutiao-draft-publisher/scripts/run-mcp.sh`
- Create: `../toutiao-draft-publisher/scripts/test_scripts.sh`
- Create: `../toutiao-draft-publisher/references/troubleshooting.md`
- Create: `../toutiao-draft-publisher/runtime/toutiaohao-server/` from the hardened repository

- [ ] **Step 1: Initialize the Skill**

Run the official initializer with `scripts,references` resources and UI metadata. Use the name `toutiao-draft-publisher`, display name `今日头条安全草稿`, and a default prompt that explicitly invokes `$toutiao-draft-publisher`.

- [ ] **Step 2: Write script behavior tests before final scripts**

Create a temporary fake `go`, fake browser, and fake server binary, prepend them to `PATH`, then assert:

- `doctor.sh` reports each dependency without reading Cookie contents;
- `run-mcp.sh` passes only `-stdio` and exports the isolated Cookie path;
- `login.sh` passes only `-login` and exports the same Cookie path;
- `build.sh` runs `go build` against `runtime/toutiaohao-server` and writes `bin/toutiaohao-server`.

Run: `bash scripts/test_scripts.sh`

Expected: FAIL until the four scripts exist.

- [ ] **Step 3: Implement the scripts**

Every script starts with `#!/usr/bin/env bash` and `set -euo pipefail`. Resolve the Skill root from the script location. Derive state from `${XDG_STATE_HOME:-${HOME}/.local/state}/toutiao-draft-publisher`, create it with mode `0700`, and export:

```bash
export TOUTIAOHAO_COOKIES_PATH="${state_dir}/cookies.json"
```

`run-mcp.sh` must end with:

```bash
exec "${skill_root}/bin/toutiaohao-server" -stdio
```

It must not set `TOUTIAO_ALLOW_PUBLISH` and must actively `unset TOUTIAO_ALLOW_PUBLISH` before `exec`.

`login.sh` ends with the same binary plus `-login`. `doctor.sh` checks commands/files/permissions without using `cat` on the Cookie file. `build.sh` checks Go 1.20+, changes into the bundled runtime source, and runs:

```bash
go build -trimpath -o "${skill_root}/bin/toutiaohao-server" .
```

- [ ] **Step 4: Write concise Skill instructions**

The frontmatter is:

```yaml
---
name: toutiao-draft-publisher
description: Use when saving an article with local inline images or a cover to the logged-in 今日头条/头条号 draft box, or when checking that such a draft was saved; do not use for final publishing.
---
```

The body requires: announce the no-publish boundary; run `doctor.sh`; use `login.sh` only when login is absent/expired; build if the binary is absent; call only `save_article_draft`; require an exact draft ID response before claiming success; stop on ambiguity or verification timeout; never set the publish unlock variables; never delete a test draft without separate authorization.

- [ ] **Step 5: Package the pinned runtime**

Copy the hardened repository into `runtime/toutiaohao-server` while excluding `.git`, `cookies*.json`, `.tmp`, binaries, screenshots, and the `docs/superpowers` planning files. Record the source commit hash in `references/troubleshooting.md`.

- [ ] **Step 6: Verify Skill structure and scripts**

Run:

```bash
python3 /Users/wangtaotao/.codex/skills/.system/skill-creator/scripts/quick_validate.py ../toutiao-draft-publisher
bash ../toutiao-draft-publisher/scripts/test_scripts.sh
```

Expected: both commands exit 0.

- [ ] **Step 7: Commit the packaging manifest in the server repository**

Record the runtime source commit and SHA-256 for the packaged source tree in `docs/superpowers/specs/2026-08-26-toutiao-draft-skill-design.md`, then commit the design update.

### Task 8: Build, install, register, and verify locally

**Files:**
- Install: `~/.codex/skills/toutiao-draft-publisher/`
- Create outside the Skill: user state directory and Cookie file only after interactive login

- [ ] **Step 1: Install Go if the doctor confirms it is absent**

Use the user's package manager only after the required host approval. Verify with `go version` and require Go 1.20 or newer.

- [ ] **Step 2: Run all fresh verification commands**

From the hardened server checkout:

```bash
go test ./...
go build ./...
git diff --check
```

From the Skill package:

```bash
bash scripts/test_scripts.sh
python3 /Users/wangtaotao/.codex/skills/.system/skill-creator/scripts/quick_validate.py .
scripts/build.sh
scripts/doctor.sh
```

Expected: every command exits 0; doctor may report only `LOGIN_REQUIRED` before login.

- [ ] **Step 3: Install the Skill**

Copy the validated directory to `~/.codex/skills/toutiao-draft-publisher` without copying Cookie or transient files. Re-run `quick_validate.py` against the installed location.

- [ ] **Step 4: Register the MCP command**

Register a stdio MCP server named `toutiao-draft` whose command is the installed `scripts/run-mcp.sh`. Inspect the resulting Codex MCP configuration and confirm no publish-unlock environment variable is present.

- [ ] **Step 5: Verify no TCP listener**

Start `run-mcp.sh`, inspect the process with `lsof -nP -iTCP -sTCP:LISTEN`, and confirm the Toutiao server PID owns no listening socket. Terminate only the test process.

- [ ] **Step 6: Initialize or validate login**

Run `scripts/doctor.sh`. If login is required, run `scripts/login.sh` and allow the user to complete the visible Toutiao login. Re-run doctor without printing Cookie content.

- [ ] **Step 7: Save one authorized test draft**

Use a unique title, the prepared article Markdown, local inline image paths, and the selected cover. Invoke only `save_article_draft`. Require a returned draft ID and exact-title draft status.

- [ ] **Step 8: Verify no published article was created**

Call the read-only article list for `published` and confirm the unique test title is absent. Call the draft list and confirm the title and returned draft ID are present. Do not delete the draft.

- [ ] **Step 9: Final evidence report**

Report fresh test counts, build status, installed Skill path, MCP registration name, listener check result, and the verified draft ID. If any live verification step fails, report the exact failure and do not claim the draft was saved.
