package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleRecommendations = `# 推荐 Compose 项目

<!--
字段说明：地址与示例二选一，标签用逗号分隔。
-->

## Paperless-ngx

- 描述：文档管理系统，支持扫描件 OCR 与全文检索
- 标签：文档, OCR, 自托管
- 地址：https://example.com/paperless/docker-compose.yml

## Nginx 静态站点

一些额外的段落文字会被解析器忽略。

- 描述：轻量示例
- 标签：示例，静态站点
- 示例：examples/nginx-static/docker-compose.yml

## 缺少来源

- 描述：这条没有地址或示例，应被跳过
`

func TestParseRecommendations(t *testing.T) {
	items, err := parseRecommendations(sampleRecommendations)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	paperless := items[0]
	if paperless.Name != "Paperless-ngx" || paperless.URL != "https://example.com/paperless/docker-compose.yml" || paperless.Example != "" {
		t.Fatalf("paperless = %+v", paperless)
	}
	if strings.Join(paperless.Tags, ",") != "文档,OCR,自托管" {
		t.Fatalf("paperless tags = %+v", paperless.Tags)
	}
	nginx := items[1]
	if nginx.Example != "examples/nginx-static/docker-compose.yml" || nginx.URL != "" || nginx.ID == paperless.ID {
		t.Fatalf("nginx = %+v", nginx)
	}
	if paperless.ID != recommendationID(paperless.Name) {
		t.Fatalf("paperless id = %q", paperless.ID)
	}
}

func TestRecommendationID(t *testing.T) {
	for name, want := range map[string]string{
		"Paperless-ngx": "paperless-ngx",
		"Nginx 静态站点":    "nginx",
		"123 abc":       "rec-123-abc",
		"":              "recommendation",
	} {
		if got := recommendationID(name); got != want {
			t.Errorf("recommendationID(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLoadRecommendationsReadsRepositoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommendations.md")
	if err := os.WriteFile(path, []byte(sampleRecommendations), 0o600); err != nil {
		t.Fatal(err)
	}
	original := recommendationsFile
	recommendationsFile = path
	t.Cleanup(func() { recommendationsFile = original })
	items, err := loadRecommendations()
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v, %v", items, err)
	}
}

func TestImportExampleProjectCopiesAndValidates(t *testing.T) {
	useTemporaryProjectRoot(t)
	workdir := t.TempDir()
	exampleDir := filepath.Join(workdir, "examples", "nginx")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  web:\n    image: nginx:alpine\n"
	if err := os.WriteFile(filepath.Join(exampleDir, "docker-compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	project, err := importExampleProject("示例项目", "examples/nginx/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if project.Source != "示例：examples/nginx/docker-compose.yml" {
		t.Fatalf("source = %q", project.Source)
	}
	reloaded, err := readProject(project.ID)
	if err != nil || reloaded.Content != compose {
		t.Fatalf("reloaded = %+v, %v", reloaded, err)
	}
	for _, bad := range []string{"../escape.yml", "/etc/passwd", "..", "a/../../b.yml", ""} {
		if _, err := importExampleProject("x", bad); err == nil {
			t.Fatalf("example %q must fail", bad)
		}
	}
}

func TestParseRecommendationsDropsInsecureURLs(t *testing.T) {
	markdown := `## 不安全的下载

- 描述：这条使用 http，应被丢弃
- 地址：http://example.com/docker-compose.yml

## 另一个示例

- 示例：examples/nginx-static/docker-compose.yml
`
	items, err := parseRecommendations(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "另一个示例" || items[0].Example == "" {
		t.Fatalf("items = %+v", items)
	}
}

func TestRepositoryRecommendationsAreUsable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "recommendations.md"))
	if err != nil {
		t.Skip("repository file not available")
	}
	items, err := parseRecommendations(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 6 {
		t.Fatalf("expected at least 6 recommendations, got %d", len(items))
	}
	for _, item := range items {
		if item.Example != "" {
			if _, err := os.Stat(filepath.Join("..", filepath.FromSlash(item.Example))); err != nil {
				t.Errorf("example %q missing: %v", item.Example, err)
			}
		}
		if item.URL != "" && !strings.HasPrefix(item.URL, "https://") {
			t.Errorf("insecure URL %q", item.URL)
		}
	}
}

func TestBundledExamplesAreValidCompose(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "examples"))
	if err != nil {
		t.Skip("examples directory not available")
	}
	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", "examples", entry.Name(), "docker-compose.yml"))
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		if err := validateCompose(string(data)); err != nil {
			t.Errorf("example %s invalid: %v", entry.Name(), err)
		}
		found++
	}
	if found < 4 {
		t.Fatalf("expected at least 4 bundled examples, got %d", found)
	}
}
