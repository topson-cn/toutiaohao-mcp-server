package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/example/toutiaohao-mcp-server/toutiaohao"
)

func TestArticleStatusIsDraft(t *testing.T) {
	cases := []interface{}{float64(1), int(1), int64(1), "1", "draft", "草稿"}
	for _, tc := range cases {
		if !toutiaohao.ArticleStatusIsDraft(tc) {
			t.Fatalf("expected %v to be treated as draft", tc)
		}
	}
}

func TestArticleStatusIsDraftFalse(t *testing.T) {
	cases := []interface{}{float64(3), int(3), int64(3), "3", "published", nil}
	for _, tc := range cases {
		if toutiaohao.ArticleStatusIsDraft(tc) {
			t.Fatalf("expected %v not to be treated as draft", tc)
		}
	}
}

func TestArticleStatusIsPublished(t *testing.T) {
	cases := []interface{}{float64(3), int(3), int64(3), float64(6), int(6), int64(6), "3", "6", "published", "已发布", "审核中", "已提交"}
	for _, tc := range cases {
		if !toutiaohao.ArticleStatusIsPublished(tc) {
			t.Fatalf("expected %v to be treated as published", tc)
		}
	}
}

func TestArticleStatusIsPublishedFalse(t *testing.T) {
	cases := []interface{}{float64(1), int(1), int64(1), "1", "draft", "草稿", nil}
	for _, tc := range cases {
		if toutiaohao.ArticleStatusIsPublished(tc) {
			t.Fatalf("expected %v not to be treated as published", tc)
		}
	}
}

func TestArticlePublishDedupeBlocksInFlightDuplicate(t *testing.T) {
	resetArticlePublishDedupeForTest()
	t.Cleanup(resetArticlePublishDedupeForTest)

	key := articlePublishDedupeKey("标题", "正文", &toutiaohao.ArticleOptions{
		Images: []string{"cover.png"},
	})
	finish, err := beginArticlePublishDedupe(key, "标题")
	if err != nil {
		t.Fatalf("beginArticlePublishDedupe() unexpected error: %v", err)
	}
	defer finish(false)

	if _, err := beginArticlePublishDedupe(key, "标题"); err == nil || !strings.Contains(err.Error(), "正在发布中") {
		t.Fatalf("duplicate in-flight publish was not blocked, err=%v", err)
	}
}

func TestArticlePublishDedupeBlocksRecentCompletedDuplicate(t *testing.T) {
	resetArticlePublishDedupeForTest()
	t.Cleanup(resetArticlePublishDedupeForTest)

	key := articlePublishDedupeKey("标题", "正文", &toutiaohao.ArticleOptions{
		Images: []string{"cover.png"},
	})
	finish, err := beginArticlePublishDedupe(key, "标题")
	if err != nil {
		t.Fatalf("beginArticlePublishDedupe() unexpected error: %v", err)
	}
	finish(true)

	if _, err := beginArticlePublishDedupe(key, "标题"); err == nil || !strings.Contains(err.Error(), "完成过发布") {
		t.Fatalf("recent completed publish was not blocked, err=%v", err)
	}
}

func TestArticlePublishDedupeKeyIncludesImages(t *testing.T) {
	resetArticlePublishDedupeForTest()
	t.Cleanup(resetArticlePublishDedupeForTest)

	left := articlePublishDedupeKey("标题", "正文", &toutiaohao.ArticleOptions{
		Images: []string{"cover-a.png"},
	})
	right := articlePublishDedupeKey("标题", "正文", &toutiaohao.ArticleOptions{
		Images: []string{"cover-b.png"},
	})
	if left == right {
		t.Fatal("dedupe key should include explicit images")
	}
}

func TestPublishFailureKeepingDraftError(t *testing.T) {
	baseErr := fmt.Errorf("封面上传失败")
	err := publishFailureKeepingDraftError(baseErr)
	if !errors.Is(err, baseErr) {
		t.Fatalf("wrapped error does not preserve cause: %v", err)
	}
	if !strings.Contains(err.Error(), "未自动删除草稿") {
		t.Fatalf("error does not explain draft preservation: %v", err)
	}
}

func TestRequirePublishUnlock(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "")
	if err := requirePublishUnlock(true); err == nil {
		t.Fatal("expected environment gate to reject publishing")
	}

	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "1")
	if err := requirePublishUnlock(false); err == nil {
		t.Fatal("expected request confirmation gate to reject publishing")
	}
	if err := requirePublishUnlock(true); err != nil {
		t.Fatalf("requirePublishUnlock() error = %v", err)
	}
}

func TestDraftModeDoesNotRequirePublishUnlock(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "")
	if err := enforceArticleWriteMode(&toutiaohao.ArticleOptions{SaveAsDraft: true}); err != nil {
		t.Fatalf("draft mode was blocked: %v", err)
	}
}

func TestArticlePublishRequiresBothUnlocks(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOW_PUBLISH", "1")
	if err := enforceArticleWriteMode(&toutiaohao.ArticleOptions{}); err == nil {
		t.Fatal("publish without confirm_publish should be blocked")
	}
	if err := enforceArticleWriteMode(&toutiaohao.ArticleOptions{ConfirmPublish: true}); err != nil {
		t.Fatalf("fully unlocked publish was blocked: %v", err)
	}
}

func TestDraftIDsByTitleKeepsOnlyExactDrafts(t *testing.T) {
	response := &toutiaohao.ArticleListResponse{Articles: []toutiaohao.ArticleItem{
		{ArticleID: "old", Title: "目标标题", Status: 1},
		{ArticleID: "published", Title: "目标标题", Status: 3},
		{ArticleID: "other", Title: "其他标题", Status: 1},
	}}
	ids := draftIDsByTitle(response, " 目标标题 ")
	if len(ids) != 1 {
		t.Fatalf("ids = %#v", ids)
	}
	if _, ok := ids["old"]; !ok {
		t.Fatalf("missing exact draft id: %#v", ids)
	}
}

func TestVerifyNewDraftReturnsNewExactDraft(t *testing.T) {
	baseline := map[string]struct{}{"old": {}}
	fetch := func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error) {
		return &toutiaohao.ArticleListResponse{Articles: []toutiaohao.ArticleItem{
			{ArticleID: "old", Title: "目标标题", Status: 1},
			{ArticleID: "new", Title: "目标标题", Status: 1},
		}}, nil
	}
	item, err := verifyNewDraft(context.Background(), "目标标题", baseline, time.Now(), 1, 0, fetch)
	if err != nil {
		t.Fatalf("verifyNewDraft() error = %v", err)
	}
	if item.ArticleID != "new" {
		t.Fatalf("ArticleID = %q, want new", item.ArticleID)
	}
}

func TestVerifyNewDraftRejectsAmbiguousNewDrafts(t *testing.T) {
	fetch := func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error) {
		return &toutiaohao.ArticleListResponse{Articles: []toutiaohao.ArticleItem{
			{ArticleID: "new-1", Title: "目标标题", Status: 1},
			{ArticleID: "new-2", Title: "目标标题", Status: 1},
		}}, nil
	}
	_, err := verifyNewDraft(context.Background(), "目标标题", map[string]struct{}{}, time.Now(), 1, 0, fetch)
	if err == nil || !strings.Contains(err.Error(), "多个新增同名草稿") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyNewDraftRetriesTemporaryFailure(t *testing.T) {
	attempts := 0
	fetch := func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary")
		}
		return &toutiaohao.ArticleListResponse{Articles: []toutiaohao.ArticleItem{
			{ArticleID: "new", Title: "目标标题", Status: "draft"},
		}}, nil
	}
	item, err := verifyNewDraft(context.Background(), "目标标题", map[string]struct{}{}, time.Now(), 2, 0, fetch)
	if err != nil || item.ArticleID != "new" || attempts != 2 {
		t.Fatalf("item=%+v err=%v attempts=%d", item, err, attempts)
	}
}

func TestVerifyNewDraftExhaustionIsNotSuccess(t *testing.T) {
	fetch := func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error) {
		return &toutiaohao.ArticleListResponse{}, nil
	}
	_, err := verifyNewDraft(context.Background(), "目标标题", map[string]struct{}{}, time.Now(), 2, 0, fetch)
	if err == nil || !strings.Contains(err.Error(), "不能确认草稿保存成功") {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyNewDraftUsesCreationTimeWhenBaselineUnavailable(t *testing.T) {
	startedAt := time.Now().Add(-time.Second)
	fetch := func(context.Context, *toutiaohao.ArticleListParams) (*toutiaohao.ArticleListResponse, error) {
		return &toutiaohao.ArticleListResponse{Articles: []toutiaohao.ArticleItem{
			{ArticleID: "new", Title: "目标标题", Status: 1, CreateTime: time.Now().Unix()},
		}}, nil
	}
	item, err := verifyNewDraft(context.Background(), "目标标题", nil, startedAt, 1, 0, fetch)
	if err != nil || item.ArticleID != "new" {
		t.Fatalf("item=%+v err=%v", item, err)
	}
}
