package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useTemporaryProjectRoot(t *testing.T) string {
	t.Helper()
	original := userConfigDir
	root := t.TempDir()
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = original })
	return root
}

func TestProjectIDRejectsPathsAndIsSafe(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../escape", "a\\b", "a/b"} {
		if _, err := projectID(name); err == nil {
			t.Fatalf("projectID(%q) must fail", name)
		}
	}
	id, err := projectID("我的 Docker 项目 2026")
	if err != nil || !projectIDPattern.MatchString(id) {
		t.Fatalf("project ID = %q, %v", id, err)
	}
	numeric, err := projectID(strings.Repeat("9", 64))
	if err != nil || len(numeric) > 64 || !projectIDPattern.MatchString(numeric) {
		t.Fatalf("numeric ID = %q, %v", numeric, err)
	}
}

func TestProjectRoundTripPreservesAdvancedYAML(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProjectWithContent("Demo", "测试", "# keep this comment\nx-meta: keep\nservices:\n  web:\n    image: nginx:latest\n    x-custom: retained\n")
	if err != nil {
		t.Fatal(err)
	}
	content := "# keep this comment\nx-meta: keep\nservices:\n  web:\n    image: nginx:1.27\n    x-custom: retained\n"
	saved, err := saveProject(project.ID, content)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UpdatedAt == "" {
		t.Fatal("save must retain an update timestamp")
	}
	reloaded, err := readProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# keep this comment", "x-meta: keep", "x-custom: retained", "nginx:1.27"} {
		if !strings.Contains(reloaded.Content, expected) {
			t.Fatalf("saved content lost %q: %s", expected, reloaded.Content)
		}
	}
	items, err := listProjects()
	if err != nil || len(items) != 1 || items[0].ID != project.ID {
		t.Fatalf("list = %#v, %v", items, err)
	}
}

func TestUploadRequiresManagedDestinationAndComposeFilename(t *testing.T) {
	root := useTemporaryProjectRoot(t)
	project, err := createProject("Imported")
	if err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "compose.yaml"), []byte("services:\n  app:\n    image: busybox\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := projectImportTarget(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importUploadedProject("", staging, target.Destination)
	if err != nil || imported.Source != "本地导入" {
		t.Fatalf("import = %#v, %v", imported, err)
	}
	if _, err := importUploadedProject("", staging, filepath.Join(root, "outside")); err == nil {
		t.Fatal("outside destination must fail")
	}
}

func TestUploadRejectsOversizedCompose(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Imported")
	if err != nil {
		t.Fatal(err)
	}
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "compose.yml"), []byte(strings.Repeat("a", maxComposeBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := projectImportTarget(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importUploadedProject("", staging, target.Destination); err == nil {
		t.Fatal("oversized upload must fail")
	}
	dir, err := projectPath(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(composePath(dir)); err != nil {
		t.Fatalf("existing project file must stay untouched: %v", err)
	}
}

func TestDownloadComposeUsesBrokerAndEnforcesSize(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("url"); got != "https://example.com/docker-compose.yml" {
			t.Errorf("unexpected url param %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("missing auth header")
		}
		_, _ = w.Write([]byte(strings.Repeat("b", maxComposeBytes+1)))
	}))
	defer broker.Close()
	t.Setenv("ALX_PLUGIN_DOWNLOAD_BROKER", broker.URL)
	t.Setenv("ALX_PLUGIN_DOWNLOAD_TOKEN", "secret")
	if _, err := downloadCompose("https://example.com/docker-compose.yml"); err == nil {
		t.Fatal("oversized download must fail")
	}
	t.Setenv("ALX_PLUGIN_DOWNLOAD_BROKER", "")
	t.Setenv("ALX_PLUGIN_DOWNLOAD_TOKEN", "")
	if _, err := downloadCompose("https://example.com/docker-compose.yml"); err == nil || !strings.Contains(err.Error(), "代理") {
		t.Fatal("missing broker must fail before download")
	}
}

func TestValidateComposeRejectsInvalidRoots(t *testing.T) {
	for _, content := range []string{"", "- item\n", "services: bad\n"} {
		if err := validateCompose(content); err == nil {
			t.Fatalf("validateCompose(%q) must fail", content)
		}
	}
	if err := validateCompose("services:\n  app:\n    image: busybox\n"); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePublicDownloadHostRejectsPrivateDestinations(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "10.0.0.1", "192.168.1.2", "169.254.1.2"} {
		if err := validatePublicDownloadHost(host); err == nil {
			t.Fatalf("host %q must fail", host)
		}
	}
}
