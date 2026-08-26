package main

import (
	"testing"
)

func TestLoginHandlerArgsValidation_EmptyUsername(t *testing.T) {
	args := map[string]interface{}{
		"username": "",
		"password": "pass",
	}
	result := handleLoginArgsValidation(args)
	if result == nil {
		t.Fatal("expected error result for empty username")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestLoginHandlerArgsValidation_EmptyPassword(t *testing.T) {
	args := map[string]interface{}{
		"username": "user",
		"password": "",
	}
	result := handleLoginArgsValidation(args)
	if result == nil {
		t.Fatal("expected error result for empty password")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestLoginHandlerArgsValidation_Valid(t *testing.T) {
	args := map[string]interface{}{
		"username": "user",
		"password": "pass",
	}
	result := handleLoginArgsValidation(args)
	if result != nil {
		t.Errorf("expected nil for valid args, got error: %v", result)
	}
}

func TestLoginToolSchema(t *testing.T) {
	service := NewToutiaoService(nil)
	server := NewAppServer(service)

	// 验证工具已注册 - InitMCPServer 在 NewAppServer 中调用
	if server.mcpServer == nil {
		t.Fatal("mcpServer is nil")
	}
}

func TestBuildDraftArticleOptionsForcesSafeMode(t *testing.T) {
	opts := buildDraftArticleOptions(map[string]interface{}{
		"images":          []string{"cover.png"},
		"cover_image":     "explicit.png",
		"publish_time":    "2030-01-01 10:00",
		"confirm_publish": true,
	})
	if !opts.SaveAsDraft {
		t.Fatal("SaveAsDraft = false")
	}
	if opts.PublishTime != nil {
		t.Fatalf("PublishTime = %#v, want nil", opts.PublishTime)
	}
	if opts.ConfirmPublish {
		t.Fatal("ConfirmPublish = true")
	}
	if opts.CoverImage != "explicit.png" || len(opts.Images) != 1 || opts.Images[0] != "cover.png" {
		t.Fatalf("safe content options were not preserved: %+v", opts)
	}
}
